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

// AuditFrameworkHandler handles /audit/frameworks routes.
type AuditFrameworkHandler struct{ svc service.AuditFrameworkService }

// NewAuditFrameworkHandler constructs an AuditFrameworkHandler.
func NewAuditFrameworkHandler(svc service.AuditFrameworkService) *AuditFrameworkHandler {
	return &AuditFrameworkHandler{svc: svc}
}

// SearchAuditFrameworks handles POST /audit/frameworks/search.
func (h *AuditFrameworkHandler) SearchAuditFrameworks(w http.ResponseWriter, r *http.Request) {
	var req domain.SearchAuditFrameworksRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	resp, err := h.svc.SearchAuditFrameworks(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// GetAuditFrameworkByID handles GET /audit/frameworks/{id}.
func (h *AuditFrameworkHandler) GetAuditFrameworkByID(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "id must be a positive integer"})
		return
	}
	fw, err := h.svc.GetAuditFrameworkByID(r.Context(), id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(fw)
}

// CreateAuditFramework handles POST /audit/frameworks.
func (h *AuditFrameworkHandler) CreateAuditFramework(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateAuditFrameworkRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	fw, err := h.svc.CreateAuditFramework(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(fw)
}

// UpdateAuditFramework handles PATCH /audit/frameworks/{id}.
func (h *AuditFrameworkHandler) UpdateAuditFramework(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "id must be a positive integer"})
		return
	}
	var req domain.UpdateAuditFrameworkRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	fw, err := h.svc.UpdateAuditFramework(r.Context(), id, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(fw)
}
