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
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/csvexport"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

const (
	// controlExportMaxRows bounds one export. An audit holds tens to a few
	// hundred controls (bulk add stops at 500), so this is far above any real
	// audit; it exists so a runaway result fails loudly instead of producing a
	// file that silently stops short.
	controlExportMaxRows = 5000
	// controlExportWriteTimeout replaces the server-wide write deadline for one
	// export.
	controlExportWriteTimeout = 2 * time.Minute
)

// controlExportHeader is the CSV header row; controlExportRecord emits the same
// columns in the same order. The columns are fixed: the web app's Columns
// picker is a per-browser preference the server never sees.
var controlExportHeader = []string{
	"Control No.", "Description", "Evidence Requirement", "Req. Type", "Control Type",
	"Status", "Overdue", "Scope", "Team", "Process Owner", "Auditor POC", "Due Date",
	"Population Due Date", "Population Owner", "Population Team",
	"Current Holder", "Current Deadline", "Planned Owner Date", "Planned Compliance Date", "Planned Auditor Date",
	"Delayed", "Delay Log",
}

// controlExportParams is what the audit detail page is currently showing:
// either one of its tabs, or the search box plus the column filters. Multi-value
// fields arrive as repeated query params (?team=A&team=B) rather than a
// comma-separated list, because team and person names can contain commas.
type controlExportParams struct {
	Tab              string // "" (All) | allPending | approved | inProgress | overdue
	Search           string
	Statuses         []string // control status values, or the pseudo-value OVERDUE
	RequirementTypes []string
	ControlTypes     []string
	Scopes           []string
	Teams            []string // display names, as the table filters on them
	Auditors         []string
	Owners           []string
}

var controlExportTabs = map[string]bool{"": true, "allPending": true, "approved": true, "inProgress": true, "overdue": true}

func parseControlExportParams(q url.Values) (controlExportParams, bool) {
	p := controlExportParams{
		Tab:              q.Get("tab"),
		Search:           q.Get("search"),
		Statuses:         q["status"],
		RequirementTypes: q["requirement_type"],
		ControlTypes:     q["control_type"],
		Scopes:           q["scope"],
		Teams:            q["team"],
		Auditors:         q["auditor"],
		Owners:           q["owner"],
	}
	return p, controlExportTabs[p.Tab]
}

// filterControls selects what the audit detail page would show for p. It mirrors
// the page's own filtering (utils/controlFilters.ts in the web app): a tab takes
// precedence and ignores the search and column filters, exactly as the page
// does; otherwise the search is a case-insensitive match on control number and
// description, and the column filters AND across fields and OR within one.
func filterControls(controls []*model.AuditControl, p controlExportParams) []*model.AuditControl {
	out := make([]*model.AuditControl, 0, len(controls))
	q := strings.ToLower(strings.TrimSpace(p.Search))
	for _, c := range controls {
		if p.Tab != "" {
			if inControlTab(c, p.Tab) {
				out = append(out, c)
			}
			continue
		}
		if q != "" &&
			!strings.Contains(strings.ToLower(c.ControlNumber), q) &&
			!strings.Contains(strings.ToLower(c.Description), q) {
			continue
		}
		if len(p.Statuses) > 0 && !matchesStatusFilter(c, p.Statuses) {
			continue
		}
		if len(p.RequirementTypes) > 0 && !slices.Contains(p.RequirementTypes, c.RequirementType) {
			continue
		}
		if len(p.ControlTypes) > 0 && !slices.Contains(p.ControlTypes, c.ControlType) {
			continue
		}
		if len(p.Scopes) > 0 && !slices.Contains(p.Scopes, c.Scope) {
			continue
		}
		if len(p.Teams) > 0 && !slices.Contains(p.Teams, csvexport.Deref(c.TeamName)) {
			continue
		}
		if len(p.Auditors) > 0 && !slices.Contains(p.Auditors, csvexport.Deref(c.AuditorName)) {
			continue
		}
		if len(p.Owners) > 0 && !slices.Contains(p.Owners, csvexport.Deref(c.OwnerName)) {
			continue
		}
		out = append(out, c)
	}
	return out
}

func inControlTab(c *model.AuditControl, tab string) bool {
	switch tab {
	case "approved":
		return c.Status == "COMPLETE"
	case "overdue":
		return c.IsOverdue
	case "allPending":
		return c.Status != "COMPLETE"
	case "inProgress":
		return c.Status != "COMPLETE" && !c.IsOverdue
	}
	return false
}

// matchesStatusFilter: the Status column filter accepts the pseudo-value
// OVERDUE alongside real statuses, and selecting both matches either.
func matchesStatusFilter(c *model.AuditControl, statuses []string) bool {
	for _, s := range statuses {
		if s == "OVERDUE" {
			if c.IsOverdue {
				return true
			}
		} else if s == c.Status {
			return true
		}
	}
	return false
}

// Display labels, matching what the table shows.
var (
	controlStatusLabels = map[string]string{
		"POPULATION_PENDING":            "Population Pending",
		"POPULATION_INTERNAL_REVIEW":    "Population Internal Review",
		"POPULATION_UNDER_VALIDATION":   "Population Under Validation",
		"POPULATION_NEED_CLARIFICATION": "Population Need Clarification",
		"POPULATION_COMPLETE":           "Population Complete",
		"AWAITING_SAMPLE":               "Awaiting Sample",
		"SUBMITTED_SAMPLE":              "Submitted Sample",
		"EVIDENCE_PENDING":              "Evidence Pending",
		"EVIDENCE_INTERNAL_REVIEW":      "Evidence Internal Review",
		"EVIDENCE_UNDER_VALIDATION":     "Evidence Under Validation",
		"EVIDENCE_NEED_CLARIFICATION":   "Evidence Need Clarification",
		"COMPLETE":                      "Complete",
	}
	requirementTypeLabels = map[string]string{"DESIGN": "Design", "OE": "OE"}
	controlTypeLabels     = map[string]string{"CONFIG": "Config", "NON_CONFIG": "Non-Config"}
	scopeLabels           = map[string]string{"COMMON": "Common", "PRODUCT_SPECIFIC": "Product Specific"}
)

// label returns the display label for v, or v itself when it has none.
func label(labels map[string]string, v string) string {
	if l, ok := labels[v]; ok {
		return l
	}
	return v
}

func controlExportRecord(c *model.AuditControl) []string {
	overdue := "No"
	if c.IsOverdue {
		overdue = "Yes"
	}
	return []string{
		csvexport.SafeCell(c.ControlNumber),
		csvexport.SafeCell(c.Description),
		csvexport.SafeCell(csvexport.Deref(c.EvidenceRequirement)),
		csvexport.SafeCell(label(requirementTypeLabels, c.RequirementType)),
		csvexport.SafeCell(label(controlTypeLabels, c.ControlType)),
		csvexport.SafeCell(label(controlStatusLabels, c.Status)),
		overdue,
		csvexport.SafeCell(label(scopeLabels, c.Scope)),
		csvexport.SafeCell(csvexport.Deref(c.TeamName)),
		csvexport.SafeCell(csvexport.Deref(c.OwnerName)),
		csvexport.SafeCell(csvexport.Deref(c.AuditorName)),
		csvexport.Date(csvexport.Deref(c.DueDate)),
		csvexport.Date(csvexport.Deref(c.PopulationDueDate)),
		csvexport.SafeCell(csvexport.Deref(c.PopulationOwnerName)),
		csvexport.SafeCell(csvexport.Deref(c.PopulationTeamName)),
		csvexport.SafeCell(handlerLabels[csvexport.Deref(c.CurrentHandler)]),
		csvexport.Date(csvexport.Deref(c.CurrentDueDate)),
		csvexport.Date(csvexport.Deref(c.PlannedOwnerDate)),
		csvexport.Date(csvexport.Deref(c.PlannedComplianceDate)),
		csvexport.Date(csvexport.Deref(c.PlannedAuditorDate)),
		delayedCell(c),
		csvexport.SafeCell(delayLogText(c.DelayLog)),
	}
}

// delayedCell is blank for a control with no hand-offs tracked, so an untracked
// control is never reported as "No".
func delayedCell(c *model.AuditControl) string {
	switch {
	case c.Delayed == nil:
		return ""
	case *c.Delayed:
		return "Yes"
	}
	return "No"
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// controlExportFileName is audit-controls-<audit name>-YYYY-MM-DD.csv, with the
// audit name reduced to a filesystem-safe slug.
func controlExportFileName(auditName string, now time.Time) string {
	slug := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(auditName), "-"), "-")
	if len(slug) > 60 {
		slug = strings.Trim(slug[:60], "-")
	}
	if slug == "" {
		slug = "audit"
	}
	return "audit-controls-" + slug + "-" + now.UTC().Format("2006-01-02") + ".csv"
}

// exportControls handles GET /api/v1/audits/{id}/controls/export.
//
// Takes the audit detail page's current view as query params and returns every
// matching control as CSV. Restricted to the Compliance Admin and Compliance
// Team. Row visibility is then that of listControls and getAudit: ViewAudits,
// the caller's row scope, and a 404 (never an empty file) for an audit outside
// that scope.
func (h *controlHandler) exportControls(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !auth.RequirePrivilege(ctx, w, privilege.ViewAudits) {
		return
	}
	// ReviewEvidence stands in for "is Compliance Admin or Compliance Team": among
	// the seeded roles only grc-platform-audit-compliance-admin and
	// grc-platform-audit-compliance-team hold it (Management, the internal team
	// and external auditors do not). Testing the privilege rather than a role
	// name keeps this consistent with the rest of the module, which never checks
	// one. It is GLOBAL-only (globalOnlyPrivileges in the entity's grant service),
	// so the unscoped check is the right one.
	if !auth.RequirePrivilege(ctx, w, privilege.ReviewEvidence) {
		return
	}
	auditID, ok := parseIntParam(w, r, "id")
	if !ok {
		return
	}
	params, ok := parseControlExportParams(r.URL.Query())
	if !ok {
		response.WriteError(w, http.StatusBadRequest, "invalid tab")
		return
	}

	scope, _ := deriveScopes(ctx)
	user := auth.FromContext(ctx)
	var userID int
	if user != nil {
		userID = user.UserID
	}
	teamIDs := managedTeamIDs(auth.Grants(ctx))
	if scope != model.ScopeAll {
		inScope, err := h.auditSvc.InScope(ctx, auditID, scope, userID, teamIDs)
		if err != nil {
			response.MapServiceError(ctx, w, err, response.ErrMsgInternal)
			return
		}
		if !inScope {
			response.WriteError(w, http.StatusNotFound, response.ErrMsgNotFound)
			return
		}
	}
	audit, err := h.auditSvc.GetByID(ctx, auditID)
	if err != nil {
		response.MapServiceError(ctx, w, err, response.ErrMsgInternal)
		return
	}
	// Names are resolved by the service, so what is exported is what the page shows.
	controls, err := h.svc.ListScoped(ctx, auditID, scope, userID, teamIDs)
	if err != nil {
		response.MapServiceError(ctx, w, err, response.ErrMsgInternal)
		return
	}
	controls = filterControls(controls, params)
	if len(controls) > controlExportMaxRows {
		response.WriteError(w, http.StatusUnprocessableEntity,
			"too many controls to export at once; narrow the filters and try again")
		return
	}

	cw := csvexport.Begin(w, controlExportFileName(audit.Name, time.Now()), controlExportWriteTimeout)
	_ = cw.Write(controlExportHeader)
	for _, c := range controls {
		_ = cw.Write(controlExportRecord(c))
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		slog.WarnContext(ctx, "control export write failed", "auditId", auditID, "err", err)
		return
	}

	h.recordExport(r, auditID, len(controls), params)
}

// recordExport appends an EXPORTED entry to the audit's activity log. Best
// effort: the file has already been sent, and the trail is advisory history, so
// a failure here is logged and nothing more.
func (h *controlHandler) recordExport(r *http.Request, auditID, rows int, p controlExportParams) {
	user := auth.FromContext(r.Context())
	if h.trail == nil || user == nil {
		return
	}
	details := map[string]any{"rows": rows}
	for k, v := range map[string]any{
		"tab": p.Tab, "search": p.Search, "status": p.Statuses, "requirementType": p.RequirementTypes,
		"controlType": p.ControlTypes, "scope": p.Scopes, "team": p.Teams, "auditor": p.Auditors, "owner": p.Owners,
	} {
		switch t := v.(type) {
		case string:
			if t != "" {
				details[k] = t
			}
		case []string:
			if len(t) > 0 {
				details[k] = t
			}
		}
	}
	if err := h.trail.RecordAuditAction(r.Context(), auditID, "EXPORTED", user.Subject, details); err != nil {
		slog.WarnContext(r.Context(), "record audit export failed", "auditId", auditID, "err", err)
	}
}
