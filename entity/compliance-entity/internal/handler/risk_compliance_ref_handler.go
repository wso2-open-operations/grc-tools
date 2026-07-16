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

// RiskComplianceRefHandler handles /risks/{riskId}/compliance-references routes.
type RiskComplianceRefHandler struct {
	svc service.RiskComplianceRefService
}

// NewRiskComplianceRefHandler constructs a RiskComplianceRefHandler.
func NewRiskComplianceRefHandler(svc service.RiskComplianceRefService) *RiskComplianceRefHandler {
	return &RiskComplianceRefHandler{svc: svc}
}

// AddRiskComplianceRef handles POST /risks/{riskId}/compliance-references.
func (h *RiskComplianceRefHandler) AddRiskComplianceRef(w http.ResponseWriter, r *http.Request) {
	riskID, err := strconv.Atoi(r.PathValue("riskId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "riskId must be a positive integer"})
		return
	}
	var req domain.AddRiskComplianceRefRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	link, err := h.svc.AddRiskComplianceRef(r.Context(), riskID, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(link)
}

// ListRiskComplianceRefs handles GET /risks/{riskId}/compliance-references.
func (h *RiskComplianceRefHandler) ListRiskComplianceRefs(w http.ResponseWriter, r *http.Request) {
	riskID, err := strconv.Atoi(r.PathValue("riskId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "riskId must be a positive integer"})
		return
	}
	resp, err := h.svc.ListRiskComplianceRefs(r.Context(), riskID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// DeleteRiskComplianceRef handles DELETE /risks/{riskId}/compliance-references/{referenceId}.
func (h *RiskComplianceRefHandler) DeleteRiskComplianceRef(w http.ResponseWriter, r *http.Request) {
	riskID, err := strconv.Atoi(r.PathValue("riskId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "riskId must be a positive integer"})
		return
	}
	referenceID, err := strconv.Atoi(r.PathValue("referenceId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "referenceId must be a positive integer"})
		return
	}
	if err := h.svc.DeleteRiskComplianceRef(r.Context(), riskID, referenceID); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
