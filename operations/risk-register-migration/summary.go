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
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
)

// The two Managed Services blocks of report.txt (RISK_MODULE_DESIGN.md §14,
// Phase 2): the lookup values the admins must add, and a per-customer summary
// with the first and last risk code the run would assign.

// riskCode builds a Managed Services risk code (RISK_MODULE_DESIGN.md §12):
// {YEAR}-{TEAM_CODE}-{CUSTOMER_CODE}-{QUARTER}-{SEQUENCE}. The entity assigns
// the real number at create time; this is the same format, used to show the
// operator what a run is about to produce.
func riskCode(year int, teamCode, customerCode, quarter string, seq int) string {
	return fmt.Sprintf("%s%04d", riskCodePrefix(year, teamCode, customerCode, quarter), seq)
}

// riskCodePrefix is everything in a Managed Services risk code before the
// number. The tool can vouch for this part (it knows the row's year, quarter,
// register and customer); the number is the entity's to assign.
func riskCodePrefix(year int, teamCode, customerCode, quarter string) string {
	return fmt.Sprintf("%d-%s-%s-%s-", year, teamCode, customerCode, quarter)
}

// riskCodeHasPrefix reports whether code is prefix followed by a number.
func riskCodeHasPrefix(code, prefix string) bool {
	rest, ok := strings.CutPrefix(code, prefix)
	if !ok || rest == "" {
		return false
	}
	for _, r := range rest {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// CustomerSummary is one customer's line in the report.
type CustomerSummary struct {
	Customer  string
	Code      string
	Risks     int // migratable rows for this customer
	Approved  int
	Closed    int
	New       int    // rows that still have to be created (no risk exists yet)
	FirstCode string // "" when New == 0
	LastCode  string
}

// buildCustomerSummaries groups the migratable rows by customer. nextSeq is
// each customer's next number as the entity reports it before this run writes
// anything. Numbers go to the rows still to be created, in Migration ID order,
// which is the order the tool writes them in; a row that already has a risk
// (progress[...].RiskID != 0) takes none.
func buildCustomerSummaries(rows []Row, rd RefData, progress map[int]ResumeState, nextSeq map[int]int) []CustomerSummary {
	sorted := append([]Row(nil), rows...)
	sortByMigrationID(sorted)

	byCustomer := map[int]*CustomerSummary{}
	seq := map[int]int{}
	for _, row := range sorted {
		if row.CustomerID == 0 {
			continue
		}
		s, ok := byCustomer[row.CustomerID]
		if !ok {
			s = &CustomerSummary{Customer: row.Customer, Code: rd.CustomerCodeByID[row.CustomerID]}
			byCustomer[row.CustomerID] = s
			seq[row.CustomerID] = nextSeq[row.CustomerID]
		}
		s.Risks++
		if row.WorkflowStatus == "CLOSED" {
			s.Closed++
		} else {
			s.Approved++
		}
		if progress[row.MigrationID].RiskID != 0 {
			continue
		}
		code := riskCode(row.RiskYear, rd.TeamCodeByID[row.SourceRegisterID], s.Code, row.RiskQuarter, seq[row.CustomerID])
		seq[row.CustomerID]++
		s.New++
		if s.FirstCode == "" {
			s.FirstCode = code
		}
		s.LastCode = code
	}

	out := make([]CustomerSummary, 0, len(byCustomer))
	for _, s := range byCustomer {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Customer) < strings.ToLower(out[j].Customer)
	})
	return out
}

// existingRisks says which rows already have a marker risk, matched by the
// same natural key resume uses. It reads only the risk search, so a dry run can
// call it. A row that matches more than one marker risk is REJECTed exactly as
// reconstructState would reject it, and is left unmatched; a row with no match
// is absent from the map (its RiskID is zero).
func existingRisks(ctx context.Context, ec *EntityClient, rows []Row, rep *Report) (map[int]ResumeState, error) {
	found, err := searchMarkerRisks(ctx, ec, rows)
	if err != nil {
		return nil, err
	}
	byKey := map[string][]Risk{}
	for _, r := range found {
		k := naturalKey(r.RiskTitle, r.SourceRegID, deref(r.CustomerName), r.RiskYear, r.RiskQuarter)
		byKey[k] = append(byKey[k], r)
	}
	out := make(map[int]ResumeState, len(rows))
	for _, row := range rows {
		matches := byKey[naturalKey(row.RiskTitle, row.SourceRegisterID, row.Customer, row.RiskYear, row.RiskQuarter)]
		switch len(matches) {
		case 0:
		case 1:
			out[row.MigrationID] = ResumeState{Progress: ProgressCreated, RiskID: matches[0].ID, CurrentStatus: matches[0].WorkflowStatus}
		default:
			rep.Add(ambiguousMatchFinding(row, len(matches)))
		}
	}
	return out, nil
}

// dryRunChecks is what a dry run learns from the entity beyond preflight. It
// reaches the same row-level verdicts the real run reaches in reconstructState
// (rows that share a natural key, rows that match two existing risks), then
// fills the per-customer summary from the rows that survive. It only reads, and
// returns the rows that survive.
func dryRunChecks(ctx context.Context, log *slog.Logger, ec *EntityClient, rd RefData, pending []Row, rep *Report) ([]Row, error) {
	rejectDuplicateKeys(pending, rep)
	pending = selectMigratable(pending, rep)

	existing, err := existingRisks(ctx, ec, pending, rep)
	if err != nil {
		log.Error("could not look up existing risks — aborting", "err", err)
		return nil, err
	}
	pending = selectMigratable(pending, rep)

	if err := summarizeCustomers(ctx, log, ec, rd, pending, existing, rep); err != nil {
		return nil, err
	}
	return pending, nil
}

// computeCustomerSummaries reads each customer's next number from the entity
// (once, and only for a customer that has a row still to create) and builds the
// summaries. Read-only, so it is safe on a dry run.
func computeCustomerSummaries(ctx context.Context, ec *EntityClient, rd RefData, rows []Row, progress map[int]ResumeState) ([]CustomerSummary, error) {
	nextSeq := map[int]int{}
	for _, row := range rows {
		if row.CustomerID == 0 || progress[row.MigrationID].RiskID != 0 {
			continue
		}
		if _, done := nextSeq[row.CustomerID]; done {
			continue
		}
		n, err := ec.NextSequenceNumber(ctx, row.SourceRegisterID, row.CustomerID)
		if err != nil {
			return nil, fmt.Errorf("next sequence number for customer %d: %w", row.CustomerID, err)
		}
		nextSeq[row.CustomerID] = n
	}
	return buildCustomerSummaries(rows, rd, progress, nextSeq), nil
}

func (r *Report) emitCustomerSummaries(w io.Writer) {
	if len(r.customerSummaries) == 0 {
		return
	}
	fmt.Fprintln(w, "per-customer summary (check the codes before the real run - they cannot be changed afterwards):")
	for _, s := range r.customerSummaries {
		fmt.Fprintf(w, "  %s (%s): %s (%d approved, %d closed), %d new",
			s.Customer, s.Code, plural(s.Risks, "risk", "risks"), s.Approved, s.Closed, s.New)
		if s.New > 0 {
			fmt.Fprintf(w, ", codes %s .. %s", s.FirstCode, s.LastCode)
		}
		fmt.Fprintln(w)
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// maxListedIDs caps the Migration IDs printed per value; a value on hundreds of
// rows would otherwise bury the rest of the report.
const maxListedIDs = 10

// lookupFailures are the columns whose unknown values the admins can add. The
// order is the order the blocks print in.
var lookupFailures = []string{"Customer", "Deployment Type", "Product"}

// emitLookupProblems prints the distinct lookup values behind the row REJECTs,
// grouped for whoever has to act: values to add, values to reactivate, and
// invalid Environment spellings (a fixed list the sheet owner must fix).
func (r *Report) emitLookupProblems(w io.Writer) {
	adds := groupLookupProblems(r.findings, ProblemUnknown, lookupFailures)
	reactivate := groupLookupProblems(r.findings, ProblemInactive, lookupFailures)
	envs := groupLookupProblems(r.findings, ProblemUnknown, []string{"Environment"})

	if len(adds) > 0 {
		fmt.Fprintln(w, "unknown values to add in the Admin Console:")
		writeLookupGroups(w, adds, true)
	}
	if len(reactivate) > 0 {
		fmt.Fprintln(w, "inactive values to reactivate in the Admin Console:")
		writeLookupGroups(w, reactivate, true)
	}
	if len(envs) > 0 {
		fmt.Fprintln(w, "invalid environments (fix in the sheet - Environment is a fixed list: Production, Non-Production, DR):")
		writeLookupGroups(w, envs, false)
	}
}

type lookupGroup struct {
	failure string
	values  []lookupValue
}

type lookupValue struct {
	name string
	ids  []int
}

// groupLookupProblems collects findings with the given Problem on the given
// columns, one entry per distinct value (case and spacing ignored, first
// spelling kept), each with the Migration IDs it appears on.
func groupLookupProblems(fs []Finding, problem string, failures []string) []lookupGroup {
	var out []lookupGroup
	for _, failure := range failures {
		byKey := map[string]*lookupValue{}
		var order []string
		for _, f := range fs {
			if f.Failure != failure || f.Problem != problem || f.Value == "" {
				continue
			}
			k := normHeader(f.Value)
			v, ok := byKey[k]
			if !ok {
				v = &lookupValue{name: f.Value}
				byKey[k] = v
				order = append(order, k)
			}
			if len(v.ids) == 0 || v.ids[len(v.ids)-1] != f.MigrationID {
				v.ids = append(v.ids, f.MigrationID)
			}
		}
		if len(order) == 0 {
			continue
		}
		sort.Strings(order)
		g := lookupGroup{failure: failure}
		for _, k := range order {
			sort.Ints(byKey[k].ids)
			g.values = append(g.values, *byKey[k])
		}
		out = append(out, g)
	}
	return out
}

func writeLookupGroups(w io.Writer, groups []lookupGroup, withHeading bool) {
	for _, g := range groups {
		indent := "  "
		if withHeading {
			fmt.Fprintf(w, "  %s (%d):\n", g.failure, len(g.values))
			indent = "    "
		} else {
			indent = "  "
		}
		for _, v := range g.values {
			fmt.Fprintf(w, "%s%q - %s (%s)\n", indent, v.name, plural(len(v.ids), "row", "rows"), idList(v.ids))
		}
	}
}

func idList(ids []int) string {
	label := "Migration IDs "
	if len(ids) == 1 {
		label = "Migration ID "
	}
	shown, more := ids, 0
	if len(ids) > maxListedIDs {
		shown, more = ids[:maxListedIDs], len(ids)-maxListedIDs
	}
	parts := make([]string, len(shown))
	for i, id := range shown {
		parts[i] = fmt.Sprint(id)
	}
	s := label + strings.Join(parts, ", ")
	if more > 0 {
		s += fmt.Sprintf(" and %d more", more)
	}
	return s
}
