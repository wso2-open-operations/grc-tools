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

// AuditHandler handles /audits routes.
type AuditHandler struct{ svc service.AuditService }

// NewAuditHandler constructs an AuditHandler.
func NewAuditHandler(svc service.AuditService) *AuditHandler { return &AuditHandler{svc: svc} }

// SearchAudits handles POST /audits/search.
func (h *AuditHandler) SearchAudits(w http.ResponseWriter, r *http.Request) {
	var req domain.SearchAuditsRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	resp, err := h.svc.SearchAudits(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// GetAuditByID handles GET /audits/{id}.
func (h *AuditHandler) GetAuditByID(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "id must be a positive integer"})
		return
	}
	a, err := h.svc.GetAuditByID(r.Context(), id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(a)
}

// CreateAudit handles POST /audits.
func (h *AuditHandler) CreateAudit(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateAuditRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	a, err := h.svc.CreateAudit(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(a)
}

// DeleteAudit handles DELETE /audits/{id}?deletedBy=.
func (h *AuditHandler) DeleteAudit(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "id must be a positive integer"})
		return
	}
	deletedBy := r.URL.Query().Get("deletedBy")
	if err := h.svc.DeleteAudit(r.Context(), id, deletedBy); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// UpdateAudit handles PATCH /audits/{id}.
func (h *AuditHandler) UpdateAudit(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "id must be a positive integer"})
		return
	}
	var req domain.UpdateAuditRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	a, err := h.svc.UpdateAudit(r.Context(), id, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(a)
}
