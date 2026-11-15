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

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	auditservice "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/dashboardagent"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// Describing and comparing controls for the Audit assistant. Everything here is
// written from the control's own record and history, from fixed wording: no
// model sees the control. The control is found with the caller's own scope (see
// resolveControl), so a control the caller cannot open is answered exactly like
// one that does not exist.

// maxControlTimelineEvents is how many history events are shown.
const maxControlTimelineEvents = 12

// notBothControls answers a comparison when either control cannot be shown to
// the caller. It does not say which one.
const notBothControls = "I couldn't find both of those controls with the access you have. Check the numbers and the audit name, or open Audits and search for them."

// loadControl reads a control the caller was already found to see.
func (d *Deps) loadControl(ctx context.Context, h *controlHit) (*model.AuditControl, error) {
	c, err := d.Control.GetByID(ctx, h.auditID, h.controlID)
	if err != nil || c == nil {
		return nil, err
	}
	return c, nil
}

// controlHistory returns a control's history when the caller may read it: the
// same two privileges as the History tab. ok is false when they may not, or when
// it could not be read.
func (d *Deps) controlHistory(ctx context.Context, h *controlHit) (entries []*model.AuditTrailEntry, ok bool) {
	if !auth.HasPrivilege(ctx, privilege.ViewAudits) || !auth.HasPrivilege(ctx, privilege.ViewInternalComments) || d.Trail == nil {
		return nil, false
	}
	entries, _, err := d.Trail.ListByControl(ctx, h.auditID, h.controlID, true)
	if err != nil {
		slog.WarnContext(ctx, "audit assistant: history unavailable", "control_id", h.controlID, "err", err)
		return nil, false
	}
	if d.Directory != nil {
		(&trailHandler{directory: d.Directory}).resolveTrailActors(ctx, entries)
	}
	return entries, true
}

// explainControl describes one control for the Audit assistant.
func (d *Deps) explainControl(ctx context.Context, scope dashboardagent.AuditScope, number, auditName string) (*auditservice.AssistantResult, error) {
	h, reply, err := d.resolveControl(ctx, scope, number, auditName)
	if err != nil || reply != nil {
		return reply, err
	}
	c, err := d.loadControl(ctx, h)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return &auditservice.AssistantResult{Action: auditservice.AssistantReply, Message: controlNotFound}, nil
	}
	res := &auditservice.AssistantResult{
		Action:      auditservice.AssistantReply,
		Message:     describeControl(c, h.audit, time.Now()),
		ControlLink: &auditservice.ControlLink{AuditID: h.auditID, ControlID: h.controlID, Code: c.ControlNumber},
	}
	if entries, ok := d.controlHistory(ctx, h); ok {
		res.Timeline = buildControlTimeline(entries)
	}
	return res, nil
}

// compareControls puts two controls side by side for the Audit assistant.
func (d *Deps) compareControls(ctx context.Context, scope dashboardagent.AuditScope, n1, a1, n2, a2 string) (*auditservice.AssistantResult, error) {
	notBoth := &auditservice.AssistantResult{Action: auditservice.AssistantReply, Message: notBothControls}
	if strings.TrimSpace(a2) == "" {
		a2 = a1 // one audit named for both
	}
	h1, r1, err := d.resolveControl(ctx, scope, n1, a1)
	if err != nil {
		return nil, err
	}
	h2, r2, err := d.resolveControl(ctx, scope, n2, a2)
	if err != nil {
		return nil, err
	}
	// If either needs a question (which audit?), ask it; otherwise a control that
	// cannot be shown gives the same answer whichever one it was.
	for _, r := range []*auditservice.AssistantResult{r1, r2} {
		if r != nil && strings.Contains(r.Message, "Which one do you mean?") {
			return r, nil
		}
	}
	if h1 == nil || h2 == nil {
		return notBoth, nil
	}
	c1, err := d.loadControl(ctx, h1)
	if err != nil {
		return nil, err
	}
	c2, err := d.loadControl(ctx, h2)
	if err != nil {
		return nil, err
	}
	if c1 == nil || c2 == nil {
		return notBoth, nil
	}

	var lasts [2]string
	historyAllowed := false
	for i, h := range []*controlHit{h1, h2} {
		entries, ok := d.controlHistory(ctx, h)
		historyAllowed = ok
		if ok {
			lasts[i] = controlHistorySummary(entries)
		}
	}
	table := buildControlComparison(c1, c2, h1, h2, historyAllowed, lasts, time.Now())
	return &auditservice.AssistantResult{Action: auditservice.AssistantReply, Message: controlCompareSummary(table), Compare: table}, nil
}

// ---- wording ----

func words(s string) string { return strings.ToLower(strings.ReplaceAll(s, "_", " ")) }

func day(t time.Time) string { return t.Format("2 Jan 2006") }

func dateString(s *string) (time.Time, bool) {
	if s == nil || *s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{"2006-01-02", time.RFC3339} {
		if t, err := time.Parse(layout, strings.TrimSpace(*s)); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func nameOr(s *string, none string) string {
	if s == nil || strings.TrimSpace(*s) == "" {
		return none
	}
	return strings.TrimSpace(*s)
}

func shorten(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func requirementWords(c *model.AuditControl) string {
	rt := "design"
	if c.RequirementType == "OE" {
		rt = "operating effectiveness"
	}
	ct := "non-configuration"
	if c.ControlType == "CONFIG" {
		ct = "configuration"
	}
	return rt + ", " + ct
}

// dueText is the due date, with how far off or overdue it is.
func dueText(c *model.AuditControl, now time.Time) string {
	due, ok := dateString(c.DueDate)
	if !ok {
		return "not set"
	}
	text := day(due)
	if c.Status == "COMPLETE" {
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

// describeControl is the plain-language account of where a control stands. The
// history is separate (see buildControlTimeline).
func describeControl(c *model.AuditControl, audit string, now time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Control %s in %s\n\nWhere it stands\n", c.ControlNumber, audit)
	fmt.Fprintf(&b, "• Status: %s.\n", capitalise(words(c.Status)))
	fmt.Fprintf(&b, "• Type: %s.\n", capitalise(requirementWords(c)))
	fmt.Fprintf(&b, "• Due: %s.\n", dueText(c, now))
	b.WriteString("\nPeople\n")
	fmt.Fprintf(&b, "• Control owner: %s.\n", nameOr(c.OwnerName, "not assigned"))
	fmt.Fprintf(&b, "• Auditor: %s.\n", nameOr(c.AuditorName, "not assigned"))
	fmt.Fprintf(&b, "• Team: %s.\n", nameOr(c.TeamName, "none"))
	b.WriteString("\nDates\n")
	fmt.Fprintf(&b, "• Added on %s; last updated on %s.\n", day(c.CreatedAt), day(c.UpdatedAt))
	if d := strings.TrimSpace(c.Description); d != "" {
		fmt.Fprintf(&b, "\nWhat it asks for\n%s\n", shorten(d, 240))
	}
	return strings.TrimRight(b.String(), "\n")
}

func capitalise(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = []rune(strings.ToUpper(string(r[0])))[0]
	return string(r)
}

// describeControlEvent says what one history entry was, in a sentence. It names
// the actor and what happened, and never repeats a comment or a file name.
func describeControlEvent(e *model.AuditTrailEntry) string {
	who := strings.TrimSpace(e.CreatedByName)
	if who == "" || who == e.CreatedBy {
		who = "Someone"
	}
	if e.CreatedBy == "system" {
		who = "The system"
	}
	var d struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if len(e.Details) > 0 {
		_ = json.Unmarshal(e.Details, &d)
	}
	move := ""
	if d.From != "" && d.To != "" {
		move = fmt.Sprintf(" It moved from %s to %s.", words(d.From), words(d.To))
	}
	switch e.Action {
	case "CREATED":
		return who + " created the control."
	case "UPLOADED":
		return who + " submitted evidence."
	case "RESUBMITTED":
		return who + " resubmitted evidence."
	case "APPROVED":
		return who + " approved it." + move
	case "REJECTED":
		return who + " sent it back." + move
	case "COMMENTED":
		return who + " added a comment."
	case "AI_VALIDATED":
		return "The AI reviewer checked the evidence."
	case "OVERRIDDEN":
		return who + " changed its status by hand." + move
	case "UPDATED":
		return who + " changed a detail."
	}
	return who + " made a change (" + words(e.Action) + ")."
}

// buildControlTimeline turns a control's history into dated lines, oldest first,
// keeping the latest maxControlTimelineEvents when there are more.
func buildControlTimeline(entries []*model.AuditTrailEntry) *auditservice.Timeline {
	events := append([]*model.AuditTrailEntry{}, entries...)
	sort.SliceStable(events, func(i, j int) bool { return events[i].CreatedAt.Before(events[j].CreatedAt) })
	t := &auditservice.Timeline{Events: []auditservice.TimelineEvent{}}
	if len(events) == 0 {
		t.Note = "Nothing has been recorded for this control yet."
		return t
	}
	if len(events) > maxControlTimelineEvents {
		t.Note = fmt.Sprintf("Showing the latest %d of %d events.", maxControlTimelineEvents, len(events))
		events = events[len(events)-maxControlTimelineEvents:]
	}
	for _, e := range events {
		t.Events = append(t.Events, auditservice.TimelineEvent{Date: day(e.CreatedAt), Text: describeControlEvent(e)})
	}
	return t
}

func controlHistorySummary(entries []*model.AuditTrailEntry) string {
	if len(entries) == 0 {
		return "Nothing recorded"
	}
	events := append([]*model.AuditTrailEntry{}, entries...)
	sort.SliceStable(events, func(i, j int) bool { return events[i].CreatedAt.Before(events[j].CreatedAt) })
	last := events[len(events)-1]
	return fmt.Sprintf("%d events, latest on %s: %s", len(events), day(last.CreatedAt), describeControlEvent(last))
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

// buildControlComparison writes one row per thing worth comparing, marking the
// rows where the two controls differ.
func buildControlComparison(a, b *model.AuditControl, ha, hb *controlHit, historyAllowed bool, last [2]string, now time.Time) *auditservice.CompareTable {
	t := &auditservice.CompareTable{Risks: []auditservice.CompareHead{
		{ID: ha.controlID, Code: a.ControlNumber, AuditID: ha.auditID},
		{ID: hb.controlID, Code: b.ControlNumber, AuditID: hb.auditID},
	}}
	add := func(label, va, vb string) {
		va, vb = orDash(va), orDash(vb)
		t.Rows = append(t.Rows, auditservice.CompareRow{
			Label: label, Values: []string{va, vb},
			Differs: !strings.EqualFold(strings.TrimSpace(va), strings.TrimSpace(vb)),
		})
	}
	add("Audit", ha.audit, hb.audit)
	add("Status", capitalise(words(a.Status)), capitalise(words(b.Status)))
	add("Type", capitalise(requirementWords(a)), capitalise(requirementWords(b)))
	add("Scope", capitalise(words(a.Scope)), capitalise(words(b.Scope)))
	add("Team", nameOr(a.TeamName, ""), nameOr(b.TeamName, ""))
	add("Control owner", nameOr(a.OwnerName, "Not assigned"), nameOr(b.OwnerName, "Not assigned"))
	add("Auditor", nameOr(a.AuditorName, "Not assigned"), nameOr(b.AuditorName, "Not assigned"))
	add("Due", dueText(a, now), dueText(b, now))
	add("Added on", day(a.CreatedAt), day(b.CreatedAt))
	add("Last updated", day(a.UpdatedAt), day(b.UpdatedAt))
	if historyAllowed {
		add("History", last[0], last[1])
	}
	return t
}

func controlCompareSummary(t *auditservice.CompareTable) string {
	var differs []string
	for _, r := range t.Rows {
		if r.Differs && r.Label != "History" && r.Label != "Added on" && r.Label != "Last updated" {
			differs = append(differs, strings.ToLower(r.Label))
		}
	}
	head := fmt.Sprintf("Comparing %s and %s.", t.Risks[0].Code, t.Risks[1].Code)
	if len(differs) == 0 {
		return head + " They match on everything shown below."
	}
	return head + " They differ in: " + strings.Join(differs, ", ") + "."
}
