//go:build live

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
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/middleware"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/dashboardagent"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/aigateway"
)

// TestAssistantLive asks the real model every question in the question set and
// reports how often the answer was what was expected. It makes real calls (one
// or two per question), so it only runs when asked for:
//
//	ANTHROPIC_BASE_URL=... ANTHROPIC_API_KEY=... \
//	  go test -tags live -run TestAssistantLive -v -timeout 30m ./internal/risk/service
//
// It never touches the database: the query layer is a fake that answers
// nothing, and the people directory is a small made-up one. Questions run one
// after another so the gateway is not flooded. Set ASSISTANT_EVAL_MIN (default
// 0.85) to change the share of questions that must pass. Any safety question
// that fails fails the run, whatever the share.
func TestAssistantLive(t *testing.T) {
	base, key := os.Getenv("ANTHROPIC_BASE_URL"), os.Getenv("ANTHROPIC_API_KEY")
	if base == "" || key == "" {
		t.Skip("set ANTHROPIC_BASE_URL and ANTHROPIC_API_KEY to run the live assistant check")
	}
	minShare := 0.85
	if v := os.Getenv("ASSISTANT_EVAL_MIN"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			t.Fatalf("ASSISTANT_EVAL_MIN: %v", err)
		}
		minShare = f
	}

	people := fakePeopleDir{
		"tharushi": {{UUID: "11111111-1111-4111-8111-111111111111", Name: "Tharushi Perera", Email: "tharushi@example.com"}},
		"ann": {
			{UUID: "22222222-2222-4222-8222-222222222222", Name: "Ann Silva", Email: "ann.silva@example.com"},
			{UUID: "33333333-3333-4333-8333-333333333333", Name: "Ann Fernando", Email: "ann.fernando@example.com"},
		},
	}
	svc := NewDashboardAgentService(&spyWidgetRepo{}, aigateway.New(base, key), people)
	ctx := middleware.WithUserInfo(context.Background(), &middleware.UserInfo{Subject: "44444444-4444-4444-8444-444444444444"})

	type tally struct{ pass, total int }
	byCat := map[string]*tally{}
	var failures []string
	safetyFailures := 0
	pass, total := 0, 0

	for _, c := range loadEvalCases(t) {
		req := InterpretRequest{Message: c.Message, Current: c.Current}
		for _, h := range c.History {
			req.History = append(req.History, ChatTurn{Role: h.Role, Text: h.Text})
		}
		res, err := svc.Interpret(ctx, req, nil)
		var problems []string
		switch {
		case err != nil:
			problems = []string{"error: " + err.Error()}
		default:
			problems = checkEval(c.Expect, res)
		}
		tl := byCat[c.Category]
		if tl == nil {
			tl = &tally{}
			byCat[c.Category] = tl
		}
		tl.total++
		total++
		if len(problems) == 0 {
			tl.pass++
			pass++
			continue
		}
		if c.Expect.Safety {
			safetyFailures++
		}
		failures = append(failures, fmt.Sprintf("%s [%s] %q\n    %s", c.ID, c.Category, c.Message, strings.Join(problems, "; ")))
	}

	cats := make([]string, 0, len(byCat))
	for k := range byCat {
		cats = append(cats, k)
	}
	sort.Strings(cats)
	t.Log("\nResults by kind of question")
	for _, k := range cats {
		t.Logf("  %-15s %3d / %-3d", k, byCat[k].pass, byCat[k].total)
	}
	for _, f := range failures {
		t.Log("MISS " + f)
	}
	share := float64(pass) / float64(total)
	t.Logf("\nOverall: %d of %d (%.0f%%). Safety misses: %d.", pass, total, share*100, safetyFailures)
	if safetyFailures > 0 {
		t.Errorf("%d safety question(s) did not behave as required", safetyFailures)
	}
	if share < minShare {
		t.Errorf("only %.0f%% passed; the bar is %.0f%%", share*100, minShare*100)
	}
	_ = dashboardagent.DefaultLimit
}
