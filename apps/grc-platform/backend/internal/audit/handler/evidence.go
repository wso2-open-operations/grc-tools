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
	"io"
	"net/http"
	"path/filepath"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// maxEvidenceUploadBytes caps a single proxied evidence upload (bytes travel
// through the backend, so bound them to protect memory and the gateway).
const maxEvidenceUploadBytes = 25 << 20 // 25 MiB

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

// uploadEvidence handles POST /api/v1/audits/{id}/controls/{controlId}/evidence/upload.
//
// The client sends the file as multipart/form-data (fields: folderPath, file).
// The backend validates size/type and proxies the bytes to Azure using its own
// account key — no SAS is ever handed to the client, so the byte transfer stays
// client -> backend (Untrust -> Trust) then backend -> Azure (Trust -> Untrust).
func (h *evidenceHandler) uploadEvidence(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.SubmitEvidence) {
		return
	}

	// Bound the request body before parsing to protect memory and the gateway.
	r.Body = http.MaxBytesReader(w, r.Body, maxEvidenceUploadBytes)
	if err := r.ParseMultipartForm(maxEvidenceUploadBytes); err != nil {
		response.WriteError(w, http.StatusRequestEntityTooLarge, "file too large or malformed upload (max 25 MB)")
		return
	}

	folderPath := r.FormValue("folderPath")
	if folderPath == "" {
		response.WriteError(w, http.StatusBadRequest, "folderPath is required")
		return
	}

	f, header, err := r.FormFile("file")
	if err != nil {
		response.WriteError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		response.WriteError(w, http.StatusBadRequest, "could not read uploaded file")
		return
	}

	// Resolve content type from the part header, sniffing the bytes as a fallback
	// rather than blindly trusting the client-declared type.
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}

	// Strip any client-supplied path; keep only the base file name.
	fileName := filepath.Base(header.Filename)

	if err := h.svc.UploadFile(r.Context(), folderPath, fileName, contentType, data); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}

	response.WriteJSONValue(w, http.StatusCreated, map[string]any{
		"fileName": fileName,
		"size":     len(data),
	})
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
