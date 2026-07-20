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
	"io"
	"log/slog"
	"mime"
	"net/http"
	"path/filepath"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/aiagent"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// maxEvidenceUploadBytes caps a single proxied evidence upload (bytes travel
// through the backend, so bound them to protect memory and the gateway).
const maxEvidenceUploadBytes = 25 << 20 // 25 MiB

type evidenceHandler struct {
	svc        service.EvidenceService
	controlSvc service.ControlService
	// popSvc records population submissions (web-app population routes).
	popSvc service.PopulationService
	// trailSvc records best-effort attribution entries on submit. May be nil.
	trailSvc service.TrailService
	// aiClient triggers async AI validation after a submission. It is nil when
	// AI_VALIDATION_ENABLED is false, which disables the trigger entirely.
	aiClient *aiagent.Client
}

// requireAssignment enforces resource-level authorization for the web-app evidence
// routes (design §F/§G): the caller must be assigned to controlID for an actionable
// status (else 403), and the route's audit id must equal the server-derived audit
// id (else 404 — a client cannot aim at another audit's control). It returns the
// derived audit id and ok=false after writing the response on failure.
//
// Users who hold ManageControls (compliance admin) bypass the team-assignment
// check — they already have full read/write over all audit data, so the IDOR
// restriction is redundant and would block legitimate admin submissions.
func (h *evidenceHandler) requireAssignment(w http.ResponseWriter, r *http.Request, auditID, controlID int) bool {
	if auth.HasPrivilege(r.Context(), privilege.ManageControls) {
		return true
	}
	actor := auth.FromContext(r.Context())
	derived, found, err := h.controlSvc.AssignedAuditID(r.Context(), actor.Email, controlID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return false
	}
	if !found {
		response.WriteError(w, http.StatusForbidden, response.ErrMsgForbidden)
		return false
	}
	if derived != auditID {
		response.WriteError(w, http.StatusNotFound, response.ErrMsgNotFound)
		return false
	}
	return true
}

// getUploadLink handles GET /api/v1/audits/{id}/controls/{controlId}/evidence/upload-link.
//
// Returns a SAS token + folder path the evidence capture agent uses to upload
// files directly to Azure Blob Storage. No file bytes pass through the backend.
func (h *evidenceHandler) getUploadLink(w http.ResponseWriter, r *http.Request) {
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

	// Bound the request body before parsing to protect memory and the gateway.
	r.Body = http.MaxBytesReader(w, r.Body, maxEvidenceUploadBytes)
	if err := r.ParseMultipartForm(maxEvidenceUploadBytes); err != nil { // #nosec G120 -- body already bounded by MaxBytesReader above
		response.WriteError(w, http.StatusRequestEntityTooLarge, "file too large or malformed upload (max 25 MB)")
		return
	}

	folderPath := r.FormValue("folderPath")
	// Bind the path exactly to this control's evidence folder (auditID is the
	// server-derived value; the session segment must be digits-only).
	if err := service.ValidateEvidenceFolderPath(folderPath, auditID, controlID); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
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

	var req model.SubmitEvidenceRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}
	if err := service.ValidateEvidenceFolderPath(req.FolderPath, auditID, controlID); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}

	user := auth.FromContext(r.Context())
	actor := user.Email

	evidence, err := h.svc.Submit(r.Context(), auditID, controlID, req.FolderPath, actor)
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

	// Best-effort audit-trail attribution: this submission came through the web app.
	recordEvidenceTrail(r.Context(), h.trailSvc, auditID, controlID, evidence.ID, actor, channelWebApp, user.Issuer)

	// Fire-and-forget AI validation. Detached from the request context (a client
	// disconnect must not cancel it) and best-effort — a failure here never
	// affects the submission the user just made.
	h.triggerAIValidation(auditID, controlID, evidence.ID, actor)

	response.WriteJSONValue(w, http.StatusCreated, evidence)
}

// withdrawEvidence handles POST /api/v1/audits/{id}/controls/{controlId}/evidence/withdraw.
//
// Lets the submitter pull a submission back while it is still in internal
// review (EVIDENCE_INTERNAL_REVIEW → EVIDENCE_PENDING) so files can be edited
// and resubmitted. Only the creator of the latest submission round (or a user
// holding ManageControls) may withdraw; once review has moved past internal
// review the submission is locked.
func (h *evidenceHandler) withdrawEvidence(w http.ResponseWriter, r *http.Request) {
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

	control, err := h.controlSvc.GetByID(r.Context(), auditID, controlID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if control == nil {
		response.WriteError(w, http.StatusNotFound, response.ErrMsgNotFound)
		return
	}
	if control.Status != "EVIDENCE_INTERNAL_REVIEW" {
		response.WriteError(w, http.StatusConflict, "submission can only be withdrawn while it is under internal review")
		return
	}

	actor := auth.FromContext(r.Context()).Email

	// Resource-level check: the caller must own the latest submission round.
	// ManageControls holders (compliance admin) can withdraw any submission.
	if !auth.HasPrivilege(r.Context(), privilege.ManageControls) {
		evidence, err := h.svc.List(r.Context(), auditID, controlID)
		if err != nil {
			response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
			return
		}
		if len(evidence) == 0 || evidence[0].CreatedBy != actor {
			response.WriteError(w, http.StatusForbidden, response.ErrMsgForbidden)
			return
		}
	}

	statusReq := model.UpdateStatusRequest{Status: "EVIDENCE_PENDING"}
	if err := h.controlSvc.UpdateStatus(r.Context(), auditID, controlID, statusReq, actor); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusOK, map[string]any{"status": "EVIDENCE_PENDING"})
}

// triggerAIValidation kicks off an advisory AI validation in the background.
// No-op when the AI agent client is not configured (AI_VALIDATION_ENABLED=false).
func (h *evidenceHandler) triggerAIValidation(auditID, controlID, evidenceID int, actor string) {
	triggerAIValidation(h.aiClient, auditID, controlID, evidenceID, actor)
}

// triggerAIValidation kicks off an advisory AI validation, detached from the
// request context so a client disconnect cannot cancel it. Best-effort and a
// no-op when the AI agent client is nil (AI_VALIDATION_ENABLED=false). Shared by
// the web-app and evidence-app submit paths.
func triggerAIValidation(aiClient *aiagent.Client, auditID, controlID, evidenceID int, actor string) {
	if aiClient == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := aiClient.Trigger(ctx, aiagent.TriggerRequest{
			Task:        "validate_evidence",
			Scope:       aiagent.Scope{AuditID: auditID, ControlID: controlID, EvidenceID: evidenceID},
			RequestedBy: actor,
		})
		if err != nil {
			slog.Warn("ai validation trigger failed", "evidenceId", evidenceID, "err", err)
		}
	}()
}

// deleteEvidenceFile handles DELETE /api/v1/evidence/files/{fileId}.
//
// Removes a single file from an evidence submission (DB record only; the blob
// in Azure is not deleted). The caller must be the file's original uploader or
// hold ManageControls.
func (h *evidenceHandler) deleteEvidenceFile(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.SubmitEvidence) {
		return
	}
	fileID, ok := parseIntParam(w, r, "fileId")
	if !ok {
		return
	}
	actor := auth.FromContext(r.Context()).Email
	isAdmin := auth.HasPrivilege(r.Context(), privilege.ManageControls)
	if err := h.svc.DeleteFile(r.Context(), fileID, actor, isAdmin); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// downloadEvidenceFile handles GET /api/v1/evidence/files/{fileId}/download.
// It proxies the file bytes from the Compliance Entity (which reads them from
// Azure) so the browser never contacts Azure directly.
func (h *evidenceHandler) downloadEvidenceFile(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ReviewEvidence) {
		return
	}
	fileID, ok := parseIntParam(w, r, "fileId")
	if !ok {
		return
	}
	data, fileName, contentType, err := h.svc.DownloadFile(r.Context(), fileID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": fileName})
	if disposition == "" {
		disposition = `attachment; filename="file"`
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", disposition)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data) // #nosec G705 -- file served with nosniff + attachment disposition, browser won't execute it inline
}

// listEvidence handles GET /api/v1/audits/{id}/controls/{controlId}/evidence.
func (h *evidenceHandler) listEvidence(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ReviewEvidence) {
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

	evidence, err := h.svc.List(r.Context(), auditID, controlID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if evidence == nil {
		evidence = []*model.AuditEvidence{}
	}
	response.WriteJSONValue(w, http.StatusOK, evidence)
}
