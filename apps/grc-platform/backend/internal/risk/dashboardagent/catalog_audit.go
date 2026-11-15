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

package dashboardagent

// The Audit Hub's part of the catalog. It mirrors, id for id, the datasets the
// Compliance Entity compiles for POST /audit/widgets/query
// (entity/compliance-entity/internal/repository/widget_catalog_audit.go); the
// drift test keeps the two in step. Only audit_control and audit_evidence rows
// are counted, and no free text (a control's description, comments, an evidence
// attestation, file names) is offered.

var (
	controlStatuses = []string{
		"POPULATION_PENDING", "POPULATION_INTERNAL_REVIEW", "POPULATION_UNDER_VALIDATION",
		"POPULATION_NEED_CLARIFICATION", "POPULATION_COMPLETE", "AWAITING_SAMPLE",
		"SUBMITTED_SAMPLE", "EVIDENCE_PENDING", "EVIDENCE_INTERNAL_REVIEW",
		"EVIDENCE_UNDER_VALIDATION", "EVIDENCE_NEED_CLARIFICATION", "COMPLETE",
	}
	evidenceStatuses = []string{"SUBMITTED", "COMPLIANCE_APPROVED", "COMPLIANCE_REJECTED", "APPROVED", "AUDITOR_REJECTED"}
	auditStatuses    = []string{"ACTIVE", "COMPLETED", "ARCHIVED"}
	requirementTypes = []string{"DESIGN", "OE"}
	controlTypes     = []string{"CONFIG", "NON_CONFIG"}
	controlScopes    = []string{"COMMON", "PRODUCT_SPECIFIC"}
)

// controlFields are what controls may be grouped or filtered by. Owner and
// Auditor are identities that the platform swaps for names; a user names a
// person with the person fields.
var controlFields = []FieldDef{
	{ID: "audit", Label: "Audit", Kind: KindText, Group: true},
	{ID: "control_number", Label: "Control number (such as CA-01)", Kind: KindText},
	{ID: "audit_status", Label: "Audit status", Kind: KindText, Group: true, Allowed: auditStatuses},
	{ID: "framework", Label: "Framework", Kind: KindText, Group: true},
	{ID: "product", Label: "Product", Kind: KindText, Group: true},
	{ID: "team", Label: "Team", Kind: KindText, Group: true},
	{ID: "control_status", Label: "Control status", Kind: KindText, Group: true, Allowed: controlStatuses},
	{ID: "requirement_type", Label: "Requirement type (design or operating effectiveness)", Kind: KindText, Group: true, Allowed: requirementTypes},
	{ID: "control_type", Label: "Control type", Kind: KindText, Group: true, Allowed: controlTypes},
	{ID: "control_scope", Label: "Control scope", Kind: KindText, Group: true, Allowed: controlScopes},
	{ID: "owner", Label: "Control owner", Kind: KindText, Group: true},
	{ID: "auditor", Label: "Auditor", Kind: KindText, Group: true},
	{ID: "involved_person", Label: "Person named on the control (owner or auditor)", Kind: KindPerson},
	{ID: "owner_person", Label: "Control owner", Kind: KindPerson},
	{ID: "auditor_person", Label: "Auditor", Kind: KindPerson},
	{ID: "due_date", Label: "Due date", Kind: KindDate},
	{ID: "created_date", Label: "Date the control was added", Kind: KindDate},
	{ID: "updated_date", Label: "Date the control was last updated", Kind: KindDate},
	{ID: "has_owner", Label: "Has an owner named", Kind: KindFlag},
	{ID: "has_auditor", Label: "Has an auditor named", Kind: KindFlag},
	{ID: "is_overdue", Label: "Overdue (past its due date and not complete)", Kind: KindFlag},
}

func controlFilters() []FieldDef {
	var out []FieldDef
	for _, f := range controlFields {
		if f.ID != "owner" && f.ID != "auditor" {
			out = append(out, f)
		}
	}
	return out
}

func pick(fields []FieldDef, ids ...string) []FieldDef {
	var out []FieldDef
	for _, id := range ids {
		for _, f := range fields {
			if f.ID == id {
				out = append(out, f)
			}
		}
	}
	return out
}

func init() {
	filters := controlFilters()
	evidenceFields := append([]FieldDef{
		{ID: "evidence_status", Label: "Evidence outcome", Kind: KindText, Group: true, Allowed: evidenceStatuses},
	}, pick(controlFields, "audit", "audit_status", "framework", "product", "team", "control_status", "requirement_type", "owner", "auditor", "involved_person", "owner_person", "auditor_person")...)

	auditFields := pick(filters, "audit", "audit_status", "framework", "product")

	datasets = append(datasets,
		DatasetDef{
			Module: ModuleAudit,
			ID:     "audits",
			Label:  "Audits",
			Fields: auditFields,
			Measures: []MeasureDef{
				{ID: "audit_count", Label: "Number of audits", Kind: KindInt, DistinctSafe: true},
			},
		},
		DatasetDef{
			Module:   ModuleAudit,
			ID:       "audit_list",
			Label:    "Individual audits, one row each (also a list of the frameworks or products)",
			Rows:     true,
			MaxRows:  50,
			ItemOne:  "audit",
			ItemMany: "audits",
			Columns: []ColumnDef{
				{ID: "audit", Label: "Audit", Kind: KindText},
				{ID: "framework", Label: "Framework", Kind: KindText},
				{ID: "product", Label: "Product", Kind: KindText},
				{ID: "audit_status", Label: "Status", Kind: KindText},
				{ID: "period_start", Label: "Period start", Kind: KindText},
				{ID: "period_end", Label: "Period end", Kind: KindText},
			},
			Fields: auditFields,
		},
		DatasetDef{
			Module: ModuleAudit,
			ID:     "controls",
			Label:  "Controls in audits",
			Fields: controlFields,
			Measures: []MeasureDef{
				{ID: "control_count", Label: "Number of controls", Kind: KindInt, DistinctSafe: true},
				{ID: "overdue_count", Label: "Number of overdue controls", Kind: KindInt, DistinctSafe: true},
				{ID: "complete_count", Label: "Number of complete controls", Kind: KindInt, DistinctSafe: true},
				{ID: "overdue_rate", Label: "Share of controls that are overdue", Kind: KindNumber, DistinctSafe: true},
				{ID: "complete_rate", Label: "Share of controls that are complete", Kind: KindNumber, DistinctSafe: true},
			},
		},
		DatasetDef{
			Module:   ModuleAudit,
			ID:       "control_list",
			Label:    "Individual controls, one row each",
			Rows:     true,
			MaxRows:  50,
			ItemOne:  "control",
			ItemMany: "controls",
			Columns: []ColumnDef{
				{ID: "control_number", Label: "Control", Kind: KindText},
				// Only the platform adds these, to open a control; never offered.
				{ID: "audit_id", Label: "Audit ID", Kind: KindInt, Internal: true},
				{ID: "control_id", Label: "Control ID", Kind: KindInt, Internal: true},
				{ID: "audit", Label: "Audit", Kind: KindText},
				{ID: "framework", Label: "Framework", Kind: KindText},
				{ID: "product", Label: "Product", Kind: KindText},
				{ID: "team", Label: "Team", Kind: KindText},
				{ID: "control_status", Label: "Status", Kind: KindText},
				{ID: "requirement_type", Label: "Requirement type", Kind: KindText},
				{ID: "control_type", Label: "Control type", Kind: KindText},
				{ID: "due_date", Label: "Due date", Kind: KindText},
				{ID: "created_date", Label: "Added on", Kind: KindText},
				{ID: "updated_date", Label: "Last updated", Kind: KindText},
				{ID: "age_days", Label: "Age in days", Kind: KindInt},
			},
			Fields: filters,
		},
		DatasetDef{
			Module: ModuleAudit,
			ID:     "evidence_rounds",
			Label:  "Evidence submissions",
			Fields: evidenceFields,
			Measures: []MeasureDef{
				{ID: "evidence_count", Label: "Number of evidence submissions", Kind: KindInt, DistinctSafe: true},
				{ID: "approved_count", Label: "Number approved", Kind: KindInt, DistinctSafe: true},
				{ID: "rejected_count", Label: "Number rejected", Kind: KindInt, DistinctSafe: true},
				{ID: "approved_rate", Label: "Share approved", Kind: KindNumber, DistinctSafe: true},
				{ID: "rejection_rate", Label: "Share rejected", Kind: KindNumber, DistinctSafe: true},
			},
		},
	)
}
