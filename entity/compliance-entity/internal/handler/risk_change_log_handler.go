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

// RiskChangeLogHandler handles /risks/{riskId}/changes routes.
type RiskChangeLogHandler struct{ svc service.RiskChangeLogService }

// NewRiskChangeLogHandler constructs a RiskChangeLogHandler.
func NewRiskChangeLogHandler(svc service.RiskChangeLogService) *RiskChangeLogHandler {
	return &RiskChangeLogHandler{svc: svc}
}

// CreateRiskChangeLog handles POST /risks/{riskId}/changes.
func (h *RiskChangeLogHandler) CreateRiskChangeLog(w http.ResponseWriter, r *http.Request) {
	riskID, err := strconv.Atoi(r.PathValue("riskId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "riskId must be a positive integer"})
		return
	}
	var req domain.CreateRiskChangeLogRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	e, err := h.svc.CreateRiskChangeLog(r.Context(), riskID, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(e)
}

// ListRiskChangeLog handles GET /risks/{riskId}/changes.
func (h *RiskChangeLogHandler) ListRiskChangeLog(w http.ResponseWriter, r *http.Request) {
	riskID, err := strconv.Atoi(r.PathValue("riskId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "riskId must be a positive integer"})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	resp, err := h.svc.ListRiskChangeLog(r.Context(), riskID, limit, offset)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
