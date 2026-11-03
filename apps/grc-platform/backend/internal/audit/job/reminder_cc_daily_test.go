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
	"slices"
	"testing"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
)

var adminCC = []string{"admin.a@x.com", "admin.b@x.com"}

// ccSweep is what one sweep delivered, with the CC each send carried.
type ccSweep struct {
	reminders   map[int][]model.ReminderItem
	reminderCC  map[int][]string
	escalations map[int][]model.ReminderItem
	leads       map[int][]model.ReminderItem
	leadCC      map[int][]string
}

type ccOptions struct {
	cc        func(context.Context, []int) []string // nil leaves WithAdminCC off
	daily     bool
	claimer   *fakeClaimer
	notifyErr error
}

func runCC(t *testing.T, opts ccOptions, controls ...*model.AuditControl) ccSweep {
	t.Helper()
	res := ccSweep{
		reminders:   map[int][]model.ReminderItem{},
		reminderCC:  map[int][]string{},
		escalations: map[int][]model.ReminderItem{},
		leads:       map[int][]model.ReminderItem{},
		leadCC:      map[int][]string{},
	}
	if opts.claimer == nil {
		opts.claimer = &fakeClaimer{claimed: map[string]bool{}}
	}
	j := NewReminderJob(
		&fakeAudits{audits: []*model.Audit{{ID: 1, Status: "ACTIVE", Name: "SOC 2"}}},
		&fakeControls{controls: controls}, opts.claimer,
		func(_ context.Context, id int, items []model.ReminderItem) error {
			res.reminders[id] = append(res.reminders[id], items...)
			return opts.notifyErr
		}).
		WithAdminAlerts(adminsFn(routeAdminA, routeAdminB),
			func(_ context.Context, id int, items []model.ReminderItem) error {
				res.escalations[id] = append(res.escalations[id], items...)
				return nil
			}, nil).
		WithLeadAlerts(func(_ context.Context, ids []int) map[int]string {
			out := map[int]string{}
			for _, id := range ids {
				out[id] = "lead@x.com"
			}
			return out
		}, func(_ context.Context, ownerID int, _ string, items []model.ReminderItem) error {
			res.leads[ownerID] = append(res.leads[ownerID], items...)
			return nil
		})
	if opts.cc != nil {
		j = j.WithAdminCC(opts.cc,
			func(_ context.Context, id int, items []model.ReminderItem, cc []string) error {
				res.reminders[id] = append(res.reminders[id], items...)
				res.reminderCC[id] = cc
				return opts.notifyErr
			},
			func(_ context.Context, ownerID int, _ string, items []model.ReminderItem, cc []string) error {
				res.leads[ownerID] = append(res.leads[ownerID], items...)
				res.leadCC[ownerID] = cc
				return nil
			})
	}
	if opts.daily {
		j = j.WithDailyReminders()
	}
	if err := j.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce: %v", err)
	}
	return res
}

func withCC(list ...string) func(context.Context, []int) []string {
	return func(context.Context, []int) []string { return list }
}

func TestCCOnAssigneeDigest(t *testing.T) {
	for _, status := range []string{"EVIDENCE_PENDING", "EVIDENCE_NEED_CLARIFICATION"} {
		res := runCC(t, ccOptions{cc: withCC(adminCC...)}, routedControl(status, 5, 5))
		if !slices.Equal(res.reminderCC[routeOwner], adminCC) {
			t.Errorf("%s: owner digest CC = %v, want %v", status, res.reminderCC[routeOwner], adminCC)
		}
	}
	// An auditor's reminder is copied the same way.
	res := runCC(t, ccOptions{cc: withCC(adminCC...)}, routedControl("EVIDENCE_UNDER_VALIDATION", 5, 5))
	if !slices.Equal(res.reminderCC[routeAuditor], adminCC) {
		t.Errorf("auditor digest CC = %v, want %v", res.reminderCC[routeAuditor], adminCC)
	}
}

func TestNoCCWhenNotWired(t *testing.T) {
	res := runCC(t, ccOptions{}, routedControl("EVIDENCE_PENDING", 5, 5))
	if len(res.reminderCC) != 0 {
		t.Errorf("CC recorded without WithAdminCC: %v", res.reminderCC)
	}
	if len(res.reminders[routeOwner]) != 1 {
		t.Errorf("owner reminders = %v, want 1 via the plain notify", res.reminders[routeOwner])
	}
}

// The admins are the To on their own review digest, so copying the rest of the
// team on it would add nothing.
func TestNoCCOnAdminDirectedDigest(t *testing.T) {
	res := runCC(t, ccOptions{cc: withCC(adminCC...)}, routedControl("EVIDENCE_INTERNAL_REVIEW", 5, 5))
	for _, id := range []int{routeAdminA, routeAdminB} {
		if len(res.reminders[id]) != 1 {
			t.Fatalf("admin %d reminders = %v, want 1", id, res.reminders[id])
		}
		if cc, ok := res.reminderCC[id]; ok {
			t.Errorf("admin %d digest carried CC %v, want none", id, cc)
		}
	}
}

// With the admins copied on the assignee's email, the separate digest would
// only repeat it, for the owner and the lead alike.
func TestOverdueAssignedItemCopiesAdminsAndSkipsDigest(t *testing.T) {
	res := runCC(t, ccOptions{cc: withCC(adminCC...)}, routedControl("EVIDENCE_PENDING", -2, -2))
	if len(res.escalations) != 0 {
		t.Errorf("admin digest sent for an assigned item: %v", res.escalations)
	}
	if !slices.Equal(res.reminderCC[routeOwner], adminCC) {
		t.Errorf("owner CC = %v, want %v", res.reminderCC[routeOwner], adminCC)
	}
	if len(res.leads[routeOwner]) != 1 || !slices.Equal(res.leadCC[routeOwner], adminCC) {
		t.Errorf("lead email = %v with CC %v, want 1 item copied to %v",
			res.leads[routeOwner], res.leadCC[routeOwner], adminCC)
	}
}

func TestOverdueAuditorItemCopiesAdminsAndSkipsDigest(t *testing.T) {
	res := runCC(t, ccOptions{cc: withCC(adminCC...)}, routedControl("EVIDENCE_UNDER_VALIDATION", -2, -2))
	if len(res.escalations) != 0 {
		t.Errorf("admin digest sent for an assigned item: %v", res.escalations)
	}
	if !slices.Equal(res.reminderCC[routeAuditor], adminCC) {
		t.Errorf("auditor CC = %v, want %v", res.reminderCC[routeAuditor], adminCC)
	}
	if len(res.leads) != 0 {
		t.Errorf("an auditor has no lead email, got %v", res.leads)
	}
}

// If the CC list can't be resolved the admins would hear nothing, so the
// digest stays.
func TestOverdueKeepsDigestWhenCCListEmpty(t *testing.T) {
	res := runCC(t, ccOptions{cc: withCC()}, routedControl("EVIDENCE_PENDING", -2, -2))
	if len(res.escalations[routeAdminA]) != 1 || len(res.escalations[routeAdminB]) != 1 {
		t.Errorf("escalations = %v, want one item per admin", res.escalations)
	}
	if _, ok := res.reminderCC[routeOwner]; ok {
		t.Errorf("owner digest carried a CC with an empty list")
	}
}

// Nobody to copy the admins on: the digest is their only notice.
func TestOverdueUnassignedItemKeepsDigest(t *testing.T) {
	c := routedControl("EVIDENCE_PENDING", -2, -2)
	c.OwnerID = nil
	res := runCC(t, ccOptions{cc: withCC(adminCC...)}, c)
	if len(res.escalations[routeAdminA]) != 1 || len(res.escalations[routeAdminB]) != 1 {
		t.Errorf("escalations = %v, want one item per admin", res.escalations)
	}
}

func TestOverdueInternalReviewKeepsDigest(t *testing.T) {
	res := runCC(t, ccOptions{cc: withCC(adminCC...)}, routedControl("EVIDENCE_INTERNAL_REVIEW", -2, -2))
	if len(res.escalations[routeAdminA]) != 1 || len(res.escalations[routeAdminB]) != 1 {
		t.Errorf("escalations = %v, want one item per admin", res.escalations)
	}
}

// An admin who owns an item is the To of its digest, and the other admins
// must still hear about it.
func TestOverdueItemOwnedByAdminStillCopiesTheOthers(t *testing.T) {
	c := routedControl("EVIDENCE_PENDING", -2, -2)
	c.OwnerID = intPtr(routeAdminA)
	res := runCC(t, ccOptions{cc: withCC(adminCC...)}, c)
	if !slices.Equal(res.reminderCC[routeAdminA], adminCC) {
		t.Errorf("admin-owned digest CC = %v, want %v", res.reminderCC[routeAdminA], adminCC)
	}
	if len(res.escalations) != 0 {
		t.Errorf("admin digest sent as well as the CC: %v", res.escalations)
	}
}

func TestFailedCCSendReleasesClaim(t *testing.T) {
	claimer := &fakeClaimer{claimed: map[string]bool{}}
	runCC(t, ccOptions{cc: withCC(adminCC...), claimer: claimer, notifyErr: errors.New("boom")},
		routedControl("EVIDENCE_PENDING", 5, 5))
	if len(claimer.released) != 1 {
		t.Errorf("released = %v, want the one failed claim given back", claimer.released)
	}
}

func daily(statuses ...string) []*model.AuditControl {
	out := make([]*model.AuditControl, 0, len(statuses))
	for i, s := range statuses {
		c := routedControl(s, 30, 30)
		c.ID = i + 1
		out = append(out, c)
	}
	return out
}

// 30 days out is no due-date mark, so these only remind because the status does.
func TestDailyStatusesRemindOnANonMarkDay(t *testing.T) {
	cases := []struct {
		status string
		to     []int
	}{
		{"POPULATION_INTERNAL_REVIEW", []int{routeAdminA, routeAdminB}},
		{"EVIDENCE_INTERNAL_REVIEW", []int{routeAdminA, routeAdminB}},
		{"SUBMITTED_SAMPLE", []int{routeAdminA, routeAdminB}},
		{"POPULATION_UNDER_VALIDATION", []int{routeAuditor}},
		{"EVIDENCE_UNDER_VALIDATION", []int{routeAuditor}},
		{"POPULATION_COMPLETE", []int{routeAuditor}},
		{"AWAITING_SAMPLE", []int{routeAuditor}},
	}
	today := time.Now().UTC().Format("2006-01-02")
	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			res := runCC(t, ccOptions{daily: true}, daily(tc.status)...)
			if got := recipients(res.reminders); !slices.Equal(got, tc.to) {
				t.Fatalf("recipients = %v, want %v", got, tc.to)
			}
			it := res.reminders[tc.to[0]][0]
			if !it.Daily || it.Tier != "Awaiting action" || it.Type != "REMINDER_DUE_TODAY" || it.DedupSnapshot != today {
				t.Errorf("item = %+v, want a daily item logged as DUE_TODAY for %s", it, today)
			}
		})
	}
}

func TestDailyStatusesStaySilentWhenOff(t *testing.T) {
	res := runCC(t, ccOptions{}, daily("EVIDENCE_UNDER_VALIDATION", "EVIDENCE_INTERNAL_REVIEW", "AWAITING_SAMPLE")...)
	if len(res.reminders) != 0 {
		t.Errorf("reminders = %v, want none off the due-date marks", res.reminders)
	}
}

// Statuses that wait on an owner keep the 10/5/0/overdue cadence.
func TestOwnerStatusesAreNotDaily(t *testing.T) {
	res := runCC(t, ccOptions{daily: true}, daily("EVIDENCE_PENDING", "POPULATION_NEED_CLARIFICATION")...)
	if len(res.reminders) != 0 {
		t.Errorf("reminders = %v, want none off the due-date marks", res.reminders)
	}
}

func TestDailyOffKeepsSubmittedSampleWithOwner(t *testing.T) {
	res := runCC(t, ccOptions{}, routedControl("SUBMITTED_SAMPLE", 5, 5))
	if got := recipients(res.reminders); !slices.Equal(got, []int{routeOwner}) {
		t.Errorf("recipients = %v, want the owner", got)
	}
}

func TestDailyReminderSendsOncePerDay(t *testing.T) {
	claimer := &fakeClaimer{claimed: map[string]bool{}}
	c := daily("EVIDENCE_UNDER_VALIDATION")
	first := runCC(t, ccOptions{daily: true, claimer: claimer}, c...)
	second := runCC(t, ccOptions{daily: true, claimer: claimer}, c...)
	if len(first.reminders[routeAuditor]) != 1 || len(second.reminders) != 0 {
		t.Errorf("first = %v, second = %v, want one send then none the same day", first.reminders, second.reminders)
	}
}

// A mark day keeps its own tier, so the wording says how far off it is.
func TestDailyStatusOnAMarkDayKeepsItsTier(t *testing.T) {
	res := runCC(t, ccOptions{daily: true}, routedControl("EVIDENCE_UNDER_VALIDATION", 5, 5))
	it := res.reminders[routeAuditor][0]
	if it.Daily || it.Type != "REMINDER_DUE_5" {
		t.Errorf("item = %+v, want the due-in-5 tier, not a daily item", it)
	}
}

func TestDailyAuditorItemIsCopiedToAdmins(t *testing.T) {
	res := runCC(t, ccOptions{daily: true, cc: withCC(adminCC...)}, daily("AWAITING_SAMPLE")...)
	if !slices.Equal(res.reminderCC[routeAuditor], adminCC) {
		t.Errorf("auditor CC = %v, want %v", res.reminderCC[routeAuditor], adminCC)
	}
}

// The CC list is built from the admins the sweep already resolved, not looked
// up a second time.
func TestCCResolvedFromTheSweepsAdmins(t *testing.T) {
	var got []int
	runCC(t, ccOptions{cc: func(_ context.Context, ids []int) []string {
		got = ids
		return adminCC
	}}, routedControl("EVIDENCE_PENDING", 5, 5))
	if !slices.Equal(got, []int{routeAdminA, routeAdminB}) {
		t.Errorf("CC resolved from %v, want the sweep's admins", got)
	}
}

// A half-wired CC would copy some emails and not others, so it stays off.
func TestWithAdminCCNeedsAllThreeFunctions(t *testing.T) {
	j := NewReminderJob(nil, nil, nil, nil).WithAdminCC(withCC(adminCC...), nil, nil)
	if j.ccEmails != nil || j.notifyCC != nil || j.notifyLeadCC != nil {
		t.Error("WithAdminCC wired part of the CC with a nil function")
	}
}
