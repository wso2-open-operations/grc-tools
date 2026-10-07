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
	"log/slog"
	"net/http"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// handleAssessRisk serves POST /api/v1/risks/{id}/assess.
// Records a residual risk assessment (likelihood, impact, progress, reassessment_date)
// stored in the risk_assessment table. This is separate from "Submit for Approval".
func (d *Deps) handleAssessRisk(w http.ResponseWriter, r *http.Request) {
	by, ok := requireCallerUUID(w, r)
	if !ok {
		return
	}
	id, ok := parseRiskID(w, r)
	if !ok {
		return
	}
	// Two layers. The scoped privilege check is the grant half: RISK_ASSESS is
	// held by grc-platform-risk-assigner (scope_basis SOURCE_REGISTER — they
	// raised it) AND grc-platform-risk-owner (scope_basis ASSIGNMENT_TEAM — it
	// was routed to them), so either alone must be enough. See
	// auth.HasPrivilegeInEither.
	//
	// The identity half then narrows it to the people this risk names: only its
	// Risk Owner and its Risk Assigner (or a compliance admin) may assess it,
	// and only the assigner may use the assessment to change the owner,
	// approver, implementation date, assigner or action owner.
	detail, err := d.Risk.GetByID(r.Context(), id)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if !auth.RequirePrivilegeInEither(r.Context(), w, privilege.AssessRisk, detail.SourceRegisterID, detail.AssignmentTeamID) {
		return
	}
	isOwner, isAssigner, ok := d.callerRiskRoles(w, r, detail)
	if !ok {
		return
	}
	if !isOwner && !isAssigner {
		response.WriteError(w, http.StatusForbidden, "only this risk's Risk Owner or Risk Assigner may assess it")
		return
	}

	var req model.CreateAssessmentRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}
	// A field that matches what the risk already holds is not a change: the
	// form may send it back untouched, and an owner must not be refused for it.
	dropUnchangedRiskFields(&req, detail)
	if (req.OwnerID != nil || req.ManagementApproverID != nil || req.ImplementationDate != nil ||
		req.AssignerID != nil || req.ActionOwnerID != nil) && !isAssigner {
		response.WriteError(w, http.StatusForbidden, "only this risk's Risk Assigner may change its owner, approver, implementation date, assigner or action owner")
		return
	}
	// The entity writes an Action Owner onto the risk's own plan and silently
	// writes nothing when there is none — refuse rather than report a change
	// that never happened.
	if req.ActionOwnerID != nil && detail.ActionPlan == nil {
		response.WriteError(w, http.StatusConflict, "this risk has no action plan to set an Action Owner on")
		return
	}
	req.Current = detail

	result, err := d.Assessment.Create(r.Context(), id, req, by)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}

	// Advisory only — the assessment is already saved successfully at this
	// point, so a failure here is logged, never returned to the caller as an
	// error. Residual, not Gross: this is the reassessment endpoint.
	if err := d.LikelihoodSuggestion.RecordDecision(r.Context(), id, req.AILikelihoodSuggestion, req.Likelihood, by); err != nil {
		slog.Error("record likelihood suggestion decision", "risk_id", id, "err", err)
	}

	// Progress carries the assessor's notes into the timeline itself — the
	// reassessment's own row is the source of truth, but the dedicated
	// Assessment History view that used to read it directly was replaced by
	// this unified timeline, so the notes need to travel here too or they're
	// fetched and never shown anywhere. Level/PreviousLevel already give the
	// score's before/after, and CreatedBy (set by recordEvent below) already
	// gives who — Comment is the one piece genuinely missing.
	details := model.HistoryDetails{Level: result.ResidualLevel, Comment: result.Progress}
	if prev := previousLevel(r.Context(), d, id, result.ID); prev != "" {
		details.PreviousLevel = prev
	}
	d.recordEvent(r.Context(), id, by, model.HistoryAssess, details)
	d.notifyRiskAssessed(detail, result, by)
	response.WriteJSONValue(w, http.StatusCreated, result)
}

// previousLevel returns the residual level immediately before assessmentID, so
// a reassessment can render as "HIGH → MEDIUM" rather than just its new level.
// Returns "" when this is the first reassessment or the lookup fails — the
// history entry then simply shows the new level on its own.
func previousLevel(ctx context.Context, d *Deps, riskID, assessmentID int) string {
	prior, err := d.Assessment.ListByRiskID(ctx, riskID)
	if err != nil {
		return ""
	}
	// ListByRiskID is newest-first and already includes the one just created,
	// so the previous level is the next entry along.
	for i, a := range prior {
		if a.ID == assessmentID && i+1 < len(prior) {
			return prior[i+1].ResidualLevel
		}
	}
	return ""
}

// callerRiskRoles reports whether the caller is the risk's owner and/or its
// assigner. A compliance admin in the risk's source register counts as both,
// the same override requireRiskActor applies. A caller with no platform user
// row is neither. ok is false once an error response has been written.
func (d *Deps) callerRiskRoles(w http.ResponseWriter, r *http.Request, detail *model.RiskDetail) (isOwner, isAssigner, ok bool) {
	if canOverrideAssigneeIn(r.Context(), detail.SourceRegisterID) {
		return true, true, true
	}
	callerID, err := d.callerUserID(r.Context())
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return false, false, false
	}
	if callerID == nil {
		return false, false, true
	}
	return *callerID == detail.OwnerID, *callerID == detail.AssignerID, true
}

// dropUnchangedRiskFields nils any owner/approver/implementation-date/assigner/action-owner field
// that equals the risk's current value.
func dropUnchangedRiskFields(req *model.CreateAssessmentRequest, current *model.RiskDetail) {
	if req.OwnerID != nil && *req.OwnerID == current.OwnerID {
		req.OwnerID = nil
	}
	if req.ManagementApproverID != nil && *req.ManagementApproverID == current.ManagementApproverID {
		req.ManagementApproverID = nil
	}
	if req.AssignerID != nil && *req.AssignerID == current.AssignerID {
		req.AssignerID = nil
	}
	if req.ActionOwnerID != nil && current.ActionPlan != nil &&
		current.ActionPlan.ActionOwnerID != nil && *req.ActionOwnerID == *current.ActionPlan.ActionOwnerID {
		req.ActionOwnerID = nil
	}
	if req.ImplementationDate != nil && current.ImplementationDate != nil && model.SameCalendarDate(*req.ImplementationDate, *current.ImplementationDate) {
		req.ImplementationDate = nil
	}
}
