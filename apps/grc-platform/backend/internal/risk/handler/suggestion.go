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
	"net/http"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// handleSuggestCategory serves POST /api/v1/risks/categories/suggest — the
// Add/Edit Risk form's "Suggest category" button. Advisory only: nothing is
// persisted by this call (see model.SuggestCategoryRequest's doc comment);
// the suggestion is recorded later, at risk-save time, via
// Deps.CategorySuggestion.RecordDecision.
//
// Gated on ViewRisks, same shape as the reference-data list endpoints
// (category.go) — this never mutates anything, so any authenticated caller
// who can see risks at all may ask for a suggestion.
func (d *Deps) handleSuggestCategory(w http.ResponseWriter, r *http.Request) {
	if !d.CategorySuggestionEnabled {
		response.WriteError(w, http.StatusNotFound, "auto-categorisation is not enabled")
		return
	}
	if !auth.RequireAnyPrivilege(r.Context(), w, privilege.ViewRisks, privilege.CreateRisk, privilege.ManageRiskHub) {
		return
	}

	var req model.SuggestCategoryRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}
	if req.Title == "" {
		response.WriteError(w, http.StatusBadRequest, "title is required")
		return
	}
	if req.Description == "" {
		response.WriteError(w, http.StatusBadRequest, "description is required")
		return
	}

	result, err := d.CategorySuggestion.Suggest(r.Context(), req)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusOK, result)
}

// handleSuggestLikelihood serves POST /api/v1/risks/likelihood/suggest — the
// Risk Assessment step's "Suggest Likelihood" button. Advisory only, same
// shape as handleSuggestCategory: nothing is persisted by this call; the
// suggestion is recorded later, at risk-save or reassess time, via
// Deps.LikelihoodSuggestion.RecordDecision.
func (d *Deps) handleSuggestLikelihood(w http.ResponseWriter, r *http.Request) {
	if !d.LikelihoodSuggestionEnabled {
		response.WriteError(w, http.StatusNotFound, "likelihood prediction is not enabled")
		return
	}
	if !auth.RequireAnyPrivilege(r.Context(), w, privilege.ViewRisks, privilege.CreateRisk, privilege.ManageRiskHub) {
		return
	}

	var req model.SuggestLikelihoodRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}
	if req.Title == "" {
		response.WriteError(w, http.StatusBadRequest, "title is required")
		return
	}
	if req.Description == "" {
		response.WriteError(w, http.StatusBadRequest, "description is required")
		return
	}
	if req.ImpactDescription == "" {
		response.WriteError(w, http.StatusBadRequest, "impact_description is required")
		return
	}
	if req.CategoryID == 0 {
		response.WriteError(w, http.StatusBadRequest, "category_id is required")
		return
	}
	if req.SourceRegisterID == 0 {
		response.WriteError(w, http.StatusBadRequest, "source_register_id is required")
		return
	}

	result, err := d.LikelihoodSuggestion.Suggest(r.Context(), req)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusOK, result)
}

// handleSuggestActionPlan serves POST /api/v1/risks/action-plans/suggest —
// the Add Risk wizard's Action Plan step and the standalone "add another
// action plan" dialog's "Suggest action plan" button. Advisory only, same
// shape as handleSuggestCategory: nothing is persisted by this call; the
// suggestion is recorded later, at risk-save or action-plan-create time, via
// Deps.ActionPlanSuggestion.RecordDecision.
func (d *Deps) handleSuggestActionPlan(w http.ResponseWriter, r *http.Request) {
	if !d.ActionPlanSuggestionEnabled {
		response.WriteError(w, http.StatusNotFound, "action plan drafting is not enabled")
		return
	}
	if !auth.RequireAnyPrivilege(r.Context(), w, privilege.ViewRisks, privilege.CreateRisk, privilege.ManageRiskHub) {
		return
	}

	var req model.SuggestActionPlanRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}
	if req.Title == "" {
		response.WriteError(w, http.StatusBadRequest, "title is required")
		return
	}
	if req.Description == "" {
		response.WriteError(w, http.StatusBadRequest, "description is required")
		return
	}

	result, err := d.ActionPlanSuggestion.Suggest(r.Context(), req)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusOK, result)
}
