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

package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	auditservice "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/middleware"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/dashboardagent"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/routeguard"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/grant"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// assistantCtx is an authenticated caller holding exactly these privileges:
// global ones, and ones held only on a team.
func assistantCtx(userID int, global []string, byTeam map[int][]string) context.Context {
	g := map[string]bool{}
	for _, p := range global {
		g[p] = true
	}
	bt := map[int]map[string]bool{}
	for team, privs := range byTeam {
		bt[team] = map[string]bool{}
		for _, p := range privs {
			bt[team][p] = true
		}
	}
	set := grant.NewForTest(g, bt, len(global)+len(byTeam))
	ctx := middleware.WithUserInfo(context.Background(), &middleware.UserInfo{Subject: "caller-uuid", UserID: userID})
	ctx = grant.WithContext(ctx, set)
	return privilege.WithContext(ctx, set.PrivilegeMap())
}

func TestAssistantScope(t *testing.T) {
	cases := []struct {
		name string
		ctx  context.Context
		want dashboardagent.AuditScope
	}{
		{"org-wide read sees everything",
			assistantCtx(5, []string{privilege.ViewAudits, privilege.ViewAllAudits}, nil),
			dashboardagent.AuditScope{Scope: "all"}},
		{"org-wide read held on a team is that team",
			assistantCtx(5, nil, map[int][]string{3: {privilege.ViewAudits, privilege.ViewAllAudits}}),
			dashboardagent.AuditScope{Scope: "team", UserID: 5, TeamIDs: []int{3}}},
		{"a submitter sees the controls they own",
			assistantCtx(5, []string{privilege.ViewAudits, privilege.SubmitEvidence}, nil),
			dashboardagent.AuditScope{Scope: "owned", UserID: 5}},
		{"a team grant on a submitter role does not widen them",
			assistantCtx(5, nil, map[int][]string{3: {privilege.ViewAudits, privilege.SubmitEvidence}}),
			dashboardagent.AuditScope{Scope: "owned", UserID: 5}},
		{"an auditor sees the controls they audit",
			assistantCtx(5, []string{privilege.ViewAudits, privilege.ValidateEvidence}, nil),
			dashboardagent.AuditScope{Scope: "assigned", UserID: 5}},
		{"no audit privilege sees nothing",
			assistantCtx(5, []string{privilege.ViewRisks}, nil),
			dashboardagent.AuditScope{}},
		{"an owned scope with no known user sees nothing",
			assistantCtx(0, []string{privilege.SubmitEvidence}, nil),
			dashboardagent.AuditScope{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := assistantScope(c.ctx)
			if got.Scope != c.want.Scope || got.UserID != c.want.UserID || len(got.TeamIDs) != len(c.want.TeamIDs) {
				t.Fatalf("got %+v, want %+v", got, c.want)
			}
			for i := range got.TeamIDs {
				if got.TeamIDs[i] != c.want.TeamIDs[i] {
					t.Fatalf("got %+v, want %+v", got, c.want)
				}
			}
		})
	}
}

type spyAssistant struct {
	hits        func(spec dashboardagent.Spec) *dashboardagent.Result
	specs       []dashboardagent.Spec
	runs        int
	gotSpec     dashboardagent.Spec
	gotScope    dashboardagent.AuditScope
	result      *dashboardagent.Result
	interpret   *auditservice.AssistantResult
	interpretEr error
}

func (s *spyAssistant) Run(_ context.Context, spec dashboardagent.Spec, scope dashboardagent.AuditScope) (*dashboardagent.Result, error) {
	s.runs++
	s.gotSpec, s.gotScope = spec, scope
	s.specs = append(s.specs, spec)
	if s.hits != nil {
		return s.hits(spec), nil
	}
	if s.result != nil {
		return s.result, nil
	}
	return &dashboardagent.Result{Columns: []dashboardagent.Column{{ID: "control_count", Role: "measure", Kind: "int"}}, Rows: [][]any{{int64(4)}}}, nil
}

func (s *spyAssistant) Interpret(context.Context, auditservice.AssistantRequest, func(string) bool) (*auditservice.AssistantResult, error) {
	if s.interpretEr != nil {
		return nil, s.interpretEr
	}
	if s.interpret != nil {
		return s.interpret, nil
	}
	return &auditservice.AssistantResult{Action: "reply", Message: "hello"}, nil
}

func assistantDeps(enabled bool, spy *spyAssistant) *Deps {
	return &Deps{
		DashboardAgent:            spy,
		DashboardAgentEnabled:     enabled,
		assistantLimiter:          dashboardagent.NewLimiter(2, time.Minute),
		assistantInterpretLimiter: dashboardagent.NewLimiter(2, time.Minute),
	}
}

func post(h http.HandlerFunc, ctx context.Context, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

var (
	submitterCtx = func() context.Context {
		return assistantCtx(5, []string{privilege.ViewAudits, privilege.SubmitEvidence}, nil)
	}
	noAuditCtx = func() context.Context { return assistantCtx(5, []string{privilege.ViewRisks}, nil) }
)

const controlsBody = `{"dataset":"controls","measures":["control_count"],"dimensions":["team"]}`

func TestAssistantQueryUsesTheCallersScopeAndNothingElse(t *testing.T) {
	spy := &spyAssistant{}
	d := assistantDeps(true, spy)
	// A client that tries to widen its own scope: the extra field is not part of
	// a spec, so the request is refused or ignored, never obeyed.
	rec := post(d.handleAssistantQuery, submitterCtx(), "/q", `{"dataset":"controls","measures":["control_count"],"scope":"all","userId":99}`)
	if rec.Code == http.StatusOK && (spy.gotScope.Scope != "owned" || spy.gotScope.UserID != 5) {
		t.Fatalf("scope taken from the request: %+v", spy.gotScope)
	}
	spy.runs = 0
	rec = post(d.handleAssistantQuery, submitterCtx(), "/q", controlsBody)
	if rec.Code != http.StatusOK || spy.runs != 1 {
		t.Fatalf("status = %d, runs = %d (%s)", rec.Code, spy.runs, rec.Body.String())
	}
	if spy.gotScope.Scope != "owned" || spy.gotScope.UserID != 5 {
		t.Fatalf("scope = %+v", spy.gotScope)
	}
	if !strings.Contains(rec.Body.String(), "There are 4 controls") {
		t.Fatalf("no sentence written from the numbers: %s", rec.Body.String())
	}
}

func TestAssistantIsUnavailableWhenOffOrWithoutAuditAccess(t *testing.T) {
	spy := &spyAssistant{}
	if rec := post(assistantDeps(false, spy).handleAssistantQuery, submitterCtx(), "/q", controlsBody); rec.Code != http.StatusNotFound {
		t.Fatalf("flag off: status = %d", rec.Code)
	}
	d := assistantDeps(true, spy)
	if rec := post(d.handleAssistantQuery, noAuditCtx(), "/q", controlsBody); rec.Code != http.StatusNotFound {
		t.Fatalf("no audit access: status = %d", rec.Code)
	}
	if rec := post(d.handleAssistantInterpret, noAuditCtx(), "/i", `{"message":"hi"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("interpret, no audit access: status = %d", rec.Code)
	}
	req := httptest.NewRequest(http.MethodGet, "/s", nil).WithContext(noAuditCtx())
	rec := httptest.NewRecorder()
	d.handleAssistantStarters(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("starters, no audit access: status = %d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/s", nil).WithContext(submitterCtx())
	rec = httptest.NewRecorder()
	d.handleAssistantStarters(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"starters":[]`) {
		t.Fatalf("starters: status = %d (%s)", rec.Code, rec.Body.String())
	}
	if spy.runs != 0 {
		t.Fatal("a refused caller reached the query")
	}
}

// The Risk Hub's datasets are not reachable through the Audit assistant.
func TestAssistantQueryRefusesTheRiskRegister(t *testing.T) {
	spy := &spyAssistant{}
	d := assistantDeps(true, spy)
	d.assistantLimiter = dashboardagent.NewLimiter(100, time.Minute)
	for _, body := range []string{
		`{"dataset":"risks","measures":["risk_count"]}`,
		`{"dataset":"risk_list","columns":["risk_code"]}`,
		`{"dataset":"ai_suggestions","measures":["suggestion_count"]}`,
		`{"dataset":"controls","measures":["control_count"],"dimensions":["risk_year"]}`,
	} {
		if rec := post(d.handleAssistantQuery, submitterCtx(), "/q", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d", body, rec.Code)
		}
	}
	if spy.runs != 0 {
		t.Fatal("an invalid spec reached the query")
	}
}

func TestAssistantRateLimits(t *testing.T) {
	d := assistantDeps(true, &spyAssistant{})
	for i := 0; i < 2; i++ {
		if rec := post(d.handleAssistantQuery, submitterCtx(), "/q", controlsBody); rec.Code != http.StatusOK {
			t.Fatalf("request %d: %d", i, rec.Code)
		}
	}
	if rec := post(d.handleAssistantQuery, submitterCtx(), "/q", controlsBody); rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("status = %d", rec.Code)
	}
	for i := 0; i < 2; i++ {
		post(d.handleAssistantInterpret, submitterCtx(), "/i", `{"message":"hi"}`)
	}
	if rec := post(d.handleAssistantInterpret, submitterCtx(), "/i", `{"message":"hi"}`); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("interpret: status = %d", rec.Code)
	}
}

func TestAssistantInterpretReportsAnUnavailableModelCalmly(t *testing.T) {
	d := assistantDeps(true, &spyAssistant{interpretEr: auditservice.ErrAssistantUnavailable})
	rec := post(d.handleAssistantInterpret, submitterCtx(), "/i", `{"message":"how many controls"}`)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "temporarily unavailable") {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	d = assistantDeps(true, &spyAssistant{interpretEr: &dashboardagent.ValidationError{Msg: "write a request"}})
	if rec := post(d.handleAssistantInterpret, submitterCtx(), "/i", `{"message":""}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
	d = assistantDeps(true, &spyAssistant{interpretEr: errors.New("db down: secret detail")})
	rec = post(d.handleAssistantInterpret, submitterCtx(), "/i", `{"message":"hi"}`)
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
}

// Owners and auditors come back from the query as identities; only names leave.
func TestLabelPeopleColumns(t *testing.T) {
	res := &dashboardagent.Result{
		Columns: []dashboardagent.Column{{ID: "owner"}, {ID: "control_count"}},
		Rows:    [][]any{{"uuid-1", int64(3)}, {nil, int64(2)}, {"uuid-9", int64(1)}},
	}
	labelPeopleColumns(res, []string{"owner", "auditor"}, func([]string) map[string]string {
		return map[string]string{"uuid-1": "Ann Silva"}
	})
	got := []any{res.Rows[0][0], res.Rows[1][0], res.Rows[2][0]}
	want := []any{"Ann Silva", "Unassigned", "Unknown"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d: %v, want %v", i, got[i], want[i])
		}
	}
}

type recordingRouter struct {
	mux      *http.ServeMux
	patterns []string
}

func (r *recordingRouter) HandleFunc(pattern string, h func(http.ResponseWriter, *http.Request)) {
	r.patterns = append(r.patterns, pattern)
	r.mux.HandleFunc(pattern, h) // panics if the pattern clashes with another
}

// The routes register without clashing with the audit routes already there, and
// none of them is open to external auditors.
func TestAssistantRoutesAreRegisteredAndInternalOnly(t *testing.T) {
	rt := &recordingRouter{mux: http.NewServeMux()}
	RegisterRoutes(rt, Deps{})
	want := []string{
		"GET /api/v1/audits/dashboard-agent/starters",
		"POST /api/v1/audits/dashboard-agent/interpret",
		"POST /api/v1/audits/dashboard-agent/query",
	}
	for _, w := range want {
		found := false
		for _, p := range rt.patterns {
			found = found || p == w
		}
		if !found {
			t.Errorf("%s is not registered", w)
		}
		if routeguard.ExternalVisible(w) {
			t.Errorf("%s is visible to external auditors", w)
		}
		if _, listed := routeguard.Patterns()[w]; !listed {
			t.Errorf("%s is missing from the external-visibility map", w)
		}
	}
}

// ---- taking the user to a control ----

func controlRows(rows ...[]any) *dashboardagent.Result {
	return &dashboardagent.Result{
		Columns: []dashboardagent.Column{{ID: "control_number"}, {ID: "audit"}, {ID: "audit_id"}, {ID: "control_id"}},
		Rows:    rows,
	}
}

func openAs(t *testing.T, spy *spyAssistant, number, audit string) (*auditservice.AssistantResult, error) {
	t.Helper()
	d := assistantDeps(true, spy)
	return d.openControl(submitterCtx(), dashboardagent.AuditScope{Scope: "owned", UserID: 5}, number, audit)
}

func TestOpenControlTakesTheUserToTheOneMatch(t *testing.T) {
	spy := &spyAssistant{hits: func(dashboardagent.Spec) *dashboardagent.Result {
		return controlRows([]any{"CA-01", "HIPAA GRC Platform 2027", int64(12), int64(340)})
	}}
	res, err := openAs(t, spy, "CA-01", "HIPAA GRC Platform 2027")
	if err != nil || res.Navigate == nil || res.Navigate.AuditID != 12 || res.Navigate.ControlID != 340 {
		t.Fatalf("%+v, %v", res, err)
	}
	if !strings.Contains(res.Message, "CA-01") || !strings.Contains(res.Message, "HIPAA GRC Platform 2027") {
		t.Fatalf("%q", res.Message)
	}
	// The lookup is an ordinary scoped query: the number and the audit are bound
	// filter values, and the caller's own scope is what limits it.
	if spy.gotScope.Scope != "owned" || spy.gotScope.UserID != 5 {
		t.Fatalf("scope = %+v", spy.gotScope)
	}
	spec := spy.specs[0]
	if spec.Dataset != "control_list" || len(spec.Filters) != 2 || spec.Filters[0].Values[0] != "CA-01" || spec.Filters[1].Values[0] != "HIPAA GRC Platform 2027" {
		t.Fatalf("%+v", spec)
	}
}

func TestOpenControlAsksWhichAuditWhenSeveralMatch(t *testing.T) {
	spy := &spyAssistant{hits: func(dashboardagent.Spec) *dashboardagent.Result {
		return controlRows(
			[]any{"CA-01", "HIPAA Test", int64(2), int64(20)},
			[]any{"CA-01", "HIPAA GRC Platform 2027", int64(12), int64(340)},
		)
	}}
	res, err := openAs(t, spy, "CA-01", "")
	if err != nil || res.Navigate != nil {
		t.Fatalf("%+v, %v", res, err)
	}
	if !strings.Contains(res.Message, "HIPAA GRC Platform 2027, HIPAA Test") || !strings.Contains(res.Message, "Which one") {
		t.Fatalf("%q", res.Message)
	}
}

// A misworded audit name falls back to the control alone, and still asks.
func TestOpenControlWithAnAuditNameThatMatchesNothing(t *testing.T) {
	spy := &spyAssistant{hits: func(spec dashboardagent.Spec) *dashboardagent.Result {
		if len(spec.Filters) == 2 {
			return controlRows()
		}
		return controlRows([]any{"CA-01", "HIPAA GRC Platform 2027", int64(12), int64(340)})
	}}
	res, err := openAs(t, spy, "CA-01", "HIPPA GRC Platform 2027")
	if err != nil || res.Navigate != nil || !strings.Contains(res.Message, "HIPAA GRC Platform 2027") || !strings.Contains(res.Message, "HIPPA GRC Platform 2027") {
		t.Fatalf("%+v, %v", res, err)
	}
}

// A control that does not exist and one the caller cannot see are answered the
// same way, so the assistant cannot be used to find out what exists.
func TestOpenControlDoesNotRevealWhatTheCallerCannotSee(t *testing.T) {
	none := &spyAssistant{hits: func(dashboardagent.Spec) *dashboardagent.Result { return controlRows() }}
	a, err := openAs(t, none, "CA-01", "HIPAA GRC Platform 2027")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := openAs(t, none, "ZZ-99", "")
	if a.Navigate != nil || b.Navigate != nil || a.Message != controlNotFound || b.Message != a.Message {
		t.Fatalf("%q / %q", a.Message, b.Message)
	}
	if res, _ := openAs(t, none, "  ", ""); res.Message != controlNotFound {
		t.Fatalf("%q", res.Message)
	}
	if none.runs != 3 { // two lookups for the named audit (named, then alone), one for ZZ-99
		t.Fatalf("lookups = %d", none.runs)
	}
}

// Through the endpoint: the web app gets a reply with a place to go, and never
// the internal action or the control number the model wrote.
func TestInterpretEndpointOpensAControl(t *testing.T) {
	spy := &spyAssistant{
		interpret: &auditservice.AssistantResult{Action: auditservice.AssistantOpenControl, ControlNumber: "CA-01", AuditName: "HIPAA GRC Platform 2027"},
		hits: func(dashboardagent.Spec) *dashboardagent.Result {
			return controlRows([]any{"CA-01", "HIPAA GRC Platform 2027", int64(12), int64(340)})
		},
	}
	rec := post(assistantDeps(true, spy).handleAssistantInterpret, submitterCtx(), "/i", `{"message":"take me to CA-01 in HIPAA GRC Platform 2027"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`"action":"reply"`, `"audit_id":12`, `"control_id":340`} {
		if !strings.Contains(body, want) {
			t.Errorf("response lacks %s: %s", want, body)
		}
	}
	if strings.Contains(body, "open_control") {
		t.Errorf("the internal action leaked: %s", body)
	}
}

// The assistant's own queries cannot ask for the ids that open a control.
func TestOpeningIdsCannotBeRequested(t *testing.T) {
	d := assistantDeps(true, &spyAssistant{})
	for _, col := range []string{"audit_id", "control_id"} {
		body := `{"dataset":"control_list","columns":["control_number","` + col + `"]}`
		if rec := post(d.handleAssistantQuery, submitterCtx(), "/q", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d", col, rec.Code)
		}
	}
}

// ---- describing and comparing controls ----

type fakeControlSvc struct {
	auditservice.ControlService
	byID map[int]*model.AuditControl
}

func (f fakeControlSvc) GetByID(_ context.Context, _, controlID int) (*model.AuditControl, error) {
	return f.byID[controlID], nil
}

type fakeTrailSvc struct {
	auditservice.TrailService
	entries []*model.AuditTrailEntry
	asked   int
}

func (f *fakeTrailSvc) ListByControl(context.Context, int, int, bool) ([]*model.AuditTrailEntry, int, error) {
	f.asked++
	return f.entries, len(f.entries), nil
}

func sp(s string) *string { return &s }

func sampleControl(id int, number string) *model.AuditControl {
	return &model.AuditControl{
		ID: id, AuditID: 12, ControlNumber: number, Description: "Access is reviewed every quarter.",
		RequirementType: "OE", ControlType: "NON_CONFIG", Scope: "COMMON", Status: "EVIDENCE_INTERNAL_REVIEW",
		DueDate: sp("2026-11-30"), TeamName: sp("Security"), OwnerName: sp("Ann Silva"),
		CreatedAt: time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 11, 9, 9, 0, 0, 0, time.UTC),
	}
}

func controlDeps(spy *spyAssistant, trail *fakeTrailSvc) *Deps {
	d := assistantDeps(true, spy)
	d.Control = fakeControlSvc{byID: map[int]*model.AuditControl{340: sampleControl(340, "CA-01"), 341: sampleControl(341, "CA-02")}}
	d.Trail = trail
	return d
}

func oneHit(spec dashboardagent.Spec) *dashboardagent.Result {
	for _, f := range spec.Filters {
		if f.Field == "control_number" {
			id := int64(340)
			if f.Values[0] == "CA-02" {
				id = 341
			}
			return controlRows([]any{f.Values[0], "HIPAA GRC Platform 2027", int64(12), id})
		}
	}
	return controlRows()
}

func allPrivCtx() context.Context {
	return assistantCtx(5, []string{privilege.ViewAudits, privilege.ViewAllAudits, privilege.ViewInternalComments}, nil)
}

func TestExplainControlDescribesItAndLinksToIt(t *testing.T) {
	trail := &fakeTrailSvc{entries: []*model.AuditTrailEntry{
		{Action: "UPLOADED", CreatedBy: "u1", CreatedByName: "Ann Silva", CreatedAt: time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)},
		{Action: "CREATED", CreatedBy: "u2", CreatedByName: "Bob", CreatedAt: time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)},
		{Action: "COMMENTED", CreatedBy: "u1", CreatedByName: "Ann Silva", CreatedAt: time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC), Details: []byte(`{"comment":"SECRET-COMMENT"}`)},
	}}
	d := controlDeps(&spyAssistant{hits: oneHit}, trail)
	res, err := d.explainControl(allPrivCtx(), dashboardagent.AuditScope{Scope: "all"}, "CA-01", "HIPAA GRC Platform 2027")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Control CA-01 in HIPAA GRC Platform 2027", "Evidence internal review", "perating effectiveness", "Ann Silva", "Security", "Access is reviewed every quarter."} {
		if !strings.Contains(res.Message, want) {
			t.Errorf("missing %q in:\n%s", want, res.Message)
		}
	}
	if res.ControlLink == nil || res.ControlLink.AuditID != 12 || res.ControlLink.ControlID != 340 || res.ControlLink.Code != "CA-01" {
		t.Fatalf("link = %+v", res.ControlLink)
	}
	// History: oldest first, the comment's text is never repeated.
	if res.Timeline == nil || len(res.Timeline.Events) != 3 || res.Timeline.Events[0].Text != "Bob created the control." {
		t.Fatalf("timeline = %+v", res.Timeline)
	}
	for _, e := range res.Timeline.Events {
		if strings.Contains(e.Text, "SECRET-COMMENT") {
			t.Fatal("a comment was repeated in the timeline")
		}
	}
}

func TestExplainControlLeavesOutHistoryWithoutThePrivilege(t *testing.T) {
	trail := &fakeTrailSvc{entries: []*model.AuditTrailEntry{{Action: "CREATED", CreatedBy: "u", CreatedAt: time.Now()}}}
	d := controlDeps(&spyAssistant{hits: oneHit}, trail)
	// Owns the control (submitter), but not allowed to read History.
	ctx := assistantCtx(5, []string{privilege.ViewAudits, privilege.SubmitEvidence}, nil)
	res, err := d.explainControl(ctx, dashboardagent.AuditScope{Scope: "owned", UserID: 5}, "CA-01", "")
	if err != nil || res.ControlLink == nil {
		t.Fatalf("%+v, %v", res, err)
	}
	if res.Timeline != nil || trail.asked != 0 {
		t.Fatalf("history was read for someone without the privilege: %+v (asked %d times)", res.Timeline, trail.asked)
	}
}

func TestExplainControlDoesNotConfirmAControlTheCallerCannotSee(t *testing.T) {
	none := &spyAssistant{hits: func(dashboardagent.Spec) *dashboardagent.Result { return controlRows() }}
	d := controlDeps(none, &fakeTrailSvc{})
	a, _ := d.explainControl(allPrivCtx(), dashboardagent.AuditScope{Scope: "owned", UserID: 9}, "CA-01", "HIPAA GRC Platform 2027")
	b, _ := d.explainControl(allPrivCtx(), dashboardagent.AuditScope{Scope: "owned", UserID: 9}, "ZZ-99", "")
	if a.Message != controlNotFound || b.Message != controlNotFound || a.ControlLink != nil || a.Timeline != nil {
		t.Fatalf("%+v / %+v", a, b)
	}
}

func TestCompareControlsMarksWhatDiffers(t *testing.T) {
	d := controlDeps(&spyAssistant{hits: oneHit}, &fakeTrailSvc{})
	d.Control.(fakeControlSvc).byID[341].Status = "COMPLETE"
	res, err := d.compareControls(allPrivCtx(), dashboardagent.AuditScope{Scope: "all"}, "CA-01", "HIPAA GRC Platform 2027", "CA-02", "")
	if err != nil || res.Compare == nil {
		t.Fatalf("%+v, %v", res, err)
	}
	if len(res.Compare.Risks) != 2 || res.Compare.Risks[1].Code != "CA-02" || res.Compare.Risks[1].ID != 341 || res.Compare.Risks[1].AuditID != 12 {
		t.Fatalf("%+v", res.Compare.Risks)
	}
	for _, row := range res.Compare.Rows {
		if want := row.Label == "Status"; row.Differs != want && row.Label != "History" && row.Label != "Due" {
			t.Errorf("%s: differs = %v", row.Label, row.Differs)
		}
	}
	if !strings.Contains(res.Message, "status") {
		t.Fatalf("%q", res.Message)
	}
}

func TestCompareControlsDoesNotRevealWhichIsHidden(t *testing.T) {
	only := &spyAssistant{hits: func(spec dashboardagent.Spec) *dashboardagent.Result {
		if spec.Filters[0].Values[0] == "CA-01" {
			return oneHit(spec)
		}
		return controlRows()
	}}
	d := controlDeps(only, &fakeTrailSvc{})
	a, _ := d.compareControls(allPrivCtx(), dashboardagent.AuditScope{Scope: "owned", UserID: 5}, "CA-01", "", "CA-02", "")
	b, _ := d.compareControls(allPrivCtx(), dashboardagent.AuditScope{Scope: "owned", UserID: 5}, "ZZ-98", "", "ZZ-99", "")
	if a.Compare != nil || b.Compare != nil || a.Message != notBothControls || b.Message != notBothControls {
		t.Fatalf("%+v / %+v", a, b)
	}
}

func TestInterpretEndpointExplainsAControl(t *testing.T) {
	spy := &spyAssistant{
		interpret: &auditservice.AssistantResult{Action: auditservice.AssistantExplainControl, ControlNumber: "CA-01", AuditName: "HIPAA GRC Platform 2027"},
		hits:      oneHit,
	}
	d := controlDeps(spy, &fakeTrailSvc{})
	rec := post(d.handleAssistantInterpret, allPrivCtx(), "/i", `{"message":"tell me about CA-01 in HIPAA GRC Platform 2027"}`)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, `"action":"reply"`) || !strings.Contains(body, `"control_link"`) || strings.Contains(body, "explain_control") {
		t.Fatalf("status = %d (%s)", rec.Code, body)
	}
	spy.interpret = &auditservice.AssistantResult{Action: auditservice.AssistantCompareControls, ControlNumber: "CA-01", ControlNumber2: "CA-02", AuditName: "HIPAA GRC Platform 2027"}
	rec = post(d.handleAssistantInterpret, allPrivCtx(), "/i", `{"message":"compare CA-01 and CA-02"}`)
	if body := rec.Body.String(); rec.Code != http.StatusOK || !strings.Contains(body, `"compare"`) || strings.Contains(body, "compare_controls") {
		t.Fatalf("status = %d (%s)", rec.Code, body)
	}
}
