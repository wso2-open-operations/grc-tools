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

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	riskservice "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/service"
)

// The save handlers (create / update risk, assess, add action plan) record any
// AI suggestion decision after the save succeeds. Tests that are not about
// that give Deps these no-op stand-ins, so the handlers have something to call.
// The embedded interfaces cover the methods those tests never reach.

type fakeCategorySuggestionSvc struct {
	riskservice.CategorySuggestionService
}

func (fakeCategorySuggestionSvc) RecordDecision(context.Context, int, *model.AICategorySuggestion, []int, string) error {
	return nil
}

type fakeLikelihoodSuggestionSvc struct {
	riskservice.LikelihoodSuggestionService
}

func (fakeLikelihoodSuggestionSvc) RecordDecision(context.Context, int, *model.AILikelihoodSuggestion, int, string) error {
	return nil
}

type fakeActionPlanSuggestionSvc struct {
	riskservice.ActionPlanSuggestionService
}

func (fakeActionPlanSuggestionSvc) RecordDecision(context.Context, int, *model.AIActionPlanSuggestion, string) error {
	return nil
}

// withNoopAISuggestions fills in the three AI suggestion services with no-ops.
func (d *Deps) withNoopAISuggestions() *Deps {
	d.CategorySuggestion = fakeCategorySuggestionSvc{}
	d.LikelihoodSuggestion = fakeLikelihoodSuggestionSvc{}
	d.ActionPlanSuggestion = fakeActionPlanSuggestionSvc{}
	return d
}
