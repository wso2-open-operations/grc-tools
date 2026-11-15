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

import (
	"fmt"
	"strings"
)

// Summarize writes one or two plain sentences about a result, from the numbers
// in it. It runs in this process and the sentences are built from fixed
// templates: nothing is sent to a model, so the figures stay inside the
// platform. It returns "" when there is nothing useful to add.
func Summarize(spec Spec, res *Result) string {
	if res == nil || len(res.Rows) == 0 {
		return ""
	}
	ds, ok := lookupDataset(spec.Dataset)
	if !ok {
		return ""
	}

	if ds.Rows {
		n := len(res.Rows)
		one, many := ds.items()
		if res.Truncated {
			return fmt.Sprintf("Showing the first %d %s; there are more. Add a filter to narrow it down.", n, many)
		}
		return fmt.Sprintf("Showing %s.", plural(n, one, many))
	}

	var dims, measures []int
	for i, c := range res.Columns {
		switch c.Role {
		case "dimension":
			dims = append(dims, i)
		case "measure":
			measures = append(measures, i)
		}
	}
	if len(measures) == 0 {
		return ""
	}

	var out string
	switch len(dims) {
	case 0:
		var parts []string
		for _, m := range measures {
			if s := measureSentence(res.Columns[m], res.Rows[0][m]); s != "" {
				parts = append(parts, s)
			}
		}
		out = strings.Join(parts, " ")
	case 1:
		out = groupedSentence(ds, res, dims[0], measures[0])
	default:
		m := measures[0]
		if !res.Truncated && additive(ds, res, dims, res.Columns[m].ID) {
			out = measureSentence(res.Columns[m], total(res, m))
			out = strings.TrimSuffix(out, ".") + " in total across this breakdown."
		}
	}
	if res.Truncated && out != "" {
		if len(res.Rows) == 1 {
			out += " Only the first row is shown."
		} else {
			out += fmt.Sprintf(" Only the first %d rows are shown.", len(res.Rows))
		}
	}
	return out
}

// groupedSentence describes a breakdown by one dimension: the total when adding
// the groups up is meaningful, and the largest group.
func groupedSentence(ds DatasetDef, res *Result, dim, measure int) string {
	col := res.Columns[measure]
	top, topVal := -1, 0.0
	for i, row := range res.Rows {
		v, ok := number(row[measure])
		if ok && (top < 0 || v > topVal) {
			top, topVal = i, v
		}
	}
	if top < 0 {
		return ""
	}
	name := cellString(res.Rows[top][dim])
	dimLabel := res.Columns[dim].Label

	// With rows missing, the groups shown do not add up to the true total.
	if res.Truncated {
		return fmt.Sprintf("The largest group shown is %s with %s.", name, formatValue(col, topVal))
	}
	if additive(ds, res, []int{dim}, col.ID) {
		return fmt.Sprintf("%s in total, grouped by %s. The largest group is %s with %s.",
			strings.TrimSuffix(measureSentence(col, total(res, measure)), "."), dimLabel, name, formatValue(col, topVal))
	}
	if col.Role == "measure" && strings.HasSuffix(col.ID, "_count") {
		// A count that cannot be added up (a risk in two categories is in both).
		return fmt.Sprintf("The largest group by %s is %s with %s.", dimLabel, name, formatValue(col, topVal))
	}
	return fmt.Sprintf("The highest %s is in %s at %s.", strings.ToLower(col.Label), name, formatValue(col, topVal))
}

// additive reports whether adding a measure's values across groups gives the
// true total. Counts of suggestions always do; a count of risks does not when
// a grouping field repeats a risk's row (a risk with two categories is counted
// in both).
func additive(ds DatasetDef, res *Result, dims []int, measureID string) bool {
	switch measureID {
	case "suggestion_count", "accepted_count":
		return true
	// Audit counts: every audit field is one value per control or evidence
	// round, so the groups always add up to the whole.
	case "audit_count", "control_count", "overdue_count", "complete_count", "evidence_count", "approved_count", "rejected_count":
		return true
	case "risk_count":
		for _, d := range dims {
			if f, ok := ds.field(res.Columns[d].ID); ok && f.Repeats {
				return false
			}
		}
		return true
	}
	return false
}

func total(res *Result, col int) float64 {
	sum := 0.0
	for _, row := range res.Rows {
		if v, ok := number(row[col]); ok {
			sum += v
		}
	}
	return sum
}

func number(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

func cellString(v any) string {
	switch x := v.(type) {
	case nil:
		return "no value"
	case string:
		if x == "" {
			return "no value"
		}
		return x
	}
	if n, ok := number(v); ok {
		return formatNumber(n, 0)
	}
	return fmt.Sprint(v)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

func formatNumber(v float64, decimals int) string {
	s := fmt.Sprintf("%.*f", decimals, v)
	if decimals > 0 {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	return s
}

// formatValue reads a measure value the way a person would.
func formatValue(col Column, v float64) string {
	if strings.HasSuffix(col.ID, "_rate") {
		return formatNumber(v*100, 1) + "%"
	}
	if col.Kind == KindNumber {
		return formatNumber(v, 2)
	}
	return formatNumber(v, 0)
}

// measureSentence turns one measure's value into a sentence.
func measureSentence(col Column, value any) string {
	v, ok := number(value)
	if !ok {
		return ""
	}
	n := int(v)
	switch col.ID {
	case "risk_count":
		if n == 1 {
			return "There is 1 risk."
		}
		return fmt.Sprintf("There are %d risks.", n)
	case "suggestion_count":
		if n == 1 {
			return "There is 1 AI suggestion."
		}
		return fmt.Sprintf("There are %d AI suggestions.", n)
	case "accepted_count":
		if n == 1 {
			return "1 AI suggestion was accepted."
		}
		return fmt.Sprintf("%d AI suggestions were accepted.", n)
	case "acceptance_rate":
		return fmt.Sprintf("Users accepted %s of AI suggestions.", formatValue(col, v))
	case "audit_count":
		return countSentence(n, "There is 1 audit.", "There are %d audits.")
	case "control_count":
		return countSentence(n, "There is 1 control.", "There are %d controls.")
	case "overdue_count":
		return countSentence(n, "1 control is overdue.", "%d controls are overdue.")
	case "complete_count":
		return countSentence(n, "1 control is complete.", "%d controls are complete.")
	case "evidence_count":
		return countSentence(n, "There is 1 evidence submission.", "There are %d evidence submissions.")
	case "approved_count":
		return countSentence(n, "1 evidence submission was approved.", "%d evidence submissions were approved.")
	case "rejected_count":
		return countSentence(n, "1 evidence submission was rejected.", "%d evidence submissions were rejected.")
	case "overdue_rate":
		return fmt.Sprintf("%s of controls are overdue.", formatValue(col, v))
	case "complete_rate":
		return fmt.Sprintf("%s of controls are complete.", formatValue(col, v))
	case "approved_rate":
		return fmt.Sprintf("%s of evidence submissions were approved.", formatValue(col, v))
	case "rejection_rate":
		return fmt.Sprintf("%s of evidence submissions were rejected.", formatValue(col, v))
	case "avg_age_days":
		return fmt.Sprintf("Risks are %s days old on average.", formatValue(col, v))
	case "avg_gross_rating":
		return fmt.Sprintf("The average gross score is %s.", formatValue(col, v))
	case "avg_residual_rating":
		return fmt.Sprintf("The average residual score is %s.", formatValue(col, v))
	}
	return fmt.Sprintf("%s: %s.", col.Label, formatValue(col, v))
}

func countSentence(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return fmt.Sprintf(many, n)
}
