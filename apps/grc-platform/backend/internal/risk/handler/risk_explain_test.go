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

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	riskservice "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/grant"
)

func sp(s string) *string { return &s }

var explainNow = time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)

func approvedRisk() *model.RiskDetail {
	return &model.RiskDetail{
		ID: 7, RiskCode: "2026-INT-Q4-0002", RiskTitle: "Ballerina integration test",
		WorkflowStatus: model.StatusApproved, SourceRegisterID: 4, AssignmentTeamID: 4,
		SourceRegisterName: "Integration Platform", AssignmentTeamName: "Ballerina",
		GrossScore:         &model.RiskScore{RiskLevel: "HIGH"},
		EffectiveScore:     &model.RiskScore{RiskLevel: "MEDIUM"},
		TreatmentStrategy:  sp("REMEDIATE"),
		ImplementationDate: sp("2026-11-30"), ReassessmentDate: sp("2027-02-01"),
		Progress:  sp("Patch   scheduled\nfor next week"),
		OwnerName: "Alice Owner", AssignerName: "Bob Assigner", ManagementApproverName: "Mia Manager",
		RiskDescription: "The integration runtime is out of date.", ImpactDescription: sp("Service could be interrupted."),
	}
}

func event(at time.Time, action, by string, details string) *model.HistoryEntry {
	e := &model.HistoryEntry{Action: action, CreatedBy: "uuid-" + by, CreatedByName: by, CreatedAt: at}
	if details != "" {
		e.Details = json.RawMessage(details)
	}
	return e
}

func TestDescribeRiskTellsWhereItStands(t *testing.T) {
	out := describeRisk(approvedRisk(), explainNow)
	for _, want := range []string{
		"Risk 2026-INT-Q4-0002: “Ballerina integration test”",
		"• Status: Approved, approved and open, with its action plan being carried out.",
		"• Register and team: raised in Integration Platform, assigned to Ballerina.",
		"• Level: High when it was raised, Medium now after reassessment.",
		"• Treatment: Remediate.",
		"• Implementation date: 30 Nov 2026 (due in 19 days).",
		"• Next reassessment: 1 Feb 2027.",
		"• Latest progress note: Patch scheduled for next week", // whitespace tidied
		"• Risk Owner: Alice Owner",
		"• Risk Assigner: Bob Assigner",
		"• Management approver: Mia Manager",
		"The integration runtime is out of date.",
		"Impact: Service could be interrupted.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "What happened") {
		t.Error("a caller who may not read the history must not get a history section")
	}
}

func TestDescribeRiskDueDates(t *testing.T) {
	cases := []struct {
		name, status, due, want string
	}{
		{"overdue", model.StatusApproved, "2026-10-31", "(overdue by 11 days)"},
		{"today", model.StatusApproved, "2026-11-11", "(due today)"},
		{"closed has no deadline wording", model.StatusClosed, "2026-10-31", "• Implementation date: 31 Oct 2026."},
		{"cancelled has none either", model.StatusCancelled, "2026-12-31", "• Implementation date: 31 Dec 2026."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := approvedRisk()
			r.WorkflowStatus, r.ImplementationDate = c.status, sp(c.due)
			if out := describeRisk(r, explainNow); !strings.Contains(out, c.want) {
				t.Fatalf("missing %q in:\n%s", c.want, out)
			}
		})
	}
}

func TestDescribeRiskExplainsEveryStatus(t *testing.T) {
	for _, status := range model.AllWorkflowStatuses {
		r := approvedRisk()
		r.WorkflowStatus = status
		out := describeRisk(r, explainNow)
		if !strings.Contains(out, "• Status: ") {
			t.Fatalf("%s: %s", status, out)
		}
		if _, ok := statusText[status]; !ok {
			t.Errorf("status %s has no plain-English explanation", status)
		}
	}
}

func TestDescribeRiskSaysWhyItWasSentBack(t *testing.T) {
	r := approvedRisk()
	r.WorkflowStatus = model.StatusPendingRevision
	r.RejectionComment = sp("Please add evidence of the patch.")
	if out := describeRisk(r, explainNow); !strings.Contains(out, "• Why it was sent back: Please add evidence of the patch.") {
		t.Fatalf("%s", out)
	}
}

func TestDescribeRiskLeavesOutWhatIsNotThere(t *testing.T) {
	out := describeRisk(&model.RiskDetail{RiskCode: "X-1", RiskTitle: "Bare", WorkflowStatus: model.StatusPendingOwnerApproval}, explainNow)
	for _, absent := range []string{"Level:", "Treatment:", "Implementation date", "People", "About it", "Impact:", "Register"} {
		if strings.Contains(out, absent) {
			t.Errorf("%q should not appear for a risk without it:\n%s", absent, out)
		}
	}
	if !strings.Contains(out, "waiting for the Risk Owner to approve it") {
		t.Errorf("%s", out)
	}
	same := approvedRisk()
	same.EffectiveScore = &model.RiskScore{RiskLevel: "HIGH"}
	if out := describeRisk(same, explainNow); !strings.Contains(out, "• Level: High.") {
		t.Errorf("an unchanged level should read plainly:\n%s", out)
	}
}

func TestDescribeRiskShortensLongText(t *testing.T) {
	r := approvedRisk()
	r.RiskDescription = strings.Repeat("word ", 400)
	out := describeRisk(r, explainNow)
	if strings.Contains(out, strings.Repeat("word ", 100)) || !strings.Contains(out, "…") {
		t.Fatalf("the description was not shortened")
	}
}

func TestDescribeHistoryReadsInOrder(t *testing.T) {
	d := func(day int) time.Time { return time.Date(2026, 10, day, 9, 0, 0, 0, time.UTC) }
	events := []*model.HistoryEntry{
		// Given out of order on purpose.
		event(d(8), model.HistoryApprove, "Alice Owner", `{"from":"PENDING_RISK_OWNER_APPROVAL","to":"PENDING_MANAGEMENT_APPROVAL","role":"Risk Owner"}`),
		event(d(5), model.HistoryCreate, "Bob Assigner", ""),
		event(d(9), model.HistoryReject, "Mia Manager", `{"stage":"MANAGEMENT","comment":"Add evidence.","from":"PENDING_MANAGEMENT_APPROVAL","to":"PENDING_REVISION"}`),
		event(d(12), model.HistoryAssess, "Alice Owner", `{"previousLevel":"HIGH","level":"MEDIUM"}`),
		{Action: model.HistoryEscalate, CreatedBy: "system", CreatedAt: d(20), Details: json.RawMessage(`{"overdueDays":3}`)},
	}
	tl := buildTimeline(events)
	want := []riskservice.TimelineEvent{
		{Date: "5 Oct 2026", Text: "Bob Assigner created the risk."},
		{Date: "8 Oct 2026", Text: "Alice Owner approved it as Risk Owner. It moved from pending risk owner approval to pending management approval."},
		{Date: "9 Oct 2026", Text: "Mia Manager sent it back (management). It moved from pending management approval to pending revision. Comment: “Add evidence.”."},
		{Date: "12 Oct 2026", Text: "Alice Owner reassessed it: the level went from high to medium."},
		{Date: "20 Oct 2026", Text: "The system escalated it because it was 3 days past its implementation date."},
	}
	if len(tl.Events) != len(want) {
		t.Fatalf("got %+v", tl.Events)
	}
	for i, w := range want {
		if tl.Events[i] != w {
			t.Errorf("event %d: got %+v, want %+v", i, tl.Events[i], w)
		}
	}
}

func TestDescribeEventCoversEveryAction(t *testing.T) {
	at := explainNow
	for action, wantSub := range map[string]string{
		model.HistoryCreate:   "created the risk",
		model.HistoryUpdate:   "changed a detail",
		model.HistoryDelete:   "removed an item",
		model.HistorySubmit:   "submitted it",
		model.HistoryApprove:  "approved it",
		model.HistoryReject:   "sent it back",
		model.HistoryEscalate: "escalated it",
		model.HistoryComment:  "added a comment",
		model.HistoryAssess:   "reassessed it",
		model.HistoryComplete: "marked the work complete",
		model.HistoryClose:    "closed it",
		model.HistoryCancel:   "cancelled it",
		"SOMETHING_NEW":       "made a change (something_new)",
	} {
		got := describeEvent(event(at, action, "Pat", ""))
		if !strings.Contains(got, "Pat") || !strings.Contains(got, wantSub) {
			t.Errorf("%s: %q does not mention %q", action, got, wantSub)
		}
	}
	changed := describeEvent(&model.HistoryEntry{Action: model.HistoryUpdate, CreatedByName: "Pat", FieldChanged: sp("risk_title"), OldValue: sp("Old"), NewValue: sp("New")})
	if changed != "Pat changed the risk title from “Old” to “New”." {
		t.Errorf("%q", changed)
	}
	if got := describeEvent(&model.HistoryEntry{Action: model.HistoryCreate, CreatedBy: "uuid-1"}); !strings.HasPrefix(got, "Someone ") {
		t.Errorf("an unnamed actor reads as Someone: %q", got)
	}
}

func TestBuildTimelineCapsTheEvents(t *testing.T) {
	var events []*model.HistoryEntry
	for i := 0; i < maxExplainEvents+5; i++ {
		events = append(events, event(explainNow.Add(time.Duration(i)*time.Hour), model.HistoryComment, "Pat", `{"comment":"c"}`))
	}
	tl := buildTimeline(events)
	if tl.Note != "Showing the latest 12 of 17 events." || len(tl.Events) != maxExplainEvents {
		t.Fatalf("%+v", tl)
	}
	if got := buildTimeline(nil); !strings.Contains(got.Note, "Nothing has been recorded") || len(got.Events) != 0 {
		t.Fatalf("%+v", got)
	}
}

// ---- explainRisk: who may be told what ----

type staticHistory struct{ entries []*model.HistoryEntry }

func (s staticHistory) List(context.Context, int) ([]*model.HistoryEntry, error) {
	return s.entries, nil
}
func (s staticHistory) Search(context.Context, model.AuditTrailFilter) (*model.AuditTrailPage, error) {
	return &model.AuditTrailPage{}, nil
}
func (s staticHistory) Record(context.Context, int, model.RecordHistoryRequest, string) error {
	return nil
}

func explainDeps() *Deps {
	r := approvedRisk()
	return &Deps{
		Risk: &fakeRiskSvc{
			page: &model.RiskListPage{Items: []*model.RiskListItem{{ID: 7, RiskCode: "2026-INT-Q4-0002"}}},
			byID: map[int]*model.RiskDetail{7: r},
		},
		History:    staticHistory{entries: []*model.HistoryEntry{event(explainNow, model.HistoryCreate, "Bob Assigner", "")}},
		ActionPlan: &fakeActionPlanSvc{},
		Users:      fakeUserRepo{uuid: "test-caller-uuid", id: 3},
	}
}

func TestExplainRiskForSomeoneWhoCanSeeIt(t *testing.T) {
	d := explainDeps()
	ctx := contextWithGrants([]grant.Grant{agentTeamGrant("risk-owner", 4)}) // the risk is in team 4
	out, link, tl, err := d.explainRisk(ctx, "2026-int-q4-0002")             // the code is matched without regard to case
	if err != nil {
		t.Fatal(err)
	}
	if link == nil || link.ID != 7 || link.Code != "2026-INT-Q4-0002" {
		t.Fatalf("expected a link to the risk, got %+v", link)
	}
	if !strings.Contains(out, "Risk 2026-INT-Q4-0002") || strings.Contains(out, "What happened") {
		t.Fatalf("%s", out)
	}
	if tl == nil || len(tl.Events) != 1 || tl.Events[0].Text != "Bob Assigner created the risk." {
		t.Fatalf("expected a one-event timeline, got %+v", tl)
	}
}

// A risk the caller may not see is answered exactly like one that does not
// exist, so the assistant cannot be used to find out that a risk is there.
func TestExplainRiskDoesNotConfirmARiskTheCallerCannotSee(t *testing.T) {
	d := explainDeps()
	other := contextWithGrants([]grant.Grant{agentTeamGrant("risk-owner", 9)}) // a different team
	hidden, hiddenLink, hiddenTL, err := d.explainRisk(other, "2026-INT-Q4-0002")
	if err != nil {
		t.Fatal(err)
	}
	missing, missingLink, missingTL, err := d.explainRisk(other, "2026-ZZZ-Q4-9999")
	if err != nil {
		t.Fatal(err)
	}
	// No link either way: a link would confirm that the risk exists.
	if hiddenLink != nil || missingLink != nil || hiddenTL != nil || missingTL != nil {
		t.Fatalf("links given for a risk the caller cannot see: %+v / %+v", hiddenLink, missingLink)
	}
	if hidden != riskNotFound || missing != riskNotFound || hidden != missing {
		t.Fatalf("hidden = %q, missing = %q", hidden, missing)
	}
	if strings.Contains(hidden, "Ballerina") || strings.Contains(hidden, "Integration") {
		t.Fatal("details of an invisible risk leaked")
	}
}

func TestExplainRiskLeavesOutHistoryWithoutThePrivilege(t *testing.T) {
	d := explainDeps()
	// Named on the risk as its Action Owner, holding no role: may open the risk,
	// but not the History tab, so the assistant does not read it out either.
	d.Risk.(*fakeRiskSvc).byID[7].OwnerID = 3
	ctx := contextWithGrants(nil)
	out, _, tl, err := d.explainRisk(ctx, "2026-INT-Q4-0002")
	if err != nil {
		t.Fatal(err)
	}
	if out == riskNotFound {
		t.Skip("a no-role caller is not admitted by the fake risk service; covered by riskVisibleToCaller's own tests")
	}
	if tl != nil {
		t.Fatalf("history was given to someone without ViewRisks: %+v", tl)
	}
}

// The interpret endpoint turns "explain this risk" into an ordinary reply: the
// web app never sees the internal action or the code.
func TestInterpretEndpointAnswersAQuestionAboutOneRisk(t *testing.T) {
	spy := &spyDashboardAgent{interpretResult: &riskservice.InterpretResult{Action: riskservice.InterpretExplainRisk, RiskCode: "2026-INT-Q4-0002"}}
	d := agentDeps(true, spy)
	full := explainDeps()
	d.Risk, d.History = full.Risk, full.History

	rec := postInterpret(d, contextWithGrants([]grant.Grant{agentTeamGrant("risk-owner", 4)}),
		`{"message":"2026-INT-Q4-0002 what happened to this risk"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	var got struct{ Action, Message string }
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Action != "reply" || !strings.Contains(got.Message, "Ballerina integration test") {
		t.Fatalf("%+v", got)
	}
	// The reply carries a link so the web app can offer to open the risk.
	if !strings.Contains(rec.Body.String(), `"risk_link":{"id":7,"code":"2026-INT-Q4-0002"}`) {
		t.Fatalf("no link in %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "explain_risk") {
		t.Fatal("the internal action reached the web app")
	}
}

// ---- comparing two risks ----

func TestCompareRisksMarksWhatDiffers(t *testing.T) {
	a := approvedRisk()
	b := approvedRisk()
	b.ID, b.RiskCode, b.SourceRegisterName = 8, "2026-INT-Q4-0003", "Other register"
	table := buildComparison(a, b, false, [2]string{}, explainNow)
	if len(table.Risks) != 2 || table.Risks[1].Code != "2026-INT-Q4-0003" {
		t.Fatalf("%+v", table.Risks)
	}
	for _, r := range table.Rows {
		if r.Label == "History" {
			t.Fatal("history compared without permission")
		}
		if want := r.Label == "Register"; r.Differs != want {
			t.Errorf("%s: differs = %v", r.Label, r.Differs)
		}
	}
	if s := compareSummary(table); !strings.Contains(s, "register") {
		t.Fatal(s)
	}
}

func TestCompareRisksDoesNotRevealWhichRiskIsHidden(t *testing.T) {
	d := explainDeps()
	other := contextWithGrants([]grant.Grant{agentTeamGrant("risk-owner", 9)})
	msg, table, err := d.compareRisks(other, []string{"2026-INT-Q4-0002", "2026-ZZZ-Q4-9999"})
	if err != nil || table != nil || msg != notBothFound {
		t.Fatalf("%q %+v %v", msg, table, err)
	}
	msg2, table2, _ := d.compareRisks(other, []string{"2026-ZZZ-Q4-9998", "2026-ZZZ-Q4-9999"})
	if table2 != nil || msg2 != msg {
		t.Fatal("answers differ between hidden and missing risks")
	}
}
