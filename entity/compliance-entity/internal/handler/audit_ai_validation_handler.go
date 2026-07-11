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

// AIValidationHandler handles evidence AI-validation-log routes.
type AIValidationHandler struct{ svc service.AIValidationService }

// NewAIValidationHandler constructs an AIValidationHandler.
func NewAIValidationHandler(svc service.AIValidationService) *AIValidationHandler {
	return &AIValidationHandler{svc: svc}
}

// CreateValidation handles POST /evidence/{evidenceId}/ai-validations.
func (h *AIValidationHandler) CreateValidation(w http.ResponseWriter, r *http.Request) {
	evidenceID, err := strconv.Atoi(r.PathValue("evidenceId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "evidenceId must be a positive integer"})
		return
	}
	var req domain.CreateAuditAIValidationLogRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	l, err := h.svc.CreateValidation(r.Context(), evidenceID, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(l)
}

// ListValidations handles GET /evidence/{evidenceId}/ai-validations.
func (h *AIValidationHandler) ListValidations(w http.ResponseWriter, r *http.Request) {
	evidenceID, err := strconv.Atoi(r.PathValue("evidenceId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "evidenceId must be a positive integer"})
		return
	}
	resp, err := h.svc.ListValidationsByEvidence(r.Context(), evidenceID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
