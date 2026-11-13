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
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

package deadline

import (
	"fmt"
	"testing"
	"time"
)

func day(m time.Month, d int) time.Time { return time.Date(2026, m, d, 0, 0, 0, 0, Location) }

func at(m time.Month, d, h, min int) time.Time {
	return time.Date(2026, m, d, h, min, 0, 0, Location)
}

func ptr[T any](v T) *T { return &v }

func logString(ev Evaluation) []string {
	out := []string{}
	names := map[string]string{Owner: "Internal", Compliance: "GRC", Auditor: "EY"}
	for _, e := range ev.DelayLog {
		out = append(out, fmt.Sprintf("%s - %02d/%02d", names[e.HandledBy], int(e.Date.Month()), e.Date.Day()))
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestHandlerForStatus(t *testing.T) {
	cases := map[string]string{
		"EVIDENCE_PENDING":            Owner,
		"EVIDENCE_NEED_CLARIFICATION": Owner,
		"EVIDENCE_INTERNAL_REVIEW":    Compliance,
		"EVIDENCE_UNDER_VALIDATION":   Auditor,
	}
	for status, want := range cases {
		got, ok := HandlerForStatus(status)
		if !ok || got != want {
			t.Errorf("%s: got %q,%v want %q", status, got, ok, want)
		}
	}
	for _, status := range []string{"COMPLETE", "POPULATION_PENDING", "AWAITING_SAMPLE", "SUBMITTED_SAMPLE"} {
		if _, ok := HandlerForStatus(status); ok {
			t.Errorf("%s should not be tracked", status)
		}
	}
}

func TestPlannedDates(t *testing.T) {
	c, a := PlannedDates(day(time.November, 7), DefaultDays())
	if FormatDay(c) != "2026-11-17" || FormatDay(a) != "2026-11-27" {
		t.Fatalf("got %s %s", FormatDay(c), FormatDay(a))
	}
}

func TestDaysFallBackToDefaults(t *testing.T) {
	d := Days{}
	if d.For(Initial, Compliance) != 10 || d.For(Revision, Owner) != 15 {
		t.Fatal("defaults not applied")
	}
	d.Set(Initial, Compliance, 12)
	if d.For(Initial, Compliance) != 12 || d.For(Initial, Auditor) != 10 {
		t.Fatal("custom value or fallback wrong")
	}
}

func TestPlanTransition(t *testing.T) {
	open := func(h, cycle string) *Stage { return &Stage{HandledBy: h, CycleType: cycle} }
	now := at(time.November, 6, 10, 0)
	cases := []struct {
		name      string
		in        TransitionInput
		wantClose bool
		wantOpen  *NewStage
	}{
		{
			name:      "owner submits, initial cycle: GRC gets 10 days from submit day",
			in:        TransitionInput{From: "EVIDENCE_PENDING", To: "EVIDENCE_INTERNAL_REVIEW", OpenStage: open(Owner, Initial), PrevCycle: Initial},
			wantClose: true,
			wantOpen:  &NewStage{HandledBy: Compliance, CycleType: Initial, StartedAt: now, DueDate: day(time.November, 16)},
		},
		{
			name:      "GRC approves: EY gets 10 days",
			in:        TransitionInput{From: "EVIDENCE_INTERNAL_REVIEW", To: "EVIDENCE_UNDER_VALIDATION", OpenStage: open(Compliance, Initial), PrevCycle: Initial},
			wantClose: true,
			wantOpen:  &NewStage{HandledBy: Auditor, CycleType: Initial, StartedAt: now, DueDate: day(time.November, 16)},
		},
		{
			name:      "EY completes: stage closes, nothing opens",
			in:        TransitionInput{From: "EVIDENCE_UNDER_VALIDATION", To: "COMPLETE", OpenStage: open(Auditor, Initial), PrevCycle: Initial},
			wantClose: true,
		},
		{
			name:      "GRC rejects: owner gets 15 days, revision",
			in:        TransitionInput{From: "EVIDENCE_INTERNAL_REVIEW", To: "EVIDENCE_PENDING", OpenStage: open(Compliance, Initial), PrevCycle: Initial},
			wantClose: true,
			wantOpen:  &NewStage{HandledBy: Owner, CycleType: Revision, StartedAt: now, DueDate: day(time.November, 21)},
		},
		{
			name:      "EY rejects: owner gets 15 days, revision",
			in:        TransitionInput{From: "EVIDENCE_UNDER_VALIDATION", To: "EVIDENCE_NEED_CLARIFICATION", OpenStage: open(Auditor, Initial), PrevCycle: Initial},
			wantClose: true,
			wantOpen:  &NewStage{HandledBy: Owner, CycleType: Revision, StartedAt: now, DueDate: day(time.November, 21)},
		},
		{
			name:      "owner resubmits after a rejection: GRC gets 5 days (stays revision)",
			in:        TransitionInput{From: "EVIDENCE_NEED_CLARIFICATION", To: "EVIDENCE_INTERNAL_REVIEW", OpenStage: open(Owner, Revision), PrevCycle: Revision},
			wantClose: true,
			wantOpen:  &NewStage{HandledBy: Compliance, CycleType: Revision, StartedAt: now, DueDate: day(time.November, 11)},
		},
		{
			name:      "admin override from COMPLETE opens a revision stage, nothing to close",
			in:        TransitionInput{From: "COMPLETE", To: "EVIDENCE_UNDER_VALIDATION", Override: true, PrevCycle: Initial},
			wantClose: false,
			wantOpen:  &NewStage{HandledBy: Auditor, CycleType: Revision, StartedAt: now, DueDate: day(time.November, 11)},
		},
		{
			name: "no-op transition changes nothing",
			in:   TransitionInput{From: "EVIDENCE_PENDING", To: "EVIDENCE_PENDING", OpenStage: open(Owner, Initial)},
		},
		{
			name: "population statuses are not tracked",
			in:   TransitionInput{From: "POPULATION_PENDING", To: "POPULATION_INTERNAL_REVIEW"},
		},
		{
			name: "same handler already holds it: nothing changes",
			in:   TransitionInput{From: "EVIDENCE_NEED_CLARIFICATION", To: "EVIDENCE_PENDING", OpenStage: open(Owner, Revision), PrevCycle: Revision},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.in.Now = now
			tc.in.Days = DefaultDays()
			got := PlanTransition(tc.in)
			if got.CloseOpen != tc.wantClose {
				t.Errorf("CloseOpen = %v, want %v", got.CloseOpen, tc.wantClose)
			}
			switch {
			case tc.wantOpen == nil && got.Open != nil:
				t.Errorf("unexpected open stage %+v", *got.Open)
			case tc.wantOpen != nil && got.Open == nil:
				t.Errorf("expected open stage %+v", *tc.wantOpen)
			case tc.wantOpen != nil:
				if got.Open.HandledBy != tc.wantOpen.HandledBy || got.Open.CycleType != tc.wantOpen.CycleType ||
					!got.Open.DueDate.Equal(tc.wantOpen.DueDate) {
					t.Errorf("open = %+v, want %+v", *got.Open, *tc.wantOpen)
				}
			}
		})
	}
}

// A control that was already waiting on the owner after an auditor rejection when
// tracking began has no stage history, but its resubmission is still a revision.
func TestPlanTransitionResubmitWithoutHistoryIsRevision(t *testing.T) {
	now := at(time.November, 6, 10, 0)
	got := PlanTransition(TransitionInput{
		From: "EVIDENCE_NEED_CLARIFICATION", To: "EVIDENCE_INTERNAL_REVIEW", Now: now, Days: DefaultDays(),
	})
	want := &NewStage{HandledBy: Compliance, CycleType: Revision, StartedAt: now, DueDate: day(time.November, 11)}
	if got.Open == nil || *got.Open != *want {
		t.Fatalf("got %+v, want %+v", got.Open, want)
	}
}

// The deadline is counted from the calendar day in Asia/Colombo, not from UTC:
// 23:30 UTC on 9 Nov is already 05:00 on 10 Nov in Sri Lanka.
func TestPlanTransitionUsesColomboDay(t *testing.T) {
	now := time.Date(2026, time.November, 9, 23, 30, 0, 0, time.UTC)
	got := PlanTransition(TransitionInput{
		From: "EVIDENCE_PENDING", To: "EVIDENCE_INTERNAL_REVIEW", Now: now,
		OpenStage: &Stage{HandledBy: Owner, CycleType: Initial}, PrevCycle: Initial, Days: DefaultDays(),
	})
	if got.Open == nil || FormatDay(got.Open.DueDate) != "2026-11-20" {
		t.Fatalf("got %+v", got.Open)
	}
}

func TestRevisionDeadlineMonthBoundary(t *testing.T) {
	got := PlanTransition(TransitionInput{
		From: "EVIDENCE_UNDER_VALIDATION", To: "EVIDENCE_NEED_CLARIFICATION", Now: at(time.November, 24, 12, 0),
		OpenStage: &Stage{HandledBy: Auditor, CycleType: Initial}, PrevCycle: Initial, Days: DefaultDays(),
	})
	if FormatDay(got.Open.DueDate) != "2026-12-09" {
		t.Fatalf("24 Nov + 15 days should be 9 Dec, got %s", FormatDay(got.Open.DueDate))
	}
}

// Example A: nobody is late, the control completes before the original EY date.
func TestEvaluateBestCase(t *testing.T) {
	stages := []Stage{
		{ID: 1, HandledBy: Owner, CycleType: Initial, StartedAt: at(time.November, 1, 9, 0), DueDate: day(time.November, 7),
			PlannedComplianceDate: ptr(day(time.November, 17)), PlannedAuditorDate: ptr(day(time.November, 27)), EndedAt: ptr(at(time.November, 6, 10, 0))},
		{ID: 2, HandledBy: Compliance, CycleType: Initial, StartedAt: at(time.November, 6, 10, 0), DueDate: day(time.November, 16), EndedAt: ptr(at(time.November, 14, 16, 0))},
		{ID: 3, HandledBy: Auditor, CycleType: Initial, StartedAt: at(time.November, 14, 16, 0), DueDate: day(time.November, 24), EndedAt: ptr(at(time.November, 24, 12, 0))},
	}
	ev := Evaluate(stages, at(time.December, 1, 9, 0))
	if ev.Delayed || len(ev.DelayLog) != 0 {
		t.Fatalf("expected no delay, got delayed=%v log=%v", ev.Delayed, logString(ev))
	}
	if ev.CurrentHandler != nil || ev.CurrentDueDate != nil {
		t.Fatal("a completed control has no current handler")
	}
	if FormatDay(*ev.PlannedOwnerDate) != "2026-11-07" || FormatDay(*ev.PlannedComplianceDate) != "2026-11-17" || FormatDay(*ev.PlannedAuditorDate) != "2026-11-27" {
		t.Fatal("planned dates wrong")
	}
}

// Example B: late at every step. The report changes as days pass.
func TestEvaluateLateAtEveryStep(t *testing.T) {
	complete := []Stage{
		{ID: 1, HandledBy: Owner, CycleType: Initial, StartedAt: at(time.November, 1, 9, 0), DueDate: day(time.November, 7),
			PlannedComplianceDate: ptr(day(time.November, 17)), PlannedAuditorDate: ptr(day(time.November, 27)), EndedAt: ptr(at(time.November, 10, 14, 0))},
		{ID: 2, HandledBy: Compliance, CycleType: Initial, StartedAt: at(time.November, 10, 14, 0), DueDate: day(time.November, 20), EndedAt: ptr(at(time.November, 23, 21, 0))},
		{ID: 3, HandledBy: Auditor, CycleType: Initial, StartedAt: at(time.November, 23, 21, 0), DueDate: day(time.December, 3), EndedAt: ptr(at(time.December, 5, 11, 0))},
	}
	// Truncate the history to what was recorded at a given moment.
	asOf := func(now time.Time) []Stage {
		out := []Stage{}
		for _, s := range complete {
			if s.StartedAt.After(now) {
				continue
			}
			if s.EndedAt != nil && s.EndedAt.After(now) {
				s.EndedAt = nil
			}
			out = append(out, s)
		}
		return out
	}
	cases := []struct {
		name    string
		now     time.Time
		handler string
		due     string
		delayed bool
		log     []string
	}{
		{"5 Nov, on time", at(time.November, 5, 12, 0), Owner, "2026-11-07", false, []string{}},
		{"8 Nov, internal late one day", at(time.November, 8, 12, 0), Owner, "2026-11-07", false, []string{"Internal - 11/08"}},
		{"10 Nov before submit", at(time.November, 10, 9, 0), Owner, "2026-11-07", false, []string{"Internal - 11/08", "Internal - 11/09", "Internal - 11/10"}},
		{"11 Nov, with GRC", at(time.November, 11, 9, 0), Compliance, "2026-11-20", false, []string{"Internal - 11/08", "Internal - 11/09", "Internal - 11/10"}},
		{"22 Nov, GRC late", at(time.November, 22, 9, 0), Compliance, "2026-11-20", false,
			[]string{"Internal - 11/08", "Internal - 11/09", "Internal - 11/10", "GRC - 11/21", "GRC - 11/22"}},
		{"24 Nov, with EY", at(time.November, 24, 9, 0), Auditor, "2026-12-03", false,
			[]string{"Internal - 11/08", "Internal - 11/09", "Internal - 11/10", "GRC - 11/21", "GRC - 11/22", "GRC - 11/23"}},
		{"28 Nov, original EY date missed", at(time.November, 28, 9, 0), Auditor, "2026-12-03", true,
			[]string{"Internal - 11/08", "Internal - 11/09", "Internal - 11/10", "GRC - 11/21", "GRC - 11/22", "GRC - 11/23"}},
		{"4 Dec, EY late", at(time.December, 4, 9, 0), Auditor, "2026-12-03", true,
			[]string{"Internal - 11/08", "Internal - 11/09", "Internal - 11/10", "GRC - 11/21", "GRC - 11/22", "GRC - 11/23", "EY - 12/04"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev := Evaluate(asOf(tc.now), tc.now)
			if ev.CurrentHandler == nil || *ev.CurrentHandler != tc.handler || FormatDay(*ev.CurrentDueDate) != tc.due {
				t.Errorf("current = %v %v, want %s %s", ev.CurrentHandler, ev.CurrentDueDate, tc.handler, tc.due)
			}
			if ev.Delayed != tc.delayed {
				t.Errorf("delayed = %v, want %v", ev.Delayed, tc.delayed)
			}
			if got := logString(ev); !equalStrings(got, tc.log) {
				t.Errorf("log = %v, want %v", got, tc.log)
			}
		})
	}

	t.Run("5 Dec approved: complete but stays Delayed, full log", func(t *testing.T) {
		ev := Evaluate(complete, at(time.December, 6, 9, 0))
		want := []string{"Internal - 11/08", "Internal - 11/09", "Internal - 11/10", "GRC - 11/21", "GRC - 11/22", "GRC - 11/23", "EY - 12/04", "EY - 12/05"}
		if got := logString(ev); !equalStrings(got, want) {
			t.Errorf("log = %v, want %v", got, want)
		}
		if !ev.Delayed {
			t.Error("a control that finished after the original EY date stays Delayed")
		}
		if ev.CurrentHandler != nil {
			t.Error("a completed control has no current handler")
		}
	})
}

// Example C: a rejection with nobody late logs nothing and is not Delayed.
func TestEvaluateRejectionWithoutLateness(t *testing.T) {
	stages := []Stage{
		{ID: 1, HandledBy: Owner, CycleType: Initial, StartedAt: at(time.November, 1, 9, 0), DueDate: day(time.November, 7),
			PlannedComplianceDate: ptr(day(time.November, 17)), PlannedAuditorDate: ptr(day(time.November, 27)), EndedAt: ptr(at(time.November, 6, 10, 0))},
		{ID: 2, HandledBy: Compliance, CycleType: Initial, StartedAt: at(time.November, 6, 10, 0), DueDate: day(time.November, 16), EndedAt: ptr(at(time.November, 14, 16, 0))},
		{ID: 3, HandledBy: Auditor, CycleType: Initial, StartedAt: at(time.November, 14, 16, 0), DueDate: day(time.November, 24), EndedAt: ptr(at(time.November, 24, 12, 0))},
		{ID: 4, HandledBy: Owner, CycleType: Revision, StartedAt: at(time.November, 24, 12, 0), DueDate: day(time.December, 9)},
	}
	ev := Evaluate(stages, at(time.December, 1, 9, 0))
	if ev.Delayed || len(ev.DelayLog) != 0 {
		t.Fatalf("expected no delay, got delayed=%v log=%v", ev.Delayed, logString(ev))
	}
	if ev.CurrentHandler == nil || *ev.CurrentHandler != Owner || FormatDay(*ev.CurrentDueDate) != "2026-12-09" {
		t.Fatalf("current = %v %v", ev.CurrentHandler, ev.CurrentDueDate)
	}

	// Once 9 Dec passes with no submission, the revision stage is late and the control is Delayed.
	late := Evaluate(stages, at(time.December, 11, 9, 0))
	if !late.Delayed {
		t.Fatal("a missed revision deadline should set Delayed")
	}
	if got := logString(late); !equalStrings(got, []string{"Internal - 12/10", "Internal - 12/11"}) {
		t.Fatalf("log = %v", got)
	}
}

func TestEvaluateBoundaries(t *testing.T) {
	mk := func(end time.Time) []Stage {
		return []Stage{{ID: 1, HandledBy: Owner, CycleType: Initial, StartedAt: at(time.November, 1, 9, 0), DueDate: day(time.November, 7), EndedAt: &end}}
	}
	if ev := Evaluate(mk(at(time.November, 7, 23, 59)), at(time.November, 8, 9, 0)); len(ev.DelayLog) != 0 {
		t.Errorf("handed on at 23:59 on the due day is on time, got %v", logString(ev))
	}
	if ev := Evaluate(mk(at(time.November, 8, 0, 1)), at(time.November, 8, 9, 0)); len(ev.DelayLog) != 1 {
		t.Errorf("handed on just after midnight is one late day, got %v", logString(ev))
	}
}

func TestEvaluateNoStages(t *testing.T) {
	ev := Evaluate(nil, time.Now())
	if ev.CurrentHandler != nil || ev.Delayed || ev.DelayLog != nil {
		t.Fatal("no stages means nothing to report")
	}
}

// A control created before this feature (lazily tracked) has no planned dates,
// so the initial-cycle Delayed rule cannot fire.
func TestEvaluateWithoutPlannedDates(t *testing.T) {
	stages := []Stage{{ID: 1, HandledBy: Compliance, CycleType: Initial, StartedAt: at(time.November, 1, 9, 0), DueDate: day(time.November, 11)}}
	ev := Evaluate(stages, at(time.December, 20, 9, 0))
	if ev.Delayed {
		t.Fatal("no planned EY date, so the initial rule cannot mark it Delayed")
	}
	if len(ev.DelayLog) == 0 {
		t.Fatal("lateness is still logged")
	}
}
