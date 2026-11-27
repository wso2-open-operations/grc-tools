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
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/csvexport"
)

// Severity of a Finding (plan §9).
type Severity string

const (
	SevReject   Severity = "REJECT"   // the row is not migrated
	SevWarn     Severity = "WARN"     // the row is migrated, with a caveat
	SevMismatch Severity = "MISMATCH" // verify.go: entity state disagrees with the CSV after a real run
)

// Finding is one row of the errors.csv block.
type Finding struct {
	MigrationID int
	CSVRow      int
	RiskTitle   string
	Severity    Severity
	Failure     string // short code, e.g. "Risk Owner", "Implementation Date"
	Detail      string
	// Value is the raw offending token when there is one — an unresolvable
	// email, an unknown reference. Feeds the report's "unresolved people" list.
	Value string
	// Problem says why a lookup value was refused, so the report can group the
	// values the admins must add (ProblemUnknown) apart from those they must
	// reactivate (ProblemInactive). Empty for every other kind of finding.
	Problem string
}

// Lookup problems (Finding.Problem).
const (
	ProblemUnknown  = "unknown"
	ProblemInactive = "inactive"
)

// personColumns are the Failure codes whose Value is an unresolvable email.
var personColumns = map[string]bool{
	"Risk Owner": true, "Risk Assigned To": true,
	"Management Approver": true, "Action Owner": true,
}

// Report accumulates findings and per-row outcomes for the run, then renders
// the errors.csv + report.txt blocks to the log (plan §9).
type Report struct {
	findings []Finding
	migrated int
	skipped  int // already complete on a resume

	migratedByBucket       map[string]int
	grantsWritten          int
	assessmentsWritten     int
	suppressingEscalations []int // Migration IDs that got a D8 suppressing escalation

	customerSummaries []CustomerSummary
	riskCodes         []RiskCodeEntry

	// plan is what a real run would do with the accepted rows, known after a
	// dry run: how many it would write and how many an earlier import already
	// completed.
	planKnown   bool
	planToWrite int
	planDone    int
}

// SetPlan records what a real run would do with the accepted rows: toWrite rows
// still to create or finish, alreadyDone rows an earlier import completed.
func (r *Report) SetPlan(toWrite, alreadyDone int) {
	r.planKnown, r.planToWrite, r.planDone = true, toWrite, alreadyDone
}

// RiskCodeEntry is one line of the Migration ID -> risk code list printed after
// a real run. The codes are assigned by the entity, are permanent, and go out in
// emails and Git issues, so whoever owns the sheet needs to match them to rows.
type RiskCodeEntry struct {
	MigrationID int
	Customer    string
	RiskCode    string
	RiskTitle   string
}

// AddRiskCode records a migrated risk's code for the risk_codes.csv block.
func (r *Report) AddRiskCode(e RiskCodeEntry) { r.riskCodes = append(r.riskCodes, e) }

func NewReport() *Report { return &Report{migratedByBucket: map[string]int{}} }

func (r *Report) Add(fs ...Finding) { r.findings = append(r.findings, fs...) }

// Migrated records one row written to completion, tallied by target bucket
// (APPROVED | CLOSED).
func (r *Report) Migrated(bucket string) {
	r.migrated++
	r.migratedByBucket[bucket]++
}

func (r *Report) Skipped() { r.skipped++ }

// SetCustomerSummaries stores the per-customer summary (summary.go) for the
// report.txt block.
func (r *Report) SetCustomerSummaries(s []CustomerSummary) { r.customerSummaries = s }

func (r *Report) GrantWritten() { r.grantsWritten++ }

// AssessmentWritten records that a row's residual value differed from its
// gross and got a synthetic risk_assessment row (needsResidualAssessment).
func (r *Report) AssessmentWritten() { r.assessmentsWritten++ }

// SuppressingEscalation records that migrationID's risk got the D8 escalation
// that keeps the nightly job (and its emails) off an already-overdue import.
func (r *Report) SuppressingEscalation(migrationID int) {
	r.suppressingEscalations = append(r.suppressingEscalations, migrationID)
}

func (r *Report) HasFindings() bool { return len(r.findings) > 0 }

// RejectedMigrationIDs is the set of Migration IDs with at least one REJECT.
func (r *Report) RejectedMigrationIDs() map[int]struct{} {
	out := map[int]struct{}{}
	for _, f := range r.findings {
		if f.Severity == SevReject {
			out[f.MigrationID] = struct{}{}
		}
	}
	return out
}

// Emit writes the two blocks the operator reads out of the Choreo logs.
//
// Everything is built into an in-memory buffer first and handed to w in one
// Write call. Choreo's log pipeline does not reliably preserve the order of a
// burst of near-simultaneous small writes to the same stream — this report is
// ~10 lines produced back-to-back with no I/O in between, and a multi-write
// Emit was observed coming out of Choreo's log viewer fully reversed
// (end-to-start). A single Write leaves nothing for that pipeline to reorder.
func (r *Report) Emit(w io.Writer) {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "\n===== risk-register-import report — %s =====\n", time.Now().UTC().Format(time.RFC3339))

	rejects, warns, mismatches := 0, 0, 0
	for _, f := range r.findings {
		switch f.Severity {
		case SevReject:
			rejects++
		case SevWarn:
			warns++
		case SevMismatch:
			mismatches++
		}
	}
	fmt.Fprintf(&buf, "migrated=%d  skipped(resume)=%d  rejected=%d  warnings=%d  mismatches=%d\n\n",
		r.migrated, r.skipped, rejects, warns, mismatches)

	fmt.Fprintln(&buf, "----- errors.csv -----")
	r.WriteErrorsCSV(&buf)

	r.emitNarrative(&buf)
	r.emitRiskCodes(&buf)
	fmt.Fprintln(&buf, "----- end report -----")

	_, _ = w.Write(buf.Bytes())
}

// WriteErrorsCSV renders every finding as errors.csv (header included), sorted
// by Migration ID then CSV row. Cells go through SafeCell: titles and details
// echo user-authored sheet text, and the file is opened in a spreadsheet.
func (r *Report) WriteErrorsCSV(w io.Writer) {
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"migration_id", "csv_row", "risk_title", "severity", "failure", "detail"})
	sorted := append([]Finding(nil), r.findings...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].MigrationID != sorted[j].MigrationID {
			return sorted[i].MigrationID < sorted[j].MigrationID
		}
		return sorted[i].CSVRow < sorted[j].CSVRow
	})
	for _, f := range sorted {
		_ = cw.Write([]string{
			strconv.Itoa(f.MigrationID), strconv.Itoa(f.CSVRow), csvexport.SafeCell(f.RiskTitle),
			string(f.Severity), f.Failure, csvexport.SafeCell(f.Detail),
		})
	}
	cw.Flush()
}

// WriteNarrative renders the report.txt body without the surrounding banner.
func (r *Report) WriteNarrative(w io.Writer) { r.emitNarrative(w) }

// WriteRiskCodesCSV renders risk_codes.csv (header included), sorted by
// Migration ID. It writes only the header when no code was recorded.
func (r *Report) WriteRiskCodesCSV(w io.Writer) {
	entries := append([]RiskCodeEntry(nil), r.riskCodes...)
	sort.Slice(entries, func(i, j int) bool { return entries[i].MigrationID < entries[j].MigrationID })
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"migration_id", "customer", "risk_code", "risk_title"})
	for _, e := range entries {
		_ = cw.Write([]string{strconv.Itoa(e.MigrationID), csvexport.SafeCell(e.Customer), e.RiskCode, csvexport.SafeCell(e.RiskTitle)})
	}
	cw.Flush()
}

// Counts is the report's headline numbers.
type Counts struct {
	Migrated   int `json:"migrated"`
	Skipped    int `json:"skipped"`
	Rejects    int `json:"rejects"`
	Warnings   int `json:"warnings"`
	Mismatches int `json:"mismatches"`
	// ToWrite and AlreadyDone are set by a dry run: the rows a real run would
	// write (new, or begun but not finished) and the rows it would skip because
	// an earlier import completed them. PlanKnown says they are meaningful.
	ToWrite     int  `json:"to_write"`
	AlreadyDone int  `json:"already_done"`
	PlanKnown   bool `json:"plan_known"`
}

// Counts tallies the findings by severity alongside the migrated / skipped rows.
func (r *Report) Counts() Counts {
	c := Counts{Migrated: r.migrated, Skipped: r.skipped, ToWrite: r.planToWrite, AlreadyDone: r.planDone, PlanKnown: r.planKnown}
	for _, f := range r.findings {
		switch f.Severity {
		case SevReject:
			c.Rejects++
		case SevWarn:
			c.Warnings++
		case SevMismatch:
			c.Mismatches++
		}
	}
	return c
}

// Findings returns a copy of the findings, in the order they were added.
func (r *Report) Findings() []Finding { return append([]Finding(nil), r.findings...) }

// emitRiskCodes prints the Migration ID -> risk code list as a CSV block, sorted
// by Migration ID. Nothing is printed when no code was recorded (a dry run, or a
// run that migrated nothing).
func (r *Report) emitRiskCodes(w io.Writer) {
	if len(r.riskCodes) == 0 {
		return
	}
	entries := append([]RiskCodeEntry(nil), r.riskCodes...)
	sort.Slice(entries, func(i, j int) bool { return entries[i].MigrationID < entries[j].MigrationID })

	fmt.Fprintln(w, "\n----- risk_codes.csv -----")
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"migration_id", "customer", "risk_code", "risk_title"})
	for _, e := range entries {
		_ = cw.Write([]string{strconv.Itoa(e.MigrationID), e.Customer, e.RiskCode, e.RiskTitle})
	}
	cw.Flush()
}

// emitNarrative is the report.txt block (plan §9): a failure-code breakdown,
// the distinct unresolved people, the rows that got a suppressing escalation,
// the grant count, and — on a real run — the per-bucket migrated counts.
func (r *Report) emitNarrative(w io.Writer) {
	fmt.Fprintln(w, "\n----- report.txt -----")

	// Failure-code counts, most frequent first.
	counts := map[string]int{}
	for _, f := range r.findings {
		counts[string(f.Severity)+"  "+f.Failure]++
	}
	if len(counts) > 0 {
		type kv struct {
			k string
			n int
		}
		rows := make([]kv, 0, len(counts))
		for k, n := range counts {
			rows = append(rows, kv{k, n})
		}
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].n != rows[j].n {
				return rows[i].n > rows[j].n
			}
			return rows[i].k < rows[j].k
		})
		fmt.Fprintln(w, "findings by code:")
		for _, row := range rows {
			fmt.Fprintf(w, "  %4d  %s\n", row.n, row.k)
		}
	}

	if r.planKnown {
		fmt.Fprintf(w, "plan: %d to write, %d already complete from an earlier import (skipped)\n", r.planToWrite, r.planDone)
	}

	r.emitLookupProblems(w)
	r.emitCustomerSummaries(w)

	// Distinct unresolved people.
	emails := map[string]struct{}{}
	for _, f := range r.findings {
		if personColumns[f.Failure] && f.Value != "" {
			emails[strings.ToLower(f.Value)] = struct{}{}
		}
	}
	if len(emails) > 0 {
		list := make([]string, 0, len(emails))
		for e := range emails {
			list = append(list, e)
		}
		sort.Strings(list)
		fmt.Fprintf(w, "unresolved people (%d):\n", len(list))
		for _, e := range list {
			fmt.Fprintf(w, "  %s\n", e)
		}
	}

	if len(r.suppressingEscalations) > 0 {
		ids := append([]int(nil), r.suppressingEscalations...)
		sort.Ints(ids)
		fmt.Fprintf(w, "suppressing escalations seeded for Migration IDs: %v (%d)\n", ids, len(ids))
	}

	if r.grantsWritten > 0 {
		fmt.Fprintf(w, "grants written: %d\n", r.grantsWritten)
	}

	if r.assessmentsWritten > 0 {
		fmt.Fprintf(w, "residual assessments written: %d\n", r.assessmentsWritten)
	}

	if r.migrated > 0 {
		buckets := make([]string, 0, len(r.migratedByBucket))
		for b := range r.migratedByBucket {
			buckets = append(buckets, b)
		}
		sort.Strings(buckets)
		fmt.Fprint(w, "migrated by bucket:")
		for _, b := range buckets {
			fmt.Fprintf(w, " %s=%d", b, r.migratedByBucket[b])
		}
		fmt.Fprintln(w)
	}
}
