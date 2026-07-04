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

type evidenceHandler struct {
	svc        service.EvidenceService
	controlSvc service.ControlService
}

// getAssignedControls handles GET /api/v1/evidence-app/controls.
//
// Returns all controls the authenticated user's team needs to act on
// (status EVIDENCE_PENDING, EVIDENCE_NEED_CLARIFICATION, SUBMITTED_SAMPLE)
// across all active audits, each with its Azure Blob base folder path.
func (h *evidenceHandler) getAssignedControls(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.SubmitEvidence) {
		return
	}
	actor := auth.FromContext(r.Context())
	controls, err := h.controlSvc.GetAssignedForEvidence(r.Context(), actor.Email)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if controls == nil {
		controls = []*model.AssignedControlForEvidence{}
	}
	response.WriteJSONValue(w, http.StatusOK, controls)
}

// getUploadLink handles GET /api/v1/audits/{id}/controls/{controlId}/evidence/upload-link.
//
// Returns a SAS token + folder path the evidence capture agent uses to upload
// files directly to Azure Blob Storage. No file bytes pass through the backend.
func (h *evidenceHandler) getUploadLink(w http.ResponseWriter, r *http.Request) {
	auditID, ok := parseIntParam(w, r, "id")
	if !ok {
		return
	}
	controlID, ok := parseIntParam(w, r, "controlId")
	if !ok {
		return
	}

	if !auth.RequirePrivilege(r.Context(), w, privilege.SubmitEvidence) {
		return
	}

	link, err := h.svc.GetUploadLink(r.Context(), auditID, controlID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusOK, link)
}

// getFileUploadURL handles POST /api/v1/audits/{id}/controls/{controlId}/evidence/file-url.
//
// Returns a blob-scoped SAS URL valid for 30 minutes, scoped to exactly the
// one blob named {folderPath}{fileName}. The agent PUTs the file directly to
// that URL with header x-ms-blob-type: BlockBlob.
func (h *evidenceHandler) getFileUploadURL(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.SubmitEvidence) {
		return
	}
	var req model.FileUploadURLRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}
	if req.FileName == "" || req.FolderPath == "" {
		response.WriteError(w, http.StatusBadRequest, "fileName and folderPath are required")
		return
	}
	result, err := h.svc.GetFileUploadURL(r.Context(), req.FolderPath, req.FileName)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusOK, result)
}

// submitEvidence handles POST /api/v1/audits/{id}/controls/{controlId}/evidence/submit.
//
// The agent has already uploaded files to Azure using the SAS token from
// getUploadLink. This endpoint discovers those blobs, records them in the DB,
// and advances the control status to EVIDENCE_INTERNAL_REVIEW.
func (h *evidenceHandler) submitEvidence(w http.ResponseWriter, r *http.Request) {
	auditID, ok := parseIntParam(w, r, "id")
	if !ok {
		return
	}
	controlID, ok := parseIntParam(w, r, "controlId")
	if !ok {
		return
	}

	if !auth.RequirePrivilege(r.Context(), w, privilege.SubmitEvidence) {
		return
	}

	var req model.SubmitEvidenceRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}

	actor := auth.FromContext(r.Context()).Email

	evidence, err := h.svc.Submit(r.Context(), controlID, req.FolderPath, actor)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}

	// Advance the control to EVIDENCE_INTERNAL_REVIEW now that files are recorded.
	statusReq := model.UpdateStatusRequest{Status: "EVIDENCE_INTERNAL_REVIEW"}
	if err := h.controlSvc.UpdateStatus(r.Context(), auditID, controlID, statusReq, actor); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}

	response.WriteJSONValue(w, http.StatusCreated, evidence)
}

// listEvidence handles GET /api/v1/audits/{id}/controls/{controlId}/evidence.
func (h *evidenceHandler) listEvidence(w http.ResponseWriter, r *http.Request) {
	_, ok := parseIntParam(w, r, "id")
	if !ok {
		return
	}
	controlID, ok := parseIntParam(w, r, "controlId")
	if !ok {
		return
	}

	if !auth.RequirePrivilege(r.Context(), w, privilege.ReviewEvidence) {
		return
	}

	evidence, err := h.svc.List(r.Context(), controlID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if evidence == nil {
		evidence = []*model.AuditEvidence{}
	}
	response.WriteJSONValue(w, http.StatusOK, evidence)
}
