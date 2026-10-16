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
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The dry run's two Managed Services blocks (RISK_MODULE_DESIGN.md §14,
// Phase 2): the distinct values the admins must add, and a per-customer summary
// with the first and last risk code the run would assign.

func TestRiskCode_Format(t *testing.T) {
	for _, tc := range []struct {
		year    int
		team    string
		cust    string
		quarter string
		seq     int
		want    string
	}{
		{2025, "MS", "BANKONESUB", "Q3", 1, "2025-MS-BANKONESUB-Q3-0001"},
		{2026, "MS", "BO", "Q2", 14, "2026-MS-BO-Q2-0014"},
		{2026, "MS", "BOC", "Q1", 12345, "2026-MS-BOC-Q1-12345"}, // wider than 4 digits is not truncated
	} {
		if got := riskCode(tc.year, tc.team, tc.cust, tc.quarter, tc.seq); got != tc.want {
			t.Errorf("riskCode = %q, want %q", got, tc.want)
		}
	}
}

func summaryRow(mig, custID int, cust string, year int, quarter, status string) Row {
	return Row{
		MigrationID: mig, SourceRegisterID: idRegMS, CustomerID: custID, Customer: cust,
		RiskYear: year, RiskQuarter: quarter, WorkflowStatus: status,
	}
}

func TestBuildCustomerSummaries(t *testing.T) {
	rd := msRefData(t) // MS register code "MS"; BankOne = BO, Bank Of China = BOC
	rows := []Row{
		summaryRow(1, idCustBankOne, "BankOne", 2025, "Q1", "APPROVED"),
		summaryRow(2, idCustBOC, "Bank Of China", 2025, "Q1", "CLOSED"),
		summaryRow(3, idCustBankOne, "BankOne", 2025, "Q3", "CLOSED"),
		summaryRow(4, idCustBankOne, "BankOne", 2026, "Q2", "APPROVED"),
	}
	progress := map[int]ResumeState{
		1: {Progress: ProgressNone}, 2: {Progress: ProgressNone},
		3: {Progress: ProgressNone}, 4: {Progress: ProgressNone},
	}
	// BankOne already has 14 risks in the entity, so its next number is 15.
	next := map[int]int{idCustBankOne: 15, idCustBOC: 1}

	got := buildCustomerSummaries(rows, rd, progress, next)
	if len(got) != 2 {
		t.Fatalf("got %d summaries, want 2: %+v", len(got), got)
	}

	// Sorted by customer name: "Bank Of China" < "BankOne".
	boc, bo := got[0], got[1]
	if boc.Customer != "Bank Of China" || boc.Code != "BOC" || boc.Risks != 1 || boc.Closed != 1 || boc.Approved != 0 || boc.New != 1 {
		t.Errorf("Bank Of China summary = %+v", boc)
	}
	if boc.FirstCode != "2025-MS-BOC-Q1-0001" || boc.LastCode != "2025-MS-BOC-Q1-0001" {
		t.Errorf("Bank Of China codes = %q .. %q", boc.FirstCode, boc.LastCode)
	}
	if bo.Customer != "BankOne" || bo.Code != "BO" || bo.Risks != 3 || bo.Approved != 2 || bo.Closed != 1 || bo.New != 3 {
		t.Errorf("BankOne summary = %+v", bo)
	}
	// Migration ID order drives the numbers: 15 (2025-Q1), 16 (2025-Q3), 17 (2026-Q2).
	if bo.FirstCode != "2025-MS-BO-Q1-0015" || bo.LastCode != "2026-MS-BO-Q2-0017" {
		t.Errorf("BankOne codes = %q .. %q", bo.FirstCode, bo.LastCode)
	}
}

func TestBuildCustomerSummaries_ExistingRisksTakeNoNewNumber(t *testing.T) {
	rd := msRefData(t)
	rows := []Row{
		summaryRow(1, idCustBankOne, "BankOne", 2025, "Q1", "APPROVED"),
		summaryRow(2, idCustBankOne, "BankOne", 2025, "Q2", "APPROVED"),
		summaryRow(3, idCustBankOne, "BankOne", 2025, "Q3", "APPROVED"),
	}
	// Rows 1 and 2 were written by an earlier run; only row 3 is still to create.
	progress := map[int]ResumeState{
		1: {Progress: ProgressComplete, RiskID: 900},
		2: {Progress: ProgressComplete, RiskID: 901},
		3: {Progress: ProgressNone},
	}
	got := buildCustomerSummaries(rows, rd, progress, map[int]int{idCustBankOne: 3})
	if len(got) != 1 || got[0].Risks != 3 || got[0].New != 1 {
		t.Fatalf("summaries = %+v, want 3 risks of which 1 new", got)
	}
	if got[0].FirstCode != "2025-MS-BO-Q3-0003" || got[0].LastCode != "2025-MS-BO-Q3-0003" {
		t.Errorf("codes = %q .. %q, want only the new row's 0003", got[0].FirstCode, got[0].LastCode)
	}
}

func TestBuildCustomerSummaries_NothingNewHasNoCodes(t *testing.T) {
	rd := msRefData(t)
	rows := []Row{summaryRow(1, idCustBankOne, "BankOne", 2025, "Q1", "CLOSED")}
	progress := map[int]ResumeState{1: {Progress: ProgressComplete, RiskID: 900}}
	got := buildCustomerSummaries(rows, rd, progress, map[int]int{idCustBankOne: 2})
	if len(got) != 1 || got[0].New != 0 || got[0].FirstCode != "" || got[0].LastCode != "" {
		t.Errorf("summaries = %+v, want 0 new and no codes", got)
	}
}

func TestReportEmit_CustomerSummaryBlock(t *testing.T) {
	r := NewReport()
	r.SetCustomerSummaries([]CustomerSummary{{
		Customer: "BankOne", Code: "BO", Risks: 3, Approved: 2, Closed: 1, New: 3,
		FirstCode: "2025-MS-BO-Q1-0015", LastCode: "2026-MS-BO-Q2-0017",
	}})
	var sb strings.Builder
	r.Emit(&sb)
	out := sb.String()
	for _, want := range []string{
		"per-customer summary",
		"BankOne (BO)",
		"3 risks (2 approved, 1 closed)",
		"3 new",
		"2025-MS-BO-Q1-0015 .. 2026-MS-BO-Q2-0017",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
	}
}

// ── grouped unknown values ──────────────────────────────────────────────────

func unknownFinding(mig int, failure, value string) Finding {
	return Finding{MigrationID: mig, CSVRow: mig + 1, Severity: SevReject, Failure: failure,
		Detail: "unknown " + failure + " " + value, Value: value, Problem: ProblemUnknown}
}

func TestReportEmit_UnknownValuesGroupedForAdmins(t *testing.T) {
	r := NewReport()
	r.Add(
		unknownFinding(4, "Customer", "Bank One"),
		unknownFinding(9, "Customer", "Bank One"),
		unknownFinding(12, "Customer", "Bank One"),
		unknownFinding(7, "Product", "Choreo"),
		// A row naming two unknown products lists each one.
		unknownFinding(8, "Product", "Choreo"),
		unknownFinding(8, "Product", "Bijira"),
		unknownFinding(3, "Deployment Type", "Public Cloud"),
	)
	var sb strings.Builder
	r.Emit(&sb)
	out := sb.String()

	for _, want := range []string{
		"unknown values to add in the Admin Console",
		`"Bank One" - 3 rows (Migration IDs 4, 9, 12)`,
		`"Choreo" - 2 rows (Migration IDs 7, 8)`,
		`"Bijira" - 1 row (Migration ID 8)`,
		`"Public Cloud" - 1 row (Migration ID 3)`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
	}
}

func TestReportEmit_UnknownValuesListIsCapped(t *testing.T) {
	r := NewReport()
	for i := 1; i <= 12; i++ {
		r.Add(unknownFinding(i, "Customer", "Bank One"))
	}
	var sb strings.Builder
	r.Emit(&sb)
	out := sb.String()
	if !strings.Contains(out, "12 rows (Migration IDs 1, 2, 3, 4, 5, 6, 7, 8, 9, 10 and 2 more)") {
		t.Errorf("long id list should be capped at 10:\n%s", out)
	}
}

func TestReportEmit_InactiveAndEnvironmentBlocks(t *testing.T) {
	r := NewReport()
	r.Add(
		Finding{MigrationID: 5, CSVRow: 6, Severity: SevReject, Failure: "Customer",
			Detail: `customer "OldCo" is inactive`, Value: "OldCo", Problem: ProblemInactive},
		unknownFinding(1, "Environment", "Prod"),
		unknownFinding(2, "Environment", "Prod"),
	)
	var sb strings.Builder
	r.Emit(&sb)
	out := sb.String()

	for _, want := range []string{
		"inactive values to reactivate in the Admin Console",
		`"OldCo" - 1 row (Migration ID 5)`,
		"invalid environments (fix in the sheet",
		`"Prod" - 2 rows (Migration IDs 1, 2)`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
	}
	// An invalid environment is a sheet problem, never something to add.
	if strings.Contains(sectionOf(out, "unknown values to add"), "Prod") {
		t.Errorf("Environment values must not appear under the add-in-Admin-Console list:\n%s", out)
	}
}

func TestReportEmit_NoLookupProblemsPrintsNoBlocks(t *testing.T) {
	r := NewReport()
	r.Add(Finding{MigrationID: 1, CSVRow: 2, Severity: SevWarn, Failure: "Git Issue URL"})
	var sb strings.Builder
	r.Emit(&sb)
	for _, absent := range []string{"unknown values to add", "inactive values to reactivate", "invalid environments", "per-customer summary"} {
		if strings.Contains(sb.String(), absent) {
			t.Errorf("report should not print %q when there is nothing to say:\n%s", absent, sb.String())
		}
	}
}

// sectionOf returns the text from the first occurrence of header to the next
// blank line, or "" if header is absent.
func sectionOf(out, header string) string {
	i := strings.Index(out, header)
	if i < 0 {
		return ""
	}
	rest := out[i:]
	if j := strings.Index(rest, "\n\n"); j >= 0 {
		return rest[:j]
	}
	return rest
}

// The mapper tags each lookup REJECT so the report can group it.
func TestMapRow_MS_LookupFindingsCarryValueAndProblem(t *testing.T) {
	_, fs := parseOne(t, msRow(map[string]string{"Product": "APIM; Choreo", "Customer": "OldCo", "Environment": "Prod"}))

	p := findingsFor(fs, "Product")
	if len(p) != 1 || p[0].Value != "Choreo" || p[0].Problem != ProblemUnknown {
		t.Errorf("Product finding = %+v, want Value Choreo / unknown", p)
	}
	c := findingsFor(fs, "Customer")
	if len(c) != 1 || c[0].Value != "OldCo" || c[0].Problem != ProblemInactive {
		t.Errorf("Customer finding = %+v, want Value OldCo / inactive", c)
	}
	e := findingsFor(fs, "Environment")
	if len(e) != 1 || e[0].Value != "Prod" || e[0].Problem != ProblemUnknown {
		t.Errorf("Environment finding = %+v, want Value Prod / unknown", e)
	}
}

// ── reading the next number from the entity ────────────────────────────────

func TestEntityClient_NextSequenceNumber(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nextSequenceNumber":15}`))
	}))
	t.Cleanup(srv.Close)

	n, err := NewEntityClient(srv.URL, 5*time.Second).NextSequenceNumber(context.Background(), 10, 21)
	if err != nil || n != 15 {
		t.Fatalf("NextSequenceNumber = %d, %v; want 15, nil", n, err)
	}
	if gotPath != "/risks/next-sequence-number" || gotQuery != "customerId=21&sourceRegisterId=10" {
		t.Errorf("request = %s?%s", gotPath, gotQuery)
	}
}

// computeCustomerSummaries reads each customer's next number once, only for
// customers that have a row still to create.
func TestComputeCustomerSummaries_ReadsCounterPerCustomerWithNewRows(t *testing.T) {
	rd := fixtureRefData(t) // BankOne = 20 (BO), Bank Of China = 21 (BOC), MS register = 1 (MS)
	fe := newFakeEntity(t)
	fe.nextSeq = map[int]int{20: 15, 21: 1}
	ec := fe.client(t)

	rows := []Row{
		{MigrationID: 1, SourceRegisterID: 1, CustomerID: 20, Customer: "BankOne", RiskYear: 2025, RiskQuarter: "Q1", WorkflowStatus: "APPROVED"},
		{MigrationID: 2, SourceRegisterID: 1, CustomerID: 21, Customer: "Bank Of China", RiskYear: 2025, RiskQuarter: "Q2", WorkflowStatus: "CLOSED"},
	}
	// Bank Of China's only row already exists: its counter must not be read.
	progress := map[int]ResumeState{1: {Progress: ProgressNone}, 2: {Progress: ProgressComplete, RiskID: 900}}

	got, err := computeCustomerSummaries(context.Background(), ec, rd, rows, progress)
	if err != nil {
		t.Fatalf("computeCustomerSummaries: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("summaries = %+v", got)
	}
	if fe.nextSeqCalls != 1 {
		t.Errorf("counter reads = %d, want 1 (only BankOne has a row to create)", fe.nextSeqCalls)
	}
	for _, s := range got {
		if s.Customer == "BankOne" && (s.FirstCode != "2025-MS-BO-Q1-0015") {
			t.Errorf("BankOne summary = %+v", s)
		}
	}
}

// A dry run does not rebuild full resume state (that reads grants for people
// the dry run has not resolved), but it still has to know which rows already
// have a risk, or the summary would hand their numbers out again.
func TestExistingRisks_MatchesByCustomerAwareKey(t *testing.T) {
	s := &stateStub{t: t, risks: []Risk{
		withCustomer(markerRisk(80, "TLS 1.0 enabled", 8, 2025, "Q3", "APPROVED"), "BankOne"),
	}}
	rowFor := func(mig int, customer string) Row {
		r := baseRow(mig, "TLS 1.0 enabled", "APPROVED")
		r.Customer = customer
		return r
	}
	got, err := existingRisks(context.Background(), s.client(t), []Row{rowFor(1, "BankOne"), rowFor(2, "Bank Of China")}, NewReport())
	if err != nil {
		t.Fatalf("existingRisks: %v", err)
	}
	if got[1].RiskID != 80 {
		t.Errorf("BankOne row should match risk 80, got %+v", got[1])
	}
	if got[2].RiskID != 0 {
		t.Errorf("Bank Of China row must not match BankOne's risk, got %+v", got[2])
	}
}

// An ambiguous existing match (two marker risks on one key) is rejected by the
// real run; the dry run's lookup must say so too, and must not hand the row a number.
func TestExistingRisks_AmbiguousMatchIsRejected(t *testing.T) {
	s := &stateStub{t: t, risks: []Risk{
		withCustomer(markerRisk(80, "TLS 1.0 enabled", 8, 2025, "Q3", "APPROVED"), "BankOne"),
		withCustomer(markerRisk(81, "TLS 1.0 enabled", 8, 2025, "Q3", "APPROVED"), "BankOne"),
	}}
	row := baseRow(1, "TLS 1.0 enabled", "APPROVED")
	row.Customer = "BankOne"

	rep := NewReport()
	got, err := existingRisks(context.Background(), s.client(t), []Row{row}, rep)
	if err != nil {
		t.Fatalf("existingRisks: %v", err)
	}
	if _, ok := rep.RejectedMigrationIDs()[1]; !ok {
		t.Errorf("row matching two marker risks should be rejected; findings=%+v", rep.findings)
	}
	if got[1].RiskID != 0 {
		t.Errorf("an ambiguous row must not be matched to either risk, got %+v", got[1])
	}
}

// dryRunChecks is everything the dry run learns from the entity beyond preflight:
// it must reach the same row-level verdicts the real run reaches in
// reconstructState, and size the per-customer summary from the rows that survive.
func TestDryRunChecks_RejectsWhatTheRealRunRejects_AndSummarisesTheRest(t *testing.T) {
	cells := func(mig, title, customer string) map[string]string {
		return msRow(map[string]string{"Migration ID": mig, "Risk Title": title, "Customer": customer})
	}
	rd := fixtureRefData(t)
	rows, fs, err := parseSheet(strings.NewReader(buildCSV(t,
		cells("1", "T1", "BankOne"), cells("2", "T1", "BankOne"), // duplicate key
		cells("3", "T1", "Bank Of China"), // fine, new
		cells("4", "T2", "BankOne"),       // two existing marker risks
		cells("5", "T3", "Bank Of China"), // fine, already exists
	)), rd)
	if err != nil || len(fs) != 0 {
		t.Fatalf("parseSheet: err=%v findings=%+v", err, fs)
	}

	fe := newFakeEntity(t)
	boc, bo := "Bank Of China", "BankOne"
	for id, r := range map[int]Risk{
		901: {ID: 901, RiskTitle: "T2", SourceRegID: 1, RiskYear: 2025, RiskQuarter: "Q3", CreatedBy: marker, CustomerName: &bo},
		902: {ID: 902, RiskTitle: "T2", SourceRegID: 1, RiskYear: 2025, RiskQuarter: "Q3", CreatedBy: marker, CustomerName: &bo},
		903: {ID: 903, RiskTitle: "T3", SourceRegID: 1, RiskYear: 2025, RiskQuarter: "Q3", CreatedBy: marker, CustomerName: &boc},
	} {
		r := r
		fe.risks[id] = &r
	}

	rep := NewReport()
	migratable, err := dryRunChecks(context.Background(), discardLogger(), fe.client(t), rd, rows, rep)
	if err != nil {
		t.Fatalf("dryRunChecks: %v", err)
	}

	rej := rep.RejectedMigrationIDs()
	for _, id := range []int{1, 2, 4} {
		if _, ok := rej[id]; !ok {
			t.Errorf("row %d should be rejected, as the real run would", id)
		}
	}
	for _, id := range []int{3, 5} {
		if _, ok := rej[id]; ok {
			t.Errorf("row %d should not be rejected", id)
		}
	}
	if len(migratable) != 2 {
		t.Fatalf("migratable = %d rows, want 2 (3 and 5)", len(migratable))
	}

	// Only Bank Of China survives: row 3 is new, row 5 already exists.
	var got []CustomerSummary
	rep.Emit(io.Discard)
	got = rep.customerSummaries
	if len(got) != 1 || got[0].Customer != "Bank Of China" || got[0].Risks != 2 || got[0].New != 1 {
		t.Fatalf("summaries = %+v, want Bank Of China with 2 risks, 1 new", got)
	}
	if got[0].FirstCode != "2025-MS-BOC-Q3-0001" {
		t.Errorf("first code = %q", got[0].FirstCode)
	}
}

// Inactive products and deployment types are grouped for the admins like an
// inactive customer: values to reactivate, not to add.
func TestReportEmit_InactiveProductAndDeploymentTypeAreGrouped(t *testing.T) {
	r := NewReport()
	r.Add(
		Finding{MigrationID: 3, CSVRow: 4, Severity: SevReject, Failure: "Product",
			Detail: `product "OldProduct" is inactive`, Value: "OldProduct", Problem: ProblemInactive},
		Finding{MigrationID: 6, CSVRow: 7, Severity: SevReject, Failure: "Deployment Type",
			Detail: `deployment type "Old Cloud" is inactive`, Value: "Old Cloud", Problem: ProblemInactive},
	)
	var sb strings.Builder
	r.Emit(&sb)
	out := sectionOf(sb.String(), "inactive values to reactivate")
	for _, want := range []string{"Product (1):", `"OldProduct" - 1 row (Migration ID 3)`, "Deployment Type (1):", `"Old Cloud" - 1 row (Migration ID 6)`} {
		if !strings.Contains(out, want) {
			t.Errorf("inactive block missing %q:\n%s", want, sb.String())
		}
	}
}

// ── Migration ID -> risk code ───────────────────────────────────────────────

// After a real run the sheet owner needs to match her rows to the numbers the
// entity assigned: they are permanent and go out in emails and Git issues.
func TestReportEmit_RiskCodesBlock(t *testing.T) {
	r := NewReport()
	// Out of order, and a title with a comma that must be CSV-quoted.
	r.AddRiskCode(RiskCodeEntry{MigrationID: 12, Customer: "Bank Of China", RiskCode: "2025-MS-BOC-Q3-0002", RiskTitle: "Second, with a comma"})
	r.AddRiskCode(RiskCodeEntry{MigrationID: 3, Customer: "BankOne", RiskCode: "2025-MS-BO-Q1-0001", RiskTitle: "First"})

	var sb strings.Builder
	r.Emit(&sb)
	out := sb.String()

	header := "migration_id,customer,risk_code,risk_title"
	i := strings.Index(out, header)
	if i < 0 {
		t.Fatalf("report missing the risk codes CSV header:\n%s", out)
	}
	block := out[i:]
	first := strings.Index(block, "3,BankOne,2025-MS-BO-Q1-0001,First")
	second := strings.Index(block, `12,Bank Of China,2025-MS-BOC-Q3-0002,"Second, with a comma"`)
	if first < 0 || second < 0 || first > second {
		t.Errorf("rows missing, unquoted or not sorted by Migration ID:\n%s", block)
	}
}

func TestReportEmit_NoRiskCodesNoBlock(t *testing.T) {
	var sb strings.Builder
	NewReport().Emit(&sb)
	if strings.Contains(sb.String(), "risk_code") {
		t.Errorf("a run that wrote nothing should print no risk code block:\n%s", sb.String())
	}
}
