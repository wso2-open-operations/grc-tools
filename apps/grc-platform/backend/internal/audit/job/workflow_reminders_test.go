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
	"testing"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
)

// wfNow is 13:30 on 10 Dec in Sri Lanka.
var wfNow = time.Date(2026, time.December, 10, 8, 0, 0, 0, time.UTC)

func wfDay(offset int) *string {
	return strPtr(colomboDay(wfNow).AddDate(0, 0, offset).Format("2006-01-02"))
}

// wfControl is a tracked Design control held by its owner, with its ultimate
// deadline ultimate days away and the owner's own deadline individual days away.
func wfControl(ultimate, individual int) *model.AuditControl {
	return &model.AuditControl{
		ID: 1, AuditID: 1, ControlNumber: "CC3.2", Description: "Risk treatment", RequirementType: "DESIGN",
		Status: "EVIDENCE_PENDING", OwnerID: intPtr(routeOwner), AuditorID: intPtr(routeAuditor),
		DueDate: wfDay(ultimate), DeadlineTracked: true, CurrentHandler: strPtr("OWNER"), CurrentDueDate: wfDay(individual),
	}
}

func TestMarkForUltimate(t *testing.T) {
	cases := []struct {
		days     int
		wantType string
		wantText string
		toLead   bool
		none     bool
	}{
		{11, "", "", false, true},
		{10, "REMINDER_DUE_10", "Due in 10 days", false, false},
		{9, "", "", false, true},
		{5, "REMINDER_DUE_5", "Due in 5 days", true, false},
		{4, "", "", false, true},
		{1, "REMINDER_DUE_TODAY", "Due tomorrow", true, false},
		{0, "REMINDER_DUE_TODAY", "Due today", true, false},
		{-1, "REMINDER_OVERDUE", "Overdue by 1 day", true, false},
		{-30, "REMINDER_OVERDUE", "Overdue by 30 days", true, false},
	}
	for _, tc := range cases {
		rs := workflowReminders(wfControl(tc.days, 50), colomboDay(wfNow), "SOC 2")
		if tc.none {
			if len(rs) != 0 {
				t.Errorf("%d days: want no reminder, got %+v", tc.days, rs)
			}
			continue
		}
		if len(rs) != 1 {
			t.Fatalf("%d days: want 1 reminder, got %d", tc.days, len(rs))
		}
		it := rs[0].item
		if it.Type != tc.wantType || it.Kind != kindUltimate || it.ToLead != tc.toLead ||
			it.Tier != "Ultimate deadline - "+tc.wantText {
			t.Errorf("%d days: got type=%s kind=%s toLead=%v tier=%q", tc.days, it.Type, it.Kind, it.ToLead, it.Tier)
		}
		if rs[0].actorID == nil || *rs[0].actorID != routeOwner {
			t.Errorf("%d days: an ultimate reminder goes to the owner", tc.days)
		}
	}
}

func TestMarkForIndividual(t *testing.T) {
	cases := []struct {
		days     int
		wantType string
		none     bool
	}{
		{10, "", true},
		{5, "", true},
		{2, "", true},
		{1, "REMINDER_DUE_TODAY", false},
		{0, "REMINDER_DUE_TODAY", false},
		{-4, "REMINDER_OVERDUE", false},
	}
	for _, tc := range cases {
		rs := workflowReminders(wfControl(50, tc.days), colomboDay(wfNow), "SOC 2")
		if tc.none {
			if len(rs) != 0 {
				t.Errorf("%d days: want no reminder, got %+v", tc.days, rs)
			}
			continue
		}
		if len(rs) != 1 || rs[0].item.Kind != kindIndividual || rs[0].item.Type != tc.wantType || rs[0].item.ToLead {
			t.Errorf("%d days: got %+v", tc.days, rs)
		}
	}
}

// "Due tomorrow" has no audit_notification type of its own: it logs under
// DUE_TODAY with today's date, so it fires once a day and cannot repeat.
func TestDueTomorrowLogsUnderTodayWithTodaysDate(t *testing.T) {
	rs := workflowReminders(wfControl(1, 50), colomboDay(wfNow), "SOC 2")
	if len(rs) != 1 || !rs[0].item.OneDayBefore || rs[0].item.DedupSnapshot != "2026-12-10" {
		t.Fatalf("got %+v", rs)
	}
}

func TestWorkflowRemindersCountDaysInColombo(t *testing.T) {
	// 20:00 UTC on 9 Dec is already 01:30 on 10 Dec in Sri Lanka.
	now := time.Date(2026, time.December, 9, 20, 0, 0, 0, time.UTC)
	c := wfControl(50, 0)
	c.CurrentDueDate = strPtr("2026-12-10")
	rs := workflowReminders(c, colomboDay(now), "SOC 2")
	if len(rs) != 1 || rs[0].item.Tier != "Your deadline - Due today" {
		t.Fatalf("a deadline on the Colombo day is due today, got %+v", rs)
	}
}

func TestSameDayUltimateAndIndividualFoldIntoOne(t *testing.T) {
	rs := workflowReminders(wfControl(1, 1), colomboDay(wfNow), "SOC 2")
	if len(rs) != 1 || len(rs[0].item.Extras) != 1 {
		t.Fatalf("want one reminder with one folded row, got %+v", rs)
	}
	if rs[0].item.Kind != kindUltimate || rs[0].item.Extras[0].Kind != kindIndividual {
		t.Fatalf("ultimate row first, individual folded in: %+v", rs[0].item)
	}
	if got := expandItems([]model.ReminderItem{rs[0].item}); len(got) != 2 {
		t.Fatalf("expandItems = %d rows, want 2", len(got))
	}
	// Only the ultimate row reaches the lead.
	if lead := leadItems([]model.ReminderItem{rs[0].item}); len(lead) != 1 || lead[0].Kind != kindUltimate {
		t.Fatalf("leadItems = %+v", lead)
	}
}

func TestDifferentTypesOnTheSameDayStaySeparate(t *testing.T) {
	// Ultimate overdue and the owner's own deadline due today are different types.
	rs := workflowReminders(wfControl(-2, 0), colomboDay(wfNow), "SOC 2")
	if len(rs) != 2 {
		t.Fatalf("want 2 reminders, got %d", len(rs))
	}
}

func TestIndividualReminderGoesToTheHolder(t *testing.T) {
	cases := []struct {
		handler string
		kind    actorKind
		id      *int
	}{
		{"OWNER", actorOwner, intPtr(routeOwner)},
		{"COMPLIANCE", actorAdmins, nil},
		{"AUDITOR", actorAuditor, intPtr(routeAuditor)},
	}
	for _, tc := range cases {
		c := wfControl(50, 1)
		c.CurrentHandler = strPtr(tc.handler)
		rs := workflowReminders(c, colomboDay(wfNow), "SOC 2")
		if len(rs) != 1 || rs[0].waitsOn != tc.kind {
			t.Fatalf("%s: got %+v", tc.handler, rs)
		}
		if (tc.id == nil) != (rs[0].actorID == nil) || (tc.id != nil && *tc.id != *rs[0].actorID) {
			t.Errorf("%s: wrong recipient %v", tc.handler, rs[0].actorID)
		}
	}
}

func TestWorkflowRemindersSkipWhatTheyDoNotCover(t *testing.T) {
	complete := wfControl(-5, -5)
	complete.Status = "COMPLETE"
	untracked := wfControl(-5, -5)
	untracked.DeadlineTracked, untracked.CurrentHandler, untracked.CurrentDueDate = false, nil, nil

	if rs := workflowReminders(complete, colomboDay(wfNow), "x"); len(rs) != 0 {
		t.Errorf("a completed control reminds nobody: %+v", rs)
	}
	if rs := workflowReminders(untracked, colomboDay(wfNow), "x"); len(rs) != 1 || rs[0].item.Kind != kindUltimate {
		t.Errorf("an untracked control gets the ultimate reminder only: %+v", rs)
	}
}

// An OE control in its population phase: the Owner's own deadline goes to the
// population's owner, and the row says Population Requirement.
func TestOEPopulationRemindersGoToThePopulationOwner(t *testing.T) {
	c := wfControl(50, 1)
	c.RequirementType, c.Status = "OE", "POPULATION_PENDING"
	c.CurrentPhase, c.PopulationOwnerID = strPtr("POPULATION"), intPtr(555)
	rs := workflowReminders(c, colomboDay(wfNow), "SOC 2")
	if len(rs) != 1 {
		t.Fatalf("got %+v", rs)
	}
	got := rs[0]
	if got.item.Kind != kindIndividual || got.actorID == nil || *got.actorID != 555 || got.item.RequirementType != "Population Requirement" {
		t.Errorf("got %+v", got)
	}
	if got.item.Status != "Population Pending" {
		t.Errorf("status label = %q", got.item.Status)
	}
}

// Once the control is in OE evidence the Owner is the control's owner again, and
// the ultimate deadline (the control's due date) still goes to the owner and lead.
func TestOEEvidenceRemindersAndUltimate(t *testing.T) {
	c := wfControl(5, 1)
	c.RequirementType, c.Status = "OE", "SUBMITTED_SAMPLE"
	c.CurrentPhase, c.PopulationOwnerID = strPtr("OE_EVIDENCE"), intPtr(555)
	rs := workflowReminders(c, colomboDay(wfNow), "SOC 2")
	if len(rs) != 2 {
		t.Fatalf("want ultimate + individual, got %d", len(rs))
	}
	for _, r := range rs {
		if r.actorID == nil || *r.actorID != routeOwner || r.item.RequirementType != "Evidence Requirement" {
			t.Errorf("got %+v", r)
		}
	}
	if !rs[0].item.ToLead || rs[0].item.Kind != kindUltimate {
		t.Errorf("ultimate at 5 days also goes to the lead: %+v", rs[0].item)
	}
}

// EY holds the control through the sample, so its deadline goes to the auditor.
func TestOEAuditorWindowGoesToAuditor(t *testing.T) {
	c := wfControl(50, 0)
	c.RequirementType, c.Status = "OE", "AWAITING_SAMPLE"
	c.CurrentPhase, c.CurrentHandler = strPtr("POPULATION"), strPtr("AUDITOR")
	rs := workflowReminders(c, colomboDay(wfNow), "SOC 2")
	if len(rs) != 1 || rs[0].actorID == nil || *rs[0].actorID != routeAuditor || rs[0].waitsOn != actorAuditor {
		t.Fatalf("got %+v", rs)
	}
}

type wfResult struct {
	reminders   map[int][]model.ReminderItem
	escalations map[int][]model.ReminderItem
	ultimate    map[int][]model.ReminderItem
	ultimateCC  map[int][]string
}

func newWorkflowJob(res *wfResult, claimer *fakeClaimer, controls ...*model.AuditControl) *ReminderJob {
	*res = wfResult{
		reminders:   map[int][]model.ReminderItem{},
		escalations: map[int][]model.ReminderItem{},
		ultimate:    map[int][]model.ReminderItem{},
		ultimateCC:  map[int][]string{},
	}
	audits := &fakeAudits{audits: []*model.Audit{{ID: 1, Status: "ACTIVE", Name: "SOC 2"}}}
	leads := func(_ context.Context, ids []int) map[int]string {
		out := map[int]string{}
		for _, id := range ids {
			out[id] = "lead@x.com"
		}
		return out
	}
	j := NewReminderJob(audits, &fakeControls{controls: controls}, claimer,
		func(_ context.Context, id int, items []model.ReminderItem) error {
			res.reminders[id] = append(res.reminders[id], items...)
			return nil
		}).
		WithAdminAlerts(adminsFn(routeAdminA, routeAdminB),
			func(_ context.Context, id int, items []model.ReminderItem) error {
				res.escalations[id] = append(res.escalations[id], items...)
				return nil
			}, func(context.Context, []int) map[int]string { return nil }).
		WithLeadAlerts(leads, func(context.Context, int, string, []model.ReminderItem) error { return nil }).
		WithWorkflowReminders(func(_ context.Context, ownerID int, _ string, items []model.ReminderItem, cc []string) error {
			res.ultimate[ownerID] = append(res.ultimate[ownerID], items...)
			res.ultimateCC[ownerID] = cc
			return nil
		})
	j.now = func() time.Time { return wfNow }
	return j
}

func TestWorkflowSweepOwnerAndLeadAtFiveDays(t *testing.T) {
	var res wfResult
	j := newWorkflowJob(&res, &fakeClaimer{claimed: map[string]bool{}}, wfControl(5, 20))
	if err := j.runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := res.reminders[routeOwner]
	if len(got) != 1 || got[0].Tier != "Ultimate deadline - Due in 5 days" || got[0].CurrentlyWith != "Owner" {
		t.Fatalf("owner digest = %+v", got)
	}
	if lead := res.ultimate[routeOwner]; len(lead) != 1 || lead[0].Kind != kindUltimate {
		t.Fatalf("lead digest = %+v", lead)
	}
}

func TestWorkflowSweepTenDaysReachesTheOwnerOnly(t *testing.T) {
	var res wfResult
	j := newWorkflowJob(&res, &fakeClaimer{claimed: map[string]bool{}}, wfControl(10, 20))
	if err := j.runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(res.reminders[routeOwner]) != 1 {
		t.Fatalf("owner digest = %+v", res.reminders)
	}
	if len(res.ultimate) != 0 {
		t.Fatalf("the lead hears nothing at 10 days: %+v", res.ultimate)
	}
}

func TestWorkflowSweepIndividualDeadlineGoesToTheHolderNotTheLead(t *testing.T) {
	var res wfResult
	c := wfControl(50, 1)
	c.Status, c.CurrentHandler = "EVIDENCE_UNDER_VALIDATION", strPtr("AUDITOR")
	j := newWorkflowJob(&res, &fakeClaimer{claimed: map[string]bool{}}, c)
	if err := j.runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := res.reminders[routeAuditor]
	if len(got) != 1 || got[0].Tier != "Your deadline - Due tomorrow" {
		t.Fatalf("auditor digest = %+v", res.reminders)
	}
	if len(res.reminders[routeOwner]) != 0 || len(res.ultimate) != 0 {
		t.Fatalf("nobody else is told: %+v %+v", res.reminders, res.ultimate)
	}
}

func TestWorkflowSweepComplianceStageReachesTheAdmins(t *testing.T) {
	var res wfResult
	c := wfControl(50, 0)
	c.Status, c.CurrentHandler = "EVIDENCE_INTERNAL_REVIEW", strPtr("COMPLIANCE")
	j := newWorkflowJob(&res, &fakeClaimer{claimed: map[string]bool{}}, c)
	if err := j.runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, admin := range []int{routeAdminA, routeAdminB} {
		if got := res.reminders[admin]; len(got) != 1 || got[0].Tier != "Your deadline - Due today" {
			t.Errorf("admin %d digest = %+v", admin, got)
		}
	}
}

func TestWorkflowSweepSameDayDeadlinesShareOneEmailAndOneClaim(t *testing.T) {
	var res wfResult
	claimer := &fakeClaimer{claimed: map[string]bool{}}
	j := newWorkflowJob(&res, claimer, wfControl(1, 1))
	if err := j.runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	items := res.reminders[routeOwner]
	if len(items) != 1 || len(items[0].Extras) != 1 {
		t.Fatalf("one digest item with one folded row, got %+v", items)
	}
	if len(claimer.claimed) != 1 {
		t.Fatalf("one claim, got %d", len(claimer.claimed))
	}
}

func TestWorkflowSweepDoesNotResendTheSameDay(t *testing.T) {
	var res wfResult
	claimer := &fakeClaimer{claimed: map[string]bool{}}
	j := newWorkflowJob(&res, claimer, wfControl(5, 20))
	if err := j.runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	j2 := newWorkflowJob(&res, claimer, wfControl(5, 20))
	if err := j2.runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(res.reminders) != 0 || len(res.ultimate) != 0 {
		t.Fatalf("the second sweep must send nothing: %+v %+v", res.reminders, res.ultimate)
	}
}

func TestWorkflowOffLeavesDesignControlsOnTheExistingReminders(t *testing.T) {
	var res wfResult
	j := newWorkflowJob(&res, &fakeClaimer{claimed: map[string]bool{}}, wfControl(5, 20))
	j.workflow = false
	if err := j.runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := res.reminders[routeOwner]
	if len(got) != 1 || got[0].Kind != "" || got[0].Tier != "Due in 5 days" {
		t.Fatalf("the existing reminder is unchanged: %+v", got)
	}
}

// An overdue item with an assignee reaches that person (and the lead), not the
// admins; the existing reminders still escalate to them.
func TestWorkflowOverdueAssignedItemIsNotEscalatedToAdmins(t *testing.T) {
	var res wfResult
	j := newWorkflowJob(&res, &fakeClaimer{claimed: map[string]bool{}}, wfControl(-3, -3))
	if err := j.runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(res.escalations) != 0 {
		t.Fatalf("admins must not be told: %+v", res.escalations)
	}
	if got := res.reminders[routeOwner]; len(got) != 1 || len(got[0].Extras) != 1 {
		t.Fatalf("owner digest with both rows = %+v", got)
	}
	if len(res.ultimate[routeOwner]) != 1 {
		t.Fatalf("lead digest = %+v", res.ultimate)
	}
}

// With nobody holding the control, only the admins can assign one, so an
// overdue item still reaches them.
func TestWorkflowOverdueUnassignedItemStillReachesTheAdmins(t *testing.T) {
	var res wfResult
	c := wfControl(-3, 50)
	c.OwnerID = nil
	j := newWorkflowJob(&res, &fakeClaimer{claimed: map[string]bool{}}, c)
	if err := j.runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(res.escalations[routeAdminA]) != 1 || len(res.escalations[routeAdminB]) != 1 {
		t.Fatalf("admins = %+v", res.escalations)
	}
}

// An OE control that predates tracking keeps the existing reminders, which use the
// population's own due date, until its next hand-off starts tracking it.
func TestUntrackedOEControlKeepsTheExistingReminders(t *testing.T) {
	c := wfControl(5, 1)
	c.RequirementType, c.Status = "OE", "POPULATION_PENDING"
	c.DeadlineTracked, c.CurrentHandler, c.CurrentDueDate = false, nil, nil
	if isWorkflowControl(c) {
		t.Fatal("an untracked OE control is not a workflow control")
	}
	c.DeadlineTracked = true
	if !isWorkflowControl(c) {
		t.Fatal("a tracked OE control is")
	}
	d := wfControl(5, 1)
	d.DeadlineTracked = false
	if !isWorkflowControl(d) {
		t.Fatal("an untracked Design control still gets the ultimate reminder")
	}
}

// The ultimate reminder is about the whole control, so even in the population
// phase it is not labelled a population item.
func TestUltimateReminderNeverReadsAsPopulation(t *testing.T) {
	c := wfControl(5, 1)
	c.RequirementType, c.Status = "OE", "POPULATION_PENDING"
	c.CurrentPhase, c.PopulationOwnerID = strPtr("POPULATION"), intPtr(555)
	for _, r := range workflowReminders(c, colomboDay(wfNow), "SOC 2") {
		if r.item.Kind == kindUltimate && r.item.RequirementType != "Evidence Requirement" {
			t.Errorf("ultimate item labelled %q", r.item.RequirementType)
		}
	}
}

// With the owner unassigned the admins stand in for the reminder, but the lead
// digest belongs to the real owner: no admin's lead is told about it.
func TestUnassignedOwnerItemIsNotEscalatedToTheAdminsLeads(t *testing.T) {
	var res wfResult
	c := wfControl(5, 50) // the ultimate deadline is 5 days away: a lead-eligible mark
	c.OwnerID = nil
	j := newWorkflowJob(&res, &fakeClaimer{claimed: map[string]bool{}}, c)
	if err := j.runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(res.reminders[routeAdminA]) != 1 || len(res.reminders[routeAdminB]) != 1 {
		t.Fatalf("the admins still get the reminder: %+v", res.reminders)
	}
	if len(res.ultimate) != 0 {
		t.Fatalf("no lead digest for an item an admin only stands in for: %+v", res.ultimate)
	}
	for _, it := range res.reminders[routeAdminA] {
		if it.ToLead {
			t.Error("the item must not stay marked for a lead")
		}
	}
}

// Owner unassigned and the control with GRC, both deadlines overdue: both rows go
// to the same admins with the same claim, so they travel as one.
func TestOverdueRowsForTheSameAdminsAreFolded(t *testing.T) {
	c := wfControl(-5, -2)
	c.OwnerID = nil
	c.Status, c.CurrentHandler = "EVIDENCE_INTERNAL_REVIEW", strPtr("COMPLIANCE")
	rs := workflowReminders(c, colomboDay(wfNow), "SOC 2")
	if len(rs) != 1 || len(rs[0].item.Extras) != 1 {
		t.Fatalf("want one folded reminder, got %+v", rs)
	}
	main, extra := rs[0].item, rs[0].item.Extras[0]
	if main.Kind != kindUltimate || extra.Kind != kindIndividual {
		t.Errorf("rows = %s then %s", main.Kind, extra.Kind)
	}
	if extra.WaitingOn != waitingOnAdmins {
		t.Errorf("the individual row says it waits on %q", extra.WaitingOn)
	}
}

func TestFoldedAdminRowsBothReachEveryAdmin(t *testing.T) {
	var res wfResult
	c := wfControl(-5, -2)
	c.OwnerID = nil
	c.Status, c.CurrentHandler = "EVIDENCE_INTERNAL_REVIEW", strPtr("COMPLIANCE")
	j := newWorkflowJob(&res, &fakeClaimer{claimed: map[string]bool{}}, c)
	if err := j.runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int{routeAdminA, routeAdminB} {
		got := res.escalations[id]
		if len(got) != 1 || len(got[0].Extras) != 1 {
			t.Fatalf("admin %d should get one item carrying both rows, got %+v", id, got)
		}
	}
}
