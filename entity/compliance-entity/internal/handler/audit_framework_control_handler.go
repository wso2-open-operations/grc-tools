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
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/middleware"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/service"
)

// FrameworkControlHandler handles routes under /audit/frameworks/{id}/controls.
type FrameworkControlHandler struct{ svc service.FrameworkControlService }

// NewFrameworkControlHandler constructs a FrameworkControlHandler.
func NewFrameworkControlHandler(svc service.FrameworkControlService) *FrameworkControlHandler {
	return &FrameworkControlHandler{svc: svc}
}

// ListCurrentControls handles GET /audit/frameworks/{id}/controls.
// Returns all is_current=TRUE controls for the framework.
func (h *FrameworkControlHandler) ListCurrentControls(w http.ResponseWriter, r *http.Request) {
	frameworkID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "framework id must be a positive integer"})
		return
	}
	resp, err := h.svc.ListCurrentControls(r.Context(), frameworkID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// ListAllVersions handles GET /audit/frameworks/{id}/controls/{controlNumber}/versions.
func (h *FrameworkControlHandler) ListAllVersions(w http.ResponseWriter, r *http.Request) {
	frameworkID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "framework id must be a positive integer"})
		return
	}
	controlNumber := r.PathValue("controlNumber")
	if controlNumber == "" {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "controlNumber path param is required"})
		return
	}
	versions, err := h.svc.ListAllVersions(r.Context(), frameworkID, controlNumber)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"versions": versions})
}

// CreateControl handles POST /audit/frameworks/{id}/controls.
func (h *FrameworkControlHandler) CreateControl(w http.ResponseWriter, r *http.Request) {
	frameworkID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "framework id must be a positive integer"})
		return
	}
	var req domain.CreateFrameworkControlRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	if req.CreatedBy == "" {
		req.CreatedBy = middleware.UserIDTokenFromContext(r.Context())
	}
	c, err := h.svc.Create(r.Context(), frameworkID, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(c)
}

// NewVersion handles PUT /audit/frameworks/{id}/controls/{controlId}.
// Creates a new version of the control; the previous row is marked is_current=FALSE.
func (h *FrameworkControlHandler) NewVersion(w http.ResponseWriter, r *http.Request) {
	controlID, err := strconv.Atoi(r.PathValue("controlId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "controlId must be a positive integer"})
		return
	}
	var req domain.UpdateFrameworkControlRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	if req.UpdatedBy == "" {
		req.UpdatedBy = middleware.UserIDTokenFromContext(r.Context())
	}
	c, err := h.svc.NewVersion(r.Context(), controlID, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(c)
}
