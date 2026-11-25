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
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

// These tests cover the template-specific parts of the import: the STANDARD
// register (compliance references) and the AGGREGATED register (platforms). The
// Managed Services paths are covered by sheet_ms_test.go and the fixture tests.

const (
	idStdReg  = 1  // Asgardeo, STANDARD, code ASG
	idAggReg  = 11 // Cloud, AGGREGATED, code CLD
	idPlatCho = 61 // Choreo (top level)
	idPlatSub = 62 // Choreo Console (sub-platform of Choreo)
	idPlatOld = 63 // INACTIVE
	idPlatDup = 64 // a second "Gateway" under another parent
	idPlatGw1 = 65
	idPlatGw2 = 66
	idPlatIS  = 67 // Identity Server (top level, no sub-platforms)
)

func templateTeams(offered []int) []RiskTeam {
	return []RiskTeam{
		{ID: idStdReg, Name: "Asgardeo", Code: strptr("ASG"), Status: "ACTIVE", RegisterTemplate: "STANDARD"},
		{ID: idAggReg, Name: "Cloud", Code: strptr("CLD"), Status: "ACTIVE", RegisterTemplate: "AGGREGATED", PlatformIDs: offered},
	}
}

func templateLookups() TemplateLookups {
	parentCho, parentIS := idPlatCho, idPlatIS
	return TemplateLookups{
		Platforms: []RiskLookup{
			{ID: idPlatCho, Name: "Choreo", Status: "ACTIVE"},
			{ID: idPlatSub, Name: "Choreo Console", Status: "ACTIVE", ParentID: &parentCho},
			{ID: idPlatOld, Name: "Legacy", Status: "INACTIVE"},
			{ID: idPlatIS, Name: "Identity Server", Status: "ACTIVE"},
			{ID: idPlatGw1, Name: "Gateway", Status: "ACTIVE", ParentID: &parentCho},
			{ID: idPlatGw2, Name: "Gateway", Status: "ACTIVE", ParentID: &parentIS},
			{ID: idPlatDup, Name: "Duplicate Parent", Status: "ACTIVE"},
		},
		ComplianceRefs: []ComplianceRef{{ID: 51, Name: "SOC2"}, {ID: 52, Name: "HIPAA"}, {ID: 53, Name: "PCI DSS"}},
	}
}

func templateRefData(t *testing.T, offered []int) RefData {
	t.Helper()
	cats := []RiskCategory{{ID: 3, Name: "Access Control & Credentials"}}
	scores := []RiskScore{{ID: 7, Likelihood: 3, Impact: 2}}
	rd, err := buildRefData(templateTeams(offered), cats, scores, goodRoles(), templateLookups())
	if err != nil {
		t.Fatalf("buildRefData: %v", err)
	}
	return rd
}

// plainRow is a clean row with no template fields: the STANDARD register's, and
// the starting point for the others.
func plainRow(overrides map[string]string) map[string]string {
	r := map[string]string{
		"Year":                "2025",
		"Quarter":             "Q3",
		"Risk Title":          "TLS 1.0 still accepted",
		"Risk Identified By":  "Employee",
		"Risk Category":       "Access Control & Credentials",
		"Risk Assigned To":    "user1@wso2.com",
		"Gross Likelihood":    "3",
		"Gross Impact":        "2",
		"Residual Likelihood": "3",
		"Residual Impact":     "2",
		"Implementation Date": "2025-06-30",
		"Assignment Team":     "Asgardeo",
		"Risk Owner":          "user2@wso2.com",
		"Management Approver": "user3@wso2.com",
		"Action Owner":        "user4@wso2.com",
		"Action Steps":        "Do the thing",
		"Treatment Strategy":  "Remediate",
		"Workflow Status":     "APPROVED",
		"Migration ID":        "1",
	}
	for k, v := range overrides {
		r[k] = v
	}
	return r
}

// templateCSV renders the template's own download header plus the given rows.
func templateCSV(t *testing.T, template string, rows ...map[string]string) string {
	t.Helper()
	header := TemplateHeaders(template)
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write(header); err != nil {
		t.Fatal(err)
	}
	for _, cells := range rows {
		rec := make([]string, len(header))
		for i, h := range header {
			rec[i] = cells[h]
		}
		if err := w.Write(rec); err != nil {
			t.Fatal(err)
		}
	}
	w.Flush()
	return buf.String()
}

func parseTemplate(t *testing.T, template string, rd RefData, rows ...map[string]string) ([]Row, []Finding) {
	t.Helper()
	reg := idStdReg
	if template == templateAggregated {
		reg = idAggReg
	}
	tgt := Target{RegisterID: reg, Name: rd.TeamNameByID[reg], Template: template}
	got, fs, err := parseSheet(strings.NewReader(templateCSV(t, template, rows...)), rd, tgt)
	if err != nil {
		t.Fatalf("parseSheet: %v", err)
	}
	return got, fs
}

func rejectsFor(fs []Finding, failure string) []Finding {
	var out []Finding
	for _, f := range fs {
		if f.Severity == SevReject && f.Failure == failure {
			out = append(out, f)
		}
	}
	return out
}

func wantRejectContaining(t *testing.T, fs []Finding, failure, substr string) {
	t.Helper()
	got := rejectsFor(fs, failure)
	if len(got) == 0 {
		t.Fatalf("no REJECT for %q; findings = %+v", failure, fs)
	}
	for _, f := range got {
		if strings.Contains(f.Detail, substr) {
			return
		}
	}
	t.Errorf("REJECT for %q = %+v, want a detail containing %q", failure, got, substr)
}

// ── the template download ───────────────────────────────────────────────────

func TestTemplateHeaders(t *testing.T) {
	for _, tc := range []struct {
		template string
		after    []string // the columns right after Year, Quarter
	}{
		{templateStandard, []string{"Security Compliance Reference"}},
		{templateAggregated, []string{"Platform"}},
		{templateManagedServices, []string{"Customer", "Deployment Type", "Product", "Environment"}},
	} {
		h := TemplateHeaders(tc.template)
		if h[0] != "Year" || h[1] != "Quarter" {
			t.Errorf("%s: header starts %v", tc.template, h[:2])
		}
		if got := h[2 : 2+len(tc.after)]; !reflect.DeepEqual(got, tc.after) {
			t.Errorf("%s: template columns = %v, want %v", tc.template, got, tc.after)
		}
		for _, other := range allTemplateColumns {
			if contains(tc.after, other) {
				continue
			}
			if contains(h, other) {
				t.Errorf("%s: header carries %q, which belongs to another template", tc.template, other)
			}
		}
		if contains(h, "Source Register") {
			t.Errorf("%s: the register is chosen in the UI, the download must not carry Source Register", tc.template)
		}
		if _, err := mapHeader(h, tc.template); err != nil {
			t.Errorf("%s: the download is not a valid sheet for its own template: %v", tc.template, err)
		}
	}
}

// ── STANDARD ────────────────────────────────────────────────────────────────

func TestStandard_CleanRowWithAndWithoutComplianceReferences(t *testing.T) {
	rd := templateRefData(t, nil)
	rows, fs := parseTemplate(t, templateStandard, rd,
		plainRow(map[string]string{"Security Compliance Reference": "SOC2; hipaa", "Migration ID": "1"}),
		plainRow(map[string]string{"Risk Title": "Other", "Migration ID": "2"}))
	if len(fs) != 0 {
		t.Fatalf("findings = %+v, want none", fs)
	}
	if !reflect.DeepEqual(rows[0].ComplianceRefIDs, []int{51, 52}) {
		t.Errorf("compliance refs = %v, want [51 52] (case-insensitive)", rows[0].ComplianceRefIDs)
	}
	if len(rows[1].ComplianceRefIDs) != 0 {
		t.Errorf("a blank compliance cell must give no references: %v", rows[1].ComplianceRefIDs)
	}
	if rows[0].SourceRegisterID != idStdReg || rows[0].SourceRegister != "Asgardeo" || rows[0].Template != templateStandard {
		t.Errorf("the register must come from the run: %+v", rows[0])
	}
}

func TestStandard_ComplianceAliasesAndUnknown(t *testing.T) {
	rd := templateRefData(t, nil)
	rows, fs := parseTemplate(t, templateStandard, rd,
		plainRow(map[string]string{"Security Compliance Reference": "pci; HIPPA"}))
	if len(fs) != 0 || !reflect.DeepEqual(rows[0].ComplianceRefIDs, []int{53, 52}) {
		t.Errorf("aliases: refs = %v, findings = %+v", rows[0].ComplianceRefIDs, fs)
	}
	_, fs = parseTemplate(t, templateStandard, rd,
		plainRow(map[string]string{"Security Compliance Reference": "SOC2; ISO-NOPE"}))
	wantRejectContaining(t, fs, "Security Compliance Reference", `unknown reference "ISO-NOPE"`)
}

// A STANDARD sheet that carries a column of another template is a file cut for
// a different register. A filled one is rejected, never quietly dropped.
func TestStandard_ForeignTemplateColumnsMustBeBlank(t *testing.T) {
	rd := templateRefData(t, nil)
	for _, col := range []string{"Customer", "Platform", "Product", "Environment", "Deployment Type"} {
		header := append(TemplateHeaders(templateStandard), col)
		var buf bytes.Buffer
		w := csv.NewWriter(&buf)
		_ = w.Write(header)
		rec := make([]string, len(header))
		for i, h := range header {
			rec[i] = plainRow(nil)[h]
		}
		rec[len(rec)-1] = "something"
		_ = w.Write(rec)
		w.Flush()
		tgt := Target{RegisterID: idStdReg, Name: "Asgardeo", Template: templateStandard}
		_, fs, err := parseSheet(strings.NewReader(buf.String()), rd, tgt)
		if err != nil {
			t.Fatalf("%s: parseSheet: %v", col, err)
		}
		wantRejectContaining(t, fs, col, "does not use this column")
	}
}

// ── AGGREGATED ──────────────────────────────────────────────────────────────

func TestAggregated_PlatformIsRequiredAndResolved(t *testing.T) {
	rd := templateRefData(t, nil)
	rows, fs := parseTemplate(t, templateAggregated, rd,
		plainRow(map[string]string{"Platform": "Choreo; choreo console"}))
	if len(fs) != 0 {
		t.Fatalf("findings = %+v, want none", fs)
	}
	if !reflect.DeepEqual(rows[0].PlatformIDs, []int{idPlatCho, idPlatSub}) {
		t.Errorf("platform ids = %v", rows[0].PlatformIDs)
	}

	_, fs = parseTemplate(t, templateAggregated, rd, plainRow(map[string]string{"Platform": ""}))
	wantRejectContaining(t, fs, "Platform", "at least one platform is required")
}

func TestAggregated_UnknownInactiveAndAmbiguousPlatforms(t *testing.T) {
	rd := templateRefData(t, nil)
	for _, tc := range []struct {
		cell, want, problem string
	}{
		{"Nowhere", `unknown platform "Nowhere"`, ProblemUnknown},
		{"Legacy", "is inactive", ProblemInactive},
		{"Gateway", "ambiguous", ProblemUnknown},
	} {
		_, fs := parseTemplate(t, templateAggregated, rd, plainRow(map[string]string{"Platform": tc.cell}))
		wantRejectContaining(t, fs, "Platform", tc.want)
		if got := rejectsFor(fs, "Platform")[0]; got.Problem != tc.problem || got.Value != tc.cell {
			t.Errorf("%q: problem=%q value=%q, want %q/%q (the report groups lookups by these)", tc.cell, got.Problem, got.Value, tc.problem, tc.cell)
		}
	}
}

func TestAggregated_SubPlatformNeedsItsPlatform(t *testing.T) {
	rd := templateRefData(t, nil)
	_, fs := parseTemplate(t, templateAggregated, rd, plainRow(map[string]string{"Platform": "Choreo Console"}))
	wantRejectContaining(t, fs, "Platform", `sub-platform "Choreo Console" needs its platform "Choreo" selected too`)
}

// An unknown name is the one thing to fix first. The register's own rules, run
// against what did resolve, used to add messages that blamed the wrong platform
// (the names were matched to ids by position, and an unresolved one shifts them).
func TestAggregated_UnknownPlatformIsNotBuriedInRegisterRuleNoise(t *testing.T) {
	rd := templateRefData(t, []int{idPlatCho, idPlatSub})
	_, fs := parseTemplate(t, templateAggregated, rd, plainRow(map[string]string{"Platform": "Nowhere; Choreo Console"}))

	got := rejectsFor(fs, "Platform")
	if len(got) != 1 || !strings.Contains(got[0].Detail, `unknown platform "Nowhere"`) {
		t.Fatalf("want exactly the unknown-platform REJECT, got %+v", got)
	}
}

// Every message names the platform it is about, by its real name.
func TestAggregated_RegisterRuleMessagesNameTheRightPlatform(t *testing.T) {
	// The register offers only Choreo and Identity Server.
	rd := templateRefData(t, []int{idPlatCho, idPlatIS})
	_, fs := parseTemplate(t, templateAggregated, rd, plainRow(map[string]string{"Platform": "Choreo Console; Identity Server"}))

	var details []string
	for _, f := range rejectsFor(fs, "Platform") {
		details = append(details, f.Detail)
	}
	joined := strings.Join(details, " | ")
	for _, want := range []string{
		`sub-platform "Choreo Console" needs its platform "Choreo" selected too`,
		`platform "Choreo Console" is not offered by Cloud`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in: %s", want, joined)
		}
	}
	if strings.Contains(joined, `"Identity Server" is not offered`) || strings.Contains(joined, `"Identity Server" needs`) {
		t.Errorf("Identity Server is offered and top-level, it must not be blamed: %s", joined)
	}
}

func TestAggregated_RegisterThatOffersAListIsEnforced(t *testing.T) {
	// The register offers Choreo, its Console sub-platform and Identity Server.
	rd := templateRefData(t, []int{idPlatCho, idPlatSub, idPlatIS})

	_, fs := parseTemplate(t, templateAggregated, rd, plainRow(map[string]string{"Platform": "Choreo; Choreo Console"}))
	if len(fs) != 0 {
		t.Errorf("an offered platform with its sub-platform must pass: %+v", fs)
	}
	_, fs = parseTemplate(t, templateAggregated, rd, plainRow(map[string]string{"Platform": "Identity Server"}))
	if len(fs) != 0 {
		t.Errorf("a platform with no offered sub-platform needs none: %+v", fs)
	}
	_, fs = parseTemplate(t, templateAggregated, rd, plainRow(map[string]string{"Platform": "Duplicate Parent"}))
	wantRejectContaining(t, fs, "Platform", "is not offered by Cloud")
	_, fs = parseTemplate(t, templateAggregated, rd, plainRow(map[string]string{"Platform": "Choreo"}))
	wantRejectContaining(t, fs, "Platform", "select at least one sub-platform")
}

func TestAggregated_RegisterWithoutAListOffersEveryPlatform(t *testing.T) {
	rd := templateRefData(t, nil)
	_, fs := parseTemplate(t, templateAggregated, rd, plainRow(map[string]string{"Platform": "Choreo"}))
	if len(fs) != 0 {
		t.Errorf("with no list, a top-level platform alone is fine: %+v", fs)
	}
}

// ── what is written ─────────────────────────────────────────────────────────

func TestBuildCreateRiskRequest_OnlyTheTemplatesFields(t *testing.T) {
	base := Row{
		RiskTitle: "T", SourceRegisterID: 1, AssignmentTeamID: 1, RiskYear: 2025, RiskQuarter: "Q3",
		GrossLikelihood: 3, GrossImpact: 2, ActionSteps: []string{"x"},
		ComplianceRefIDs: []int{51}, PlatformIDs: []int{61}, CustomerID: 21, DeploymentTypeID: 41,
		ProductIDs: []int{31}, Environments: []string{"DR"},
	}
	std := base
	std.Template = templateStandard
	if req := buildCreateRiskRequest(std); !reflect.DeepEqual(req.ComplianceReferenceIDs, []int{51}) ||
		req.PlatformIDs != nil || req.CustomerID != nil || req.DeploymentTypeID != nil || req.ProductIDs != nil || req.Environments != nil {
		t.Errorf("STANDARD must send compliance references only: %+v", req)
	}

	agg := base
	agg.Template = templateAggregated
	if req := buildCreateRiskRequest(agg); !reflect.DeepEqual(req.PlatformIDs, []int{61}) ||
		req.ComplianceReferenceIDs != nil || req.CustomerID != nil || req.DeploymentTypeID != nil || req.ProductIDs != nil || req.Environments != nil {
		t.Errorf("AGGREGATED must send platforms only: %+v", req)
	}

	ms := base
	ms.Template = templateManagedServices
	req := buildCreateRiskRequest(ms)
	if req.CustomerID == nil || *req.CustomerID != 21 || req.DeploymentTypeID == nil || !reflect.DeepEqual(req.ProductIDs, []int{31}) ||
		!reflect.DeepEqual(req.Environments, []string{"DR"}) || req.ComplianceReferenceIDs != nil || req.PlatformIDs != nil {
		t.Errorf("MANAGED_SERVICES must send its four fields only: %+v", req)
	}
}

// ── risk codes ──────────────────────────────────────────────────────────────

func TestRiskCode_RegisterWithoutCustomersHasNoCustomerSegment(t *testing.T) {
	if got := riskCode(2025, "ASG", "", "Q3", 7); got != "2025-ASG-Q3-0007" {
		t.Errorf("riskCode = %q, want 2025-ASG-Q3-0007", got)
	}
	if got := riskCode(2025, "MS", "BO", "Q3", 7); got != "2025-MS-BO-Q3-0007" {
		t.Errorf("a Managed Services code keeps the customer: %q", got)
	}
	if !riskCodeHasPrefix("2025-ASG-Q3-0007", riskCodePrefix(2025, "ASG", "", "Q3")) {
		t.Error("the prefix of a register-counter code must match it")
	}
}

func TestSummaries_RegisterWithoutCustomersIsOneSequence(t *testing.T) {
	rd := templateRefData(t, nil)
	rows := []Row{
		{MigrationID: 1, Template: templateStandard, SourceRegisterID: idStdReg, SourceRegister: "Asgardeo", RiskYear: 2025, RiskQuarter: "Q1", WorkflowStatus: "APPROVED"},
		{MigrationID: 2, Template: templateStandard, SourceRegisterID: idStdReg, SourceRegister: "Asgardeo", RiskYear: 2026, RiskQuarter: "Q2", WorkflowStatus: "CLOSED"},
	}
	progress := map[int]ResumeState{1: {Progress: ProgressNone}, 2: {Progress: ProgressNone}}
	got := buildCustomerSummaries(rows, rd, progress, map[int]int{-1: 40})
	if len(got) != 1 {
		t.Fatalf("summaries = %+v, want one line for the whole register", got)
	}
	s := got[0]
	if s.Customer != "Asgardeo" || s.Risks != 2 || s.New != 2 || s.FirstCode != "2025-ASG-Q1-0040" || s.LastCode != "2026-ASG-Q2-0041" {
		t.Errorf("summary = %+v, want the counter to run across years and quarters", s)
	}
}

func TestComputeSummaries_ReadsTheRegisterCounterWithoutACustomer(t *testing.T) {
	rd := templateRefData(t, nil)
	fe := newFakeEntity(t)
	ec := fe.client(t)
	rows := []Row{{MigrationID: 1, Template: templateStandard, SourceRegisterID: idStdReg, SourceRegister: "Asgardeo", RiskYear: 2025, RiskQuarter: "Q1", WorkflowStatus: "APPROVED"}}
	if _, err := computeCustomerSummaries(context.Background(), ec, rd, rows, map[int]ResumeState{1: {Progress: ProgressNone}}); err != nil {
		t.Fatal(err)
	}
	if len(fe.nextSeqQueries) != 1 || strings.Contains(fe.nextSeqQueries[0], "customerId") {
		t.Errorf("queries = %v; the entity rejects a customer on a register that has none", fe.nextSeqQueries)
	}
}

// ── the whole run, through the service ──────────────────────────────────────

// newRunService serves the preflight lists from the canned reference stub and
// everything else from the stateful fake, so Service.Run exercises preflight,
// parse, resolve, resume state, write and verify together.
func newRunService(t *testing.T, fe *fakeEntity) *Service {
	t.Helper()
	ref := entityRefStub(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /health", "GET /roles", "POST /risk/teams/search", "GET /risk/categories", "GET /risk/customers",
			"GET /risk/products", "GET /risk/deployment-types", "GET /risk/platforms",
			"POST /risk/compliance-references/search", "GET /risk/scores":
			ref(w, r)
		default:
			fe.handle(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	fe.teamCodes[idStdReg] = "ASG"
	fe.teamCodes[idAggReg] = "CLD"
	return NewService(NewEntityClient(srv.URL, 5*time.Second), snapshotLookup(fixtureSnapshot()), nil)
}

func runRequest(t *testing.T, registerID int, template string, dryRun bool, rows ...map[string]string) Request {
	t.Helper()
	return Request{
		RegisterID: registerID, DryRun: dryRun, MigrationDate: "2026-09-15",
		CSV: strings.NewReader(templateCSV(t, template, rows...)),
	}
}

func TestRun_StandardDryRunWritesNothingAndPreviewsTheCodes(t *testing.T) {
	fe := newFakeEntity(t)
	svc := newRunService(t, fe)

	res, err := svc.Run(context.Background(), runRequest(t, idStdReg, templateStandard, true,
		plainRow(map[string]string{"Security Compliance Reference": "SOC2"})))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Outcome != OutcomeClean || res.Target.Template != templateStandard || res.Rows != 1 {
		t.Fatalf("result = %+v, report = %+v", res, res.Report.Findings())
	}
	if len(fe.createdRisks) != 0 || len(fe.usersCreated) != 0 || len(fe.grantPosts) != 0 {
		t.Errorf("a dry run wrote: risks=%d users=%d grants=%d", len(fe.createdRisks), len(fe.usersCreated), len(fe.grantPosts))
	}
	var narrative bytes.Buffer
	res.Report.WriteNarrative(&narrative)
	if !strings.Contains(narrative.String(), "2025-ASG-Q3-0001") {
		t.Errorf("the dry run must preview the first code the run would assign:\n%s", narrative.String())
	}
}

func TestRun_StandardRealRunThenSecondRunIsANoOp(t *testing.T) {
	fe := newFakeEntity(t)
	svc := newRunService(t, fe)
	rows := []map[string]string{
		plainRow(map[string]string{"Security Compliance Reference": "SOC2; HIPAA", "Migration ID": "1"}),
		plainRow(map[string]string{"Risk Title": "Second", "Workflow Status": "CLOSED", "Migration ID": "2"}),
	}

	res, err := svc.Run(context.Background(), runRequest(t, idStdReg, templateStandard, false, rows...))
	if err != nil || res.Outcome != OutcomeClean {
		t.Fatalf("Run: outcome=%v err=%v findings=%+v", res.Outcome, err, res.Report.Findings())
	}
	if len(fe.createdRisks) != 2 {
		t.Fatalf("created %d risks, want 2", len(fe.createdRisks))
	}
	req := fe.createdRisks[0]
	if !reflect.DeepEqual(req.ComplianceReferenceIDs, []int{51, 52}) || req.CustomerID != nil || req.PlatformIDs != nil {
		t.Errorf("STANDARD request carried the wrong template fields: %+v", req)
	}
	if req.CreatedBy != marker || req.SourceRegisterID != idStdReg {
		t.Errorf("request = %+v", req)
	}
	var codes bytes.Buffer
	res.Report.WriteRiskCodesCSV(&codes)
	if !strings.Contains(codes.String(), "2025-ASG-Q3-0001") || !strings.Contains(codes.String(), "2025-ASG-Q3-0002") {
		t.Errorf("risk_codes.csv:\n%s", codes.String())
	}

	// The same file again: every row is already there.
	before := len(fe.createdRisks)
	res, err = svc.Run(context.Background(), runRequest(t, idStdReg, templateStandard, false, rows...))
	if err != nil || res.Outcome != OutcomeClean {
		t.Fatalf("second Run: outcome=%v err=%v findings=%+v", res.Outcome, err, res.Report.Findings())
	}
	if len(fe.createdRisks) != before {
		t.Errorf("the second run created %d more risks, want 0", len(fe.createdRisks)-before)
	}
	if got := res.Report.Counts(); got.Skipped != 2 || got.Migrated != 0 {
		t.Errorf("counts = %+v, want 2 skipped and 0 migrated", got)
	}
}

func TestRun_AggregatedRealRunWritesPlatformsAndVerifiesThem(t *testing.T) {
	fe := newFakeEntity(t)
	svc := newRunService(t, fe)

	res, err := svc.Run(context.Background(), runRequest(t, idAggReg, templateAggregated, false,
		plainRow(map[string]string{"Platform": "Choreo; Choreo Console"})))
	if err != nil || res.Outcome != OutcomeClean {
		t.Fatalf("Run: outcome=%v err=%v findings=%+v", res.Outcome, err, res.Report.Findings())
	}
	req := fe.createdRisks[0]
	if !reflect.DeepEqual(req.PlatformIDs, []int{idPlatCho, idPlatSub}) || req.ComplianceReferenceIDs != nil || req.CustomerID != nil {
		t.Errorf("AGGREGATED request carried the wrong template fields: %+v", req)
	}
	if res.Report.Counts().Mismatches != 0 {
		t.Errorf("verify disagreed with what was written: %+v", res.Report.Findings())
	}
}

// The rule the feature rests on: a file with a bad row is never imported, not
// even its good rows.
func TestRun_RealRunWithARejectedRowWritesNothing(t *testing.T) {
	fe := newFakeEntity(t)
	svc := newRunService(t, fe)

	res, err := svc.Run(context.Background(), runRequest(t, idStdReg, templateStandard, false,
		plainRow(map[string]string{"Migration ID": "1"}),
		plainRow(map[string]string{"Risk Title": "Bad", "Quarter": "Q9", "Migration ID": "2"})))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Outcome != OutcomeFindings || res.Report.Counts().Rejects == 0 {
		t.Fatalf("outcome = %v, counts = %+v", res.Outcome, res.Report.Counts())
	}
	if len(fe.createdRisks) != 0 || len(fe.usersCreated) != 0 || len(fe.grantPosts) != 0 || len(fe.riskPatches) != 0 {
		t.Errorf("a file with a rejected row wrote: risks=%d users=%d grants=%d patches=%d",
			len(fe.createdRisks), len(fe.usersCreated), len(fe.grantPosts), len(fe.riskPatches))
	}
}

func TestRun_AnUnresolvablePersonBlocksTheRealRun(t *testing.T) {
	fe := newFakeEntity(t)
	svc := newRunService(t, fe)

	res, err := svc.Run(context.Background(), runRequest(t, idStdReg, templateStandard, false,
		plainRow(map[string]string{"Risk Owner": "ghost@wso2.com"})))
	if err != nil || res.Outcome != OutcomeFindings {
		t.Fatalf("outcome = %v, err = %v", res.Outcome, err)
	}
	if len(fe.createdRisks) != 0 {
		t.Error("a risk was created for a person who does not exist")
	}
}

func TestRun_FileCutForAnotherTemplateIsStructural(t *testing.T) {
	fe := newFakeEntity(t)
	svc := newRunService(t, fe)

	// The Managed Services header against a STANDARD register: Customer and the
	// rest are filled, so the rows are rejected instead of imported.
	res, err := svc.Run(context.Background(), Request{
		RegisterID: idStdReg, DryRun: true,
		CSV: strings.NewReader(templateCSV(t, templateManagedServices, msRow(nil))),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Outcome != OutcomeFindings || len(rejectsFor(res.Report.Findings(), "Customer")) == 0 {
		t.Errorf("outcome = %v, findings = %+v", res.Outcome, res.Report.Findings())
	}
}

func TestRun_StructuralFailures(t *testing.T) {
	fe := newFakeEntity(t)
	svc := newRunService(t, fe)

	_, err := svc.Run(context.Background(), Request{RegisterID: 999, DryRun: true, CSV: strings.NewReader("")})
	if !errors.Is(err, ErrStructural) {
		t.Errorf("unknown register: err = %v, want ErrStructural", err)
	}
	_, err = svc.Run(context.Background(), Request{RegisterID: idStdReg, DryRun: true, CSV: strings.NewReader("Year,Quarter\n2025,Q1\n")})
	if !errors.Is(err, ErrStructural) || !strings.Contains(err.Error(), "missing column") {
		t.Errorf("wrong header: err = %v", err)
	}
	_, err = svc.Run(context.Background(), Request{RegisterID: idStdReg, DryRun: true, CSV: strings.NewReader("")})
	if !errors.Is(err, ErrStructural) {
		t.Errorf("empty file: err = %v", err)
	}
}

func TestRun_RowLimit(t *testing.T) {
	fe := newFakeEntity(t)
	svc := newRunService(t, fe)

	req := runRequest(t, idStdReg, templateStandard, true,
		plainRow(map[string]string{"Migration ID": "1"}),
		plainRow(map[string]string{"Risk Title": "Two", "Migration ID": "2"}))
	req.MaxRows = 1
	_, err := svc.Run(context.Background(), req)
	if !errors.Is(err, ErrTooManyRows) {
		t.Fatalf("err = %v, want ErrTooManyRows", err)
	}
	if len(fe.createdRisks) != 0 {
		t.Error("nothing may be written for an oversized file")
	}
}

func TestRun_ReportsProgressPhasesInOrder(t *testing.T) {
	fe := newFakeEntity(t)
	svc := newRunService(t, fe)

	var phases []string
	req := runRequest(t, idStdReg, templateStandard, false, plainRow(nil))
	req.Progress = func(p Progress) {
		if len(phases) == 0 || phases[len(phases)-1] != p.Phase {
			phases = append(phases, p.Phase)
		}
	}
	if _, err := svc.Run(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if want := []string{"preflight", "parse", "resolve", "write", "verify"}; !reflect.DeepEqual(phases, want) {
		t.Errorf("phases = %v, want %v", phases, want)
	}
}

// ── the report artifacts ────────────────────────────────────────────────────

func TestReport_ErrorsCSVNeutralisesFormulaCells(t *testing.T) {
	rep := NewReport()
	rep.Add(Finding{MigrationID: 1, CSVRow: 2, RiskTitle: "=HYPERLINK(\"http://evil\")", Severity: SevReject, Failure: "Risk Owner", Detail: "+cmd"})
	var buf bytes.Buffer
	rep.WriteErrorsCSV(&buf)
	out := buf.String()
	if strings.Contains(out, ",=HYPERLINK") || strings.Contains(out, ",+cmd") {
		t.Errorf("a sheet-authored cell reached the download as a live formula:\n%s", out)
	}
	if !strings.Contains(out, "'=HYPERLINK") {
		t.Errorf("expected the quoted form:\n%s", out)
	}
}

func TestReport_CountsAndSeparateArtifacts(t *testing.T) {
	rep := NewReport()
	rep.Add(
		Finding{Severity: SevReject, Failure: "a"}, Finding{Severity: SevReject, Failure: "b"},
		Finding{Severity: SevWarn, Failure: "c"}, Finding{Severity: SevMismatch, Failure: "d"},
	)
	rep.Migrated("APPROVED")
	rep.Skipped()
	if got, want := rep.Counts(), (Counts{Migrated: 1, Skipped: 1, Rejects: 2, Warnings: 1, Mismatches: 1}); got != want {
		t.Errorf("counts = %+v, want %+v", got, want)
	}
	var codes bytes.Buffer
	rep.WriteRiskCodesCSV(&codes)
	if strings.TrimSpace(codes.String()) != "migration_id,customer,risk_code,risk_title" {
		t.Errorf("an empty risk_codes.csv is the header alone: %q", codes.String())
	}
}

// ── what a dry run says a real run would do ─────────────────────────────────

func TestRun_DryRunSaysWhatTheImportWouldWrite(t *testing.T) {
	fe := newFakeEntity(t)
	svc := newRunService(t, fe)
	first := plainRow(map[string]string{"Migration ID": "1"})
	second := plainRow(map[string]string{"Risk Title": "Second", "Workflow Status": "CLOSED", "Migration ID": "2"})
	third := plainRow(map[string]string{"Risk Title": "Third", "Migration ID": "3"})

	plan := func(rows ...map[string]string) Counts {
		t.Helper()
		res, err := svc.Run(context.Background(), runRequest(t, idStdReg, templateStandard, true, rows...))
		if err != nil || res.Outcome != OutcomeClean {
			t.Fatalf("dry run: outcome=%v err=%v findings=%+v", res.Outcome, err, res.Report.Findings())
		}
		return res.Report.Counts()
	}

	// Nothing imported yet: every row is new.
	if c := plan(first, second); !c.PlanKnown || c.ToWrite != 2 || c.AlreadyDone != 0 {
		t.Errorf("before any import: counts = %+v, want 2 to write, 0 already done", c)
	}

	if res, err := svc.Run(context.Background(), runRequest(t, idStdReg, templateStandard, false, first, second)); err != nil || res.Outcome != OutcomeClean {
		t.Fatalf("import: outcome=%v err=%v", res.Outcome, err)
	}
	usersAfter := len(fe.usersCreated)

	// The same file again: the dry run says there is nothing left to do.
	if c := plan(first, second); c.ToWrite != 0 || c.AlreadyDone != 2 {
		t.Errorf("same file again: counts = %+v, want 0 to write, 2 already done", c)
	}
	// A file with one new row: only that one would be written.
	if c := plan(first, second, third); c.ToWrite != 1 || c.AlreadyDone != 2 {
		t.Errorf("one new row: counts = %+v, want 1 to write, 2 already done", c)
	}

	created := len(fe.createdRisks)
	plan(first, second, third)
	if len(fe.createdRisks) != created {
		t.Error("a dry run created a risk")
	}
	if len(fe.usersCreated) != usersAfter {
		t.Errorf("the dry runs after the import provisioned %d more users", len(fe.usersCreated)-usersAfter)
	}
}

// A person with no user row yet must not be provisioned by a dry run, and the
// rows that name them read as still to be written.
func TestRun_DryRunPlanDoesNotProvisionUsers(t *testing.T) {
	fe := newFakeEntity(t)
	svc := newRunService(t, fe)

	res, err := svc.Run(context.Background(), runRequest(t, idStdReg, templateStandard, true, plainRow(nil)))
	if err != nil {
		t.Fatal(err)
	}
	if len(fe.usersCreated) != 0 || len(fe.grantPosts) != 0 || len(fe.createdRisks) != 0 {
		t.Errorf("a dry run wrote: users=%d grants=%d risks=%d", len(fe.usersCreated), len(fe.grantPosts), len(fe.createdRisks))
	}
	if c := res.Report.Counts(); c.ToWrite != 1 || c.AlreadyDone != 0 {
		t.Errorf("counts = %+v", c)
	}
}

func TestRun_DryRunWithARejectHasNoPlan(t *testing.T) {
	fe := newFakeEntity(t)
	svc := newRunService(t, fe)

	res, err := svc.Run(context.Background(), runRequest(t, idStdReg, templateStandard, true,
		plainRow(map[string]string{"Quarter": "Q9"})))
	if err != nil {
		t.Fatal(err)
	}
	if res.Report.Counts().PlanKnown {
		t.Error("a file with a rejected row cannot be imported, so there is no plan to report")
	}
}

func TestRun_DryRunNarrativeStatesThePlan(t *testing.T) {
	fe := newFakeEntity(t)
	svc := newRunService(t, fe)
	res, err := svc.Run(context.Background(), runRequest(t, idStdReg, templateStandard, true, plainRow(nil)))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	res.Report.WriteNarrative(&out)
	if !strings.Contains(out.String(), "plan: 1 to write, 0 already complete") {
		t.Errorf("report.txt:\n%s", out.String())
	}
}

// ── review fixes: rows and columns must not vanish or collide silently ──────

func TestMigrationID_MustBePositive(t *testing.T) {
	rd := templateRefData(t, nil)
	for _, id := range []string{"0", "0.0", "-3"} {
		_, fs := parseTemplate(t, templateStandard, rd, plainRow(map[string]string{"Migration ID": id}))
		wantRejectContaining(t, fs, "Migration ID", "positive")
	}
	_, fs := parseTemplate(t, templateStandard, rd, plainRow(map[string]string{"Migration ID": "3.0"}))
	if len(fs) != 0 {
		t.Errorf("3.0 is a valid Migration ID: %+v", fs)
	}
}

// Two rows that both carry ID 0 used to slip past the duplicate check (0 meant
// "unset") and then overwrote each other's resume state.
func TestMigrationID_ZeroTwiceIsRejectedNotMerged(t *testing.T) {
	rd := templateRefData(t, nil)
	_, fs := parseTemplate(t, templateStandard, rd,
		plainRow(map[string]string{"Migration ID": "0"}),
		plainRow(map[string]string{"Risk Title": "Other", "Migration ID": "0"}))
	if got := rejectsFor(fs, "Migration ID"); len(got) < 2 {
		t.Errorf("both rows must be rejected, got %+v", got)
	}
}

func TestMapHeader_DuplicateRecognisedColumnIsRefused(t *testing.T) {
	base := TemplateHeaders(templateStandard)
	for _, dup := range []string{"Risk Title", "risk  title", "Customer", "Source Register"} {
		header := append(append([]string{}, base...), dup)
		if !contains(base, dup) && normHeader(dup) != normHeader("Risk Title") {
			header = append(header, dup) // not in the base header: repeat it so it appears twice
		}
		_, err := mapHeader(header, templateStandard)
		if err == nil || !strings.Contains(err.Error(), "more than once") {
			t.Errorf("duplicate %q: err = %v, want it refused", dup, err)
		}
	}
	// Blank header cells and columns nobody reads may repeat freely.
	header := append(append([]string{}, base...), "", "", "Notes", "Notes")
	if _, err := mapHeader(header, templateStandard); err != nil {
		t.Errorf("repeated blank or unknown columns are harmless: %v", err)
	}
}

// A populated column hiding behind a second, blank one of the same name used to
// pass the wrong-template check, because the last column won.
func TestStandard_DuplicateForeignColumnCannotHideData(t *testing.T) {
	rd := templateRefData(t, nil)
	header := append(TemplateHeaders(templateStandard), "Customer", "Customer")
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write(header)
	rec := make([]string, len(header))
	for i, h := range header {
		rec[i] = plainRow(nil)[h]
	}
	rec[len(rec)-2] = "BankOne"
	_ = w.Write(rec)
	w.Flush()
	_, _, err := parseSheet(strings.NewReader(buf.String()), rd, Target{RegisterID: idStdReg, Name: "Asgardeo", Template: templateStandard})
	if err == nil || !strings.Contains(err.Error(), "more than once") {
		t.Fatalf("err = %v, want the duplicate column refused", err)
	}
}

func TestIsSkippable(t *testing.T) {
	header := TemplateHeaders(templateStandard)
	idx, err := mapHeader(header, templateStandard)
	if err != nil {
		t.Fatal(err)
	}
	rec := func(cells map[string]string) []string {
		r := make([]string, len(header))
		for i, h := range header {
			r[i] = cells[h]
		}
		return r
	}
	for _, tc := range []struct {
		name string
		rec  []string
		want bool
	}{
		{"fully blank", rec(nil), true},
		{"whitespace only", rec(map[string]string{"Year": "  ", "Remarks": " "}), true},
		{"the legend row", rec(map[string]string{"Quarter": "Required fields are coloured in yellow. Do not edit the header row."}), true},
		{"the legend in another column, with different case", rec(map[string]string{"Risk Title": "REQUIRED FIELDS ARE COLOURED"}), true},
		{"a risk whose description mentions the legend", rec(map[string]string{
			"Year": "2025", "Risk Title": "T", "Risk Description": "required fields are coloured in the form"}), false},
		{"data but no year and no title", rec(map[string]string{"Risk Owner": "a@wso2.com", "Migration ID": "7"}), false},
		{"an ordinary row", rec(map[string]string{"Year": "2025", "Risk Title": "T"}), false},
	} {
		if got := isSkippable(tc.rec, idx); got != tc.want {
			t.Errorf("%s: isSkippable = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// A row with data but no Year and no Risk Title is rejected with reasons. It
// used to be dropped without a word, so the admin never learned a row was lost.
func TestParse_RowWithDataButNoYearOrTitleIsRejectedNotDropped(t *testing.T) {
	rd := templateRefData(t, nil)
	rows, fs := parseTemplate(t, templateStandard, rd,
		plainRow(map[string]string{"Year": "", "Risk Title": "", "Migration ID": "4"}))
	if len(rows) != 1 {
		t.Fatalf("the row was dropped: rows = %d", len(rows))
	}
	wantRejectContaining(t, fs, "Risk Title", "empty")
	if len(rejectsFor(fs, "Year")) == 0 {
		t.Errorf("a missing Year must be a finding too: %+v", fs)
	}
}

func TestParse_RiskMentioningTheLegendIsImportedNotDropped(t *testing.T) {
	rd := templateRefData(t, nil)
	rows, fs := parseTemplate(t, templateStandard, rd,
		plainRow(map[string]string{"Risk Description": "Required fields are coloured in yellow on the form"}))
	if len(rows) != 1 || len(fs) != 0 {
		t.Errorf("rows = %d, findings = %+v", len(rows), fs)
	}
}
