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

package job

import (
	"fmt"
	"strings"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
)

// Workflow-aware reminders for Design controls. A control has two deadlines:
//
//   - the ultimate deadline (its due date): 10 days before it reminds the owner;
//     5 days before, 1 day before, on the day and every day after it until the
//     control is Complete, it reminds the owner and the owner's lead.
//   - the individual deadline (the current holder's own deadline): 1 day before,
//     on the day and every day after it until the control moves to the next
//     stage, it reminds whoever holds the control.
//
// Every day is counted in Asia/Colombo, like the deadlines themselves.

const (
	kindUltimate   = "ULTIMATE"
	kindIndividual = "INDIVIDUAL"
)

// colombo is the one timezone deadline days are counted in.
var colombo = loadColombo()

func loadColombo() *time.Location {
	if loc, err := time.LoadLocation("Asia/Colombo"); err == nil {
		return loc
	}
	// tzdata may be missing in a slim image; Sri Lanka is a fixed UTC+5:30.
	return time.FixedZone("Asia/Colombo", 5*3600+30*60)
}

// colomboDay returns the Colombo calendar day of t, as midnight UTC so two days
// subtract into a whole number of 24-hour spans.
func colomboDay(t time.Time) time.Time {
	c := t.In(colombo)
	return time.Date(c.Year(), c.Month(), c.Day(), 0, 0, 0, 0, time.UTC)
}

// daysUntil is how many days away due ("YYYY-MM-DD") is from today: negative
// once it has passed. ok is false for a date that cannot be read.
// dayPrefix trims a date or datetime string to YYYY-MM-DD.
func dayPrefix(s string) string {
	if len(s) > 10 {
		return s[:10]
	}
	return s
}

func daysUntil(due string, today time.Time) (int, bool) {
	d, err := time.Parse("2006-01-02", dayPrefix(due))
	if err != nil {
		return 0, false
	}
	return int(d.Sub(today).Hours() / 24), true
}

// workflowMark is the reminder a deadline calls for today.
type workflowMark struct {
	// Type is the audit_notification type the send is logged under. There is
	// no "1 day before" type, so that mark logs as DUE_TODAY with today's date,
	// the same device the daily reminder uses.
	Type string
	// Dedup is the date logged with it: the deadline for a mark that fires once,
	// today for one that fires again each day.
	Dedup   string
	Text    string // "Due in 5 days", "Overdue by 3 days", ...
	OneDay  bool
	Overdue bool
}

// markFor returns the mark a deadline calls for today, or ok=false when none
// does. withTenDay is true for the ultimate deadline, the only one with a
// 10-day and a 5-day reminder.
func markFor(days int, due, todayStr string, withTenDay bool) (workflowMark, bool) {
	switch {
	case days == 10 && withTenDay:
		return workflowMark{Type: "REMINDER_DUE_10", Dedup: due, Text: "Due in 10 days"}, true
	case days == 5 && withTenDay:
		return workflowMark{Type: "REMINDER_DUE_5", Dedup: due, Text: "Due in 5 days"}, true
	case days == 1:
		return workflowMark{Type: "REMINDER_DUE_TODAY", Dedup: todayStr, Text: "Due tomorrow", OneDay: true}, true
	case days == 0:
		return workflowMark{Type: "REMINDER_DUE_TODAY", Dedup: due, Text: "Due today"}, true
	case days < 0:
		n := -days
		unit := "days"
		if n == 1 {
			unit = "day"
		}
		return workflowMark{Type: "REMINDER_OVERDUE", Dedup: todayStr, Text: fmt.Sprintf("Overdue by %d %s", n, unit), Overdue: true}, true
	}
	return workflowMark{}, false
}

// isWorkflowControl is whether a control gets the workflow-aware reminders.
// Release 1 covers Design controls; OE controls keep the existing reminders
// until their timeline is agreed.
func isWorkflowControl(c *model.AuditControl) bool {
	return c != nil && strings.EqualFold(c.RequirementType, "DESIGN") && c.Status != "COMPLETE"
}

// workflowReminder is one reminder for one control, ready for the job to route.
type workflowReminder struct {
	item    model.ReminderItem
	waitsOn actorKind
	actorID *int
	overdue bool
}

// currentlyWith names who a status waits on, for the ultimate rows.
func currentlyWith(kind actorKind) string {
	switch kind {
	case actorAdmins:
		return "Compliance"
	case actorAuditor:
		return "Auditor"
	default:
		return "Owner"
	}
}

func handlerKind(handler string) (actorKind, bool) {
	switch handler {
	case "OWNER":
		return actorOwner, true
	case "COMPLIANCE":
		return actorAdmins, true
	case "AUDITOR":
		return actorAuditor, true
	}
	return 0, false
}

func holderID(c *model.AuditControl, kind actorKind) *int {
	switch kind {
	case actorOwner:
		return c.OwnerID
	case actorAuditor:
		return c.AuditorID
	}
	return nil
}

// workflowReminders works out today's reminders for one Design control: at most
// one for its ultimate deadline and one for the holder's own deadline. When both
// land on the same person with the same type and date they are folded into one,
// so that person gets a single row set and a single claim.
func workflowReminders(c *model.AuditControl, today time.Time, auditName string) []workflowReminder {
	if !isWorkflowControl(c) {
		return nil
	}
	route, ok := statusRoutes[c.Status]
	if !ok {
		return nil
	}
	todayStr := today.Format("2006-01-02")

	base := func(due string, m workflowMark, kind, label string) model.ReminderItem {
		return model.ReminderItem{
			AuditID:         c.AuditID,
			ControlID:       &c.ID,
			Type:            m.Type,
			ControlNumber:   c.ControlNumber,
			Description:     c.Description,
			DueDate:         due,
			Tier:            label + " - " + m.Text,
			RequirementType: "Evidence Requirement",
			DedupSnapshot:   m.Dedup,
			AuditName:       auditName,
			LinkControlID:   c.ID,
			Status:          route.label,
			Kind:            kind,
			OneDayBefore:    m.OneDay,
		}
	}

	var out []workflowReminder

	// Ultimate deadline: the control's due date, to its owner.
	if c.DueDate != nil {
		if days, ok := daysUntil(*c.DueDate, today); ok {
			due := dayPrefix(*c.DueDate)
			if m, ok := markFor(days, due, todayStr, true); ok {
				item := base(due, m, kindUltimate, "Ultimate deadline")
				item.CurrentlyWith = currentlyWith(route.waitsOn)
				item.ToLead = days <= 5
				id := c.OwnerID
				if id != nil {
					item.WaitingOnUserID = *id
				}
				out = append(out, workflowReminder{item: item, waitsOn: actorOwner, actorID: id, overdue: m.Overdue})
			}
		}
	}

	// Individual deadline: the current holder's own deadline, to the holder.
	if c.DeadlineTracked && c.CurrentHandler != nil && c.CurrentDueDate != nil {
		if kind, ok := handlerKind(*c.CurrentHandler); ok {
			due := dayPrefix(*c.CurrentDueDate)
			if days, ok := daysUntil(due, today); ok {
				if m, ok := markFor(days, due, todayStr, false); ok {
					item := base(due, m, kindIndividual, "Your deadline")
					id := holderID(c, kind)
					if id != nil {
						item.WaitingOnUserID = *id
					}
					out = append(out, workflowReminder{item: item, waitsOn: kind, actorID: id, overdue: m.Overdue})
				}
			}
		}
	}

	// Same person, same type and date: one claim, both rows.
	if len(out) == 2 {
		a, b := out[0], out[1]
		if a.actorID != nil && b.actorID != nil && *a.actorID == *b.actorID &&
			a.item.Type == b.item.Type && a.item.DedupSnapshot == b.item.DedupSnapshot {
			a.item.Extras = append(a.item.Extras, b.item)
			return []workflowReminder{a}
		}
	}
	return out
}

// expandItems returns items followed by the rows folded into them.
func expandItems(items []model.ReminderItem) []model.ReminderItem {
	out := make([]model.ReminderItem, 0, len(items))
	for _, it := range items {
		out = append(out, it)
		out = append(out, it.Extras...)
	}
	return out
}

// leadItems are the ultimate-deadline rows an owner's lead is told about.
func leadItems(items []model.ReminderItem) []model.ReminderItem {
	var out []model.ReminderItem
	for _, it := range expandItems(items) {
		if it.ToLead {
			out = append(out, it)
		}
	}
	return out
}

func hasLeadItems(items []model.ReminderItem) bool { return len(leadItems(items)) > 0 }
