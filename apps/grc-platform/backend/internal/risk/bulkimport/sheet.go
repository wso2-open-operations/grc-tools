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

package bulkimport

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Row is one register row after header-name mapping and value transforms
// (plan §6). Person columns hold work emails at this point; the *ID fields are
// filled later by the resolver (resolve.go).
type Row struct {
	MigrationID int // added "Migration ID" column — errors.csv correlation only
	CSVLine     int // 1-based line in the source file, for findings

	RiskYear        int
	RiskQuarter     string // Q1..Q4
	SourceRegister  string // raw name/code from the sheet
	AssignmentTeam  string // raw name/code from the sheet
	RiskTitle       string
	RiskDescription string
	RiskCategory    string // raw name, pre-lookup

	// STANDARD template: compliance references, as written in the sheet.
	ComplianceRefs []string
	// AGGREGATED template: platforms, as written in the sheet.
	Platforms []string

	// Managed Services template fields (RISK_MODULE_DESIGN.md §14), as written in
	// the sheet. Their entity ids are below, filled by mapRow.
	Customer       string
	DeploymentType string
	Products       []string
	Environments   []string // PRODUCTION | NON_PRODUCTION | DR (already the entity's values)

	IdentifiedByType   string // EMPLOYEE | EXTERNAL_PERSON | TOOL | ""
	IdentifiedByName   string
	RiskIdentifiedDate string // YYYY-MM-DD | ""
	// GrossLikelihood/GrossImpact identify the risk's original, immutable
	// rating (risk.gross_score_id) — the rating first given when the risk was
	// raised. ResidualLikelihood/ResidualImpact identify its current standing;
	// when they differ from Gross, migrateRow writes a synthetic
	// risk_assessment row so the residual value is reflected the same way a
	// real reassessment would be (see needsResidualAssessment).
	GrossLikelihood    int // 1..3
	GrossImpact        int // 1..3
	ResidualLikelihood int // 1..3
	ResidualImpact     int // 1..3
	ImpactDescription  string
	ImplementationDate string // YYYY-MM-DD (required — see parseSheet)
	ReassessmentDate   string // YYYY-MM-DD | ""

	AssignerEmail           string
	OwnerEmail              string
	ManagementApproverEmail string
	ActionOwnerEmail        string // "" allowed only for CLOSED rows

	ActionPlanDescription string
	ActionSteps           []string
	TreatmentStrategy     string // ACCEPT | TRANSFER | REMEDIATE | AVOID
	Progress              string
	GitIssueURL           string
	EmailSubject          string
	Remarks               string

	WorkflowStatus string // APPROVED | CLOSED

	// Template is the target register's template, copied onto every row so the
	// write and verify steps need not carry the Target around.
	Template string

	// ── filled by resolve.go ────────────────────────────────────────────────
	SourceRegisterID     int
	AssignmentTeamID     int
	CustomerID           int
	DeploymentTypeID     int
	ProductIDs           []int
	ComplianceRefIDs     []int
	PlatformIDs          []int
	RiskCategoryIDs      []int
	AssignerID           int
	OwnerID              int
	ManagementApproverID int
	ActionOwnerID        *int
}

// actionStepPlaceholder stands in when the source register row has no action
// steps — the entity requires at least one (see mapRow).
const actionStepPlaceholder = "Migrated from the historical risk register; original action steps were not recorded."

// baseHeaders are the columns every register's sheet carries. Matching is case-
// and space-insensitive (plan §4). "Source Register" is deliberately absent: the
// register is chosen in the UI, so a sheet does not name one. A column by that
// name is still accepted and, when filled, must agree with the chosen register
// (mapRow).
var baseHeaders = []string{
	"Year", "Quarter", "Risk Title", "Risk Description",
	"Risk Category", "Risk Identified By",
	"Select Employee/ Name of External Person/ Tool", "Risk Identified Date",
	"Risk Assigned To", "Gross Likelihood", "Gross Impact",
	"Residual Likelihood", "Residual Impact", "Impact Description",
	"Implementation Date", "Reassessment Date", "Assignment Team", "Risk Owner",
	"Management Approver", "Action Owner", "Action Plan Description",
	"Action Steps", "Treatment Strategy", "Progress", "Git Issue URL",
	"Email Subject", "Remarks",
	"Workflow Status", "Migration ID",
}

// templateColumns are the columns only one template's sheet carries, and so the
// ones its template download adds after Quarter. STANDARD's compliance column is
// optional on upload (a sheet may leave it out); the others are required.
var templateColumns = map[string][]string{
	templateStandard:        {"Security Compliance Reference"},
	templateAggregated:      {"Platform"},
	templateManagedServices: {"Customer", "Deployment Type", "Product", "Environment"},
}

// allTemplateColumns is every template-specific column, so a column that
// belongs to a different template can be told apart from an unknown one.
var allTemplateColumns = []string{
	"Security Compliance Reference", "Platform",
	"Customer", "Deployment Type", "Product", "Environment",
}

// requiredHeaders are the columns a sheet for template must carry.
func requiredHeaders(template string) []string {
	out := append([]string(nil), baseHeaders...)
	for _, c := range templateColumns[template] {
		if template == templateStandard && c == "Security Compliance Reference" {
			continue
		}
		out = append(out, c)
	}
	return out
}

// TemplateHeaders is the header row of the template download for template: the
// shared columns in sheet order, with the template's own columns after Quarter.
func TemplateHeaders(template string) []string {
	out := []string{"Year", "Quarter"}
	out = append(out, templateColumns[template]...)
	return append(out, baseHeaders[2:]...)
}

// parseSheet reads the CSV, maps every row, and applies the value rules in §6.
// A structural problem (bad/short header, unreadable file) returns an error and
// aborts the run; a row-level problem becomes a REJECT/WARN Finding and the row
// is still returned (callers drop REJECTed rows via the Report).
func parseSheet(r io.Reader, refs RefData, tgt Target) ([]Row, []Finding, error) {
	cr := csv.NewReader(stripBOM(r))
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true

	records, err := cr.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("read csv: %w", err)
	}
	if len(records) == 0 {
		return nil, nil, fmt.Errorf("csv is empty")
	}

	idx, err := mapHeader(records[0], tgt.Template)
	if err != nil {
		return nil, nil, err
	}

	var rows []Row
	var findings []Finding
	seenID := map[int]int{} // MigrationID -> first CSV line that used it
	for i, rec := range records[1:] {
		line := i + 2 // 1-based, past the header
		if isSkippable(rec, idx) {
			continue
		}
		row, rf := mapRow(rec, idx, line, refs, tgt)
		findings = append(findings, rf...)
		// A repeated Migration ID silently collides downstream: reconstructState
		// keys resume state by it (map[int]ResumeState) and RejectedMigrationIDs
		// is a map[int]struct{}. REJECT every row that shares an ID — the natural
		// key check can't catch two genuinely different risks with a mistyped ID.
		if row.MigrationID != 0 {
			if first, dup := seenID[row.MigrationID]; dup {
				findings = append(findings, Finding{
					MigrationID: row.MigrationID, CSVRow: line, RiskTitle: row.RiskTitle,
					Severity: SevReject, Failure: "Migration ID",
					Detail: fmt.Sprintf("duplicate of the row on line %d", first),
				})
			} else {
				seenID[row.MigrationID] = line
			}
		}
		rows = append(rows, row)
	}
	return rows, findings, nil
}

// mapHeader returns a column-name → index map, tolerating trailing spaces and
// case. It errors if any expected column is missing.
func mapHeader(header []string, template string) (map[string]int, error) {
	got := make(map[string]int, len(header))
	for i, h := range header {
		got[normHeader(h)] = i
	}
	var missing []string
	for _, want := range requiredHeaders(template) {
		if _, ok := got[normHeader(want)]; !ok {
			missing = append(missing, want)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("csv is missing column(s): %s", strings.Join(missing, "; "))
	}
	return got, nil
}

func normHeader(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// utf8BOM is the byte-order mark some spreadsheet tools prepend when saving
// "CSV UTF-8" (Excel does this on Windows; a plain "Download as CSV" from
// Google Sheets does not). It lands as an invisible prefix on the raw bytes of
// the file, ahead of the first header cell — for a file that is otherwise
// perfectly valid. Operators exporting the register shouldn't have to know
// this; strip it instead of documenting around it.
const utf8BOM = "\xEF\xBB\xBF"

// stripBOM discards a leading UTF-8 BOM from r, if present, before any CSV
// tokenization happens. This has to run on the raw byte stream, not on the
// parsed header string afterwards: if the first header cell is quoted (some
// exporters quote every field), a BOM sitting before the opening `"` moves
// that quote off position zero of the field, and encoding/csv's non-lazy
// quote handling rejects the whole file with "bare \" in non-quoted-field"
// before a post-parse strip ever gets a chance to run.
func stripBOM(r io.Reader) io.Reader {
	br := bufio.NewReader(r)
	if prefix, _ := br.Peek(len(utf8BOM)); string(prefix) == utf8BOM {
		_, _ = br.Discard(len(utf8BOM))
	}
	return br
}

// isSkippable drops the legend row and fully blank rows (plan §4).
func isSkippable(rec []string, idx map[string]int) bool {
	joined := strings.ToLower(strings.Join(rec, ""))
	if strings.TrimSpace(joined) == "" {
		return true
	}
	if strings.Contains(joined, "required fields are coloured") {
		return true
	}
	// A row with neither a Year nor a Risk Title is not a data row.
	return get(rec, idx, "Year") == "" && get(rec, idx, "Risk Title") == ""
}

// mapRow applies every rule in plan §6: the value transforms in transform.go
// and the reference-data lookups against refs (loaded in preflight, T3). A
// row-level problem becomes a REJECT or WARN Finding; the Row is still returned
// (callers drop REJECTed rows via the Report). People columns are left as
// lowercased emails here — resolve.go (T5) turns them into user ids.
func mapRow(rec []string, idx map[string]int, line int, refs RefData, tgt Target) (Row, []Finding) {
	row := Row{CSVLine: line, Template: tgt.Template}
	var fs []Finding
	title := strings.TrimSpace(get(rec, idx, "Risk Title"))
	reject := func(field, detail string) {
		fs = append(fs, Finding{MigrationID: row.MigrationID, CSVRow: line, RiskTitle: title,
			Severity: SevReject, Failure: field, Detail: detail})
	}
	// rejectLookup is reject plus the offending value and why it was refused, so
	// the report can group them (Finding.Problem).
	rejectLookup := func(field, detail, value, problem string) {
		fs = append(fs, Finding{MigrationID: row.MigrationID, CSVRow: line, RiskTitle: title,
			Severity: SevReject, Failure: field, Detail: detail, Value: value, Problem: problem})
	}
	warn := func(field, detail string) {
		fs = append(fs, Finding{MigrationID: row.MigrationID, CSVRow: line, RiskTitle: title,
			Severity: SevWarn, Failure: field, Detail: detail})
	}

	// Migration ID first, so every finding below can carry it.
	if n, err := parseIntish(get(rec, idx, "Migration ID")); err == nil {
		row.MigrationID = n
	} else {
		reject("Migration ID", "missing or non-integer")
	}

	// ── verbatim text ──────────────────────────────────────────────────────
	row.RiskTitle = title
	if row.RiskTitle == "" {
		reject("Risk Title", "empty")
	}
	row.RiskDescription = strings.TrimSpace(get(rec, idx, "Risk Description"))
	row.ImpactDescription = strings.TrimSpace(get(rec, idx, "Impact Description"))
	row.Progress = strings.TrimSpace(get(rec, idx, "Progress"))
	row.EmailSubject = strings.TrimSpace(get(rec, idx, "Email Subject"))
	row.Remarks = strings.TrimSpace(get(rec, idx, "Remarks"))
	row.IdentifiedByName = strings.TrimSpace(get(rec, idx, "Select Employee/ Name of External Person/ Tool"))
	row.ActionPlanDescription = strings.TrimSpace(get(rec, idx, "Action Plan Description"))
	row.ActionSteps = splitSteps(get(rec, idx, "Action Steps"))
	if len(row.ActionSteps) == 0 {
		// The entity's POST /risks rejects a stepless plan (a plan with no steps
		// can never be marked COMPLETED). Insert a placeholder and flag it.
		row.ActionSteps = []string{actionStepPlaceholder}
		warn("Action Steps", "no action steps in the source register — a placeholder step was inserted (the entity requires at least one)")
	}

	// ── people (emails; resolved to ids in resolve.go, T5) ─────────────────
	row.AssignerEmail = strings.ToLower(strings.TrimSpace(get(rec, idx, "Risk Assigned To")))
	row.OwnerEmail = strings.ToLower(strings.TrimSpace(get(rec, idx, "Risk Owner")))
	row.ManagementApproverEmail = strings.ToLower(strings.TrimSpace(get(rec, idx, "Management Approver")))
	row.ActionOwnerEmail = strings.ToLower(strings.TrimSpace(get(rec, idx, "Action Owner")))

	// ── parsed scalars ────────────────────────────────────────────────────
	if y, err := parseYear(get(rec, idx, "Year")); err == nil {
		row.RiskYear = y
	} else {
		reject("Year", err.Error())
	}
	if q, err := parseQuarter(get(rec, idx, "Quarter")); err == nil {
		row.RiskQuarter = q
	} else {
		reject("Quarter", err.Error())
	}
	if l, err := parseScore(get(rec, idx, "Gross Likelihood")); err == nil {
		row.GrossLikelihood = l
	} else {
		reject("Gross Likelihood", err.Error())
	}
	if im, err := parseScore(get(rec, idx, "Gross Impact")); err == nil {
		row.GrossImpact = im
	} else {
		reject("Gross Impact", err.Error())
	}
	if l, err := parseScore(get(rec, idx, "Residual Likelihood")); err == nil {
		row.ResidualLikelihood = l
	} else {
		reject("Residual Likelihood", err.Error())
	}
	if im, err := parseScore(get(rec, idx, "Residual Impact")); err == nil {
		row.ResidualImpact = im
	} else {
		reject("Residual Impact", err.Error())
	}
	if ts, err := mapTreatment(get(rec, idx, "Treatment Strategy")); err == nil {
		row.TreatmentStrategy = ts
	} else {
		reject("Treatment Strategy", err.Error())
	}
	if ws, err := mapWorkflowStatus(get(rec, idx, "Workflow Status")); err == nil {
		row.WorkflowStatus = ws
	} else {
		reject("Workflow Status", err.Error())
	}
	if typ, ok := mapIdentifiedByType(get(rec, idx, "Risk Identified By")); ok {
		row.IdentifiedByType = typ
	} else if raw := strings.TrimSpace(get(rec, idx, "Risk Identified By")); raw == "" {
		warn("Risk Identified By", "blank — identified_by_type left null")
	} else {
		warn("Risk Identified By", fmt.Sprintf("unrecognised value %q — identified_by_type left null", raw))
	}

	// ── dates ─────────────────────────────────────────────────────────────
	if d, err := parseDate(get(rec, idx, "Implementation Date")); err != nil {
		reject("Implementation Date", err.Error())
	} else if d == "" {
		reject("Implementation Date", "empty — a real deadline is required")
	} else {
		row.ImplementationDate = d
	}
	if d, err := parseDate(get(rec, idx, "Risk Identified Date")); err != nil {
		warn("Risk Identified Date", err.Error()+" — left null")
	} else {
		row.RiskIdentifiedDate = d
	}
	if d, err := parseDate(get(rec, idx, "Reassessment Date")); err != nil {
		warn("Reassessment Date", err.Error()+" — left null")
	} else {
		row.ReassessmentDate = d
	}

	// ── git issue url (placeholder scrub) ────────────────────────────────
	if raw := strings.TrimSpace(get(rec, idx, "Git Issue URL")); raw != "" {
		if u, ok := scrubHTTPURL(raw); ok {
			row.GitIssueURL = u
		} else {
			warn("Git Issue URL", fmt.Sprintf("%q is not an http(s) URL — dropped", raw))
		}
	}

	// ── reference-data lookups (RefData from preflight, T3) ──────────────
	if cat := strings.TrimSpace(get(rec, idx, "Risk Category")); cat == "" {
		reject("Risk Category", "empty")
	} else {
		row.RiskCategory = cat
		if id, ok := refs.CategoryIDByName[strings.ToLower(cat)]; ok {
			row.RiskCategoryIDs = []int{id}
		} else {
			reject("Risk Category", fmt.Sprintf("unknown category %q", cat))
		}
	}

	// The register is the one chosen for the run. A sheet that names one anyway
	// must name that one, so a file cut for a different register is caught here
	// instead of being quietly imported into this one.
	row.SourceRegisterID = tgt.RegisterID
	row.SourceRegister = tgt.Name
	if sr := strings.TrimSpace(get(rec, idx, "Source Register")); sr != "" {
		if id, ok := refs.TeamIDByKey[strings.ToLower(sr)]; !ok || id != tgt.RegisterID {
			reject("Source Register", fmt.Sprintf("%q is not the register this file is being imported into (%s)", sr, tgt.Name))
		}
	}

	if at := strings.TrimSpace(get(rec, idx, "Assignment Team")); at == "" {
		reject("Assignment Team", "empty")
	} else {
		row.AssignmentTeam = at
		if id, ok := refs.TeamIDByKey[strings.ToLower(at)]; ok {
			// Any active team may be the assignment team: the entity only checks it
			// exists (checkAssignmentTeam), because a register's template decides
			// which fields its risks carry, not who they can be assigned to.
			row.AssignmentTeamID = id
		} else {
			reject("Assignment Team", fmt.Sprintf("unknown team %q", at))
		}
	}

	// ── template fields: only the target register's template applies ─────
	// A column that belongs to another template must be blank, never silently
	// dropped: a filled Customer on a STANDARD register's sheet means the file
	// was cut for a different register.
	for _, col := range allTemplateColumns {
		if contains(templateColumns[tgt.Template], col) {
			continue
		}
		if v := strings.TrimSpace(get(rec, idx, col)); v != "" {
			reject(col, fmt.Sprintf("%q: the %s template does not use this column — clear it", v, tgt.Template))
		}
	}

	switch tgt.Template {
	case templateStandard:
		row.ComplianceRefs = dedupeFold(splitTokens(get(rec, idx, "Security Compliance Reference")))
		for _, tok := range row.ComplianceRefs {
			if id, ok := refs.ComplianceIDByName[normalizeComplianceToken(tok)]; ok {
				row.ComplianceRefIDs = append(row.ComplianceRefIDs, id)
			} else {
				reject("Security Compliance Reference", fmt.Sprintf("unknown reference %q", tok))
			}
		}

	case templateAggregated:
		row.Platforms = dedupeFold(splitTokens(get(rec, idx, "Platform")))
		if len(row.Platforms) == 0 {
			reject("Platform", "empty — at least one platform is required")
		}
		for _, p := range row.Platforms {
			ids := refs.PlatformIDByName[normHeader(p)]
			switch {
			case len(ids) == 1:
				row.PlatformIDs = append(row.PlatformIDs, ids[0])
			case len(ids) > 1:
				rejectLookup("Platform", fmt.Sprintf("platform %q is ambiguous — it exists under more than one parent", p), p, ProblemUnknown)
			case refs.InactiveLookup[lookupPlatform+":"+normHeader(p)]:
				rejectLookup("Platform", fmt.Sprintf("platform %q is inactive - reactivate it in the Admin Console", p), p, ProblemInactive)
			default:
				rejectLookup("Platform", fmt.Sprintf("unknown platform %q", p), p, ProblemUnknown)
			}
		}
		// The register's own rules (sub-platforms, the platforms it offers) are only
		// meaningful once every name has resolved: checked against a partial list
		// they would blame the wrong platform or report problems that vanish once
		// the unknown name is fixed.
		if len(row.PlatformIDs) == len(row.Platforms) {
			fs = append(fs, checkRegisterPlatforms(row, refs, tgt, line, title)...)
		}

	case templateManagedServices:
		row.Customer = strings.TrimSpace(get(rec, idx, "Customer"))
		if row.Customer == "" {
			reject("Customer", "empty")
		} else if id, problem, detail := lookupID(refs.CustomerIDByName, refs, lookupCustomer, row.Customer); problem != "" {
			rejectLookup("Customer", detail, row.Customer, problem)
		} else {
			row.CustomerID = id
		}

		row.DeploymentType = strings.TrimSpace(get(rec, idx, "Deployment Type"))
		if row.DeploymentType == "" {
			reject("Deployment Type", "empty")
		} else if id, problem, detail := lookupID(refs.DeploymentTypeIDByName, refs, lookupDeploymentType, row.DeploymentType); problem != "" {
			rejectLookup("Deployment Type", detail, row.DeploymentType, problem)
		} else {
			row.DeploymentTypeID = id
		}

		row.Products = dedupeFold(splitTokens(get(rec, idx, "Product")))
		if len(row.Products) == 0 {
			reject("Product", "empty — at least one product is required")
		}
		for _, p := range row.Products {
			if id, problem, detail := lookupID(refs.ProductIDByName, refs, lookupProduct, p); problem != "" {
				rejectLookup("Product", detail, p, problem)
			} else {
				row.ProductIDs = append(row.ProductIDs, id)
			}
		}

		envTokens := dedupeFold(splitTokens(get(rec, idx, "Environment")))
		if len(envTokens) == 0 {
			reject("Environment", "empty — at least one environment is required")
		}
		for _, e := range envTokens {
			if env, ok := environmentByName[normHeader(e)]; ok {
				row.Environments = append(row.Environments, env)
			} else {
				rejectLookup("Environment", fmt.Sprintf("%q is not one of Production, Non-Production, DR", e), e, ProblemUnknown)
			}
		}

	}

	return row, fs
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// checkRegisterPlatforms applies the entity's platform rules to one row's
// chosen platforms, so the dry run reports what a real run would be refused for
// (the entity's own check, risk_template.go checkRegisterPlatforms, is the
// authority; this mirrors it):
//   - a sub-platform needs its platform on the row too;
//   - when the register offers a list, every platform must be on it;
//   - and a platform whose active sub-platforms the register offers needs at
//     least one of them on the row.
//
// It reports only ids that resolved; unresolved names already have a finding.
func checkRegisterPlatforms(row Row, refs RefData, tgt Target, line int, title string) []Finding {
	var fs []Finding
	reject := func(detail string) {
		fs = append(fs, Finding{MigrationID: row.MigrationID, CSVRow: line, RiskTitle: title,
			Severity: SevReject, Failure: "Platform", Detail: detail})
	}
	onRow := map[int]bool{}
	for _, id := range row.PlatformIDs {
		onRow[id] = true
	}
	// Names come from the reference list, never from the sheet cell by position:
	// the row's ids are only the ones that resolved, so they do not line up with
	// the cell's tokens.
	label := func(id int) string {
		if n := refs.PlatformNameByID[id]; n != "" {
			return fmt.Sprintf("%q", n)
		}
		return fmt.Sprintf("platform %d", id)
	}

	for _, id := range row.PlatformIDs {
		if parent := refs.PlatformParentByID[id]; parent != 0 && !onRow[parent] {
			reject(fmt.Sprintf("sub-platform %s needs its platform %s selected too", label(id), label(parent)))
		}
	}

	offered := map[int]bool{}
	for _, id := range refs.TeamPlatformIDs[tgt.RegisterID] {
		offered[id] = true
	}
	if len(offered) == 0 {
		return fs // the register offers every active platform
	}
	for _, id := range row.PlatformIDs {
		if !offered[id] {
			reject(fmt.Sprintf("platform %s is not offered by %s", label(id), tgt.Name))
		}
	}
	for _, id := range row.PlatformIDs {
		if refs.PlatformParentByID[id] != 0 {
			continue // a sub-platform
		}
		hasOfferedSub, hasChosenSub := false, false
		for sub := range offered {
			if refs.PlatformParentByID[sub] != id {
				continue
			}
			hasOfferedSub = true
			if onRow[sub] {
				hasChosenSub = true
			}
		}
		if hasOfferedSub && !hasChosenSub {
			reject(fmt.Sprintf("select at least one sub-platform of %s", label(id)))
		}
	}
	return fs
}

// environmentByName maps the three accepted spellings (normHeader form) onto
// the entity's fixed environment enum. Anything else — "Prod", "UAT",
// "Pre-production" — is rejected, not guessed at.
var environmentByName = map[string]string{
	"production":     "PRODUCTION",
	"non-production": "NON_PRODUCTION",
	"dr":             "DR",
}

// lookupID resolves one value against a lookup map keyed by normHeader(name).
// A failure returns the problem (ProblemUnknown | ProblemInactive) and a
// human-readable detail, never an id, so the REJECT names the exact value the
// admins need to add or the sheet owner fix.
func lookupID(m map[string]int, refs RefData, kind, name string) (id int, problem, detail string) {
	key := normHeader(name)
	if id, ok := m[key]; ok {
		return id, "", ""
	}
	if refs.InactiveLookup[kind+":"+key] {
		return 0, ProblemInactive, fmt.Sprintf("%s %q is inactive - reactivate it in the Admin Console", kind, name)
	}
	return 0, ProblemUnknown, fmt.Sprintf("unknown %s %q", kind, name)
}

// dedupeFold drops repeats, ignoring case and inner spacing, keeping first-seen
// order.
func dedupeFold(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		k := normHeader(s)
		if !seen[k] {
			seen[k] = true
			out = append(out, s)
		}
	}
	return out
}

// splitTokens splits a multi-value cell on semicolon, comma or newline. The
// handoff doc asks for semicolons; the other two are tolerated.
func splitTokens(s string) []string {
	f := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == '\n' })
	out := make([]string, 0, len(f))
	for _, t := range f {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// splitSteps splits an action-steps cell on newlines, keeping any "1." / "2."
// prefixes as written.
func splitSteps(s string) []string {
	var out []string
	for _, ln := range strings.Split(s, "\n") {
		if ln = strings.TrimSpace(ln); ln != "" {
			out = append(out, ln)
		}
	}
	return out
}

func get(rec []string, idx map[string]int, col string) string {
	i, ok := idx[normHeader(col)]
	if !ok || i >= len(rec) {
		return ""
	}
	return rec[i]
}

func sortByMigrationID(rows []Row) {
	sort.Slice(rows, func(i, j int) bool { return rows[i].MigrationID < rows[j].MigrationID })
}
