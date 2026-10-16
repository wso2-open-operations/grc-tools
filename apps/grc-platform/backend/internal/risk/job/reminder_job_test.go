// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package job

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
)

// dueIn renders the implementation date of a risk that is n days from its
// deadline, in the RFC3339 shape the risk list actually returns.
func dueIn(n int) *string {
	s := dateOnly(time.Now().UTC()).AddDate(0, 0, n).Format(time.RFC3339)
	return &s
}

// reminderRisks serves one page of risks and records the filter it was asked
// for, so the tests can assert the sweep narrows its query to the reminder
// window instead of listing every risk in remediation.
type reminderRisks struct {
	items      []*model.RiskListItem
	lastFilter model.ListRisksFilter
	queries    int
}

// List pages over the fixed risks and records the filter it was given.
func (f *reminderRisks) List(_ context.Context, filter model.ListRisksFilter) (*model.RiskListPage, error) {
	f.queries++
	f.lastFilter = filter
	if filter.Offset >= len(f.items) {
		return &model.RiskListPage{Items: nil, Total: len(f.items)}, nil
	}
	end := filter.Offset + filter.Limit
	if end > len(f.items) {
		end = len(f.items)
	}
	return &model.RiskListPage{Items: f.items[filter.Offset:end], Total: len(f.items)}, nil
}

type claimCall struct {
	riskID  int
	tier    string
	dueDate string
}

// fakeClaimer stands in for the entity's claim row. taken models the unique
// key: a second claim on the same (risk, tier, date) loses, exactly as another
// replica's sweep would make it lose.
type fakeClaimer struct {
	calls     []claimCall
	taken     map[claimCall]bool
	released  []int64
	failClaim map[int]bool
	failRelea bool
	nextID    int64
}

// newFakeClaimer returns a claimer with no claims taken and no failures.
func newFakeClaimer() *fakeClaimer {
	return &fakeClaimer{taken: map[claimCall]bool{}, failClaim: map[int]bool{}}
}

// Claim wins the first claim per (risk, tier, date) and loses any repeat,
// like the unique key. It fails for risk ids in failClaim.
func (f *fakeClaimer) Claim(_ context.Context, riskID int, tier, dueDate string) (bool, int64, error) {
	c := claimCall{riskID, tier, dueDate}
	f.calls = append(f.calls, c)
	if f.failClaim[riskID] {
		return false, 0, errors.New("entity unreachable")
	}
	if f.taken[c] {
		return false, 0, nil
	}
	f.taken[c] = true
	f.nextID++
	return true, f.nextID, nil
}

// ReleaseClaim frees a taken claim, or fails when failRelea is set.
func (f *fakeClaimer) ReleaseClaim(_ context.Context, id int64) error {
	if f.failRelea {
		return errors.New("still unreachable")
	}
	f.released = append(f.released, id)
	// Releasing frees the key again, which is what makes a same-day re-run
	// retry the reminder.
	for c, taken := range f.taken {
		if taken {
			delete(f.taken, c)
			break
		}
	}
	return nil
}

type sentReminder struct {
	riskID  int
	tier    string
	dueDate string
}

// newTestJob builds a ReminderJob whose notify appends to sent, and fails for
// risk ids in failIDs.
func newTestJob(risks riskLister, claim reminderClaimer, sent *[]sentReminder, failIDs map[int]bool) *ReminderJob {
	return NewReminderJob(risks, claim, func(_ context.Context, riskID int, tier, dueDate string) error {
		if failIDs[riskID] {
			return errors.New("email service down")
		}
		*sent = append(*sent, sentReminder{riskID, tier, dueDate})
		return nil
	})
}

// TestRunOnceSendsTheTierEachLevelEarns is the schedule itself: a HIGH risk is
// warned three times, MEDIUM twice, LOW once, and each of those only on its
// own day.
func TestRunOnceSendsTheTierEachLevelEarns(t *testing.T) {
	cases := []struct {
		level    string
		daysOut  int
		wantTier string
	}{
		{"HIGH", 15, model.ReminderDueIn15Days},
		{"HIGH", 5, model.ReminderDueIn5Days},
		{"HIGH", 0, model.ReminderDueToday},
		{"MEDIUM", 15, ""},
		{"MEDIUM", 5, model.ReminderDueIn5Days},
		{"MEDIUM", 0, model.ReminderDueToday},
		{"LOW", 15, ""},
		{"LOW", 5, ""},
		{"LOW", 0, model.ReminderDueToday},
		// Days no tier falls on, for any level.
		{"HIGH", 14, ""},
		{"HIGH", 6, ""},
		{"HIGH", 1, ""},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("%s/%dd", c.level, c.daysOut), func(t *testing.T) {
			risks := &reminderRisks{items: []*model.RiskListItem{
				{ID: 1, RiskLevel: c.level, ImplementationDate: dueIn(c.daysOut)},
			}}
			var sent []sentReminder
			j := newTestJob(risks, newFakeClaimer(), &sent, nil)

			if err := j.RunOnce(context.Background()); err != nil {
				t.Fatalf("RunOnce: %v", err)
			}

			if c.wantTier == "" {
				if len(sent) != 0 {
					t.Fatalf("sent %+v, want nothing on day %d", sent, c.daysOut)
				}
				return
			}
			if len(sent) != 1 || sent[0].tier != c.wantTier {
				t.Fatalf("sent %+v, want one %s", sent, c.wantTier)
			}
		})
	}
}

// An overdue risk belongs to the escalation job, not this one — it must never
// pick up a reminder, however far past the date it is.
func TestRunOnceIgnoresOverdueRisks(t *testing.T) {
	risks := &reminderRisks{items: []*model.RiskListItem{
		{ID: 1, RiskLevel: "HIGH", ImplementationDate: dueIn(-1)},
		{ID: 2, RiskLevel: "HIGH", ImplementationDate: dueIn(-15)},
	}}
	var sent []sentReminder
	j := newTestJob(risks, newFakeClaimer(), &sent, nil)

	if err := j.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(sent) != 0 {
		t.Fatalf("sent %+v for overdue risks, want nothing", sent)
	}
}

// A risk with no resolvable level (no score at all) is skipped rather than
// treated as LOW — guessing would email people about a risk we can't rate.
func TestRunOnceSkipsRisksWithNoLevel(t *testing.T) {
	risks := &reminderRisks{items: []*model.RiskListItem{
		{ID: 1, RiskLevel: "", ImplementationDate: dueIn(0)},
		{ID: 2, RiskLevel: "UNKNOWN", ImplementationDate: dueIn(0)},
	}}
	var sent []sentReminder
	j := newTestJob(risks, newFakeClaimer(), &sent, nil)

	if err := j.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(sent) != 0 {
		t.Fatalf("sent %+v, want nothing for unrateable risks", sent)
	}
}

// The de-dup that the whole design rests on: a second sweep the same day —
// another replica, or a manual re-run — must send nothing again.
func TestRunOnceDoesNotResendWhatIsAlreadyClaimed(t *testing.T) {
	risks := &reminderRisks{items: []*model.RiskListItem{
		{ID: 1, RiskLevel: "HIGH", ImplementationDate: dueIn(5)},
	}}
	claimer := newFakeClaimer()
	var sent []sentReminder
	j := newTestJob(risks, claimer, &sent, nil)

	if err := j.RunOnce(context.Background()); err != nil {
		t.Fatalf("first RunOnce: %v", err)
	}
	if err := j.RunOnce(context.Background()); err != nil {
		t.Fatalf("second RunOnce: %v", err)
	}

	if len(sent) != 1 {
		t.Fatalf("sent %d reminders across two sweeps, want exactly 1", len(sent))
	}
	if len(claimer.calls) != 2 {
		t.Fatalf("claim attempts = %d, want 2 (the second must be attempted and lose)", len(claimer.calls))
	}
}

// A failed send must give the claim back, or the reminder is lost while the
// log says it went out.
func TestRunOnceReleasesTheClaimWhenTheEmailFails(t *testing.T) {
	risks := &reminderRisks{items: []*model.RiskListItem{
		{ID: 1, RiskLevel: "HIGH", ImplementationDate: dueIn(15)},
	}}
	claimer := newFakeClaimer()
	var sent []sentReminder
	j := newTestJob(risks, claimer, &sent, map[int]bool{1: true})

	if err := j.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if len(claimer.released) != 1 {
		t.Fatalf("released %v, want the one failed claim back", claimer.released)
	}
	if len(sent) != 0 {
		t.Fatalf("sent %+v despite the send failing", sent)
	}
}

// A claim the entity couldn't answer must fail CLOSED: we can't tell whether
// we hold it, and a duplicate email is worse than a missed nudge. The rest of
// the sweep carries on.
func TestRunOnceFailsClosedOnClaimError(t *testing.T) {
	risks := &reminderRisks{items: []*model.RiskListItem{
		{ID: 1, RiskLevel: "HIGH", ImplementationDate: dueIn(0)},
		{ID: 2, RiskLevel: "HIGH", ImplementationDate: dueIn(0)},
	}}
	claimer := newFakeClaimer()
	claimer.failClaim[1] = true
	var sent []sentReminder
	j := newTestJob(risks, claimer, &sent, nil)

	if err := j.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if len(sent) != 1 || sent[0].riskID != 2 {
		t.Fatalf("sent %+v, want only risk 2 — risk 1's claim errored", sent)
	}
}

// The sweep asks only for risks inside the reminder window, and only for ones
// still in remediation. Widening either would mean reminding people about
// risks that can't be reminded (approval stages) or won't fire today.
func TestRunOnceQueriesOnlyTheReminderWindow(t *testing.T) {
	risks := &reminderRisks{}
	var sent []sentReminder
	j := newTestJob(risks, newFakeClaimer(), &sent, nil)

	if err := j.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	today := dateOnly(time.Now().UTC())
	f := risks.lastFilter
	if len(f.Statuses) != 1 || f.Statuses[0] != model.StatusApproved {
		t.Errorf("statuses = %v, want only APPROVED", f.Statuses)
	}
	if f.DueFrom != today.Format(dateLayout) {
		t.Errorf("dueFrom = %q, want today %q", f.DueFrom, today.Format(dateLayout))
	}
	wantTo := today.AddDate(0, 0, model.MaxReminderLeadDays).Format(dateLayout)
	if f.DueTo != wantTo {
		t.Errorf("dueTo = %q, want %q", f.DueTo, wantTo)
	}
}

// Paging must walk forward. Unlike the escalation sweep nothing leaves the
// result set here, so re-querying from offset 0 would loop forever.
func TestRunOncePagesForwardThroughEveryRisk(t *testing.T) {
	items := make([]*model.RiskListItem, 0, pageLimit*2+7)
	for i := 1; i <= pageLimit*2+7; i++ {
		items = append(items, &model.RiskListItem{ID: i, RiskLevel: "LOW", ImplementationDate: dueIn(0)})
	}
	risks := &reminderRisks{items: items}
	var sent []sentReminder
	j := newTestJob(risks, newFakeClaimer(), &sent, nil)

	if err := j.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if len(sent) != len(items) {
		t.Fatalf("sent %d reminders, want %d (one per risk)", len(sent), len(items))
	}
}

// A second sweep starting while one is in flight is rejected rather than
// allowed to run alongside it — two concurrent sweeps would both claim and
// both send for any risk whose claim the other hadn't written yet.
func TestRunOnceRejectsAnOverlappingSweep(t *testing.T) {
	risks := &reminderRisks{items: []*model.RiskListItem{
		{ID: 1, RiskLevel: "LOW", ImplementationDate: dueIn(0)},
	}}
	started := make(chan struct{})
	release := make(chan struct{})

	j := NewReminderJob(risks, newFakeClaimer(), func(context.Context, int, string, string) error {
		close(started)
		<-release // hold the first sweep open
		return nil
	})

	done := make(chan error, 1)
	go func() { done <- j.RunOnce(context.Background()) }()
	<-started

	if err := j.RunOnce(context.Background()); err == nil {
		t.Error("second RunOnce started a sweep while one was already running")
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("first RunOnce: %v", err)
	}
	// And once it has finished, the job is usable again.
	if err := j.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce after the sweep finished: %v", err)
	}
}

// The due date the reminder carries — and de-dups on — is the risk's own date
// in YYYY-MM-DD, whichever shape it arrived in.
func TestRunOnceNormalisesTheDueDate(t *testing.T) {
	iso := dateOnly(time.Now().UTC()).AddDate(0, 0, 5).Format(dateLayout)
	risks := &reminderRisks{items: []*model.RiskListItem{
		{ID: 1, RiskLevel: "MEDIUM", ImplementationDate: &iso}, // bare YYYY-MM-DD
		{ID: 2, RiskLevel: "MEDIUM", ImplementationDate: dueIn(5)},
	}}
	claimer := newFakeClaimer()
	var sent []sentReminder
	j := newTestJob(risks, claimer, &sent, nil)

	if err := j.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if len(sent) != 2 {
		t.Fatalf("sent %+v, want both risks reminded", sent)
	}
	for _, s := range sent {
		if s.dueDate != iso {
			t.Errorf("dueDate = %q, want %q", s.dueDate, iso)
		}
	}
	for _, c := range claimer.calls {
		if c.dueDate != iso {
			t.Errorf("claim dueDate = %q, want %q", c.dueDate, iso)
		}
	}
}
