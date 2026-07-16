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

// RiskAssessmentHandler handles risk assessment routes.
type RiskAssessmentHandler struct{ svc service.RiskAssessmentService }

// NewRiskAssessmentHandler constructs a RiskAssessmentHandler.
func NewRiskAssessmentHandler(svc service.RiskAssessmentService) *RiskAssessmentHandler {
	return &RiskAssessmentHandler{svc: svc}
}

// CreateRiskAssessment handles POST /risks/{riskId}/assessments.
func (h *RiskAssessmentHandler) CreateRiskAssessment(w http.ResponseWriter, r *http.Request) {
	riskID, err := strconv.Atoi(r.PathValue("riskId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "riskId must be a positive integer"})
		return
	}
	var req domain.CreateRiskAssessmentRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	a, err := h.svc.CreateRiskAssessment(r.Context(), riskID, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(a)
}

// ListRiskAssessments handles GET /risks/{riskId}/assessments.
func (h *RiskAssessmentHandler) ListRiskAssessments(w http.ResponseWriter, r *http.Request) {
	riskID, err := strconv.Atoi(r.PathValue("riskId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "riskId must be a positive integer"})
		return
	}
	resp, err := h.svc.ListRiskAssessments(r.Context(), riskID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
