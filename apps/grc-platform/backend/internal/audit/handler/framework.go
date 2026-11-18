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
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

type frameworkHandler struct {
	svc service.FrameworkService
}

// listFrameworks handles GET /api/v1/audits/frameworks.
func (h *frameworkHandler) listFrameworks(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ViewAudits) {
		return
	}
	ctx := r.Context()
	scope, _ := deriveScopes(ctx)
	user := auth.FromContext(ctx)
	var userID int
	if user != nil {
		userID = user.UserID
	}
	// Default ACTIVE keeps the dropdown contract; the Frameworks page asks for
	// status=ALL (or ARCHIVED) to see and restore archived ones. Deleted
	// frameworks are never listed.
	var status string
	switch q := strings.ToUpper(r.URL.Query().Get("status")); q {
	case "", "ACTIVE":
		status = "ACTIVE"
	case "ARCHIVED":
		status = "ARCHIVED"
	case "ALL":
		status = ""
	default:
		response.WriteError(w, http.StatusBadRequest, "status must be ACTIVE, ARCHIVED or ALL")
		return
	}
	frameworks, err := h.svc.ListFrameworks(ctx, scope, userID, managedTeamIDs(auth.Grants(ctx)), status)
	if err != nil {
		response.MapServiceError(ctx, w, err, response.ErrMsgInternal)
		return
	}
	if frameworks == nil {
		frameworks = []*model.AuditFramework{}
	}
	response.WriteJSONValue(w, http.StatusOK, frameworks)
}

// createFramework handles POST /api/v1/audits/frameworks.
func (h *frameworkHandler) createFramework(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ManageFrameworks) {
		return
	}
	var req model.CreateFrameworkRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}
	actor := auth.FromContext(r.Context()).Subject
	fw, err := h.svc.CreateFramework(r.Context(), req, actor)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusCreated, fw)
}

// updateFramework handles PUT /api/v1/audits/frameworks/{id} — rename, archive
// (status ARCHIVED) or restore (status ACTIVE).
func (h *frameworkHandler) updateFramework(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ManageFrameworks) {
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		response.WriteError(w, http.StatusBadRequest, "invalid framework id")
		return
	}
	var req model.UpdateFrameworkRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}
	actor := auth.FromContext(r.Context()).Subject
	fw, err := h.svc.UpdateFramework(r.Context(), id, req, actor)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusOK, fw)
}

// deleteFramework handles DELETE /api/v1/audits/frameworks/{id}. A soft delete
// (the framework's status becomes INACTIVE): only an empty framework (no audits,
// no controls) can go; otherwise 409 and the UI points the user at archive.
func (h *frameworkHandler) deleteFramework(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ManageFrameworks) {
		return
	}
	// Delete is stricter than archive: the privilege must be held at GLOBAL scope
	// (grc-platform-audit-compliance-admin). Local dev allow-all has no grants.
	if !auth.AllowAll(r.Context()) && !auth.Grants(r.Context()).HasGlobal(privilege.ManageFrameworks) {
		response.WriteError(w, http.StatusForbidden, response.ErrMsgForbidden)
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		response.WriteError(w, http.StatusBadRequest, "invalid framework id")
		return
	}
	actor := auth.FromContext(r.Context()).Subject
	if err := h.svc.DeleteFramework(r.Context(), id, actor); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listProducts handles GET /api/v1/audits/products.
func (h *frameworkHandler) listProducts(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ViewAudits) {
		return
	}
	ctx := r.Context()
	scope, _ := deriveScopes(ctx)
	user := auth.FromContext(ctx)
	var userID int
	if user != nil {
		userID = user.UserID
	}
	products, err := h.svc.ListProducts(ctx, scope, userID, managedTeamIDs(auth.Grants(ctx)))
	if err != nil {
		response.MapServiceError(ctx, w, err, response.ErrMsgInternal)
		return
	}
	if products == nil {
		products = []*model.AuditProduct{}
	}
	response.WriteJSONValue(w, http.StatusOK, products)
}

// createProduct handles POST /api/v1/audits/products.
func (h *frameworkHandler) createProduct(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ManageFrameworks) {
		return
	}
	var req model.CreateProductRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}
	actor := auth.FromContext(r.Context()).Subject
	p, err := h.svc.CreateProduct(r.Context(), req, actor)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusCreated, p)
}

// listFrameworkControls handles
// GET /api/v1/audits/frameworks/controls?frameworkId={id}.
//
// frameworkId is a query param, not a path segment: a path segment here
// would collide with GET /api/v1/audits/{id}/controls/{controlId} — both are
// 3-segment patterns after /audits/ that alternate wildcard/literal in
// opposite positions, which net/http's ServeMux refuses to register as
// ambiguous (panics at startup, not a graceful fallback).
func (h *frameworkHandler) listFrameworkControls(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ViewAudits) {
		return
	}
	id, err := strconv.Atoi(r.URL.Query().Get("frameworkId"))
	if err != nil {
		response.WriteError(w, http.StatusBadRequest, "invalid frameworkId parameter")
		return
	}
	if !frameworkInScope(w, r, h.svc, id) {
		return
	}
	controls, err := h.svc.ListFrameworkControls(r.Context(), id)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if controls == nil {
		controls = []*model.AuditFrameworkControl{}
	}
	response.WriteJSONValue(w, http.StatusOK, model.FrameworkControlListResponse{
		Controls: controls,
		Total:    len(controls),
	})
}

// createFrameworkControl handles POST /api/v1/audits/frameworks/controls.
//
// frameworkId is a body field, not a path segment — see listFrameworkControls
// for why this route can't carry it in the path.
func (h *frameworkHandler) createFrameworkControl(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ManageFrameworks) {
		return
	}
	var req model.CreateFrameworkControlRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}
	actor := auth.FromContext(r.Context()).Subject
	fc, err := h.svc.CreateFrameworkControl(r.Context(), req.FrameworkID, req, actor)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusCreated, fc)
}
