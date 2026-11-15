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
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/dashboardagent"
)

// The Audit assistant question set (testdata/audit_assistant_questions.json) is
// a list of things people might type, each with what a sensible answer looks
// like. This file holds the pieces shared by the offline check below, which runs
// with the normal tests, and the live run in assistant_eval_live_test.go, which
// asks the real model and needs a gateway key.

type evalFilter struct {
	Field  string   `json:"field"`
	Values []string `json:"values,omitempty"` // when set, one of these must appear among the filter's values
}

type evalExpect struct {
	Actions      []string     `json:"actions"` // the answer must be one of these
	Dataset      string       `json:"dataset,omitempty"`
	Visual       []string     `json:"visual,omitempty"`
	Dims         []string     `json:"dims,omitempty"`
	Measures     []string     `json:"measures,omitempty"`
	Columns      []string     `json:"columns,omitempty"`
	Filters      []evalFilter `json:"filters,omitempty"`
	ForbidFields []string     `json:"forbid_fields,omitempty"` // must not appear anywhere in a built widget
	ReplyAny     []string     `json:"reply_any,omitempty"`     // a reply must mention at least one
	ReplyNone    []string     `json:"reply_none,omitempty"`    // a reply must mention none
	Control      string       `json:"control,omitempty"`       // for opening, describing or comparing controls
	Audit        string       `json:"audit,omitempty"`         // the audit name, when one was given
	Control2     string       `json:"control2,omitempty"`
	Safety       bool         `json:"safety,omitempty"` // a miss here counts as a safety failure
	Note         string       `json:"note,omitempty"`
}

type evalTurn struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

type evalCase struct {
	ID       string                 `json:"id"`
	Category string                 `json:"category"`
	Message  string                 `json:"message"`
	History  []evalTurn             `json:"history,omitempty"`
	Current  *dashboardagent.Widget `json:"current,omitempty"`
	Expect   evalExpect             `json:"expect"`
}

func loadEvalCases(t testing.TB) []evalCase {
	t.Helper()
	b, err := os.ReadFile("testdata/audit_assistant_questions.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Cases []evalCase `json:"cases"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	return f.Cases
}

func containsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}

func anyEqual(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// checkEval says what is wrong with an answer, or nothing if it is as expected.
func checkEval(e evalExpect, res *AssistantResult) []string {
	var problems []string
	if !anyEqual(e.Actions, res.Action) {
		return []string{fmt.Sprintf("action %q, wanted one of %v", res.Action, e.Actions)}
	}
	if e.Control != "" && !strings.EqualFold(res.ControlNumber, e.Control) {
		problems = append(problems, fmt.Sprintf("control %q, wanted %q", res.ControlNumber, e.Control))
	}
	if e.Control2 != "" && !strings.EqualFold(res.ControlNumber2, e.Control2) {
		problems = append(problems, fmt.Sprintf("second control %q, wanted %q", res.ControlNumber2, e.Control2))
	}
	if e.Audit != "" && res.AuditName != "" && !containsFold(res.AuditName, e.Audit) && !containsFold(e.Audit, res.AuditName) {
		problems = append(problems, fmt.Sprintf("audit %q, wanted %q", res.AuditName, e.Audit))
	}
	if res.Widget != nil {
		w, sp := res.Widget, res.Widget.Spec
		if e.Dataset != "" && sp.Dataset != e.Dataset {
			problems = append(problems, fmt.Sprintf("dataset %q, wanted %q", sp.Dataset, e.Dataset))
		}
		if len(e.Visual) > 0 && !anyEqual(e.Visual, w.Visual) {
			problems = append(problems, fmt.Sprintf("visual %q, wanted one of %v", w.Visual, e.Visual))
		}
		for _, d := range e.Dims {
			if !anyEqual(sp.Dimensions, d) {
				problems = append(problems, "missing dimension "+d)
			}
		}
		for _, m := range e.Measures {
			if !anyEqual(sp.Measures, m) {
				problems = append(problems, "missing measure "+m)
			}
		}
		for _, c := range e.Columns {
			if !anyEqual(sp.Columns, c) {
				problems = append(problems, "missing column "+c)
			}
		}
		for _, want := range e.Filters {
			found := false
			for _, f := range sp.Filters {
				if f.Field != want.Field {
					continue
				}
				if len(want.Values) == 0 {
					found = true
				}
				for _, v := range f.Values {
					for _, wv := range want.Values {
						if containsFold(v, wv) {
							found = true
						}
					}
				}
			}
			if !found {
				problems = append(problems, fmt.Sprintf("missing filter %s %v", want.Field, want.Values))
			}
		}
		raw, _ := json.Marshal(w)
		for _, bad := range e.ForbidFields {
			if containsFold(string(raw), bad) {
				problems = append(problems, "forbidden field in widget: "+bad)
			}
		}
	} else if e.Dataset != "" && len(e.Actions) == 1 && e.Actions[0] == "widget" {
		problems = append(problems, "no widget built")
	}
	text := res.Message + " " + res.Question
	if len(e.ReplyAny) > 0 && res.Action == AssistantReply {
		hit := false
		for _, s := range e.ReplyAny {
			hit = hit || containsFold(text, s)
		}
		if !hit {
			problems = append(problems, fmt.Sprintf("reply mentions none of %v", e.ReplyAny))
		}
	}
	for _, s := range e.ReplyNone {
		if containsFold(text, s) {
			problems = append(problems, "reply mentions "+s)
		}
	}
	return problems
}

// The question set itself must be sound, so a typo in it does not look like a
// failure of the assistant.
func TestAuditAssistantQuestionSetIsSound(t *testing.T) {
	cases := loadEvalCases(t)
	if len(cases) < 100 {
		t.Fatalf("only %d questions; the set should have at least 100", len(cases))
	}
	valid := map[string]bool{"widget": true, "reply": true, "clarify": true, "unsupported": true, "open_control": true, "explain_control": true, "compare_controls": true}
	known := map[string]bool{}
	datasets := map[string]bool{}
	for _, ds := range dashboardagent.CatalogForModule(dashboardagent.ModuleAudit, nil) {
		datasets[ds.ID] = true
		for _, f := range ds.Fields {
			known[ds.ID+"."+f.ID], known["*."+f.ID] = true, true
		}
		for _, m := range ds.Measures {
			known["*."+m.ID] = true
		}
		for _, c := range ds.Columns {
			known["*."+c.ID] = true
		}
	}
	seen := map[string]bool{}
	for _, c := range cases {
		if c.ID == "" || seen[c.ID] {
			t.Errorf("missing or repeated id %q", c.ID)
		}
		seen[c.ID] = true
		if strings.TrimSpace(c.Message) == "" || c.Category == "" || len(c.Expect.Actions) == 0 {
			t.Errorf("%s: needs a message, a category and actions", c.ID)
		}
		if len([]rune(c.Message)) > maxAssistantMessageLen {
			t.Errorf("%s: message too long", c.ID)
		}
		for _, a := range c.Expect.Actions {
			if !valid[a] {
				t.Errorf("%s: unknown action %q", c.ID, a)
			}
		}
		if c.Expect.Dataset != "" && !datasets[c.Expect.Dataset] {
			t.Errorf("%s: dataset %q is not in the audit catalog", c.ID, c.Expect.Dataset)
		}
		var named []string
		named = append(named, c.Expect.Dims...)
		named = append(named, c.Expect.Measures...)
		named = append(named, c.Expect.Columns...)
		for _, f := range c.Expect.Filters {
			named = append(named, f.Field)
		}
		for _, id := range named {
			if !known["*."+id] {
				t.Errorf("%s: %q is not in the audit catalog", c.ID, id)
			}
		}
		if c.Current != nil {
			if _, err := dashboardagent.ValidateWidgetIn(dashboardagent.ModuleAudit, *c.Current, nil); err != nil {
				t.Errorf("%s: the current widget is not valid: %v", c.ID, err)
			}
		}
	}
}
