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

// The widget catalog is the only place a widget query's SQL comes from. A spec
// names datasets, fields and measures by id; this file maps each id to the SQL
// text. A spec can therefore never introduce a table, column, join or
// condition of its own, and every value it supplies is bound as an argument.
//
// Each dataset aliases the risk table as "r": the visibility rule is written
// against that alias, so every dataset must expose the risk row it is counting.

const (
	widgetKindText   = "text"
	widgetKindInt    = "int"
	widgetKindDate   = "date"
	widgetKindFlag   = "flag"
	widgetKindNumber = "number"
	// widgetKindPerson is a filter on a person. The spec carries identities
	// (Asgardeo uuids) that the GRC backend resolved from a name; the platform
	// stores no names, so a name can never reach the SQL.
	widgetKindPerson = "person"

	widgetOpIn  = "in"
	widgetOpGte = "gte"
	widgetOpLte = "lte"
	widgetOpIs  = "is"
)

// widgetJoin is one optional join. It is added to a query only when a selected
// dimension, filter or measure needs it. multiplies marks a join that can
// return several rows per risk, which makes row-level averages unreliable.
type widgetJoin struct {
	order      int
	sql        string
	multiplies bool
}

// widgetField is something a spec may group by or filter on.
type widgetField struct {
	expr    string
	kind    string
	joins   []string
	allowed []string // enum values; nil means any value up to widgetMaxValueLen
	min     int      // int range, used when max > 0
	max     int
	group   bool   // may be used as a dimension
	person  string // for widgetKindPerson: involved, owner, assigner or action_owner
}

// widgetMeasure is an aggregate. distinctSafe is true when the aggregate is
// correct even if a multiplying join repeats a risk's row.
type widgetMeasure struct {
	expr         string
	kind         string
	joins        []string
	distinctSafe bool
}

// widgetColumn is something a list dataset can show.
type widgetColumn struct {
	expr  string
	kind  string
	joins []string
}

// widgetModuleAudit marks a dataset as belonging to the Audit Hub. The risk
// register's datasets leave module empty.
const widgetModuleAudit = "audit"

type widgetDataset struct {
	// module says which hub's endpoint may use the dataset ("" is the risk
	// register, widgetModuleAudit the Audit Hub).
	module string
	// wrapVisibility, when set, rewrites the row-visibility condition for a
	// dataset whose rows are not the rows the rule is written against (an audit
	// is visible when one of its controls is). It is never applied to an empty
	// condition, which means "everything".
	wrapVisibility func(clause string) string
	// rows marks a list dataset: one row per risk, columns instead of measures
	// and dimensions, capped at maxRows. Aggregate datasets leave it false.
	rows    bool
	maxRows int
	columns map[string]widgetColumn

	from      string
	baseWhere string // already starts with " AND " when set
	baseArgs  []any
	joins     map[string]widgetJoin
	fields    map[string]widgetField
	measures  map[string]widgetMeasure
}

var (
	widgetStatuses = []string{
		"DRAFT", "PENDING_RISK_OWNER_APPROVAL", "PENDING_MANAGEMENT_APPROVAL",
		"PENDING_COMPLIANCE_REVIEW", "APPROVED", "PENDING_OWNER_COMPLETION_APPROVAL",
		"PENDING_MANAGEMENT_CLOSURE_APPROVAL", "PENDING_COMPLIANCE_CLOSURE",
		"PENDING_AMENDMENT", "PENDING_REVISION", "ESCALATED", "CLOSED", "CANCELLED",
	}
	widgetTreatments = []string{"REMEDIATE", "ACCEPT", "TRANSFER", "AVOID", "UNSPECIFIED"}
	widgetQuarters   = []string{"Q1", "Q2", "Q3", "Q4"}
	widgetLevels     = []string{"LOW", "MEDIUM", "HIGH"}
)

// Decisions written by the quarterly re-check job are the platform's own, not a
// user accepting a suggestion, so acceptance statistics leave them out. Both
// spellings are excluded because the identifier has been written both ways.
var widgetSystemDeciders = []any{"system:likelihood-recheck-job", "likelihood-recheck-job"}

// widgetRiskJoins are shared by the datasets that count risks. The residual
// score join mirrors dashboardScoreJoin: the latest reassessment's score, else
// the gross score. It is a LEFT JOIN so a risk with no score still counts.
var widgetRiskJoins = map[string]widgetJoin{
	"srt": {order: 1, sql: " JOIN risk_team srt ON srt.id = r.source_register_id"},
	"att": {order: 2, sql: " JOIN risk_team att ON att.id = r.assignment_team_id"},
	"gs":  {order: 3, sql: " LEFT JOIN risk_score gs ON gs.id = r.gross_score_id"},
	"rs": {order: 4, sql: " LEFT JOIN risk_score rs ON rs.id = COALESCE(" +
		"(SELECT ra.score_id FROM risk_assessment ra WHERE ra.risk_id = r.id " +
		"ORDER BY ra.created_at DESC, ra.id DESC LIMIT 1), r.gross_score_id)"},
	"ap": {order: 5, sql: " LEFT JOIN risk_action_plan ap ON ap.risk_id = r.id", multiplies: true},
	"au": {order: 6, sql: " LEFT JOIN `user` au ON au.id = ap.action_owner_id", multiplies: true},
	"cat": {order: 7, sql: " LEFT JOIN risk_category_reference rcat ON rcat.risk_id = r.id" +
		" LEFT JOIN risk_category cat ON cat.id = rcat.category_id", multiplies: true},
	"cref": {order: 8, sql: " LEFT JOIN risk_compliance_reference rcr ON rcr.risk_id = r.id" +
		" LEFT JOIN risk_security_compliance_reference cref ON cref.id = rcr.reference_id", multiplies: true},
}

// widgetRiskFields are the fields of the risk register that a spec may group by
// or filter on. The risks dataset uses all of them; the list dataset filters on
// most of them.
var widgetRiskFields = map[string]widgetField{
	"source_register":      {expr: "srt.name", kind: widgetKindText, joins: []string{"srt"}, group: true},
	"assignment_team":      {expr: "att.name", kind: widgetKindText, joins: []string{"att"}, group: true},
	"workflow_status":      {expr: "r.workflow_status", kind: widgetKindText, allowed: widgetStatuses, group: true},
	"treatment_strategy":   {expr: "COALESCE(r.treatment_strategy, 'UNSPECIFIED')", kind: widgetKindText, allowed: widgetTreatments, group: true},
	"risk_year":            {expr: "r.risk_year", kind: widgetKindInt, min: 2000, max: 2100, group: true},
	"risk_quarter":         {expr: "r.risk_quarter", kind: widgetKindText, allowed: widgetQuarters, group: true},
	"gross_level":          {expr: "gs.risk_level", kind: widgetKindText, joins: []string{"gs"}, allowed: widgetLevels, group: true},
	"gross_likelihood":     {expr: "gs.likelihood", kind: widgetKindInt, joins: []string{"gs"}, min: 1, max: 3, group: true},
	"gross_impact":         {expr: "gs.impact", kind: widgetKindInt, joins: []string{"gs"}, min: 1, max: 3, group: true},
	"residual_level":       {expr: "rs.risk_level", kind: widgetKindText, joins: []string{"rs"}, allowed: widgetLevels, group: true},
	"residual_likelihood":  {expr: "rs.likelihood", kind: widgetKindInt, joins: []string{"rs"}, min: 1, max: 3, group: true},
	"residual_impact":      {expr: "rs.impact", kind: widgetKindInt, joins: []string{"rs"}, min: 1, max: 3, group: true},
	"category":             {expr: "COALESCE(cat.name, 'Uncategorised')", kind: widgetKindText, joins: []string{"cat"}, group: true},
	"compliance_reference": {expr: "COALESCE(cref.name, 'None')", kind: widgetKindText, joins: []string{"cref"}, group: true},
	// The user's identity (Asgardeo uuid): the platform stores no names, so
	// the GRC backend swaps it for a display name before it leaves.
	"action_owner": {expr: "au.uuid", kind: widgetKindText, joins: []string{"ap", "au"}, group: true},
	// People. "involved" is anyone named on the risk, in any of its roles; the
	// others name one role. The SQL is built in widgetPersonClause.
	"involved_person":     {kind: widgetKindPerson, person: "involved"},
	"owner_person":        {kind: widgetKindPerson, person: "owner"},
	"assigner_person":     {kind: widgetKindPerson, person: "assigner"},
	"action_owner_person": {kind: widgetKindPerson, person: "action_owner"},
	"implementation_date": {expr: "r.implementation_date", kind: widgetKindDate},
	// When a risk was raised and last changed, and whether anyone is named to
	// act on it. Filter-only: they find risks that need attention.
	"created_date": {expr: "DATE(r.created_at)", kind: widgetKindDate},
	"updated_date": {expr: "DATE(r.updated_at)", kind: widgetKindDate},
	"has_action_owner": {
		expr: "EXISTS (SELECT 1 FROM risk_action_plan hap WHERE hap.risk_id = r.id AND hap.action_owner_id IS NOT NULL)",
		kind: widgetKindFlag,
	},
	"is_overdue": {
		expr: "(r.implementation_date IS NOT NULL AND r.implementation_date < CURDATE() " +
			"AND r.workflow_status NOT IN ('CLOSED', 'CANCELLED'))",
		kind: widgetKindFlag,
	},
}

// widgetListFilters are the risk fields a list may be filtered on. The Action
// Owner is left out: it is an identity, which a person cannot name in a filter.
var widgetListFilters = func() map[string]widgetField {
	out := map[string]widgetField{}
	for id, f := range widgetRiskFields {
		if id != "action_owner" {
			out[id] = f
		}
	}
	return out
}()

// widgetMaxListRows is the most rows a list may return.
const widgetMaxListRows = 50

var widgetDatasets = map[string]widgetDataset{
	"risks": {
		from:   "FROM risk r",
		joins:  widgetRiskJoins,
		fields: widgetRiskFields,
		measures: map[string]widgetMeasure{
			"risk_count":          {expr: "COUNT(DISTINCT r.id)", kind: widgetKindInt, distinctSafe: true},
			"avg_gross_rating":    {expr: "AVG(gs.risk_rating)", kind: widgetKindNumber, joins: []string{"gs"}},
			"avg_residual_rating": {expr: "AVG(rs.risk_rating)", kind: widgetKindNumber, joins: []string{"rs"}},
			"avg_age_days":        {expr: "AVG(DATEDIFF(CURDATE(), r.created_at))", kind: widgetKindNumber},
		},
	},

	// risk_list shows individual risks, one row each, with a short set of
	// columns and no long text (no description). The visibility rule applies to
	// it exactly as it does to the counts. DISTINCT keeps a risk to one row when
	// a filter joins to a category or compliance reference it has several of.
	"risk_list": {
		rows:    true,
		maxRows: widgetMaxListRows,
		from:    "FROM risk r",
		joins:   widgetRiskJoins,
		fields:  widgetListFilters,
		columns: map[string]widgetColumn{
			"risk_code": {expr: "r.risk_code", kind: widgetKindText},
			// risk_id lets the web app link a row to its risk. The backend adds
			// it itself; it is not something a user or the model can ask for.
			"risk_id":             {expr: "r.id", kind: widgetKindInt},
			"risk_title":          {expr: "r.risk_title", kind: widgetKindText},
			"source_register":     {expr: "srt.name", kind: widgetKindText, joins: []string{"srt"}},
			"assignment_team":     {expr: "att.name", kind: widgetKindText, joins: []string{"att"}},
			"workflow_status":     {expr: "r.workflow_status", kind: widgetKindText},
			"treatment_strategy":  {expr: "COALESCE(r.treatment_strategy, 'UNSPECIFIED')", kind: widgetKindText},
			"gross_level":         {expr: "gs.risk_level", kind: widgetKindText, joins: []string{"gs"}},
			"residual_level":      {expr: "rs.risk_level", kind: widgetKindText, joins: []string{"rs"}},
			"implementation_date": {expr: "DATE_FORMAT(r.implementation_date, '%Y-%m-%d')", kind: widgetKindText},
			"risk_year":           {expr: "r.risk_year", kind: widgetKindInt},
			"risk_quarter":        {expr: "r.risk_quarter", kind: widgetKindText},
			"created_date":        {expr: "DATE_FORMAT(r.created_at, '%Y-%m-%d')", kind: widgetKindText},
			"updated_date":        {expr: "DATE_FORMAT(r.updated_at, '%Y-%m-%d')", kind: widgetKindText},
			"age_days":            {expr: "DATEDIFF(CURDATE(), r.created_at)", kind: widgetKindInt},
		},
	},

	// ai_suggestions counts decided AI suggestions. Every join is one row per
	// suggestion, so plain COUNT/SUM are exact.
	"ai_suggestions": {
		from:      "FROM risk_ai_suggestion s JOIN risk r ON r.id = s.risk_id",
		baseWhere: " AND s.status IN ('ACCEPTED', 'OVERRIDDEN') AND COALESCE(s.decided_by, '') NOT IN (?, ?)",
		baseArgs:  widgetSystemDeciders,
		joins: map[string]widgetJoin{
			"srt": widgetRiskJoins["srt"],
		},
		fields: map[string]widgetField{
			"feature":         {expr: "s.feature", kind: widgetKindText, allowed: []string{"CATEGORY", "LIKELIHOOD", "ACTION_PLAN"}, group: true},
			"status":          {expr: "s.status", kind: widgetKindText, allowed: []string{"ACCEPTED", "OVERRIDDEN"}, group: true},
			"confidence":      {expr: "s.confidence", kind: widgetKindText, allowed: []string{"HIGH", "MEDIUM", "LOW"}, group: true},
			"source_register": {expr: "srt.name", kind: widgetKindText, joins: []string{"srt"}, group: true},
			"risk_year":       {expr: "r.risk_year", kind: widgetKindInt, min: 2000, max: 2100, group: true},
			"risk_quarter":    {expr: "r.risk_quarter", kind: widgetKindText, allowed: widgetQuarters, group: true},
		},
		measures: map[string]widgetMeasure{
			"suggestion_count": {expr: "COUNT(*)", kind: widgetKindInt, distinctSafe: true},
			"accepted_count":   {expr: "COALESCE(SUM(CASE WHEN s.status = 'ACCEPTED' THEN 1 ELSE 0 END), 0)", kind: widgetKindInt, distinctSafe: true},
			"acceptance_rate":  {expr: "COALESCE(SUM(CASE WHEN s.status = 'ACCEPTED' THEN 1 ELSE 0 END) / NULLIF(COUNT(*), 0), 0)", kind: widgetKindNumber, distinctSafe: true},
		},
	},
}
