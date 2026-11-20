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

// Field and measure kinds. A field's kind decides which filter ops it accepts
// and how its values are checked.
const (
	KindText   = "text"
	KindInt    = "int"
	KindDate   = "date"
	KindFlag   = "flag"
	KindNumber = "number"
	// KindPerson is a filter on a person. The user types a name (or "me"); the
	// platform looks it up and only the resolved identity reaches the query.
	KindPerson = "person"
)

// FieldDef is something a spec may group by or filter on. Repeats marks a field
// whose data comes from a table with several rows per risk, which makes
// averages over risks unreliable. RequiredPrivilege, when set, hides the field
// from callers who do not hold it: for them it is simply unknown.
type FieldDef struct {
	ID                string
	Label             string
	Kind              string
	Group             bool // usable as a dimension
	Allowed           []string
	Min, Max          int // int range, enforced when Max > 0
	Repeats           bool
	RequiredPrivilege string
}

// MeasureDef is an aggregate. DistinctSafe is true when it stays correct on
// rows a join has repeated.
type MeasureDef struct {
	ID                string
	Label             string
	Kind              string
	DistinctSafe      bool
	RequiredPrivilege string
}

// ColumnDef is something a list dataset can show.
type ColumnDef struct {
	ID    string
	Label string
	Kind  string
	// Internal marks a column only the platform may add (a risk's id, for the
	// web app to link a row). It is never offered to the model, and a request
	// that names it is refused.
	Internal bool
}

// The hubs that have an agent. The risk register's datasets leave Module empty.
const (
	ModuleRisk  = ""
	ModuleAudit = "audit"
)

// MaxListColumns is the most columns a list may show.
const MaxListColumns = 8

// DatasetDef is one thing the agent can count, or list. A list dataset (Rows)
// shows one row per risk with the chosen Columns, up to MaxRows; its Fields are
// what it can be filtered on.
type DatasetDef struct {
	// Module is the hub the dataset belongs to: ModuleRisk or ModuleAudit. A
	// hub's agent can only reach its own datasets.
	Module string
	// ItemOne and ItemMany name what one row of a list is ("risk", "risks"), for
	// the sentence written about it. They default to risk and risks.
	ItemOne, ItemMany string
	ID                string
	Label             string
	Rows              bool
	MaxRows           int
	Columns           []ColumnDef
	Fields            []FieldDef
	Measures          []MeasureDef
}

// The catalog describes the risk register only. Audit Hub data has its own
// agent and its own catalog; nothing here names an audit table or field.
//
// The ids must match the Compliance Entity's catalog
// (entity/compliance-entity/internal/repository/widget_catalog.go), which maps
// each one to SQL and rejects any id it does not know.
var (
	statuses = []string{
		"DRAFT", "PENDING_RISK_OWNER_APPROVAL", "PENDING_MANAGEMENT_APPROVAL",
		"PENDING_COMPLIANCE_REVIEW", "APPROVED", "PENDING_OWNER_COMPLETION_APPROVAL",
		"PENDING_MANAGEMENT_CLOSURE_APPROVAL", "PENDING_COMPLIANCE_CLOSURE",
		"PENDING_AMENDMENT", "PENDING_REVISION", "ESCALATED", "CLOSED", "CANCELLED",
	}
	treatments = []string{"REMEDIATE", "ACCEPT", "TRANSFER", "AVOID", "UNSPECIFIED"}
	quarters   = []string{"Q1", "Q2", "Q3", "Q4"}
	levels     = []string{"LOW", "MEDIUM", "HIGH"}
)

// riskFields are the fields of the risk register a spec may group by or filter
// on.
var riskFields = []FieldDef{
	{ID: "source_register", Label: "Source Register", Kind: KindText, Group: true},
	{ID: "assignment_team", Label: "Assignment Team", Kind: KindText, Group: true},
	{ID: "workflow_status", Label: "Workflow Status", Kind: KindText, Group: true, Allowed: statuses},
	{ID: "treatment_strategy", Label: "Treatment Strategy", Kind: KindText, Group: true, Allowed: treatments},
	{ID: "risk_year", Label: "Year", Kind: KindInt, Group: true, Min: 2000, Max: 2100},
	{ID: "risk_quarter", Label: "Quarter", Kind: KindText, Group: true, Allowed: quarters},
	{ID: "gross_level", Label: "Gross risk level", Kind: KindText, Group: true, Allowed: levels},
	{ID: "gross_likelihood", Label: "Gross Likelihood", Kind: KindInt, Group: true, Min: 1, Max: 3},
	{ID: "gross_impact", Label: "Gross Impact", Kind: KindInt, Group: true, Min: 1, Max: 3},
	{ID: "residual_level", Label: "Residual risk level", Kind: KindText, Group: true, Allowed: levels},
	{ID: "residual_likelihood", Label: "Residual Likelihood", Kind: KindInt, Group: true, Min: 1, Max: 3},
	{ID: "residual_impact", Label: "Residual Impact", Kind: KindInt, Group: true, Min: 1, Max: 3},
	{ID: "category", Label: "Risk Category", Kind: KindText, Group: true, Repeats: true},
	{ID: "compliance_reference", Label: "Compliance Reference", Kind: KindText, Group: true, Repeats: true},
	{ID: "action_owner", Label: "Action Owner", Kind: KindText, Group: true, Repeats: true},
	{ID: "involved_person", Label: "Person named on the risk (owner, assigner, approver or action owner)", Kind: KindPerson},
	{ID: "owner_person", Label: "Risk Owner", Kind: KindPerson},
	{ID: "assigner_person", Label: "Risk Assigner", Kind: KindPerson},
	{ID: "action_owner_person", Label: "Action Owner", Kind: KindPerson},
	{ID: "implementation_date", Label: "Implementation Date", Kind: KindDate},
	{ID: "created_date", Label: "Date the risk was raised", Kind: KindDate},
	{ID: "updated_date", Label: "Date the risk was last updated", Kind: KindDate},
	{ID: "has_action_owner", Label: "Has an Action Owner named", Kind: KindFlag},
	{ID: "is_overdue", Label: "Overdue on Implementation Date", Kind: KindFlag},
}

// listFilters are the risk fields a list may be filtered on. The Action Owner is
// left out: it is an identity, which a person cannot name in a filter.
func listFilters() []FieldDef {
	var out []FieldDef
	for _, f := range riskFields {
		if f.ID != "action_owner" {
			out = append(out, f)
		}
	}
	return out
}

// MaxListRows is the most rows a list may return.
const MaxListRows = 50

var datasets = []DatasetDef{
	{
		ID:     "risks",
		Label:  "Risks",
		Fields: riskFields,
		Measures: []MeasureDef{
			{ID: "risk_count", Label: "Number of risks", Kind: KindInt, DistinctSafe: true},
			{ID: "avg_gross_rating", Label: "Average gross score", Kind: KindNumber},
			{ID: "avg_residual_rating", Label: "Average residual score", Kind: KindNumber},
			{ID: "avg_age_days", Label: "Average age in days", Kind: KindNumber},
		},
	},
	{
		ID:      "risk_list",
		Label:   "Individual risks (a list, one row per risk, never the long description)",
		Rows:    true,
		MaxRows: MaxListRows,
		Columns: []ColumnDef{
			{ID: "risk_code", Label: "Risk code", Kind: KindText},
			{ID: "risk_id", Label: "Risk ID", Kind: KindInt, Internal: true},
			{ID: "risk_title", Label: "Title", Kind: KindText},
			{ID: "source_register", Label: "Source Register", Kind: KindText},
			{ID: "assignment_team", Label: "Assignment Team", Kind: KindText},
			{ID: "workflow_status", Label: "Workflow Status", Kind: KindText},
			{ID: "treatment_strategy", Label: "Treatment Strategy", Kind: KindText},
			{ID: "gross_level", Label: "Gross level", Kind: KindText},
			{ID: "residual_level", Label: "Residual level", Kind: KindText},
			{ID: "implementation_date", Label: "Implementation Date", Kind: KindText},
			{ID: "risk_year", Label: "Year", Kind: KindInt},
			{ID: "risk_quarter", Label: "Quarter", Kind: KindText},
			{ID: "created_date", Label: "Raised on", Kind: KindText},
			{ID: "updated_date", Label: "Last updated", Kind: KindText},
			{ID: "age_days", Label: "Age in days", Kind: KindInt},
		},
		Fields: listFilters(),
	},
	{
		ID:    "ai_suggestions",
		Label: "AI suggestions that a user decided on (as reported by the web app)",
		Fields: []FieldDef{
			{ID: "feature", Label: "Suggestion type", Kind: KindText, Group: true, Allowed: []string{"CATEGORY", "LIKELIHOOD", "ACTION_PLAN"}},
			{ID: "status", Label: "Outcome", Kind: KindText, Group: true, Allowed: []string{"ACCEPTED", "OVERRIDDEN"}},
			{ID: "confidence", Label: "Confidence", Kind: KindText, Group: true, Allowed: []string{"HIGH", "MEDIUM", "LOW"}},
			{ID: "source_register", Label: "Source Register", Kind: KindText, Group: true},
			{ID: "risk_year", Label: "Year", Kind: KindInt, Group: true, Min: 2000, Max: 2100},
			{ID: "risk_quarter", Label: "Quarter", Kind: KindText, Group: true, Allowed: quarters},
		},
		Measures: []MeasureDef{
			{ID: "suggestion_count", Label: "Number of suggestions", Kind: KindInt, DistinctSafe: true},
			{ID: "accepted_count", Label: "Number accepted", Kind: KindInt, DistinctSafe: true},
			{ID: "acceptance_rate", Label: "Acceptance rate", Kind: KindNumber, DistinctSafe: true},
		},
	},
}

// Datasets returns the catalog, for building prompts and for tests. The slice
// is shared; callers must not modify it.
func Datasets() []DatasetDef { return datasets }

// items names what a list dataset's rows are, for the sentence written about it.
func (d DatasetDef) items() (one, many string) {
	if d.ItemOne != "" && d.ItemMany != "" {
		return d.ItemOne, d.ItemMany
	}
	return "risk", "risks"
}

func lookupDataset(id string) (DatasetDef, bool) {
	for _, d := range datasets {
		if d.ID == id {
			return d, true
		}
	}
	return DatasetDef{}, false
}

func (d DatasetDef) field(id string) (FieldDef, bool) {
	for _, f := range d.Fields {
		if f.ID == id {
			return f, true
		}
	}
	return FieldDef{}, false
}

func (d DatasetDef) column(id string) (ColumnDef, bool) {
	for _, c := range d.Columns {
		if c.ID == id {
			return c, true
		}
	}
	return ColumnDef{}, false
}

func (d DatasetDef) measure(id string) (MeasureDef, bool) {
	for _, m := range d.Measures {
		if m.ID == id {
			return m, true
		}
	}
	return MeasureDef{}, false
}
