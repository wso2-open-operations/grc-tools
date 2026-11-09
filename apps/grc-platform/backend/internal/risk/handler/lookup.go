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
	"context"
	"net/http"
	"strconv"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	riskservice "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/adminactivity"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// lookupKind is one register-template lookup served under
// /api/v1/risks/{path}. The four share one shape and one set of handlers.
type lookupKind struct {
	path       string // URL segment, e.g. "customers"
	svc        riskservice.LookupService
	entityType string // admin activity log entity type
}

func (d *Deps) lookupKinds() []lookupKind {
	return []lookupKind{
		{path: "platforms", svc: d.Platforms, entityType: adminactivity.EntityRiskPlatform},
		{path: "customers", svc: d.Customers, entityType: adminactivity.EntityRiskCustomer},
		{path: "products", svc: d.Products, entityType: adminactivity.EntityRiskProduct},
		{path: "deployment-types", svc: d.DeploymentTypes, entityType: adminactivity.EntityRiskDeploymentType},
	}
}

// handleListLookups serves GET /api/v1/risks/{lookup}?status=ACTIVE|INACTIVE.
// Readable by anyone who can view risks, not only admins: Add Risk's pickers
// ask for ACTIVE values, and the register table's filters need inactive ones
// too, because older risks still carry them.
func (d *Deps) handleListLookups(k lookupKind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !auth.RequireAnyPrivilege(r.Context(), w, privilege.ViewRisks, privilege.ManageRiskHub) {
			return
		}
		values, err := k.svc.List(r.Context(), r.URL.Query().Get("status"))
		if err != nil {
			response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
			return
		}
		if values == nil {
			values = []*model.Lookup{}
		}
		response.WriteJSONValue(w, http.StatusOK, values)
	}
}

// handleCreateLookup serves POST /api/v1/risks/{lookup}.
//
// Customers can also be added, with no approval, by anyone who can raise a
// Managed Services risk, so a missing customer never blocks Add Risk. Every
// other lookup stays with the platform admins.
func (d *Deps) handleCreateLookup(k lookupKind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !d.canCreateLookup(w, r, k) {
			return
		}
		var req model.CreateLookupRequest
		if err := response.DecodeJSON(w, r, &req); err != nil {
			return
		}
		if req.Name == "" {
			response.WriteError(w, http.StatusBadRequest, "name is required")
			return
		}
		createdBy := callerSubject(r)
		v, err := k.svc.Create(r.Context(), req, createdBy)
		if err != nil {
			response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
			return
		}
		d.ActivityLog.Log(r.Context(), createdBy, adminactivity.ActionCreated, k.entityType, v.ID, lookupDetails(v))
		response.WriteJSONValue(w, http.StatusCreated, v)
	}
}

// canCreateLookup answers 403 itself unless the caller may add a value of this
// kind: a platform admin for any kind, or someone who can raise Managed
// Services risks for a customer.
func (d *Deps) canCreateLookup(w http.ResponseWriter, r *http.Request, k lookupKind) bool {
	if auth.HasPrivilege(r.Context(), privilege.ManageRiskHub) {
		return true
	}
	if k.path == "customers" {
		canRaise, err := d.canCreateManagedServicesRisk(r.Context())
		if err != nil {
			response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
			return false
		}
		if canRaise {
			return true
		}
	}
	return auth.RequirePrivilege(r.Context(), w, privilege.ManageRiskHub)
}

// canCreateManagedServicesRisk reports whether the caller holds RISK_CREATE
// in at least one register on the Managed Services template.
func (d *Deps) canCreateManagedServicesRisk(ctx context.Context) (bool, error) {
	registers, err := d.Team.List(ctx, model.ListTeamsFilter{Type: "SOURCE_REGISTER"})
	if err != nil {
		return false, err
	}
	for _, t := range registers {
		if t.RegisterTemplate == model.TemplateManagedServices && auth.HasPrivilegeIn(ctx, privilege.CreateRisk, t.ID) {
			return true, nil
		}
	}
	return false, nil
}

// handleUpdateLookup serves PUT /api/v1/risks/{lookup}/{id}: rename,
// (de)activate, or — customers only, while unused — recode.
func (d *Deps) handleUpdateLookup(k lookupKind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !auth.RequirePrivilege(r.Context(), w, privilege.ManageRiskHub) {
			return
		}
		id, ok := lookupID(w, r)
		if !ok {
			return
		}
		var req model.UpdateLookupRequest
		if err := response.DecodeJSON(w, r, &req); err != nil {
			return
		}
		updatedBy := callerSubject(r)
		v, err := k.svc.Update(r.Context(), id, req, updatedBy)
		if err != nil {
			response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
			return
		}
		// A status change is logged as such, so the activity log reads
		// "deactivated" rather than a generic edit.
		action := adminactivity.ActionUpdated
		if req.Status != nil && req.Name == nil && req.Code == nil {
			action = adminactivity.ActionStatusChanged
		}
		d.ActivityLog.Log(r.Context(), updatedBy, action, k.entityType, id, lookupDetails(v))
		response.WriteJSONValue(w, http.StatusOK, v)
	}
}

// handleDeleteLookup serves DELETE /api/v1/risks/{lookup}/{id}. Only a value
// no risk has ever used can be deleted; the entity answers 409 otherwise.
func (d *Deps) handleDeleteLookup(k lookupKind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !auth.RequirePrivilege(r.Context(), w, privilege.ManageRiskHub) {
			return
		}
		id, ok := lookupID(w, r)
		if !ok {
			return
		}
		// Read the name first: once deleted, the log entry is all that is
		// left to say what it was.
		var details map[string]any
		if values, listErr := k.svc.List(r.Context(), ""); listErr == nil {
			for _, v := range values {
				if v.ID == id {
					details = lookupDetails(v)
				}
			}
		}
		if err := k.svc.Delete(r.Context(), id); err != nil {
			response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
			return
		}
		d.ActivityLog.Log(r.Context(), callerSubject(r), adminactivity.ActionDeleted, k.entityType, id, details)
		w.WriteHeader(http.StatusNoContent)
	}
}

func lookupDetails(v *model.Lookup) map[string]any {
	details := map[string]any{"name": v.Name, "status": v.Status}
	if v.Code != nil {
		details["code"] = *v.Code
	}
	if v.ParentID != nil {
		details["parentId"] = *v.ParentID
	}
	return details
}

// callerSubject is the authenticated caller's subject, or "" when there is
// none, matching the createdBy/updatedBy convention of the other admin
// handlers.
func callerSubject(r *http.Request) string {
	if user := auth.FromContext(r.Context()); user != nil {
		return user.Subject
	}
	return ""
}

// lookupID parses the {id} path value, answering 400 itself when it isn't a
// positive integer.
func lookupID(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		response.WriteError(w, http.StatusBadRequest, "id must be a positive integer")
		return 0, false
	}
	return id, true
}
