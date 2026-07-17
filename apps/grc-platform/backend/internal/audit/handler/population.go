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

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// Web-app population submission routes. These mirror the evidence submission
// flow (upload-link → upload → submit) but write POPULATION files against the
// control's active population round, then advance the control to
// POPULATION_INTERNAL_REVIEW. The Evidence Portal has its own equivalents under
// /api/v1/evidence-app (see evidence_app.go).

// activePopulationID resolves the active population round for an OE control.
// Writes 409 and returns ok=false when there is none (e.g. DESIGN control).
func (h *evidenceHandler) activePopulationID(w http.ResponseWriter, r *http.Request, controlID int) (int, bool) {
	populationID, found, err := h.controlSvc.ActivePopulationID(r.Context(), controlID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return 0, false
	}
	if !found {
		response.WriteError(w, http.StatusConflict, "this control has no active population phase; use the evidence endpoints")
		return 0, false
	}
	return populationID, true
}

// getPopulationUploadLink handles
// GET /api/v1/audits/{id}/controls/{controlId}/population/upload-link.
func (h *evidenceHandler) getPopulationUploadLink(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.SubmitEvidence) {
		return
	}
	auditID, ok := parseIntParam(w, r, "id")
	if !ok {
		return
	}
	controlID, ok := parseIntParam(w, r, "controlId")
	if !ok {
		return
	}
	if !h.requireAssignment(w, r, auditID, controlID) {
		return
	}
	populationID, ok := h.activePopulationID(w, r, controlID)
	if !ok {
		return
	}
	link := h.svc.PopulationUploadLink(auditID, controlID, populationID)
	response.WriteJSONValue(w, http.StatusOK, link)
}

// uploadPopulation handles
// POST /api/v1/audits/{id}/controls/{controlId}/population/upload.
//
// Like uploadEvidence, the client sends multipart/form-data (folderPath, file)
// and the backend proxies the bytes to Azure — no SAS reaches the client.
func (h *evidenceHandler) uploadPopulation(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.SubmitEvidence) {
		return
	}
	auditID, ok := parseIntParam(w, r, "id")
	if !ok {
		return
	}
	controlID, ok := parseIntParam(w, r, "controlId")
	if !ok {
		return
	}
	if !h.requireAssignment(w, r, auditID, controlID) {
		return
	}
	populationID, ok := h.activePopulationID(w, r, controlID)
	if !ok {
		return
	}
	folderPath, fileName, contentType, data, ok := readUpload(w, r)
	if !ok {
		return
	}
	if err := service.ValidatePopulationFolderPath(folderPath, auditID, controlID, populationID); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if err := h.svc.UploadFile(r.Context(), folderPath, fileName, contentType, data); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusCreated, map[string]any{"fileName": fileName, "size": len(data)})
}

// submitPopulation handles
// POST /api/v1/audits/{id}/controls/{controlId}/population/submit.
//
// Records every blob at folderPath as a POPULATION file on the active round and
// advances the control to POPULATION_INTERNAL_REVIEW.
func (h *evidenceHandler) submitPopulation(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.SubmitEvidence) {
		return
	}
	auditID, ok := parseIntParam(w, r, "id")
	if !ok {
		return
	}
	controlID, ok := parseIntParam(w, r, "controlId")
	if !ok {
		return
	}
	if !h.requireAssignment(w, r, auditID, controlID) {
		return
	}
	populationID, ok := h.activePopulationID(w, r, controlID)
	if !ok {
		return
	}
	var req model.SubmitEvidenceRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}
	if err := service.ValidatePopulationFolderPath(req.FolderPath, auditID, controlID, populationID); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}

	user := auth.FromContext(r.Context())
	actor := user.Email

	result, err := h.popSvc.SubmitPopulation(r.Context(), controlID, populationID, req.FolderPath, actor)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	statusReq := model.UpdateStatusRequest{Status: "POPULATION_INTERNAL_REVIEW"}
	if err := h.controlSvc.UpdateStatus(r.Context(), auditID, controlID, statusReq, actor); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}

	// Best-effort audit-trail attribution: this submission came through the web app.
	recordEvidenceTrail(r.Context(), h.trailSvc, auditID, controlID, 0, actor, channelWebApp, user.Issuer)

	response.WriteJSONValue(w, http.StatusCreated, result)
}
