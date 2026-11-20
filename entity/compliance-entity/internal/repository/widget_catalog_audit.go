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

package repository

import (
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

// The Audit Hub's widget datasets. They follow the rules in widget_catalog.go:
// ids map to SQL here and nowhere else, values are bound, and a row-visibility
// condition is always added. Every dataset aliases the control row as "c" (and
// its audit as "a"), because the scope rule is written against those aliases.
//
// Only audit_control and audit_evidence rows are counted. Free text (a control's
// description, evidence requirement, comments, an evidence attestation, a file
// name) is never selectable, and no dataset touches evidence files.

var (
	widgetControlStatuses = []string{
		"POPULATION_PENDING", "POPULATION_INTERNAL_REVIEW", "POPULATION_UNDER_VALIDATION",
		"POPULATION_NEED_CLARIFICATION", "POPULATION_COMPLETE", "AWAITING_SAMPLE",
		"SUBMITTED_SAMPLE", "EVIDENCE_PENDING", "EVIDENCE_INTERNAL_REVIEW",
		"EVIDENCE_UNDER_VALIDATION", "EVIDENCE_NEED_CLARIFICATION", "COMPLETE",
	}
	widgetEvidenceStatuses = []string{"SUBMITTED", "COMPLIANCE_APPROVED", "COMPLIANCE_REJECTED", "APPROVED", "AUDITOR_REJECTED"}
	widgetAuditStatuses    = []string{"ACTIVE", "COMPLETED", "ARCHIVED"}
	widgetRequirementTypes = []string{"DESIGN", "OE"}
	widgetControlTypes     = []string{"CONFIG", "NON_CONFIG"}
	widgetControlScopes    = []string{"COMMON", "PRODUCT_SPECIFIC"}
)

// widgetAuditJoins are shared by the audit datasets. Each is at most one row per
// control, so plain COUNT is exact. Removed audits are left out by the dataset's
// baseWhere, not by a join.
var widgetAuditJoins = map[string]widgetJoin{
	"fw": {order: 1, sql: " JOIN audit_framework fw ON fw.id = a.framework_id"},
	"pr": {order: 2, sql: " JOIN audit_product pr ON pr.id = a.product_id"},
	"tm": {order: 3, sql: " LEFT JOIN audit_team tm ON tm.id = c.team_id"},
	"ow": {order: 4, sql: " LEFT JOIN `user` ow ON ow.id = c.owner_id"},
	"ad": {order: 5, sql: " LEFT JOIN `user` ad ON ad.id = c.auditor_id"},
}

const widgetAuditFrom = "FROM audit_control c JOIN audit a ON a.id = c.audit_id"
const widgetAuditBaseWhere = " AND a.status <> 'REMOVED'"
const widgetAuditName = "COALESCE(a.display_name, a.name)"

// widgetControlFields are what a control may be grouped or filtered by.
var widgetControlFields = map[string]widgetField{
	"audit": {expr: widgetAuditName, kind: widgetKindText, group: true},
	// control_number can be filtered on but not grouped by.
	"control_number":   {expr: "c.control_number", kind: widgetKindText},
	"audit_status":     {expr: "a.status", kind: widgetKindText, allowed: widgetAuditStatuses, group: true},
	"framework":        {expr: "fw.name", kind: widgetKindText, joins: []string{"fw"}, group: true},
	"product":          {expr: "pr.name", kind: widgetKindText, joins: []string{"pr"}, group: true},
	"team":             {expr: "COALESCE(tm.name, 'No team')", kind: widgetKindText, joins: []string{"tm"}, group: true},
	"control_status":   {expr: "c.status", kind: widgetKindText, allowed: widgetControlStatuses, group: true},
	"requirement_type": {expr: "c.requirement_type", kind: widgetKindText, allowed: widgetRequirementTypes, group: true},
	"control_type":     {expr: "c.control_type", kind: widgetKindText, allowed: widgetControlTypes, group: true},
	"control_scope":    {expr: "c.scope", kind: widgetKindText, allowed: widgetControlScopes, group: true},
	// Identities (Asgardeo uuids): the platform stores no names, so the GRC
	// backend swaps them for display names before they leave.
	"owner":   {expr: "ow.uuid", kind: widgetKindText, joins: []string{"ow"}, group: true},
	"auditor": {expr: "ad.uuid", kind: widgetKindText, joins: []string{"ad"}, group: true},
	// People a user names. The SQL is built in widgetPersonClause.
	"involved_person": {kind: widgetKindPerson, person: "control_involved"},
	"owner_person":    {kind: widgetKindPerson, person: "control_owner"},
	"auditor_person":  {kind: widgetKindPerson, person: "control_auditor"},
	"due_date":        {expr: "c.due_date", kind: widgetKindDate},
	// When a control was added and last changed, and whether anyone is named on
	// it. Filter-only: they find controls that need attention.
	"created_date": {expr: "DATE(c.created_at)", kind: widgetKindDate},
	"updated_date": {expr: "DATE(c.updated_at)", kind: widgetKindDate},
	"has_owner":    {expr: "(c.owner_id IS NOT NULL)", kind: widgetKindFlag},
	"has_auditor":  {expr: "(c.auditor_id IS NOT NULL)", kind: widgetKindFlag},
	"is_overdue": {
		expr: "(c.due_date IS NOT NULL AND c.due_date < CURDATE() AND c.status <> 'COMPLETE')",
		kind: widgetKindFlag,
	},
}

// widgetControlFilters are the control fields a list or an evidence count may be
// filtered on. The owner and auditor identities are left out: a person cannot
// name an identity in a filter, only a name, which becomes a person filter.
var widgetControlFilters = func() map[string]widgetField {
	out := map[string]widgetField{}
	for id, f := range widgetControlFields {
		if id != "owner" && id != "auditor" {
			out[id] = f
		}
	}
	return out
}()

// widgetAuditFields are what audits may be grouped or filtered by.
var widgetAuditFields = map[string]widgetField{
	"audit":        {expr: widgetAuditName, kind: widgetKindText, group: true},
	"audit_status": {expr: "a.status", kind: widgetKindText, allowed: widgetAuditStatuses, group: true},
	"framework":    {expr: "fw.name", kind: widgetKindText, joins: []string{"fw"}, group: true},
	"product":      {expr: "pr.name", kind: widgetKindText, joins: []string{"pr"}, group: true},
}

// widgetAuditVisible makes an audit visible when the caller may see at least one
// of its controls: the control rule (written against "c") goes inside an EXISTS.
func widgetAuditVisible(clause string) string {
	return " AND EXISTS (SELECT 1 FROM audit_control c WHERE c.audit_id = a.id" + clause + ")"
}

func init() {
	for id, ds := range map[string]widgetDataset{
		// audits counts audits.
		"audits": {
			module:         widgetModuleAudit,
			from:           "FROM audit a",
			baseWhere:      widgetAuditBaseWhere,
			joins:          widgetAuditJoins,
			fields:         widgetAuditFields,
			wrapVisibility: widgetAuditVisible,
			measures: map[string]widgetMeasure{
				"audit_count": {expr: "COUNT(*)", kind: widgetKindInt, distinctSafe: true},
			},
		},

		// audit_list shows individual audits, one row each. With only the
		// framework or product column, it is a list of those names.
		"audit_list": {
			module:         widgetModuleAudit,
			rows:           true,
			maxRows:        widgetMaxListRows,
			from:           "FROM audit a",
			baseWhere:      widgetAuditBaseWhere,
			joins:          widgetAuditJoins,
			fields:         widgetAuditFields,
			wrapVisibility: widgetAuditVisible,
			columns: map[string]widgetColumn{
				"audit":        {expr: widgetAuditName, kind: widgetKindText},
				"framework":    {expr: "fw.name", kind: widgetKindText, joins: []string{"fw"}},
				"product":      {expr: "pr.name", kind: widgetKindText, joins: []string{"pr"}},
				"audit_status": {expr: "a.status", kind: widgetKindText},
				"period_start": {expr: "DATE_FORMAT(a.period_start, '%Y-%m-%d')", kind: widgetKindText},
				"period_end":   {expr: "DATE_FORMAT(a.period_end, '%Y-%m-%d')", kind: widgetKindText},
			},
		},

		// controls counts controls.
		"controls": {
			module:    widgetModuleAudit,
			from:      widgetAuditFrom,
			baseWhere: widgetAuditBaseWhere,
			joins:     widgetAuditJoins,
			fields:    widgetControlFields,
			measures: map[string]widgetMeasure{
				"control_count": {expr: "COUNT(*)", kind: widgetKindInt, distinctSafe: true},
				"overdue_count": {
					expr: "COALESCE(SUM(CASE WHEN c.due_date IS NOT NULL AND c.due_date < CURDATE() AND c.status <> 'COMPLETE' THEN 1 ELSE 0 END), 0)",
					kind: widgetKindInt, distinctSafe: true,
				},
				"overdue_rate": {
					expr: "COALESCE(SUM(CASE WHEN c.due_date IS NOT NULL AND c.due_date < CURDATE() AND c.status <> 'COMPLETE' THEN 1 ELSE 0 END) / NULLIF(COUNT(*), 0), 0)",
					kind: widgetKindNumber, distinctSafe: true,
				},
				"complete_rate":  {expr: "COALESCE(SUM(CASE WHEN c.status = 'COMPLETE' THEN 1 ELSE 0 END) / NULLIF(COUNT(*), 0), 0)", kind: widgetKindNumber, distinctSafe: true},
				"complete_count": {expr: "COALESCE(SUM(CASE WHEN c.status = 'COMPLETE' THEN 1 ELSE 0 END), 0)", kind: widgetKindInt, distinctSafe: true},
			},
		},

		// control_list shows individual controls, one row each, with short
		// columns and no long text.
		"control_list": {
			module:    widgetModuleAudit,
			rows:      true,
			maxRows:   widgetMaxListRows,
			from:      widgetAuditFrom,
			baseWhere: widgetAuditBaseWhere,
			joins:     widgetAuditJoins,
			fields:    widgetControlFilters,
			columns: map[string]widgetColumn{
				"control_number": {expr: "c.control_number", kind: widgetKindText},
				// audit_id and control_id let the web app open a control. The
				// backend adds them itself; a user or the model cannot ask for them.
				"audit_id":         {expr: "c.audit_id", kind: widgetKindInt},
				"control_id":       {expr: "c.id", kind: widgetKindInt},
				"audit":            {expr: widgetAuditName, kind: widgetKindText},
				"framework":        {expr: "fw.name", kind: widgetKindText, joins: []string{"fw"}},
				"product":          {expr: "pr.name", kind: widgetKindText, joins: []string{"pr"}},
				"team":             {expr: "COALESCE(tm.name, 'No team')", kind: widgetKindText, joins: []string{"tm"}},
				"control_status":   {expr: "c.status", kind: widgetKindText},
				"requirement_type": {expr: "c.requirement_type", kind: widgetKindText},
				"control_type":     {expr: "c.control_type", kind: widgetKindText},
				"due_date":         {expr: "DATE_FORMAT(c.due_date, '%Y-%m-%d')", kind: widgetKindText},
				"created_date":     {expr: "DATE_FORMAT(c.created_at, '%Y-%m-%d')", kind: widgetKindText},
				"updated_date":     {expr: "DATE_FORMAT(c.updated_at, '%Y-%m-%d')", kind: widgetKindText},
				"age_days":         {expr: "DATEDIFF(CURDATE(), c.created_at)", kind: widgetKindInt},
			},
		},

		// evidence_rounds counts evidence submissions (one row each) by their
		// outcome, joined to the control they belong to.
		"evidence_rounds": {
			module:    widgetModuleAudit,
			from:      "FROM audit_evidence e JOIN audit_control c ON c.id = e.control_id JOIN audit a ON a.id = c.audit_id",
			baseWhere: widgetAuditBaseWhere,
			joins:     widgetAuditJoins,
			fields: func() map[string]widgetField {
				out := map[string]widgetField{
					"evidence_status": {expr: "e.status", kind: widgetKindText, allowed: widgetEvidenceStatuses, group: true},
				}
				for _, id := range []string{"audit", "audit_status", "framework", "product", "team", "control_status", "requirement_type", "involved_person", "owner_person", "auditor_person"} {
					out[id] = widgetControlFilters[id]
				}
				// Who owns the control and who audits it, to see outcomes by person.
				out["owner"], out["auditor"] = widgetControlFields["owner"], widgetControlFields["auditor"]
				return out
			}(),
			measures: map[string]widgetMeasure{
				"evidence_count": {expr: "COUNT(*)", kind: widgetKindInt, distinctSafe: true},
				"approved_count": {expr: "COALESCE(SUM(CASE WHEN e.status = 'APPROVED' THEN 1 ELSE 0 END), 0)", kind: widgetKindInt, distinctSafe: true},
				"approved_rate":  {expr: "COALESCE(SUM(CASE WHEN e.status = 'APPROVED' THEN 1 ELSE 0 END) / NULLIF(COUNT(*), 0), 0)", kind: widgetKindNumber, distinctSafe: true},
				"rejection_rate": {expr: "COALESCE(SUM(CASE WHEN e.status IN ('COMPLIANCE_REJECTED', 'AUDITOR_REJECTED') THEN 1 ELSE 0 END) / NULLIF(COUNT(*), 0), 0)", kind: widgetKindNumber, distinctSafe: true},
				"rejected_count": {expr: "COALESCE(SUM(CASE WHEN e.status IN ('COMPLIANCE_REJECTED', 'AUDITOR_REJECTED') THEN 1 ELSE 0 END), 0)", kind: widgetKindInt, distinctSafe: true},
			},
		},
	} {
		widgetDatasets[id] = ds
	}
}

// auditWidgetVisibilityClause is the rule for which controls a query may count.
// It is the same rule the Audit Hub applies everywhere else (controlScopeWhere,
// written against the control alias "c"), and fail-closed: an unknown scope, or
// a scope that needs a user or a team and has none, matches no row.
func auditWidgetVisibilityClause(req domain.AuditWidgetQueryRequest) (string, []any, error) {
	if len(req.ScopeTeamIDs) > widgetMaxTeamIDs {
		return "", nil, widgetErr("too many teams in scope")
	}
	for _, id := range req.ScopeTeamIDs {
		if id <= 0 {
			return "", nil, widgetErr("invalid team in scope")
		}
	}
	switch req.Scope {
	case domain.ScopeAll:
		return "", nil, nil
	case domain.ScopeOwned, domain.ScopeAssigned, domain.ScopeTeam:
		if req.UserID <= 0 {
			return " AND 1=0", nil, nil
		}
		clause, args := controlScopeWhere(req.Scope, req.UserID, req.ScopeTeamIDs)
		return clause, args, nil
	}
	return " AND 1=0", nil, nil
}
