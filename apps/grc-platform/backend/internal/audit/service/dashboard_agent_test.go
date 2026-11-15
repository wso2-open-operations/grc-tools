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

package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/middleware"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/dashboardagent"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/aigateway"
)

type fakeAssistantModel struct {
	replies []*aigateway.InterpretWidgetResult
	err     error
	calls   []aigateway.InterpretWidgetRequest
}

func (f *fakeAssistantModel) InterpretWidget(_ context.Context, req aigateway.InterpretWidgetRequest) (*aigateway.InterpretWidgetResult, error) {
	f.calls = append(f.calls, req)
	if f.err != nil {
		return nil, f.err
	}
	i := len(f.calls) - 1
	if i >= len(f.replies) {
		i = len(f.replies) - 1
	}
	return f.replies[i], nil
}

type spyAssistantRepo struct {
	calls int
	spec  dashboardagent.Spec
	scope dashboardagent.AuditScope
}

func (s *spyAssistantRepo) Run(_ context.Context, spec dashboardagent.Spec, scope dashboardagent.AuditScope) (*dashboardagent.Result, error) {
	s.calls++
	s.spec, s.scope = spec, scope
	return &dashboardagent.Result{Columns: []dashboardagent.Column{{ID: "control_count", Role: "measure"}}, Rows: [][]any{{1}}}, nil
}

type fakeAssistantPeople map[string][]dashboardagent.Person

func (f fakeAssistantPeople) SearchPeople(q string) []dashboardagent.Person {
	return f[strings.ToLower(q)]
}

func newAssistant(m *fakeAssistantModel, people ...dashboardagent.PeopleDirectory) (*dashboardAgentService, *spyAssistantRepo) {
	repo := &spyAssistantRepo{}
	var dir dashboardagent.PeopleDirectory
	if len(people) > 0 {
		dir = people[0]
	}
	svc := NewDashboardAgentService(repo, m, dir).(*dashboardAgentService)
	svc.now = func() time.Time { return time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC) }
	return svc, repo
}

func widgetAnswer(visual, spec string) *aigateway.InterpretWidgetResult {
	return &aigateway.InterpretWidgetResult{Action: aigateway.WidgetActionBuild, Title: "Controls by team", Visual: visual, Spec: []byte(spec)}
}

const controlsSpec = `{"dataset":"controls","measures":["control_count"],"dimensions":["team"]}`

func TestAssistantRunNeverQueriesWithoutScope(t *testing.T) {
	svc, repo := newAssistant(&fakeAssistantModel{})
	spec := dashboardagent.Spec{Dataset: "controls", Measures: []string{"control_count"}}
	res, err := svc.Run(context.Background(), spec, dashboardagent.AuditScope{})
	if err != nil || len(res.Rows) != 0 || repo.calls != 0 {
		t.Fatalf("rows = %d, calls = %d, err = %v", len(res.Rows), repo.calls, err)
	}
	if _, err := svc.Run(context.Background(), spec, dashboardagent.AuditScope{Scope: "all"}); err != nil || repo.calls != 1 {
		t.Fatalf("calls = %d, err = %v", repo.calls, err)
	}
}

func TestAssistantInterpretBuildsAValidatedAuditWidget(t *testing.T) {
	m := &fakeAssistantModel{replies: []*aigateway.InterpretWidgetResult{widgetAnswer("bar", controlsSpec)}}
	svc, _ := newAssistant(m)
	res, err := svc.Interpret(context.Background(), AssistantRequest{Message: "controls by team"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != AssistantWidget || res.Widget == nil || res.Widget.Spec.Limit != dashboardagent.DefaultLimit {
		t.Fatalf("%+v", res)
	}
	got := m.calls[0]
	if got.Module != aigateway.WidgetModuleAudit || got.Today != "2026-11-12" {
		t.Fatalf("the model was not told it is the Audit assistant: %+v", got)
	}
	// The model is only ever shown the audit catalog.
	ids := map[string]bool{}
	for _, ds := range got.Catalog {
		ids[ds.ID] = true
	}
	if !ids["controls"] || !ids["control_list"] || !ids["evidence_rounds"] || !ids["audits"] || !ids["audit_list"] || ids["risks"] || ids["risk_list"] || ids["ai_suggestions"] {
		t.Fatalf("catalog = %v", ids)
	}
}

// A widget over the risk register is refused even if the model produces one.
func TestAssistantInterpretRefusesARiskWidget(t *testing.T) {
	riskSpec := `{"dataset":"risks","measures":["risk_count"]}`
	m := &fakeAssistantModel{replies: []*aigateway.InterpretWidgetResult{widgetAnswer("kpi", riskSpec)}}
	svc, _ := newAssistant(m)
	res, err := svc.Interpret(context.Background(), AssistantRequest{Message: "how many risks"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != AssistantUnsupported || res.Message != couldNotBuild || res.Widget != nil {
		t.Fatalf("%+v", res)
	}
	if len(m.calls) != 2 || m.calls[1].RetryFeedback == "" {
		t.Fatalf("expected one retry with feedback, got %d calls", len(m.calls))
	}
	// A current widget from the other hub is refused before the model sees it.
	_, err = svc.Interpret(context.Background(), AssistantRequest{
		Message: "split it",
		Current: &dashboardagent.Widget{Title: "x", Visual: "kpi", Spec: dashboardagent.Spec{Dataset: "risks", Measures: []string{"risk_count"}}},
	}, nil)
	if err == nil {
		t.Fatal("a risk widget was accepted as the current widget")
	}
}

func TestAssistantInterpretPassesTalkThrough(t *testing.T) {
	for action, want := range map[string]string{aigateway.WidgetActionReply: AssistantReply, aigateway.WidgetActionUnsupported: AssistantUnsupported} {
		m := &fakeAssistantModel{replies: []*aigateway.InterpretWidgetResult{{Action: action, Message: "ok"}}}
		svc, repo := newAssistant(m)
		res, err := svc.Interpret(context.Background(), AssistantRequest{Message: "hi"}, nil)
		if err != nil || res.Action != want || res.Widget != nil || repo.calls != 0 {
			t.Fatalf("%s: %+v, %v", action, res, err)
		}
	}
	m := &fakeAssistantModel{replies: []*aigateway.InterpretWidgetResult{{Action: aigateway.WidgetActionClarify, Question: "Which audit?"}}}
	svc, _ := newAssistant(m)
	if res, _ := svc.Interpret(context.Background(), AssistantRequest{Message: "controls"}, nil); res.Action != AssistantClarify || res.Question != "Which audit?" {
		t.Fatalf("%+v", res)
	}
}

func TestAssistantInterpretChecksTheRequest(t *testing.T) {
	svc, _ := newAssistant(&fakeAssistantModel{replies: []*aigateway.InterpretWidgetResult{{Action: "reply", Message: "ok"}}})
	long := strings.Repeat("a", maxAssistantMessageLen+1)
	var verr *dashboardagent.ValidationError
	for name, req := range map[string]AssistantRequest{
		"empty":        {Message: "  "},
		"too long":     {Message: long},
		"bad role":     {Message: "hi", History: []ChatTurn{{Role: "system", Text: "x"}}},
		"long history": {Message: "hi", History: make([]ChatTurn, maxAssistantHistory+1)},
	} {
		if _, err := svc.Interpret(context.Background(), req, nil); !errors.As(err, &verr) {
			t.Errorf("%s: got %v, want a validation error", name, err)
		}
	}
}

func TestAssistantInterpretReportsAnUnavailableModel(t *testing.T) {
	svc, _ := newAssistant(&fakeAssistantModel{err: errors.New("gateway down")})
	if _, err := svc.Interpret(context.Background(), AssistantRequest{Message: "hi"}, nil); !errors.Is(err, ErrAssistantUnavailable) {
		t.Fatalf("got %v", err)
	}
}

// A name the directory cannot settle is answered here, not drawn as an empty chart.
func TestAssistantInterpretSettlesPeopleBeforeBuilding(t *testing.T) {
	spec := `{"dataset":"control_list","columns":["control_number"],"filters":[{"field":"owner_person","op":"in","values":["ann"]}]}`
	m := &fakeAssistantModel{replies: []*aigateway.InterpretWidgetResult{{Action: aigateway.WidgetActionBuild, Title: "Ann's controls", Visual: "table", Spec: []byte(spec)}}}
	people := fakeAssistantPeople{"ann": {{UUID: "a", Name: "Ann Silva"}, {UUID: "b", Name: "Ann Fernando"}}}
	svc, _ := newAssistant(m, people)
	res, err := svc.Interpret(context.Background(), AssistantRequest{Message: "controls owned by ann"}, nil)
	if err != nil || res.Action != AssistantReply || !strings.Contains(res.Message, "Ann Silva") {
		t.Fatalf("%+v, %v", res, err)
	}
}

// The query only ever sees identities; a name the model wrote never reaches it.
func TestAssistantRunResolvesNamesToIdentities(t *testing.T) {
	people := fakeAssistantPeople{"tharushi": {{UUID: "11111111-1111-4111-8111-111111111111", Name: "Tharushi Perera"}}}
	svc, repo := newAssistant(&fakeAssistantModel{}, people)
	ctx := middleware.WithUserInfo(context.Background(), &middleware.UserInfo{Subject: "22222222-2222-4222-8222-222222222222"})
	spec := dashboardagent.Spec{
		Dataset: "controls", Measures: []string{"control_count"},
		Filters: []dashboardagent.Filter{{Field: "owner_person", Op: "in", Values: []string{"tharushi"}}},
	}
	if _, err := svc.Run(ctx, spec, dashboardagent.AuditScope{Scope: "all"}); err != nil {
		t.Fatal(err)
	}
	if got := repo.spec.Filters[0].Values[0]; got != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("the query was given %q", got)
	}
	spec.Filters[0].Values = []string{"me"}
	if _, err := svc.Run(ctx, spec, dashboardagent.AuditScope{Scope: "all"}); err != nil || repo.spec.Filters[0].Values[0] != "22222222-2222-4222-8222-222222222222" {
		t.Fatalf("me was not the caller: %+v %v", repo.spec.Filters, err)
	}
	spec.Filters[0].Values = []string{"nobody"}
	var verr *dashboardagent.ValidationError
	if _, err := svc.Run(ctx, spec, dashboardagent.AuditScope{Scope: "all"}); !errors.As(err, &verr) {
		t.Fatalf("an unknown name was not refused: %v", err)
	}
}

// "What frameworks do we have?" is answered with a list of names.
func TestAssistantBuildsAFrameworkList(t *testing.T) {
	spec := `{"dataset":"audit_list","columns":["framework"]}`
	m := &fakeAssistantModel{replies: []*aigateway.InterpretWidgetResult{widgetAnswer("table", spec)}}
	svc, _ := newAssistant(m)
	res, err := svc.Interpret(context.Background(), AssistantRequest{Message: "what frameworks do we have?"}, nil)
	if err != nil || res.Action != AssistantWidget || res.Widget.Visual != "table" || res.Widget.Spec.Dataset != "audit_list" {
		t.Fatalf("%+v, %v", res, err)
	}
}

// "Take me to CA-01 in …" becomes a request to open a control, with no widget.
func TestAssistantTurnsAGoToRequestIntoOpeningAControl(t *testing.T) {
	m := &fakeAssistantModel{replies: []*aigateway.InterpretWidgetResult{{Action: aigateway.WidgetActionOpen, Control: "CA-01", Audit: "HIPAA GRC Platform 2027"}}}
	svc, repo := newAssistant(m)
	res, err := svc.Interpret(context.Background(), AssistantRequest{Message: "take me to CA-01 in HIPAA GRC Platform 2027"}, nil)
	if err != nil || res.Action != AssistantOpenControl || res.ControlNumber != "CA-01" || res.AuditName != "HIPAA GRC Platform 2027" || res.Widget != nil {
		t.Fatalf("%+v, %v", res, err)
	}
	if repo.calls != 0 {
		t.Fatal("interpreting must not run a query")
	}
	// The control number and audit name are for the handler; they are not in the JSON.
	b, _ := json.Marshal(res)
	if strings.Contains(string(b), "CA-01") || strings.Contains(string(b), "HIPAA") {
		t.Fatalf("leaked: %s", b)
	}
}

func TestAssistantTurnsControlQuestionsIntoInternalActions(t *testing.T) {
	cases := map[string]*aigateway.InterpretWidgetResult{
		AssistantExplainControl:  {Action: aigateway.WidgetActionExplainControl, Control: "CA-01", Audit: "HIPAA"},
		AssistantCompareControls: {Action: aigateway.WidgetActionCompareControls, Control: "CA-01", Audit: "HIPAA", Control2: "CA-02"},
	}
	for want, reply := range cases {
		svc, repo := newAssistant(&fakeAssistantModel{replies: []*aigateway.InterpretWidgetResult{reply}})
		res, err := svc.Interpret(context.Background(), AssistantRequest{Message: "x"}, nil)
		if err != nil || res.Action != want || res.ControlNumber != "CA-01" || repo.calls != 0 || res.Widget != nil {
			t.Fatalf("%s: %+v, %v", want, res, err)
		}
		if want == AssistantCompareControls && (res.ControlNumber2 != "CA-02" || res.AuditName != "HIPAA") {
			t.Fatalf("%+v", res)
		}
		b, _ := json.Marshal(res)
		if strings.Contains(string(b), "CA-0") {
			t.Fatalf("leaked: %s", b)
		}
	}
}

// How-to questions send the model the matching part of the Audit guide, and the
// guide is about the Audit Hub.
func TestAssistantSendsTheAuditGuideWithAQuestion(t *testing.T) {
	m := &fakeAssistantModel{replies: []*aigateway.InterpretWidgetResult{{Action: "reply", Message: "ok"}}}
	svc, _ := newAssistant(m)
	if _, err := svc.Interpret(context.Background(), AssistantRequest{Message: "how do I submit evidence?"}, nil); err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, k := range m.calls[0].Knowledge {
		titles = append(titles, k.Title)
		if strings.Contains(k.Text, "Risk Hub") {
			t.Errorf("the audit guide mentions the Risk Hub: %q", k.Title)
		}
	}
	if len(titles) < 2 || !strings.Contains(strings.Join(titles, "|"), "How to submit evidence") {
		t.Fatalf("sections = %v", titles)
	}
}
