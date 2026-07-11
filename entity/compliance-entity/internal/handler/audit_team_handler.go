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

// AuditTeamHandler handles /audit/teams routes.
type AuditTeamHandler struct{ svc service.AuditTeamService }

// NewAuditTeamHandler constructs an AuditTeamHandler.
func NewAuditTeamHandler(svc service.AuditTeamService) *AuditTeamHandler {
	return &AuditTeamHandler{svc: svc}
}

// SearchAuditTeams handles POST /audit/teams/search.
func (h *AuditTeamHandler) SearchAuditTeams(w http.ResponseWriter, r *http.Request) {
	var req domain.SearchAuditTeamsRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	resp, err := h.svc.SearchAuditTeams(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// GetAuditTeamByID handles GET /audit/teams/{id}.
func (h *AuditTeamHandler) GetAuditTeamByID(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "id must be a positive integer"})
		return
	}
	team, err := h.svc.GetAuditTeamByID(r.Context(), id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(team)
}

// CreateAuditTeam handles POST /audit/teams.
func (h *AuditTeamHandler) CreateAuditTeam(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateAuditTeamRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	team, err := h.svc.CreateAuditTeam(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(team)
}

// UpdateAuditTeam handles PATCH /audit/teams/{id}.
func (h *AuditTeamHandler) UpdateAuditTeam(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "id must be a positive integer"})
		return
	}
	var req domain.UpdateAuditTeamRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	team, err := h.svc.UpdateAuditTeam(r.Context(), id, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(team)
}
