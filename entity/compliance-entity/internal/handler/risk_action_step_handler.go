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
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/service"
)

// RiskActionStepHandler handles /action-plans/{planId}/steps routes.
type RiskActionStepHandler struct{ svc service.RiskActionStepService }

// NewRiskActionStepHandler constructs a RiskActionStepHandler.
func NewRiskActionStepHandler(svc service.RiskActionStepService) *RiskActionStepHandler {
	return &RiskActionStepHandler{svc: svc}
}

// CreateRiskActionStep handles POST /action-plans/{planId}/steps.
func (h *RiskActionStepHandler) CreateRiskActionStep(w http.ResponseWriter, r *http.Request) {
	planID, err := strconv.Atoi(r.PathValue("planId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "planId must be a positive integer"})
		return
	}
	var req domain.CreateRiskActionStepRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	step, err := h.svc.CreateRiskActionStep(r.Context(), planID, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(step)
}

// ListRiskActionSteps handles GET /action-plans/{planId}/steps.
func (h *RiskActionStepHandler) ListRiskActionSteps(w http.ResponseWriter, r *http.Request) {
	planID, err := strconv.Atoi(r.PathValue("planId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "planId must be a positive integer"})
		return
	}
	resp, err := h.svc.ListRiskActionSteps(r.Context(), planID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// GetRiskActionStepByID handles GET /action-plans/{planId}/steps/{stepId}.
func (h *RiskActionStepHandler) GetRiskActionStepByID(w http.ResponseWriter, r *http.Request) {
	planID, err := strconv.Atoi(r.PathValue("planId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "planId must be a positive integer"})
		return
	}
	stepID, err := strconv.Atoi(r.PathValue("stepId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "stepId must be a positive integer"})
		return
	}
	step, err := h.svc.GetRiskActionStepByID(r.Context(), planID, stepID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(step)
}

// UpdateRiskActionStep handles PATCH /action-plans/{planId}/steps/{stepId}.
func (h *RiskActionStepHandler) UpdateRiskActionStep(w http.ResponseWriter, r *http.Request) {
	planID, err := strconv.Atoi(r.PathValue("planId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "planId must be a positive integer"})
		return
	}
	stepID, err := strconv.Atoi(r.PathValue("stepId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "stepId must be a positive integer"})
		return
	}
	var req domain.UpdateRiskActionStepRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	step, err := h.svc.UpdateRiskActionStep(r.Context(), planID, stepID, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(step)
}

// DeleteRiskActionStep handles DELETE /action-plans/{planId}/steps/{stepId}.
func (h *RiskActionStepHandler) DeleteRiskActionStep(w http.ResponseWriter, r *http.Request) {
	planID, err := strconv.Atoi(r.PathValue("planId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "planId must be a positive integer"})
		return
	}
	stepID, err := strconv.Atoi(r.PathValue("stepId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "stepId must be a positive integer"})
		return
	}
	if err := h.svc.DeleteRiskActionStep(r.Context(), planID, stepID); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
