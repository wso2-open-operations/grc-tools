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
	"slices"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

type aiValidationHandler struct {
	svc         service.AIValidationService
	evidenceSvc service.EvidenceService
	controlSvc  service.ControlService
	popSvc      service.PopulationService
}

// listValidations handles
// GET /api/v1/audits/{id}/controls/{controlId}/evidence/{evidenceId}/ai-validations.
//
// Advisory review hints, internal-only — an external auditor gets no AI
// validation data at all, for any evidence or population, in any
// state, including SKIPPED. This replaces the previous partial visibility
// (an external auditor could see AI validation for evidence tied to their
// own assigned control); the checkbox that can produce a SKIPPED row is
// itself internal-only, so nothing external is lost by this tightening.
//
// The route carries only a bare evidenceId (no team/control context), so —
// same as requireEvidenceFileAccess for file downloads — the owning
// control's team must be resolved first and the privileges checked scoped to
// that team (HasPrivilegeIn), not the unscoped HasPrivilege/
// RequireAnyPrivilege: otherwise a grant scoped to one team would let its
// holder read every other team's AI validation results by evidenceId.
func (h *aiValidationHandler) listValidations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	evidenceID, ok := parseIntParam(w, r, "evidenceId")
	if !ok {
		return
	}
	_, evidenceTeamID, _, err := h.evidenceSvc.EvidenceAuditorID(ctx, evidenceID)
	if err != nil {
		response.MapServiceError(ctx, w, err, response.ErrMsgInternal)
		return
	}
	teamID := 0
	if evidenceTeamID != nil {
		teamID = *evidenceTeamID
	}
	if !internalAIValidationViewer(ctx, teamID) {
		response.WriteError(w, http.StatusForbidden, response.ErrMsgForbidden)
		return
	}
	validations, err := h.svc.ListByEvidence(ctx, evidenceID)
	if err != nil {
		response.MapServiceError(ctx, w, err, response.ErrMsgInternal)
		return
	}
	if validations == nil {
		validations = []*model.AIValidationLog{}
	}
	response.WriteJSONValue(w, http.StatusOK, &model.AIValidationListResponse{Validations: validations})
}

// listPopulationValidations handles
// GET /api/v1/audits/{id}/controls/{controlId}/population/{populationId}/ai-validations.
// Internal-only, same rule as listValidations above.
func (h *aiValidationHandler) listPopulationValidations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	auditID, ok := parseIntParam(w, r, "id")
	if !ok {
		return
	}
	controlID, ok := parseIntParam(w, r, "controlId")
	if !ok {
		return
	}
	populationID, ok := parseIntParam(w, r, "populationId")
	if !ok {
		return
	}
	control, err := h.controlSvc.GetByID(ctx, auditID, controlID)
	if err != nil {
		response.MapServiceError(ctx, w, err, response.ErrMsgInternal)
		return
	}
	if control == nil {
		response.WriteError(w, http.StatusNotFound, response.ErrMsgNotFound)
		return
	}
	if !internalEvidenceViewer(r, control) {
		response.WriteError(w, http.StatusForbidden, response.ErrMsgForbidden)
		return
	}
	// The privilege check above is against this control's team, so the
	// population must belong to this control — otherwise any populationId
	// would be readable through a control the caller can see.
	rounds, err := h.popSvc.ListRounds(ctx, auditID, controlID)
	if err != nil {
		response.MapServiceError(ctx, w, err, response.ErrMsgInternal)
		return
	}
	if !slices.ContainsFunc(rounds, func(p *model.AuditPopulation) bool { return p.ID == populationID }) {
		response.WriteError(w, http.StatusNotFound, response.ErrMsgNotFound)
		return
	}
	validations, err := h.svc.ListByPopulation(ctx, populationID)
	if err != nil {
		response.MapServiceError(ctx, w, err, response.ErrMsgInternal)
		return
	}
	if validations == nil {
		validations = []*model.AIValidationLog{}
	}
	response.WriteJSONValue(w, http.StatusOK, &model.AIValidationListResponse{Validations: validations})
}

// internalAIValidationViewer is internalEvidenceViewer's team-scoped
// privilege check, usable when only a team id is in hand (no *model.AuditControl) —
// the evidence AI-validation route only ever resolves a team id from
// evidenceId, never the full control.
func internalAIValidationViewer(ctx context.Context, teamID int) bool {
	return auth.HasPrivilegeIn(ctx, privilege.ManageControls, teamID) ||
		auth.HasPrivilegeIn(ctx, privilege.SubmitEvidence, teamID) ||
		auth.HasPrivilegeIn(ctx, privilege.ReviewEvidence, teamID) ||
		auth.HasPrivilegeIn(ctx, privilege.ViewAllAudits, teamID)
}
