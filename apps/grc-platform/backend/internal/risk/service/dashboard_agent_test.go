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

type fakeInterpreter struct {
	replies []*aigateway.InterpretWidgetResult
	err     error
	calls   []aigateway.InterpretWidgetRequest
}

func (f *fakeInterpreter) InterpretWidget(_ context.Context, req aigateway.InterpretWidgetRequest) (*aigateway.InterpretWidgetResult, error) {
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

type spyWidgetRepo struct {
	calls int
	spec  dashboardagent.Spec
}

func (s *spyWidgetRepo) Run(_ context.Context, spec dashboardagent.Spec, _ dashboardagent.Visibility) (*dashboardagent.Result, error) {
	s.calls++
	s.spec = spec
	return &dashboardagent.Result{Columns: []dashboardagent.Column{{ID: "risk_count", Role: "measure"}}, Rows: [][]any{{1}}}, nil
}

type fakePeopleDir map[string][]dashboardagent.Person

func (f fakePeopleDir) SearchPeople(q string) []dashboardagent.Person { return f[strings.ToLower(q)] }

func newTestAgent(in *fakeInterpreter, people ...dashboardagent.PeopleDirectory) (*dashboardAgentService, *spyWidgetRepo) {
	repo := &spyWidgetRepo{}
	var dir dashboardagent.PeopleDirectory
	if len(people) > 0 {
		dir = people[0]
	}
	svc := NewDashboardAgentService(repo, in, dir).(*dashboardAgentService)
	svc.now = func() time.Time { return time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC) }
	return svc, repo
}

const goodSpec = `{"dataset":"risks","measures":["risk_count"],"dimensions":["assignment_team"]}`

func widgetReply(visual, spec string) *aigateway.InterpretWidgetResult {
	return &aigateway.InterpretWidgetResult{Action: aigateway.WidgetActionBuild, Title: "Risks by team", Visual: visual, Spec: []byte(spec)}
}

func TestRunNeverQueriesWithoutVisibility(t *testing.T) {
	svc, repo := newTestAgent(&fakeInterpreter{})
	res, err := svc.Run(context.Background(), dashboardagent.Spec{Dataset: "risks", Measures: []string{"risk_count"}}, dashboardagent.Visibility{})
	if err != nil || len(res.Rows) != 0 || repo.calls != 0 {
		t.Fatalf("rows = %d, calls = %d, err = %v", len(res.Rows), repo.calls, err)
	}
	if _, err := svc.Run(context.Background(), dashboardagent.Spec{Dataset: "risks", Measures: []string{"risk_count"}}, dashboardagent.Visibility{All: true}); err != nil || repo.calls != 1 {
		t.Fatalf("calls = %d, err = %v", repo.calls, err)
	}
}

func TestInterpretReturnsAValidatedWidget(t *testing.T) {
	in := &fakeInterpreter{replies: []*aigateway.InterpretWidgetResult{widgetReply("bar", goodSpec)}}
	svc, _ := newTestAgent(in)

	res, err := svc.Interpret(context.Background(), InterpretRequest{Message: "risks by team"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != InterpretWidget || res.Widget == nil || res.Widget.Visual != "bar" || res.Widget.Spec.Limit != dashboardagent.DefaultLimit {
		t.Fatalf("%+v", res)
	}
	if got := in.calls[0]; got.Today != "2026-11-11" || len(got.Catalog) == 0 || got.Message != "risks by team" {
		t.Fatalf("the model was not given the date, catalog and message: %+v", got)
	}
}

// Talking is the default: a reply goes through as plain text, with no widget.
func TestInterpretPassesAReplyThrough(t *testing.T) {
	svc, repo := newTestAgent(&fakeInterpreter{replies: []*aigateway.InterpretWidgetResult{{Action: aigateway.WidgetActionReply, Message: "Hello! What would you like to know?"}}})
	res, err := svc.Interpret(context.Background(), InterpretRequest{Message: "hi"}, nil)
	if err != nil || res.Action != InterpretReply || res.Message != "Hello! What would you like to know?" || res.Widget != nil {
		t.Fatalf("%+v, %v", res, err)
	}
	if repo.calls != 0 {
		t.Fatal("a reply must not run a query")
	}
}

// The ready-made views reach the model as named options, so asking for one by
// name produces it; they are not something the popup shows.
func TestInterpretTellsTheModelAboutNamedViews(t *testing.T) {
	in := &fakeInterpreter{replies: []*aigateway.InterpretWidgetResult{{Action: aigateway.WidgetActionReply, Message: "ok"}}}
	svc, _ := newTestAgent(in)
	if _, err := svc.Interpret(context.Background(), InterpretRequest{Message: "hi"}, nil); err != nil {
		t.Fatal(err)
	}
	views := in.calls[0].KnownViews
	if len(views) != 4 {
		t.Fatalf("expected the 4 ready-made views, got %d", len(views))
	}
	found := false
	for _, v := range views {
		if v.Title == "Risk heat map" && strings.Contains(v.Widget, `"visual":"heatmap"`) {
			found = true
		}
	}
	if !found {
		t.Fatalf("the heat map is missing from the named views: %+v", views)
	}
}

func TestInterpretPassesClarifyAndUnsupportedThrough(t *testing.T) {
	svc, _ := newTestAgent(&fakeInterpreter{replies: []*aigateway.InterpretWidgetResult{{Action: aigateway.WidgetActionClarify, Question: "Which quarter?"}}})
	res, err := svc.Interpret(context.Background(), InterpretRequest{Message: "trend"}, nil)
	if err != nil || res.Action != InterpretClarify || res.Question != "Which quarter?" || res.Widget != nil {
		t.Fatalf("%+v, %v", res, err)
	}
	svc, _ = newTestAgent(&fakeInterpreter{replies: []*aigateway.InterpretWidgetResult{{Action: aigateway.WidgetActionUnsupported, Message: "Risk data only."}}})
	res, err = svc.Interpret(context.Background(), InterpretRequest{Message: "close all risks"}, nil)
	if err != nil || res.Action != InterpretUnsupported || res.Message != "Risk data only." {
		t.Fatalf("%+v, %v", res, err)
	}
}

// A bad first answer gets one more try, told why it failed.
func TestInterpretRetriesOnceWithTheReason(t *testing.T) {
	in := &fakeInterpreter{replies: []*aigateway.InterpretWidgetResult{
		widgetReply("bar", `{"dataset":"risks","measures":["risk_count"],"dimensions":["owner_id"]}`),
		widgetReply("bar", goodSpec),
	}}
	svc, _ := newTestAgent(in)
	res, err := svc.Interpret(context.Background(), InterpretRequest{Message: "by owner"}, nil)
	if err != nil || res.Action != InterpretWidget {
		t.Fatalf("%+v, %v", res, err)
	}
	if len(in.calls) != 2 || !strings.Contains(in.calls[1].RetryFeedback, "owner_id") || in.calls[0].RetryFeedback != "" {
		t.Fatalf("retry feedback = %q (calls %d)", in.calls[1].RetryFeedback, len(in.calls))
	}
}

// Whatever the model returns, nothing invalid reaches the popup.
func TestInterpretNeverReturnsAnInvalidWidget(t *testing.T) {
	bad := map[string]*aigateway.InterpretWidgetResult{
		"unknown field":  widgetReply("bar", `{"dataset":"risks","measures":["risk_count"],"dimensions":["owner_id"]}`),
		"sql in a value": widgetReply("bar", `{"dataset":"risks","measures":["risk_count"],"filters":[{"field":"risk_year","op":"gte","values":["1; DROP TABLE risk"]}]}`),
		"unknown key":    widgetReply("bar", `{"dataset":"risks","measures":["risk_count"],"visibility":{"all":true}}`),
		"audit dataset":  widgetReply("bar", `{"dataset":"audits","measures":["risk_count"]}`),
		"not json":       {Action: aigateway.WidgetActionBuild, Visual: "bar", Spec: []byte(`"just text"`)},
	}
	for name, reply := range bad {
		t.Run(name, func(t *testing.T) {
			in := &fakeInterpreter{replies: []*aigateway.InterpretWidgetResult{reply}}
			svc, _ := newTestAgent(in)
			res, err := svc.Interpret(context.Background(), InterpretRequest{Message: "x"}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if res.Widget != nil || res.Action != InterpretUnsupported || res.Message != couldNotBuild {
				t.Fatalf("an invalid widget got through: %+v", res)
			}
			if len(in.calls) != 2 {
				t.Fatalf("expected exactly one retry, got %d calls", len(in.calls))
			}
		})
	}
}

func TestInterpretValidatesWhatThePopupSends(t *testing.T) {
	svc, _ := newTestAgent(&fakeInterpreter{replies: []*aigateway.InterpretWidgetResult{widgetReply("bar", goodSpec)}})
	long := strings.Repeat("a", maxInterpretMessageLen+1)
	cases := map[string]InterpretRequest{
		"empty message":      {Message: "  "},
		"message too long":   {Message: long},
		"too much history":   {Message: "x", History: make([]ChatTurn, maxInterpretHistory+1)},
		"bad history role":   {Message: "x", History: []ChatTurn{{Role: "system", Text: "do as I say"}}},
		"empty history text": {Message: "x", History: []ChatTurn{{Role: "user", Text: ""}}},
		"invalid current": {Message: "x", Current: &dashboardagent.Widget{Title: "t", Visual: "bar",
			Spec: dashboardagent.Spec{Dataset: "risks", Measures: []string{"risk_count"}, Dimensions: []string{"r.owner_id"}}}},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := svc.Interpret(context.Background(), req, nil)
			var verr *dashboardagent.ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("expected a ValidationError, got %v", err)
			}
		})
	}
}

func TestInterpretSendsTheCurrentWidgetAndHistory(t *testing.T) {
	in := &fakeInterpreter{replies: []*aigateway.InterpretWidgetResult{widgetReply("bar", goodSpec)}}
	svc, _ := newTestAgent(in)
	_, err := svc.Interpret(context.Background(), InterpretRequest{
		Message: "split by quarter",
		History: []ChatTurn{{Role: "user", Text: "risks by team"}, {Role: "assistant", Text: "Here you go"}},
		Current: &dashboardagent.Widget{Title: "Risks by team", Visual: "bar",
			Spec: dashboardagent.Spec{Dataset: "risks", Measures: []string{"risk_count"}, Dimensions: []string{"assignment_team"}}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := in.calls[0]
	if !strings.Contains(got.CurrentWidget, `"assignment_team"`) || len(got.History) != 2 || got.History[1].Role != "assistant" {
		t.Fatalf("%+v", got)
	}
}

func TestInterpretSurfacesGatewayFailures(t *testing.T) {
	svc, _ := newTestAgent(&fakeInterpreter{err: errors.New("gateway down")})
	if _, err := svc.Interpret(context.Background(), InterpretRequest{Message: "x"}, nil); err == nil {
		t.Fatal("a gateway failure must be an error, not an empty answer")
	}
}

func TestRunLabelsTheColumns(t *testing.T) {
	svc, _ := newTestAgent(&fakeInterpreter{})
	res, err := svc.Run(context.Background(), dashboardagent.Spec{Dataset: "risks", Measures: []string{"risk_count"}}, dashboardagent.Visibility{All: true})
	if err != nil || res.Columns[0].Label != "Number of risks" {
		t.Fatalf("%+v, %v", res, err)
	}
}

// The slips a model makes in ordinary use must not turn into "I couldn't build
// that": they are corrected, and the widget still passes full validation.
func TestInterpretForgivesHarmlessSlips(t *testing.T) {
	cases := map[string]struct {
		reply      *aigateway.InterpretWidgetResult
		wantVisual string
	}{
		"a list with a limit over the cap and a made-up visual": {
			widgetReply("list", `{"dataset":"risk_list","columns":["risk_code","risk_title"],"measures":[],"dimensions":[],"limit":100}`),
			"table",
		},
		"a list sorted by a column it does not show": {
			widgetReply("table", `{"dataset":"risk_list","columns":["risk_code"],"sort":{"by":"risk_year","desc":true}}`),
			"table",
		},
		"a list drawn as a bar chart": {
			widgetReply("bar", `{"dataset":"risk_list","columns":["risk_code"]}`),
			"table",
		},
		"a breakdown with a visual that does not fit": {
			widgetReply("heatmap", goodSpec),
			"bar",
		},
		"a single number drawn as a bar": {
			widgetReply("bar", `{"dataset":"risks","measures":["risk_count"]}`),
			"kpi",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			in := &fakeInterpreter{replies: []*aigateway.InterpretWidgetResult{c.reply}}
			svc, _ := newTestAgent(in)
			res, err := svc.Interpret(context.Background(), InterpretRequest{Message: "what are the risks I have"}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if res.Action != InterpretWidget || res.Widget == nil {
				t.Fatalf("a harmless slip was refused: %+v", res)
			}
			if res.Widget.Visual != c.wantVisual {
				t.Fatalf("visual = %q, want %q", res.Widget.Visual, c.wantVisual)
			}
			if len(in.calls) != 1 {
				t.Fatalf("no retry should be needed, got %d calls", len(in.calls))
			}
		})
	}
}

// What protects data is not forgiven: an unknown field or a bad value is still a
// refusal, whatever else is tidied.
func TestInterpretStillRefusesWhatProtectsData(t *testing.T) {
	for name, spec := range map[string]string{
		"description column": `{"dataset":"risk_list","columns":["risk_description"],"limit":100}`,
		"owner field":        `{"dataset":"risks","measures":["risk_count"],"dimensions":["owner_id"]}`,
		"injection":          `{"dataset":"risks","measures":["risk_count"],"filters":[{"field":"risk_year","op":"gte","values":["1; DROP TABLE risk"]}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			svc, _ := newTestAgent(&fakeInterpreter{replies: []*aigateway.InterpretWidgetResult{widgetReply("table", spec)}})
			res, err := svc.Interpret(context.Background(), InterpretRequest{Message: "x"}, nil)
			if err != nil || res.Widget != nil || res.Message != couldNotBuild {
				t.Fatalf("%+v, %v", res, err)
			}
		})
	}
}

var tharushi = fakePeopleDir{"tharushi": {{UUID: "uuid-tharushi", Name: "Tharushi Pushpakumara"}}}

func asCaller(uuid string) context.Context {
	return middleware.WithUserInfo(context.Background(), &middleware.UserInfo{Subject: uuid})
}

func personListSpec(name string) dashboardagent.Spec {
	return dashboardagent.Spec{
		Dataset: "risk_list", Columns: []string{"risk_code"}, Limit: 50,
		Filters: []dashboardagent.Filter{{Field: "involved_person", Op: "in", Values: []string{name}}},
	}
}

// The query only ever sees identities: the name the user typed is looked up
// first and replaced.
func TestRunTurnsANameIntoAnIdentity(t *testing.T) {
	svc, repo := newTestAgent(&fakeInterpreter{}, tharushi)
	_, err := svc.Run(asCaller("uuid-caller"), personListSpec("Tharushi"), dashboardagent.Visibility{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := repo.spec.Filters[0].Values; len(got) != 1 || got[0] != "uuid-tharushi" {
		t.Fatalf("the query was given %v", got)
	}
}

func TestRunTreatsMeAsTheCaller(t *testing.T) {
	svc, repo := newTestAgent(&fakeInterpreter{}, tharushi)
	if _, err := svc.Run(asCaller("uuid-caller"), personListSpec("me"), dashboardagent.Visibility{TeamIDs: []int{1}}); err != nil {
		t.Fatal(err)
	}
	if got := repo.spec.Filters[0].Values[0]; got != "uuid-caller" {
		t.Fatalf("me resolved to %q", got)
	}
}

func TestRunDoesNotQueryWhenAPersonCannotBeSettled(t *testing.T) {
	svc, repo := newTestAgent(&fakeInterpreter{}, tharushi)
	_, err := svc.Run(asCaller("u"), personListSpec("nobody"), dashboardagent.Visibility{All: true})
	var verr *dashboardagent.ValidationError
	if !errors.As(err, &verr) || !strings.Contains(verr.Msg, "couldn't find anyone") {
		t.Fatalf("err = %v", err)
	}
	if repo.calls != 0 {
		t.Fatal("the database must not be reached")
	}
}

func TestInterpretAnswersAnUnknownPersonInWords(t *testing.T) {
	in := &fakeInterpreter{replies: []*aigateway.InterpretWidgetResult{
		widgetReply("table", `{"dataset":"risk_list","columns":["risk_code"],"filters":[{"field":"involved_person","op":"in","values":["nobody"]}]}`),
	}}
	svc, repo := newTestAgent(in, tharushi)
	res, err := svc.Interpret(asCaller("u"), InterpretRequest{Message: "nobody's risks"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != InterpretReply || res.Widget != nil || !strings.Contains(res.Message, "couldn't find anyone called “nobody”") {
		t.Fatalf("%+v", res)
	}
	if repo.calls != 0 {
		t.Fatal("nothing should run")
	}
}

// When the person is found the widget keeps the name the user typed: the
// identity is looked up again each time it runs, never stored in the widget.
func TestInterpretKeepsTheNameInTheWidget(t *testing.T) {
	in := &fakeInterpreter{replies: []*aigateway.InterpretWidgetResult{
		widgetReply("table", `{"dataset":"risk_list","columns":["risk_code"],"filters":[{"field":"involved_person","op":"in","values":["tharushi"]}]}`),
	}}
	svc, _ := newTestAgent(in, tharushi)
	res, err := svc.Interpret(asCaller("u"), InterpretRequest{Message: "tharushi's risks"}, nil)
	if err != nil || res.Action != InterpretWidget {
		t.Fatalf("%+v, %v", res, err)
	}
	if got := res.Widget.Spec.Filters[0].Values[0]; got != "tharushi" {
		t.Fatalf("the widget holds %q", got)
	}
}

func TestSingleRiskCode(t *testing.T) {
	cases := map[string]string{
		"2026-INT-Q4-0002 what happened to this risk": "2026-INT-Q4-0002",
		"what happened to 2025-MS-BO-Q3-0001?":        "2025-MS-BO-Q3-0001",
		"tell me about 2026-Biz-Q4-0023.":             "2026-Biz-Q4-0023",
		"explain seed-ms-01 please":                   "seed-ms-01",
		"2026-ASG-Q3-0001 and 2026-ASG-q3-0001":       "2026-ASG-Q3-0001", // the same code twice
		"what are the risks I have":                   "",
		"in 2026 Q4 how many risks":                   "",
		"the year 2026-10-05":                         "",
	}
	for msg, want := range cases {
		got := riskCodesIn(msg)
		if want == "" {
			if len(got) != 0 {
				t.Errorf("%q: got %v, want none", msg, got)
			}
			continue
		}
		if len(got) != 1 || !strings.EqualFold(got[0], want) {
			t.Errorf("%q: got %v, want %q", msg, got, want)
		}
	}
	two := riskCodesIn("compare 2026-ASG-Q3-0001 with 2026-CHO-Q3-0001")
	if len(two) != 2 {
		t.Errorf("two codes expected, got %v", two)
	}
}

// Two codes go to a comparison and three or more get a plain reply, all
// without asking the model.
func TestInterpretSendsTwoRisksToAComparison(t *testing.T) {
	in := &fakeInterpreter{replies: []*aigateway.InterpretWidgetResult{{Action: aigateway.WidgetActionReply, Message: "unused"}}}
	svc, _ := newTestAgent(in)
	res, err := svc.Interpret(context.Background(), InterpretRequest{Message: "compare 2026-ASG-Q3-0001 and 2026-CHO-Q3-0001"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != InterpretCompareRisks || len(res.RiskCodes) != 2 {
		t.Fatalf("%+v", res)
	}
	res, err = svc.Interpret(context.Background(), InterpretRequest{Message: "compare 2026-ASG-Q3-0001, 2026-CHO-Q3-0001 and 2026-INT-Q4-0002"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != InterpretReply || !strings.Contains(res.Message, "two risks at a time") {
		t.Fatalf("%+v", res)
	}
	if len(in.calls) != 0 {
		t.Fatal("the model must not be called when risk codes are named")
	}
}

// A question that names a risk is answered from that risk's record, with no
// model call at all.
func TestInterpretSendsAnAskAboutOneRiskToItsRecord(t *testing.T) {
	in := &fakeInterpreter{replies: []*aigateway.InterpretWidgetResult{{Action: aigateway.WidgetActionReply, Message: "should not be used"}}}
	svc, _ := newTestAgent(in)
	res, err := svc.Interpret(context.Background(), InterpretRequest{Message: "2026-INT-Q4-0002 what happened to this risk"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != InterpretExplainRisk || res.RiskCode != "2026-INT-Q4-0002" {
		t.Fatalf("%+v", res)
	}
	if len(in.calls) != 0 {
		t.Fatal("the model must not be called for a question about one named risk")
	}
	// The code is for the handler only: it never appears in what the web app gets.
	if b, _ := json.Marshal(res); strings.Contains(string(b), "2026-INT") {
		t.Fatalf("the risk code is in the JSON: %s", b)
	}
}

// The model is given the help that matches the question, and the map of the
// Risk Hub with every one.
func TestInterpretGivesTheModelTheRelevantHelp(t *testing.T) {
	in := &fakeInterpreter{replies: []*aigateway.InterpretWidgetResult{{Action: aigateway.WidgetActionReply, Message: "1. Open Risk Hub, then Add Risk."}}}
	svc, _ := newTestAgent(in)
	res, err := svc.Interpret(context.Background(), InterpretRequest{Message: "where can I add a risk"}, nil)
	if err != nil || res.Action != InterpretReply {
		t.Fatalf("%+v, %v", res, err)
	}
	var titles []string
	for _, k := range in.calls[0].Knowledge {
		titles = append(titles, k.Title)
		if k.Source == "" || k.Text == "" {
			t.Errorf("incomplete knowledge section %+v", k)
		}
	}
	joined := strings.Join(titles, "|")
	if !strings.Contains(joined, "How to add a risk") || !strings.Contains(joined, "Where things are in the Risk Hub") {
		t.Fatalf("knowledge sent: %v", titles)
	}
}

// A list that shows risk codes gets the hidden id, so the web app can link them.
func TestRunAddsTheLinkColumnToARiskList(t *testing.T) {
	svc, repo := newTestAgent(&fakeInterpreter{})
	spec := dashboardagent.Spec{Dataset: "risk_list", Columns: []string{"risk_code", "risk_title"}, Limit: 50}
	if _, err := svc.Run(context.Background(), spec, dashboardagent.Visibility{All: true}); err != nil {
		t.Fatal(err)
	}
	if got := repo.spec.Columns; len(got) != 3 || got[2] != "risk_id" {
		t.Fatalf("the query was asked for %v", got)
	}
	if len(spec.Columns) != 2 {
		t.Fatal("the caller's spec was modified")
	}
	counts := dashboardagent.Spec{Dataset: "risks", Measures: []string{"risk_count"}}
	if _, err := svc.Run(context.Background(), counts, dashboardagent.Visibility{All: true}); err != nil {
		t.Fatal(err)
	}
	if len(repo.spec.Columns) != 0 {
		t.Fatalf("a count was given columns: %v", repo.spec.Columns)
	}
}

func TestTheModelIsNeverOfferedTheInternalColumn(t *testing.T) {
	in := &fakeInterpreter{replies: []*aigateway.InterpretWidgetResult{{Action: aigateway.WidgetActionReply, Message: "ok"}}}
	svc, _ := newTestAgent(in)
	if _, err := svc.Interpret(context.Background(), InterpretRequest{Message: "list the risks"}, nil); err != nil {
		t.Fatal(err)
	}
	for _, ds := range in.calls[0].Catalog {
		for _, c := range ds.Columns {
			if c.ID == "risk_id" {
				t.Fatalf("the model was offered risk_id in %s", ds.ID)
			}
		}
	}
}

// If the AI service fails the error is the calm sentinel; the cause is logged.
func TestInterpretReportsAnUnavailableAssistant(t *testing.T) {
	svc, _ := newTestAgent(&fakeInterpreter{err: errors.New("gateway responded 500: upstream detail")})
	_, err := svc.Interpret(context.Background(), InterpretRequest{Message: "x"}, nil)
	if !errors.Is(err, ErrAssistantUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "upstream detail") {
		t.Fatalf("the cause leaked into the error: %v", err)
	}
}
