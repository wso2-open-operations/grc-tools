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

// RiskAISuggestionHandler handles /risk/ai-suggestions routes.
type RiskAISuggestionHandler struct{ svc service.RiskAISuggestionService }

// NewRiskAISuggestionHandler constructs a RiskAISuggestionHandler.
func NewRiskAISuggestionHandler(svc service.RiskAISuggestionService) *RiskAISuggestionHandler {
	return &RiskAISuggestionHandler{svc: svc}
}

// CreateRiskAISuggestion handles POST /risk/ai-suggestions — called the
// moment an AI suggestion is generated and shown to a user, before they've
// decided anything about it.
func (h *RiskAISuggestionHandler) CreateRiskAISuggestion(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateRiskAISuggestionRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	s, err := h.svc.CreateRiskAISuggestion(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(s)
}

// DecideRiskAISuggestion handles PATCH /risk/ai-suggestions/{id}/decide —
// called once the user accepts the suggestion as-is, or picks something else.
func (h *RiskAISuggestionHandler) DecideRiskAISuggestion(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "id must be a positive integer"})
		return
	}
	var req domain.DecideRiskAISuggestionRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	s, err := h.svc.DecideRiskAISuggestion(r.Context(), id, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s)
}

// ListRiskAISuggestions handles
// GET /risk/ai-suggestions?riskId={id}&feature={CATEGORY|LIKELIHOOD}&status={SUGGESTED|ACCEPTED|OVERRIDDEN}.
// status is optional — omitted, every suggestion for riskId+feature is
// returned regardless of outcome.
func (h *RiskAISuggestionHandler) ListRiskAISuggestions(w http.ResponseWriter, r *http.Request) {
	riskID, err := strconv.Atoi(r.URL.Query().Get("riskId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "riskId must be a positive integer"})
		return
	}
	feature := domain.RiskAISuggestionFeature(r.URL.Query().Get("feature"))

	var status *domain.RiskAISuggestionStatus
	if raw := r.URL.Query().Get("status"); raw != "" {
		s := domain.RiskAISuggestionStatus(raw)
		status = &s
	}

	list, err := h.svc.ListRiskAISuggestionsByRisk(r.Context(), riskID, feature, status)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"suggestions": list})
}
