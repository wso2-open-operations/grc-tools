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
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	riskservice "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// riskNotFound is the answer for a code that matches no risk and for one the
// caller may not see. The two are worded the same on purpose: the assistant must
// not confirm that a risk exists to someone who cannot open it.
const riskNotFound = "I couldn't find a risk with that code that you have access to. Check the code, or open Risk Registers and search for it."

// maxExplainEvents is how many history events an explanation lists.
const maxExplainEvents = 12

// loadVisibleRisk finds the risk with a code and returns it only if the caller
// may open it. A code that matches nothing and a risk the caller may not see
// both come back as nil, so nothing built on this can tell them apart. The
// directory names on the risk are filled in.
func (d *Deps) loadVisibleRisk(ctx context.Context, code string) (*model.RiskDetail, error) {
	page, err := d.Risk.List(ctx, model.ListRisksFilter{Search: code, Limit: 20})
	if err != nil {
		return nil, err
	}
	id := 0
	for _, item := range page.Items {
		if strings.EqualFold(item.RiskCode, code) {
			id = item.ID
			break
		}
	}
	if id == 0 {
		return nil, nil
	}
	visible, err := d.riskVisibleToCaller(ctx, id)
	if err != nil || !visible {
		return nil, err
	}
	detail, err := d.Risk.GetByID(ctx, id)
	if err != nil || detail == nil {
		return nil, err
	}
	d.enrichDetail(ctx, detail)
	return detail, nil
}

// riskHistory returns a risk's history when the caller may read it, under the
// same privilege as the History tab. ok is false when they may not, or when it
// could not be read.
func (d *Deps) riskHistory(ctx context.Context, id int) (entries []*model.HistoryEntry, ok bool) {
	if !auth.HasPrivilege(ctx, privilege.ViewRisks) {
		return nil, false
	}
	entries, err := d.History.List(ctx, id)
	if err != nil {
		slog.WarnContext(ctx, "risk assistant: history unavailable", "risk_id", id, "err", err)
		return nil, false
	}
	d.enrichHistory(ctx, entries)
	return entries, true
}

// explainRisk writes a plain-language account of one risk, from its own record
// and history, for the risk assistant. It also returns a link the web app can use
// to open the risk, and the risk's history as a timeline, when the caller may
// read it. It is written here from fixed wording; no model sees the risk. The
// caller must be allowed to see the risk (the same rule as opening it).
func (d *Deps) explainRisk(ctx context.Context, code string) (string, *riskservice.RiskLink, *riskservice.Timeline, error) {
	detail, err := d.loadVisibleRisk(ctx, code)
	if err != nil {
		return "", nil, nil, err
	}
	if detail == nil {
		return riskNotFound, nil, nil, nil
	}
	var timeline *riskservice.Timeline
	if history, ok := d.riskHistory(ctx, detail.ID); ok {
		timeline = buildTimeline(history)
	}
	return describeRisk(detail, time.Now()), &riskservice.RiskLink{ID: detail.ID, Code: detail.RiskCode}, timeline, nil
}

// statusText says, in plain words, where a risk is in the approval workflow.
var statusText = map[string]string{
	model.StatusPendingOwnerApproval:      "waiting for the Risk Owner to approve it",
	model.StatusPendingManagementApproval: "waiting for management to approve it",
	model.StatusPendingComplianceReview:   "waiting for the Compliance team's review",
	model.StatusApproved:                  "approved and open, with its action plan being carried out",
	model.StatusPendingOwnerCompletion:    "waiting for the Risk Owner to confirm the work is complete",
	model.StatusPendingManagementClosure:  "waiting for management to approve closing it",
	model.StatusPendingComplianceClosure:  "waiting for the Compliance team to close it",
	model.StatusPendingAmendment:          "sent back to be amended",
	model.StatusPendingRevision:           "sent back for revision",
	model.StatusEscalated:                 "escalated because it is overdue",
	model.StatusClosed:                    "closed",
	model.StatusCancelled:                 "cancelled",
	"DRAFT":                               "a draft that has not been submitted yet",
}

// words turns an enum such as PENDING_RISK_OWNER_APPROVAL into "Pending risk
// owner approval".
func words(enum string) string {
	s := strings.ToLower(strings.ReplaceAll(enum, "_", " "))
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func shorten(s string, n int) string {
	s = strings.TrimSpace(strings.Join(strings.Fields(s), " "))
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return strings.TrimSpace(string([]rune(s)[:n])) + "…"
}

func dateOnly(s *string) (time.Time, bool) {
	if s == nil || len(*s) < 10 {
		return time.Time{}, false
	}
	t, err := time.Parse("2006-01-02", (*s)[:10])
	return t, err == nil
}

func day(t time.Time) string { return t.Format("2 Jan 2006") }

// describeRisk writes the explanation of where a risk stands: its status, people
// and dates. The history is separate (see buildTimeline). now is passed in so the
// "due in" and "overdue" wording can be tested.
func describeRisk(r *model.RiskDetail, now time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Risk %s: “%s”\n", r.RiskCode, shorten(r.RiskTitle, 160))

	b.WriteString("\nWhere it stands\n")
	status := words(r.WorkflowStatus)
	if text, ok := statusText[r.WorkflowStatus]; ok {
		fmt.Fprintf(&b, "• Status: %s, %s.\n", status, text)
	} else {
		fmt.Fprintf(&b, "• Status: %s.\n", status)
	}
	if r.SourceRegisterName != "" || r.AssignmentTeamName != "" {
		switch {
		case r.SourceRegisterName != "" && r.AssignmentTeamName != "":
			fmt.Fprintf(&b, "• Register and team: raised in %s, assigned to %s.\n", r.SourceRegisterName, r.AssignmentTeamName)
		case r.SourceRegisterName != "":
			fmt.Fprintf(&b, "• Register: %s.\n", r.SourceRegisterName)
		default:
			fmt.Fprintf(&b, "• Team: %s.\n", r.AssignmentTeamName)
		}
	}
	if r.GrossScore != nil {
		gross := words(r.GrossScore.RiskLevel)
		switch {
		case r.EffectiveScore == nil || r.EffectiveScore.RiskLevel == r.GrossScore.RiskLevel:
			fmt.Fprintf(&b, "• Level: %s.\n", gross)
		default:
			fmt.Fprintf(&b, "• Level: %s when it was raised, %s now after reassessment.\n", gross, words(r.EffectiveScore.RiskLevel))
		}
	}
	if r.TreatmentStrategy != nil && *r.TreatmentStrategy != "" {
		fmt.Fprintf(&b, "• Treatment: %s.\n", words(*r.TreatmentStrategy))
	}
	if due, ok := dateOnly(r.ImplementationDate); ok {
		line := "• Implementation date: " + day(due)
		if r.WorkflowStatus != model.StatusClosed && r.WorkflowStatus != model.StatusCancelled {
			days := int(due.Sub(now.Truncate(24*time.Hour)).Hours() / 24)
			switch {
			case days > 0:
				line += fmt.Sprintf(" (due in %d days)", days)
			case days == 0:
				line += " (due today)"
			default:
				line += fmt.Sprintf(" (overdue by %d days)", -days)
			}
		}
		b.WriteString(line + ".\n")
	}
	if next, ok := dateOnly(r.ReassessmentDate); ok {
		fmt.Fprintf(&b, "• Next reassessment: %s.\n", day(next))
	}
	if r.Progress != nil && strings.TrimSpace(*r.Progress) != "" {
		fmt.Fprintf(&b, "• Latest progress note: %s\n", shorten(*r.Progress, 300))
	}
	if r.WorkflowStatus == model.StatusPendingRevision || r.WorkflowStatus == model.StatusPendingAmendment {
		if r.RejectionComment != nil && strings.TrimSpace(*r.RejectionComment) != "" {
			fmt.Fprintf(&b, "• Why it was sent back: %s\n", shorten(*r.RejectionComment, 300))
		}
	}

	var people []string
	for _, p := range []struct{ role, name string }{
		{"Risk Owner", r.OwnerName},
		{"Risk Assigner", r.AssignerName},
		{"Management approver", r.ManagementApproverName},
	} {
		if p.name != "" {
			people = append(people, p.role+": "+p.name)
		}
	}
	if len(people) > 0 {
		b.WriteString("\nPeople\n• " + strings.Join(people, "\n• ") + "\n")
	}

	if d := strings.TrimSpace(r.RiskDescription); d != "" {
		fmt.Fprintf(&b, "\nAbout it\n%s\n", shorten(d, 400))
	}
	if r.ImpactDescription != nil && strings.TrimSpace(*r.ImpactDescription) != "" {
		fmt.Fprintf(&b, "Impact: %s\n", shorten(*r.ImpactDescription, 300))
	}

	return strings.TrimRight(b.String(), "\n")
}

// buildTimeline turns a risk's history into dated lines, oldest first, keeping
// the latest maxExplainEvents when there are more.
func buildTimeline(history []*model.HistoryEntry) *riskservice.Timeline {
	events := append([]*model.HistoryEntry{}, history...)
	sort.SliceStable(events, func(i, j int) bool { return events[i].CreatedAt.Before(events[j].CreatedAt) })

	t := &riskservice.Timeline{Events: []riskservice.TimelineEvent{}}
	if len(events) == 0 {
		t.Note = "Nothing has been recorded for this risk yet."
		return t
	}
	if len(events) > maxExplainEvents {
		t.Note = fmt.Sprintf("Showing the latest %d of %d events.", maxExplainEvents, len(events))
		events = events[len(events)-maxExplainEvents:]
	}
	for _, e := range events {
		t.Events = append(t.Events, riskservice.TimelineEvent{Date: day(e.CreatedAt), Text: describeEvent(e)})
	}
	return t
}

func actorName(e *model.HistoryEntry) string {
	switch {
	case e.CreatedByName != "":
		return e.CreatedByName
	case e.CreatedByEmail != "":
		return e.CreatedByEmail
	case strings.HasPrefix(strings.ToLower(e.CreatedBy), "system"):
		return "The system"
	}
	return "Someone"
}

// describeEvent writes one history event as a sentence that starts with who did
// it.
func describeEvent(e *model.HistoryEntry) string {
	who := actorName(e)
	var d model.HistoryDetails
	if len(e.Details) > 0 {
		_ = json.Unmarshal(e.Details, &d)
	}
	moved := ""
	if d.From != "" && d.To != "" {
		moved = fmt.Sprintf(" It moved from %s to %s.", strings.ToLower(words(d.From)), strings.ToLower(words(d.To)))
	}
	role := ""
	if d.Role != "" {
		role = " as " + d.Role
	}
	comment := ""
	if d.Comment != "" {
		comment = " Comment: “" + shorten(d.Comment, 200) + "”."
	}

	switch e.Action {
	case model.HistoryCreate:
		if d.Plan != "" {
			return fmt.Sprintf("%s added the action plan “%s”.", who, shorten(d.Plan, 80))
		}
		return who + " created the risk."
	case model.HistoryUpdate:
		target := "a detail"
		if e.FieldChanged != nil && *e.FieldChanged != "" {
			target = "the " + strings.ToLower(words(*e.FieldChanged))
		}
		switch {
		case e.OldValue != nil && e.NewValue != nil:
			return fmt.Sprintf("%s changed %s from “%s” to “%s”.", who, target, shorten(*e.OldValue, 60), shorten(*e.NewValue, 60))
		case e.NewValue != nil:
			return fmt.Sprintf("%s set %s to “%s”.", who, target, shorten(*e.NewValue, 60))
		}
		return fmt.Sprintf("%s changed %s.", who, target)
	case model.HistoryDelete:
		return who + " removed an item from the risk."
	case model.HistorySubmit:
		return who + " submitted it." + moved
	case model.HistoryApprove:
		return who + " approved it" + role + "." + moved
	case model.HistoryReject:
		stage := ""
		if d.Stage != "" {
			stage = " (" + strings.ToLower(words(d.Stage)) + ")"
		}
		return who + " sent it back" + stage + "." + moved + comment
	case model.HistoryEscalate:
		if d.OverdueDays > 0 {
			return fmt.Sprintf("%s escalated it because it was %d days past its implementation date.", who, d.OverdueDays)
		}
		return who + " escalated it." + comment
	case model.HistoryComment:
		if d.Comment != "" {
			return who + " commented: “" + shorten(d.Comment, 200) + "”."
		}
		return who + " added a comment."
	case model.HistoryAssess:
		if d.PreviousLevel != "" && d.Level != "" {
			return fmt.Sprintf("%s reassessed it: the level went from %s to %s.", who, strings.ToLower(d.PreviousLevel), strings.ToLower(d.Level))
		}
		return who + " reassessed it."
	case model.HistoryComplete:
		return who + " marked the work complete." + moved
	case model.HistoryClose:
		return who + " closed it." + moved
	case model.HistoryCancel:
		return who + " cancelled it." + comment
	}
	return fmt.Sprintf("%s made a change (%s).", who, strings.ToLower(e.Action))
}
