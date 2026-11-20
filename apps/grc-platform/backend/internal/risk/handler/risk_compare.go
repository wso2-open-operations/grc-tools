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

package handler

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	riskservice "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/service"
)

// notBothFound answers a comparison when either risk cannot be shown to the
// caller. It does not say which one, so it cannot be used to find out that a
// risk exists.
const notBothFound = "I couldn't find both of those risks with the access you have. Check the codes, or open Risk Registers and search for them."

// compareRisks puts two risks side by side, from their own records, for the risk
// assistant. Both must be risks the caller may open; if either is not, the answer
// is the same whichever one it was.
func (d *Deps) compareRisks(ctx context.Context, codes []string) (string, *riskservice.CompareTable, error) {
	if len(codes) != 2 {
		return notBothFound, nil, nil
	}
	first, err := d.loadVisibleRisk(ctx, codes[0])
	if err != nil {
		return "", nil, err
	}
	second, err := d.loadVisibleRisk(ctx, codes[1])
	if err != nil {
		return "", nil, err
	}
	if first == nil || second == nil {
		return notBothFound, nil, nil
	}

	var lasts [2]string
	historyAllowed := false
	for i, r := range []*model.RiskDetail{first, second} {
		entries, ok := d.riskHistory(ctx, r.ID)
		historyAllowed = ok
		if ok {
			lasts[i] = historySummary(entries)
		}
	}
	table := buildComparison(first, second, historyAllowed, lasts, time.Now())
	return compareSummary(table), table, nil
}

// historySummary says how many events a risk has and what the latest was.
func historySummary(entries []*model.HistoryEntry) string {
	if len(entries) == 0 {
		return "Nothing recorded"
	}
	events := append([]*model.HistoryEntry{}, entries...)
	sort.SliceStable(events, func(i, j int) bool { return events[i].CreatedAt.Before(events[j].CreatedAt) })
	last := events[len(events)-1]
	return fmt.Sprintf("%d events, latest on %s: %s", len(events), day(last.CreatedAt), describeEvent(last))
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

func levelNow(r *model.RiskDetail) string {
	switch {
	case r.EffectiveScore != nil:
		return words(r.EffectiveScore.RiskLevel)
	case r.GrossScore != nil:
		return words(r.GrossScore.RiskLevel)
	}
	return ""
}

func levelRaised(r *model.RiskDetail) string {
	if r.GrossScore != nil {
		return words(r.GrossScore.RiskLevel)
	}
	return ""
}

func strOf(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func dateText(s *string) string {
	if t, ok := dateOnly(s); ok {
		return day(t)
	}
	return ""
}

// implementationText is the implementation date, with how far off or overdue it
// is while the risk is still open.
func implementationText(r *model.RiskDetail, now time.Time) string {
	due, ok := dateOnly(r.ImplementationDate)
	if !ok {
		return ""
	}
	text := day(due)
	if r.WorkflowStatus == model.StatusClosed || r.WorkflowStatus == model.StatusCancelled {
		return text
	}
	days := int(due.Sub(now.Truncate(24*time.Hour)).Hours() / 24)
	switch {
	case days > 0:
		return fmt.Sprintf("%s (due in %d days)", text, days)
	case days == 0:
		return text + " (due today)"
	}
	return fmt.Sprintf("%s (overdue by %d days)", text, -days)
}

// buildComparison writes one row per thing worth comparing, marking the rows
// where the two risks differ. last holds each risk's history summary and is
// used only when the caller may read history.
func buildComparison(a, b *model.RiskDetail, historyAllowed bool, last [2]string, now time.Time) *riskservice.CompareTable {
	t := &riskservice.CompareTable{
		Risks: []riskservice.RiskLink{{ID: a.ID, Code: a.RiskCode}, {ID: b.ID, Code: b.RiskCode}},
	}
	add := func(label, va, vb string) {
		va, vb = orDash(va), orDash(vb)
		t.Rows = append(t.Rows, riskservice.CompareRow{
			Label:   label,
			Values:  []string{va, vb},
			Differs: !strings.EqualFold(strings.TrimSpace(va), strings.TrimSpace(vb)),
		})
	}
	add("Title", shorten(a.RiskTitle, 120), shorten(b.RiskTitle, 120))
	add("Status", words(a.WorkflowStatus), words(b.WorkflowStatus))
	add("Register", a.SourceRegisterName, b.SourceRegisterName)
	add("Assignment team", a.AssignmentTeamName, b.AssignmentTeamName)
	add("Level when raised", levelRaised(a), levelRaised(b))
	add("Level now", levelNow(a), levelNow(b))
	add("Treatment", words(strOf(a.TreatmentStrategy)), words(strOf(b.TreatmentStrategy)))
	add("Implementation date", implementationText(a, now), implementationText(b, now))
	add("Next reassessment", dateText(a.ReassessmentDate), dateText(b.ReassessmentDate))
	add("Risk Owner", a.OwnerName, b.OwnerName)
	add("Risk Assigner", a.AssignerName, b.AssignerName)
	add("Management approver", a.ManagementApproverName, b.ManagementApproverName)
	add("Raised on", dateText(strPtr(a.CreatedAt)), dateText(strPtr(b.CreatedAt)))
	add("Last updated", dateText(strPtr(a.UpdatedAt)), dateText(strPtr(b.UpdatedAt)))
	if historyAllowed {
		add("History", last[0], last[1])
	}
	return t
}

func strPtr(s string) *string { return &s }

// compareSummary says in a sentence where the two risks differ.
func compareSummary(t *riskservice.CompareTable) string {
	var differs []string
	for _, r := range t.Rows {
		if r.Differs && r.Label != "Title" && r.Label != "History" {
			differs = append(differs, strings.ToLower(r.Label))
		}
	}
	head := fmt.Sprintf("Comparing %s and %s.", t.Risks[0].Code, t.Risks[1].Code)
	if len(differs) == 0 {
		return head + " They match on everything shown below."
	}
	return head + " They differ in: " + strings.Join(differs, ", ") + "."
}
