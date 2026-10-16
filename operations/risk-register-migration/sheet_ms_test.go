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

package main

import (
	"bytes"
	"encoding/csv"
	"reflect"
	"strings"
	"testing"
)

// The tool imports the Managed Services register only (RISK_MODULE_DESIGN.md
// §14, Phase 2). These tests cover the sheet layer: the template columns, the
// lookups by name, and the rules that keep a row off any register that is not
// on the MANAGED_SERVICES template.

const (
	idRegStandard = 1  // Asgardeo, STANDARD
	idRegMS       = 10 // Managed Services (MS), MANAGED_SERVICES, BOTH
	idTeamSRE     = 11 // SRE One, MANAGED_SERVICES, ASSIGNMENT (no code)
	idRegCloud    = 12 // WSO2 Cloud, AGGREGATED

	idCustBankOne = 21
	idCustBOC     = 22
	idCustOldCo   = 23 // INACTIVE

	idProdAPIM = 31
	idProdMI   = 32
	idProdIS   = 33
	idProdOld  = 34 // INACTIVE

	idDeployPrivate = 41
	idDeployOnPrem  = 42
	idDeployOld     = 43 // INACTIVE
)

func msLookups() TemplateLookups {
	return TemplateLookups{
		Customers: []RiskLookup{
			{ID: idCustBankOne, Name: "BankOne", Code: strptr("BO"), Status: "ACTIVE"},
			{ID: idCustBOC, Name: "Bank Of China", Code: strptr("BOC"), Status: "ACTIVE"},
			{ID: idCustOldCo, Name: "OldCo", Code: strptr("OC"), Status: "INACTIVE"},
		},
		Products: []RiskLookup{
			{ID: idProdAPIM, Name: "APIM", Status: "ACTIVE"},
			{ID: idProdMI, Name: "MI", Status: "ACTIVE"},
			{ID: idProdIS, Name: "IS", Status: "ACTIVE"},
			{ID: idProdOld, Name: "OldProduct", Status: "INACTIVE"},
		},
		DeploymentTypes: []RiskLookup{
			{ID: idDeployPrivate, Name: "Private Cloud", Status: "ACTIVE"},
			{ID: idDeployOnPrem, Name: "Managed Services - Customer's On Prem", Status: "ACTIVE"},
			{ID: idDeployOld, Name: "Old Cloud", Status: "INACTIVE"},
		},
	}
}

func msTeams() []RiskTeam {
	return []RiskTeam{
		{ID: idRegStandard, Name: "Asgardeo", Code: strptr("ASG"), Status: "ACTIVE", RegisterTemplate: "STANDARD"},
		{ID: idRegMS, Name: "Managed Services", Code: strptr("MS"), Status: "ACTIVE", RegisterTemplate: "MANAGED_SERVICES"},
		{ID: idTeamSRE, Name: "SRE One", Code: nil, Status: "ACTIVE", RegisterTemplate: "MANAGED_SERVICES"},
		{ID: idRegCloud, Name: "WSO2 Cloud", Code: strptr("WSO2CLOUD"), Status: "ACTIVE", RegisterTemplate: "AGGREGATED"},
	}
}

func msRefData(t *testing.T) RefData {
	t.Helper()
	cats := []RiskCategory{{ID: 3, Name: "Access Control & Credentials"}}
	scores := []RiskScore{{ID: 7, Likelihood: 3, Impact: 2}}
	rd, err := buildRefData(msTeams(), cats, scores, goodRoles(), msLookups())
	if err != nil {
		t.Fatalf("buildRefData: %v", err)
	}
	return rd
}

// msRow is a clean Managed Services row; overrides replace single cells.
func msRow(overrides map[string]string) map[string]string {
	r := map[string]string{
		"Year":                "2025",
		"Quarter":             "Q3",
		"Source Register":     "Managed Services",
		"Customer":            "BankOne",
		"Deployment Type":     "Private Cloud",
		"Product":             "APIM; MI",
		"Environment":         "Production; DR",
		"Risk Title":          "TLS 1.0 still accepted",
		"Risk Identified By":  "Employee",
		"Risk Category":       "Access Control & Credentials",
		"Risk Assigned To":    "user1@wso2.com",
		"Gross Likelihood":    "3",
		"Gross Impact":        "2",
		"Residual Likelihood": "3",
		"Residual Impact":     "2",
		"Implementation Date": "2025-06-30",
		"Assignment Team":     "SRE One",
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

func parseOne(t *testing.T, cells map[string]string) (Row, []Finding) {
	t.Helper()
	rows, fs, err := parseSheet(strings.NewReader(buildCSV(t, cells)), msRefData(t))
	if err != nil {
		t.Fatalf("parseSheet: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	return rows[0], fs
}

func wantReject(t *testing.T, fs []Finding, failure, detailContains string) {
	t.Helper()
	got := findingsFor(fs, failure)
	if len(got) == 0 || got[0].Severity != SevReject {
		t.Fatalf("want a REJECT for %q, findings = %+v", failure, fs)
	}
	if !strings.Contains(got[0].Detail, detailContains) {
		t.Errorf("REJECT for %q detail = %q, want it to contain %q", failure, got[0].Detail, detailContains)
	}
}

// ── reference data ──────────────────────────────────────────────────────────

func TestBuildRefData_TemplateLookups(t *testing.T) {
	rd := msRefData(t)

	if rd.CustomerIDByName["bankone"] != idCustBankOne || rd.CustomerIDByName["bank of china"] != idCustBOC {
		t.Errorf("CustomerIDByName: %+v", rd.CustomerIDByName)
	}
	if rd.ProductIDByName["apim"] != idProdAPIM || rd.ProductIDByName["mi"] != idProdMI {
		t.Errorf("ProductIDByName: %+v", rd.ProductIDByName)
	}
	if rd.DeploymentTypeIDByName["private cloud"] != idDeployPrivate {
		t.Errorf("DeploymentTypeIDByName: %+v", rd.DeploymentTypeIDByName)
	}
	if _, ok := rd.CustomerIDByName["oldco"]; ok {
		t.Error("an INACTIVE customer must not be selectable")
	}
	if _, ok := rd.ProductIDByName["oldproduct"]; ok {
		t.Error("an INACTIVE product must not be selectable")
	}
	if _, ok := rd.DeploymentTypeIDByName["old cloud"]; ok {
		t.Error("an INACTIVE deployment type must not be selectable")
	}
	if rd.TeamTemplateByID[idRegMS] != "MANAGED_SERVICES" || rd.TeamTemplateByID[idRegStandard] != "STANDARD" {
		t.Errorf("TeamTemplateByID: %+v", rd.TeamTemplateByID)
	}
}

func TestBuildRefData_EmptyLookupAborts(t *testing.T) {
	cats := []RiskCategory{{ID: 3, Name: "Access Control & Credentials"}}
	scores := []RiskScore{{ID: 7, Likelihood: 3, Impact: 2}}

	for name, mutate := range map[string]func(*TemplateLookups){
		"customers":        func(l *TemplateLookups) { l.Customers = nil },
		"products":         func(l *TemplateLookups) { l.Products = nil },
		"deployment types": func(l *TemplateLookups) { l.DeploymentTypes = nil },
	} {
		lk := msLookups()
		mutate(&lk)
		_, err := buildRefData(msTeams(), cats, scores, goodRoles(), lk)
		if err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("empty %s: err = %v, want an abort naming %q", name, err, name)
		}
	}
}

func TestBuildRefData_NoManagedServicesRegisterAborts(t *testing.T) {
	cats := []RiskCategory{{ID: 3, Name: "Access Control & Credentials"}}
	scores := []RiskScore{{ID: 7, Likelihood: 3, Impact: 2}}
	teams := []RiskTeam{
		{ID: idRegStandard, Name: "Asgardeo", Code: strptr("ASG"), Status: "ACTIVE", RegisterTemplate: "STANDARD"},
	}
	_, err := buildRefData(teams, cats, scores, goodRoles(), msLookups())
	if err == nil || !strings.Contains(err.Error(), "MANAGED_SERVICES") {
		t.Errorf("err = %v, want an abort saying no register is on the MANAGED_SERVICES template", err)
	}
}

// ── header ──────────────────────────────────────────────────────────────────

func TestMapHeader_TemplateColumnsRequired(t *testing.T) {
	for _, col := range []string{"Customer", "Deployment Type", "Product", "Environment"} {
		var header []string
		for _, h := range expectedHeaders {
			if h != col {
				header = append(header, h)
			}
		}
		if _, err := mapHeader(header); err == nil || !strings.Contains(err.Error(), col) {
			t.Errorf("header without %q: err = %v, want it named as missing", col, err)
		}
	}
}

func TestMapHeader_ComplianceColumnOptional(t *testing.T) {
	if _, err := mapHeader(expectedHeaders); err != nil {
		t.Fatalf("headers without Security Compliance Reference must be accepted: %v", err)
	}
	withIt := append(append([]string{}, expectedHeaders...), "Security Compliance Reference ")
	if _, err := mapHeader(withIt); err != nil {
		t.Errorf("a left-in Security Compliance Reference column must be tolerated: %v", err)
	}
}

// ── a clean Managed Services row ────────────────────────────────────────────

func TestMapRow_MS_CleanRow(t *testing.T) {
	r, fs := parseOne(t, msRow(nil))

	if len(fs) != 0 {
		t.Fatalf("findings = %+v, want none", fs)
	}
	if r.SourceRegisterID != idRegMS || r.AssignmentTeamID != idTeamSRE {
		t.Errorf("team ids: sr=%d at=%d", r.SourceRegisterID, r.AssignmentTeamID)
	}
	if r.CustomerID != idCustBankOne || r.DeploymentTypeID != idDeployPrivate {
		t.Errorf("customer=%d deployment=%d", r.CustomerID, r.DeploymentTypeID)
	}
	if !reflect.DeepEqual(r.ProductIDs, []int{idProdAPIM, idProdMI}) {
		t.Errorf("ProductIDs = %v", r.ProductIDs)
	}
	if !reflect.DeepEqual(r.Environments, []string{"PRODUCTION", "DR"}) {
		t.Errorf("Environments = %v", r.Environments)
	}
}

func TestMapRow_MS_MultiValueSeparators(t *testing.T) {
	// ";" is what the handoff doc asks for; "," and line breaks are tolerated.
	for name, cell := range map[string]string{
		"semicolon": "APIM; MI",
		"comma":     "APIM, MI",
		"newline":   "APIM\nMI",
		"no space":  "APIM;MI",
	} {
		r, fs := parseOne(t, msRow(map[string]string{"Product": cell}))
		if len(fs) != 0 || !reflect.DeepEqual(r.ProductIDs, []int{idProdAPIM, idProdMI}) {
			t.Errorf("%s: ProductIDs = %v, findings = %+v", name, r.ProductIDs, fs)
		}
	}
}

func TestMapRow_MS_DuplicateMultiValuesCollapse(t *testing.T) {
	r, fs := parseOne(t, msRow(map[string]string{"Product": "APIM; apim; MI", "Environment": "DR; dr"}))
	if len(fs) != 0 {
		t.Fatalf("findings = %+v", fs)
	}
	if !reflect.DeepEqual(r.ProductIDs, []int{idProdAPIM, idProdMI}) || !reflect.DeepEqual(r.Environments, []string{"DR"}) {
		t.Errorf("products = %v, environments = %v", r.ProductIDs, r.Environments)
	}
}

func TestMapRow_MS_NamesAreCaseAndSpaceInsensitive(t *testing.T) {
	r, fs := parseOne(t, msRow(map[string]string{
		"Customer":        "  bank of china ",
		"Deployment Type": "MANAGED SERVICES - CUSTOMER'S ON PREM",
		"Product":         " is ",
		"Environment":     "non-production",
	}))
	if len(fs) != 0 {
		t.Fatalf("findings = %+v", fs)
	}
	if r.CustomerID != idCustBOC || r.DeploymentTypeID != idDeployOnPrem ||
		!reflect.DeepEqual(r.ProductIDs, []int{idProdIS}) || !reflect.DeepEqual(r.Environments, []string{"NON_PRODUCTION"}) {
		t.Errorf("row = %+v", r)
	}
}

// ── the four mandatory template fields ──────────────────────────────────────

func TestMapRow_MS_EachTemplateFieldIsMandatory(t *testing.T) {
	for _, col := range []string{"Customer", "Deployment Type", "Product", "Environment"} {
		_, fs := parseOne(t, msRow(map[string]string{col: ""}))
		wantReject(t, fs, col, "empty")
	}
}

func TestMapRow_MS_UnknownValuesReject(t *testing.T) {
	cases := []struct{ col, val string }{
		{"Customer", "Bank One"},
		{"Customer", "BankOne; Bank Of China"}, // exactly one customer
		{"Deployment Type", "Public Cloud"},
		{"Deployment Type", "Private Cloud; Managed Services - Customer's On Prem"}, // exactly one
		{"Product", "APIM; Choreo"},
	}
	for _, c := range cases {
		_, fs := parseOne(t, msRow(map[string]string{c.col: c.val}))
		wantReject(t, fs, c.col, "")
	}
	// The unknown token is named so the admins' list is exact.
	_, fs := parseOne(t, msRow(map[string]string{"Product": "APIM; Choreo"}))
	wantReject(t, fs, "Product", "Choreo")
}

func TestMapRow_MS_InactiveLookupRejectsWithReason(t *testing.T) {
	_, fs := parseOne(t, msRow(map[string]string{"Customer": "OldCo"}))
	wantReject(t, fs, "Customer", "inactive")

	// An inactive deployment type, and an inactive product among active ones.
	_, fs = parseOne(t, msRow(map[string]string{"Deployment Type": "Old Cloud"}))
	wantReject(t, fs, "Deployment Type", "inactive")

	r, fs := parseOne(t, msRow(map[string]string{"Product": "APIM; OldProduct; MI"}))
	wantReject(t, fs, "Product", "inactive")
	if got := findingsFor(fs, "Product"); len(got) != 1 || got[0].Value != "OldProduct" || got[0].Problem != ProblemInactive {
		t.Errorf("Product finding = %+v, want exactly one for OldProduct / inactive", got)
	}
	// The usable products are still resolved; the row is rejected on the bad one.
	if !reflect.DeepEqual(r.ProductIDs, []int{idProdAPIM, idProdMI}) {
		t.Errorf("ProductIDs = %v, want the two active ones", r.ProductIDs)
	}
}

func TestMapRow_MS_EnvironmentMustBeExactlyTheFixedList(t *testing.T) {
	for _, bad := range []string{"Prod", "UAT", "Staging", "Pre-production", "Production; UAT"} {
		_, fs := parseOne(t, msRow(map[string]string{"Environment": bad}))
		wantReject(t, fs, "Environment", "")
	}
	for cell, want := range map[string][]string{
		"Production":                 {"PRODUCTION"},
		"Non-Production":             {"NON_PRODUCTION"},
		"DR":                         {"DR"},
		"Production; Non-Production": {"PRODUCTION", "NON_PRODUCTION"},
	} {
		r, fs := parseOne(t, msRow(map[string]string{"Environment": cell}))
		if len(fs) != 0 || !reflect.DeepEqual(r.Environments, want) {
			t.Errorf("%q: environments = %v, findings = %+v", cell, r.Environments, fs)
		}
	}
}

// ── Security Compliance Reference ───────────────────────────────────────────

func TestMapRow_MS_ComplianceReferenceMustBeBlank(t *testing.T) {
	csvText := buildCSVWithExtra(t, "Security Compliance Reference", msRow(nil), "SOC2")
	_, fs, err := parseSheet(strings.NewReader(csvText), msRefData(t))
	if err != nil {
		t.Fatalf("parseSheet: %v", err)
	}
	wantReject(t, fs, "Security Compliance Reference", "Managed Services")

	// A left-in but empty column is fine.
	csvText = buildCSVWithExtra(t, "Security Compliance Reference", msRow(nil), "")
	rows, fs, err := parseSheet(strings.NewReader(csvText), msRefData(t))
	if err != nil || len(rows) != 1 || len(fs) != 0 {
		t.Errorf("blank compliance column: err = %v, rows = %d, findings = %+v", err, len(rows), fs)
	}
}

// ── register and assignment team ────────────────────────────────────────────

func TestMapRow_MS_SourceRegisterAcceptsNameOrCode(t *testing.T) {
	for _, v := range []string{"Managed Services", "MS", "ms"} {
		r, fs := parseOne(t, msRow(map[string]string{"Source Register": v}))
		if len(fs) != 0 || r.SourceRegisterID != idRegMS {
			t.Errorf("%q: sr=%d, findings = %+v", v, r.SourceRegisterID, fs)
		}
	}
}

func TestMapRow_MS_OtherRegistersReject(t *testing.T) {
	_, fs := parseOne(t, msRow(map[string]string{"Source Register": "Asgardeo"}))
	wantReject(t, fs, "Source Register", "Managed Services")

	// WSO2 Cloud is a real register but out of scope: say so, don't just call it unknown.
	_, fs = parseOne(t, msRow(map[string]string{"Source Register": "WSO2 Cloud"}))
	wantReject(t, fs, "Source Register", "not supported")

	_, fs = parseOne(t, msRow(map[string]string{"Source Register": "Nowhere"}))
	wantReject(t, fs, "Source Register", "unknown")
}

// Any active team may be the assignment team (RISK_MODULE_DESIGN.md §14, the
// entity's checkAssignmentTeam): the template only decides which fields a
// register's risks carry, not who they can be assigned to. For Managed Services
// the sheet will simply name the Managed Services team.
func TestMapRow_MS_AssignmentTeamIsAnyKnownTeam(t *testing.T) {
	for _, tc := range []struct {
		cell string
		want int
	}{
		{"Managed Services", idRegMS},
		{"MS", idRegMS},
		{"SRE One", idTeamSRE},
		{"Asgardeo", idRegStandard},
		{"asgardeo", idRegStandard},
	} {
		r, fs := parseOne(t, msRow(map[string]string{"Assignment Team": tc.cell}))
		if len(fs) != 0 || r.AssignmentTeamID != tc.want {
			t.Errorf("%q: at=%d, findings = %+v", tc.cell, r.AssignmentTeamID, fs)
		}
	}

	_, fs := parseOne(t, msRow(map[string]string{"Assignment Team": "Nowhere"}))
	wantReject(t, fs, "Assignment Team", "unknown")
	_, fs = parseOne(t, msRow(map[string]string{"Assignment Team": ""}))
	wantReject(t, fs, "Assignment Team", "empty")
}

// buildCSVWithExtra renders expectedHeaders plus one extra trailing column, for
// the left-in Security Compliance Reference case.
func buildCSVWithExtra(t *testing.T, extraHeader string, cells map[string]string, extraValue string) string {
	t.Helper()
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write(append(append([]string{}, expectedHeaders...), extraHeader)); err != nil {
		t.Fatal(err)
	}
	rec := make([]string, 0, len(expectedHeaders)+1)
	for _, h := range expectedHeaders {
		rec = append(rec, cells[h])
	}
	if err := w.Write(append(rec, extraValue)); err != nil {
		t.Fatal(err)
	}
	w.Flush()
	return buf.String()
}
