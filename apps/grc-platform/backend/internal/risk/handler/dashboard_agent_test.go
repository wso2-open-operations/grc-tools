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
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/middleware"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/dashboardagent"
	riskservice "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/grant"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// roleResolver maps role names to privileges, standing in for privilege.Store.
type roleResolver map[string]map[string]bool

func (r roleResolver) Resolve(roles []string) map[string]bool {
	out := map[string]bool{}
	for _, role := range roles {
		for p := range r[role] {
			out[p] = true
		}
	}
	return out
}

var agentRoles = roleResolver{
	"risk-compliance-team": {privilege.ViewRisks: true, privilege.ViewRiskDashboard: true},
	"risk-owner":           {privilege.ViewRisks: true, privilege.OwnerApproveRisk: true},
	"platform-admin":       {"MANAGE_USERS": true},
	"audit-internal-team":  {"AUDIT_SUBMIT_EVIDENCE": true},
}

func contextWithGrants(grants []grant.Grant) context.Context {
	set := grant.Resolve(grants, agentRoles)
	ctx := middleware.WithUserInfo(context.Background(), &middleware.UserInfo{Subject: "test-caller-uuid"})
	ctx = grant.WithContext(ctx, set)
	return privilege.WithContext(ctx, set.PrivilegeMap())
}

func agentTeamGrant(role string, team int) grant.Grant {
	return grant.Grant{RoleName: role, Module: "RISK", ScopeType: grant.ScopeRiskTeam, ScopeID: team,
		ScopeBasis: grant.BasisSourceRegister, ScopeTeamType: grant.TeamBoth}
}

func agentGlobalGrant(role string) grant.Grant {
	return grant.Grant{RoleName: role, ScopeType: grant.ScopeGlobal}
}

func TestWidgetVisibility(t *testing.T) {
	seven := 7
	cases := []struct {
		name   string
		grants []grant.Grant
		users  fakeUserRepo
		want   dashboardagent.Visibility
	}{
		{
			name:   "GLOBAL ViewRisks sees every risk",
			grants: []grant.Grant{agentGlobalGrant("risk-compliance-team")},
			want:   dashboardagent.Visibility{All: true},
		},
		{
			name:   "team scoped sees the risks of their teams",
			grants: []grant.Grant{agentTeamGrant("risk-owner", 4), agentTeamGrant("risk-owner", 2)},
			want:   dashboardagent.Visibility{TeamIDs: []int{2, 4}},
		},
		{
			name:   "no grants at all sees the risks they are action owner on",
			grants: nil,
			users:  fakeUserRepo{uuid: "test-caller-uuid", id: seven},
			want:   dashboardagent.Visibility{ActionOwnerUserID: &seven},
		},
		{
			name:   "no grants and no user row sees nothing",
			grants: nil,
			users:  fakeUserRepo{uuid: "someone-else", id: seven},
			want:   dashboardagent.Visibility{},
		},
		{
			// Holds a grant, but not one that gives risk visibility: it must not
			// fall through to the action-owner path or to "everything".
			name:   "platform admin only sees nothing",
			grants: []grant.Grant{agentGlobalGrant("platform-admin")},
			users:  fakeUserRepo{uuid: "test-caller-uuid", id: seven},
			want:   dashboardagent.Visibility{},
		},
		{
			// AUDIT_TEAM scope ids share a number space with risk teams; they
			// must not count as risk teams.
			name: "audit-only grants see nothing",
			grants: []grant.Grant{{RoleName: "audit-internal-team", Module: "AUDIT",
				ScopeType: grant.ScopeAuditTeam, ScopeID: 4}},
			users: fakeUserRepo{uuid: "test-caller-uuid", id: seven},
			want:  dashboardagent.Visibility{},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := &Deps{Users: c.users}
			got, err := d.widgetVisibility(contextWithGrants(c.grants))
			if err != nil {
				t.Fatal(err)
			}
			// An empty team list and a nil one both mean "no teams".
			if len(got.TeamIDs) == 0 {
				got.TeamIDs = nil
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("visibility = %+v, want %+v", got, c.want)
			}
		})
	}
}

type spyDashboardAgent struct {
	calls        int
	spec         dashboardagent.Spec
	vis          dashboardagent.Visibility
	interpreted  int
	interpretReq riskservice.InterpretRequest
	canUseProbe  bool
	// interpretResult, when set, is what Interpret returns.
	interpretResult *riskservice.InterpretResult
	interpretErr    error
	runErr          error
}

func (s *spyDashboardAgent) Interpret(_ context.Context, req riskservice.InterpretRequest, canUse func(string) bool) (*riskservice.InterpretResult, error) {
	s.interpreted++
	s.interpretReq = req
	s.canUseProbe = canUse != nil
	if s.interpretErr != nil {
		return nil, s.interpretErr
	}
	if s.interpretResult != nil {
		return s.interpretResult, nil
	}
	return &riskservice.InterpretResult{Action: riskservice.InterpretClarify, Question: "Which quarter?"}, nil
}

func (s *spyDashboardAgent) Run(_ context.Context, spec dashboardagent.Spec, vis dashboardagent.Visibility) (*dashboardagent.Result, error) {
	s.calls++
	s.spec, s.vis = spec, vis
	if s.runErr != nil {
		return nil, s.runErr
	}
	return &dashboardagent.Result{
		Columns: []dashboardagent.Column{{ID: "risk_count", Role: "measure", Kind: "int"}},
		Rows:    [][]any{{3}},
	}, nil
}

func agentDeps(enabled bool, spy *spyDashboardAgent) *Deps {
	return (&Deps{
		DashboardAgent:                 spy,
		DashboardAgentEnabled:          enabled,
		Users:                          fakeUserRepo{uuid: "test-caller-uuid", id: 7},
		dashboardAgentLimiter:          dashboardagent.NewLimiter(2, 60e9),
		dashboardAgentInterpretLimiter: dashboardagent.NewLimiter(2, 60e9),
	})
}

func postSpec(d *Deps, ctx context.Context, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/risks/dashboard-agent/query", strings.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	d.handleDashboardAgentQuery(rec, req)
	return rec
}

const countBody = `{"dataset":"risks","measures":["risk_count"],"dimensions":["assignment_team"]}`

func TestDashboardAgentFlagOff(t *testing.T) {
	spy := &spyDashboardAgent{}
	rec := postSpec(agentDeps(false, spy), contextWithGrants([]grant.Grant{agentGlobalGrant("risk-compliance-team")}), countBody)
	if rec.Code != http.StatusNotFound || spy.calls != 0 {
		t.Fatalf("status = %d, calls = %d", rec.Code, spy.calls)
	}
}

func TestDashboardAgentNoVisibilityIsNotFound(t *testing.T) {
	for name, grants := range map[string][]grant.Grant{
		"platform admin only": {agentGlobalGrant("platform-admin")},
		"audit only":          {{RoleName: "audit-internal-team", ScopeType: grant.ScopeAuditTeam, ScopeID: 1}},
	} {
		t.Run(name, func(t *testing.T) {
			spy := &spyDashboardAgent{}
			rec := postSpec(agentDeps(true, spy), contextWithGrants(grants), countBody)
			if rec.Code != http.StatusNotFound || spy.calls != 0 {
				t.Fatalf("status = %d, calls = %d (the query must not run)", rec.Code, spy.calls)
			}
		})
	}
}

// The visibility the query runs with is the caller's own, whatever the body
// says: the request has no field that could carry one.
func TestDashboardAgentRunsWithTheCallersVisibility(t *testing.T) {
	spy := &spyDashboardAgent{}
	body := `{"dataset":"risks","measures":["risk_count"],"visibility":{"all":true},"teamIds":[1,2,3],"all":true}`
	rec := postSpec(agentDeps(true, spy), contextWithGrants([]grant.Grant{agentTeamGrant("risk-owner", 4)}), body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	if want := (dashboardagent.Visibility{TeamIDs: []int{4}}); !reflect.DeepEqual(spy.vis, want) {
		t.Fatalf("visibility = %+v, want %+v", spy.vis, want)
	}
	if spy.spec.Limit != dashboardagent.DefaultLimit {
		t.Fatalf("spec was not validated and cleaned: %+v", spy.spec)
	}
}

func TestDashboardAgentRejectsAnInvalidSpec(t *testing.T) {
	spy := &spyDashboardAgent{}
	ctx := contextWithGrants([]grant.Grant{agentGlobalGrant("risk-compliance-team")})
	for name, body := range map[string]string{
		"unknown dataset": `{"dataset":"audits","measures":["risk_count"]}`,
		"column name":     `{"dataset":"risks","measures":["risk_count"],"dimensions":["r.owner_id"]}`,
		"sql in a value":  `{"dataset":"risks","measures":["risk_count"],"filters":[{"field":"risk_year","op":"gte","values":["1 OR 1=1"]}]}`,
		"malformed json":  `{"dataset":`,
		"not an object":   `[]`,
	} {
		t.Run(name, func(t *testing.T) {
			rec := postSpec(agentDeps(true, spy), ctx, body)
			if rec.Code != http.StatusBadRequest || spy.calls != 0 {
				t.Fatalf("status = %d, calls = %d", rec.Code, spy.calls)
			}
		})
	}
}

func TestDashboardAgentIsRateLimitedPerUser(t *testing.T) {
	spy := &spyDashboardAgent{}
	d := agentDeps(true, spy) // 2 requests per window
	ctx := contextWithGrants([]grant.Grant{agentGlobalGrant("risk-compliance-team")})
	for i := 0; i < 2; i++ {
		if rec := postSpec(d, ctx, countBody); rec.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d", i, rec.Code)
		}
	}
	rec := postSpec(d, ctx, countBody)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" || spy.calls != 2 {
		t.Fatalf("status = %d, calls = %d", rec.Code, spy.calls)
	}
}

func getStarters(d *Deps, ctx context.Context) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/risks/dashboard-agent/starters", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	d.handleDashboardAgentStarters(rec, req)
	return rec
}

func postInterpret(d *Deps, ctx context.Context, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/risks/dashboard-agent/interpret", strings.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	d.handleDashboardAgentInterpret(rec, req)
	return rec
}

// The starters call is also how the web app decides whether to show the agent,
// so it must answer 404 exactly when the agent is not available to the caller.
func TestDashboardAgentStarters(t *testing.T) {
	spy := &spyDashboardAgent{}
	teamCtx := contextWithGrants([]grant.Grant{agentTeamGrant("risk-owner", 4)})

	if rec := getStarters(agentDeps(true, spy), teamCtx); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"risk-heat-map"`) {
		t.Fatalf("available: status = %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := getStarters(agentDeps(false, spy), teamCtx); rec.Code != http.StatusNotFound {
		t.Fatalf("flag off: status = %d", rec.Code)
	}
	for name, grants := range map[string][]grant.Grant{
		"platform admin only": {agentGlobalGrant("platform-admin")},
		"audit only":          {{RoleName: "audit-internal-team", ScopeType: grant.ScopeAuditTeam, ScopeID: 1}},
	} {
		if rec := getStarters(agentDeps(true, spy), contextWithGrants(grants)); rec.Code != http.StatusNotFound {
			t.Fatalf("%s: status = %d, want 404", name, rec.Code)
		}
	}
}

func TestDashboardAgentInterpret(t *testing.T) {
	ctx := contextWithGrants([]grant.Grant{agentTeamGrant("risk-owner", 4)})

	spy := &spyDashboardAgent{}
	rec := postInterpret(agentDeps(true, spy), ctx, `{"message":"risks by team","history":[{"role":"user","text":"hi"}]}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"clarify"`) {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	if spy.interpretReq.Message != "risks by team" || len(spy.interpretReq.History) != 1 || !spy.canUseProbe {
		t.Fatalf("%+v canUse=%v", spy.interpretReq, spy.canUseProbe)
	}

	// Unavailable to the caller: never reaches the model.
	for name, c := range map[string]struct {
		d   *Deps
		ctx context.Context
	}{
		"flag off":            {agentDeps(false, &spyDashboardAgent{}), ctx},
		"platform admin only": {agentDeps(true, &spyDashboardAgent{}), contextWithGrants([]grant.Grant{agentGlobalGrant("platform-admin")})},
	} {
		if rec := postInterpret(c.d, c.ctx, `{"message":"x"}`); rec.Code != http.StatusNotFound {
			t.Fatalf("%s: status = %d", name, rec.Code)
		}
		if spy := c.d.DashboardAgent.(*spyDashboardAgent); spy.interpreted != 0 {
			t.Fatalf("%s: the model was called", name)
		}
	}
}

func TestDashboardAgentInterpretErrors(t *testing.T) {
	ctx := contextWithGrants([]grant.Grant{agentTeamGrant("risk-owner", 4)})

	spy := &spyDashboardAgent{interpretErr: &dashboardagent.ValidationError{Msg: "write a request of up to 500 characters"}}
	rec := postInterpret(agentDeps(true, spy), ctx, `{"message":""}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "500 characters") {
		t.Fatalf("validation: status = %d (%s)", rec.Code, rec.Body.String())
	}

	spy = &spyDashboardAgent{interpretErr: errors.New("gateway responded 500: secret upstream detail")}
	rec = postInterpret(agentDeps(true, spy), ctx, `{"message":"x"}`)
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "secret upstream detail") {
		t.Fatalf("upstream failure: status = %d, body leaks detail: %s", rec.Code, rec.Body.String())
	}

	if rec := postInterpret(agentDeps(true, &spyDashboardAgent{}), ctx, `{"message":`); rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed body: status = %d", rec.Code)
	}
}

func TestDashboardAgentInterpretIsRateLimited(t *testing.T) {
	spy := &spyDashboardAgent{}
	d := agentDeps(true, spy) // 2 per window
	ctx := contextWithGrants([]grant.Grant{agentTeamGrant("risk-owner", 4)})
	for i := 0; i < 2; i++ {
		if rec := postInterpret(d, ctx, `{"message":"x"}`); rec.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d", i, rec.Code)
		}
	}
	if rec := postInterpret(d, ctx, `{"message":"x"}`); rec.Code != http.StatusTooManyRequests || spy.interpreted != 2 {
		t.Fatalf("status = %d, model calls = %d", rec.Code, spy.interpreted)
	}
}

func TestLabelActionOwners(t *testing.T) {
	res := &dashboardagent.Result{
		Columns: []dashboardagent.Column{{ID: "action_owner", Role: "dimension"}, {ID: "risk_count", Role: "measure"}},
		Rows: [][]any{
			{"uuid-ann", 3},
			{"uuid-nobody", 2},
			{nil, 1},
			{"", 1},
		},
	}
	labelActionOwners(res, func(uuids []string) map[string]string {
		if len(uuids) != 2 {
			t.Errorf("only real identities should be looked up, got %v", uuids)
		}
		return map[string]string{"uuid-ann": "Ann Perera"}
	})
	want := []string{"Ann Perera", actionOwnerUnknown, actionOwnerUnassigned, actionOwnerUnassigned}
	for i, row := range res.Rows {
		if row[0] != want[i] {
			t.Errorf("row %d = %v, want %s", i, row[0], want[i])
		}
	}
	for _, row := range res.Rows {
		if strings.HasPrefix(row[0].(string), "uuid-") {
			t.Errorf("a raw identity was left in the result: %v", row[0])
		}
	}
}

func TestLabelActionOwnersLeavesOtherResultsAlone(t *testing.T) {
	res := &dashboardagent.Result{
		Columns: []dashboardagent.Column{{ID: "assignment_team"}, {ID: "risk_count"}},
		Rows:    [][]any{{"Choreo", 3}},
	}
	labelActionOwners(res, func([]string) map[string]string { t.Error("lookup should not run"); return nil })
	if res.Rows[0][0] != "Choreo" {
		t.Fatalf("%v", res.Rows)
	}
}

// resultAgent answers every query with a fixed result.
type resultAgent struct {
	spyDashboardAgent
	result dashboardagent.Result
}

func (r *resultAgent) Run(context.Context, dashboardagent.Spec, dashboardagent.Visibility) (*dashboardagent.Result, error) {
	cp := r.result
	cp.Rows = make([][]any, len(r.result.Rows))
	for i, row := range r.result.Rows {
		cp.Rows[i] = append([]any(nil), row...)
	}
	return &cp, nil
}

func TestDashboardAgentQueryAddsASummaryWithoutAnyIdentity(t *testing.T) {
	agent := &resultAgent{result: dashboardagent.Result{
		Columns: []dashboardagent.Column{
			{ID: "action_owner", Role: "dimension", Kind: "text", Label: "Action Owner"},
			{ID: "risk_count", Role: "measure", Kind: "int", Label: "Number of risks"},
		},
		Rows: [][]any{{"uuid-ann-1234", float64(5)}, {"uuid-bob-5678", float64(2)}},
	}}
	d := agentDeps(true, &spyDashboardAgent{})
	d.DashboardAgent = agent // no directory: every owner is "Unknown"

	body := `{"dataset":"risks","measures":["risk_count"],"dimensions":["action_owner"]}`
	rec := postSpec(d, contextWithGrants([]grant.Grant{agentGlobalGrant("risk-compliance-team")}), body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	out := rec.Body.String()
	// No total is given: a risk can have several action plans, so adding the
	// groups up would overstate the number of risks.
	if !strings.Contains(out, `"summary":"The largest group by Action Owner is Unknown with 5."`) {
		t.Fatalf("unexpected summary in %s", out)
	}
	for _, leaked := range []string{"uuid-ann-1234", "uuid-bob-5678"} {
		if strings.Contains(out, leaked) {
			t.Fatalf("an identity reached the response: %s", out)
		}
	}
}

func TestDashboardAgentQueryListsRisks(t *testing.T) {
	agent := &resultAgent{result: dashboardagent.Result{
		Columns: []dashboardagent.Column{
			{ID: "risk_code", Role: "column", Kind: "text", Label: "Risk code"},
			{ID: "risk_title", Role: "column", Kind: "text", Label: "Title"},
		},
		Rows: [][]any{{"2026-ASG-Q4-0001", "Weak TLS"}, {"2026-ASG-Q4-0002", "Open bucket"}},
	}}
	d := agentDeps(true, &spyDashboardAgent{})
	d.DashboardAgent = agent
	rec := postSpec(d, contextWithGrants([]grant.Grant{agentTeamGrant("risk-owner", 4)}),
		`{"dataset":"risk_list","columns":["risk_code","risk_title"]}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"summary":"Showing 2 risks."`) {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
}

// A message written for the user (for example "I couldn't find anyone called
// ...") reaches them as a 400; anything else stays a generic failure.
func TestDashboardAgentQueryShowsAUserMessageButHidesOtherFailures(t *testing.T) {
	ctx := contextWithGrants([]grant.Grant{agentGlobalGrant("risk-compliance-team")})
	body := `{"dataset":"risks","measures":["risk_count"]}`

	spy := &spyDashboardAgent{runErr: &dashboardagent.ValidationError{Msg: "I couldn't find anyone called “x”."}}
	rec := postSpec(agentDeps(true, spy), ctx, body)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "couldn't find anyone") {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}

	spy = &spyDashboardAgent{runErr: errors.New("entity responded 500: table risk_x exploded")}
	rec = postSpec(agentDeps(true, spy), ctx, body)
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "exploded") {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

// When the AI service fails the user gets a calm 503, never the cause.
func TestInterpretEndpointSaysTheAssistantIsUnavailable(t *testing.T) {
	spy := &spyDashboardAgent{interpretErr: riskservice.ErrAssistantUnavailable}
	rec := postInterpret(agentDeps(true, spy), contextWithGrants([]grant.Grant{agentTeamGrant("risk-owner", 4)}), `{"message":"hi"}`)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "temporarily unavailable") {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
}

// captureLog sends the process's log to a buffer for the length of a test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })
	return &buf
}

// The audit log says who asked, how it ended and what kind of access they had,
// and which fields were used, but never the message, a filter value or a row.
func TestAuditLogNamesFieldsButNeverContent(t *testing.T) {
	buf := captureLog(t)
	agent := &resultAgent{result: dashboardagent.Result{
		Columns: []dashboardagent.Column{{ID: "risk_count", Role: "measure", Kind: "int", Label: "n"}},
		Rows:    [][]any{{float64(4127)}},
	}}
	d := agentDeps(true, &spyDashboardAgent{})
	d.DashboardAgent = agent
	ctx := contextWithGrants([]grant.Grant{agentTeamGrant("risk-owner", 4)})

	body := `{"dataset":"risks","measures":["risk_count"],"filters":[{"field":"source_register","op":"in","values":["TOP-SECRET-REGISTER-NAME"]}]}`
	if rec := postSpec(d, ctx, body); rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	log := buf.String()
	for _, want := range []string{"endpoint=query", "outcome=allowed", "access=team", "user=test-caller-uuid", "dataset=risks", "filterFields=[source_register]", "rows=1"} {
		if !strings.Contains(log, want) {
			t.Errorf("the audit log is missing %q:\n%s", want, log)
		}
	}
	for _, leaked := range []string{"TOP-SECRET-REGISTER-NAME", "4127"} {
		if strings.Contains(log, leaked) {
			t.Errorf("the audit log holds content (%q):\n%s", leaked, log)
		}
	}
}

func TestAuditLogRecordsEveryOutcome(t *testing.T) {
	ctx := contextWithGrants([]grant.Grant{agentTeamGrant("risk-owner", 4)})

	// refused: no risk access
	buf := captureLog(t)
	postInterpret(agentDeps(true, &spyDashboardAgent{}), contextWithGrants([]grant.Grant{agentGlobalGrant("platform-admin")}), `{"message":"x"}`)
	if log := buf.String(); !strings.Contains(log, "outcome=not_available") || !strings.Contains(log, "access=none") {
		t.Errorf("refusal not logged: %s", log)
	}

	// refused: the starters check is made on every page and is not logged
	buf = captureLog(t)
	getStarters(agentDeps(true, &spyDashboardAgent{}), contextWithGrants([]grant.Grant{agentGlobalGrant("platform-admin")}))
	if buf.Len() != 0 {
		t.Errorf("the starters check must not be logged: %s", buf.String())
	}

	// rate limited
	buf = captureLog(t)
	d := agentDeps(true, &spyDashboardAgent{})
	for i := 0; i < 3; i++ {
		postInterpret(d, ctx, `{"message":"x"}`)
	}
	if !strings.Contains(buf.String(), "outcome=rate_limited") {
		t.Errorf("rate limiting not logged: %s", buf.String())
	}

	// invalid
	buf = captureLog(t)
	postSpec(agentDeps(true, &spyDashboardAgent{}), ctx, `{"dataset":"nope","measures":["risk_count"]}`)
	if !strings.Contains(buf.String(), "outcome=invalid") {
		t.Errorf("an invalid request was not logged: %s", buf.String())
	}

	// the AI service failed: logged with the model, and the reply stays calm
	buf = captureLog(t)
	postInterpret(agentDeps(true, &spyDashboardAgent{interpretErr: riskservice.ErrAssistantUnavailable}), ctx, `{"message":"x"}`)
	log := buf.String()
	if !strings.Contains(log, "outcome=unavailable") || !strings.Contains(log, "model=claude-") {
		t.Errorf("an AI failure was not logged with the model: %s", log)
	}

	// an answer from the AI names the model; an explained risk does not
	buf = captureLog(t)
	postInterpret(agentDeps(true, &spyDashboardAgent{}), ctx, `{"message":"hi"}`)
	if log := buf.String(); !strings.Contains(log, "outcome=allowed") || !strings.Contains(log, "model=claude-") || strings.Contains(log, `"hi"`) {
		t.Errorf("an AI answer was not logged as expected: %s", log)
	}
}
