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

package model

// ReminderItem is one control or population line queued into a recipient's
// daily due-date reminder digest — both what the email should display and
// what the send-log needs to record it (for de-dup on future runs). Lives in
// this package (not internal/audit/job or internal/audit/handler) so both
// can share it without either importing the other: internal/audit/job builds
// these while sweeping controls, and internal/audit/handler.Deps.
// SendReminderDigestSync (the function job.ReminderJob is wired to at
// startup, exactly as internal/risk/job.EscalationJob is wired to
// NotifyEscalationSync) turns them into one combined email plus one
// audit_notification row per item.
type ReminderItem struct {
	AuditID      int
	ControlID    *int
	PopulationID *int
	// Type is the audit_notification.type value for this item's tier:
	// REMINDER_DUE_10 | REMINDER_DUE_5 | REMINDER_DUE_TODAY | REMINDER_OVERDUE.
	Type string
	// ControlNumber/Description/DueDate/Tier/RequirementType mirror
	// emailer.AuditEventItem's display fields.
	ControlNumber   string
	Description     string
	DueDate         string
	Tier            string
	RequirementType string // "Evidence Requirement" | "Population Requirement"
	// AuditName is the item's audit: the "Audit: ..." line on the overdue
	// admin alert, and each row's audit on a digest that spans audits. Filled by the reminder job from the audit list
	// it already fetches to decide which audits are in scope, so naming the
	// audit on every alert costs no extra lookups.
	AuditName string
	// LinkControlID is the owning control's id, used only to deep-link the
	// admin alert at that control. Distinct from ControlID, which is the
	// audit_notification.control_id value and must stay nil for a population
	// item (the table treats control_id/population_id as mutually exclusive) —
	// this is always set, for both item kinds.
	LinkControlID int
	// WaitingOnUserID is the platform user id of the person this item is
	// waiting on — an owner or the auditor. Zero when it is waiting on the
	// admins as a group, or nobody is assigned.
	WaitingOnUserID int
	// WaitingOn names who the item is waiting on, for the admin escalation's
	// per-row column: the person's "Display Name (email)" (filled in once per
	// sweep, see ReminderJob.resolveUserNames), "Compliance Admins" or "Unassigned".
	WaitingOn string
	// Status is the control's status label, shown under WaitingOn.
	Status string
	// UnassignedNote flags a row an admin receives in place of a missing owner/auditor.
	UnassignedNote string
	// EscalatesToLead is true while the item is waiting on an owner's
	// submission — the only items an owner's lead is told about.
	EscalatesToLead bool
	// DedupSnapshot is the date written to audit_notification.due_date_snapshot
	// for this item's log row — distinct from DueDate (which is always the
	// item's real due date, for display). For the DUE_10/DUE_5/DUE_TODAY tiers
	// it's the same value as DueDate, so each tier logs (and therefore fires)
	// exactly once per due date. For OVERDUE it is instead the date the
	// reminder was sent, so — unlike the other three tiers — an overdue item
	// logs a new row, and therefore re-sends, every day it stays overdue.
	DedupSnapshot string
	// NotificationID is the audit_notification row ID this item's claim
	// reserved — set by internal/audit/job.ReminderJob's queue closure once
	// job.claimer.Claim succeeds, and used only by that same job to release
	// the claim (delete the row) if the owner's digest send then fails, so
	// the item is retried on a future run instead of staying claimed forever
	// with nothing sent.
	NotificationID int64
	// Daily marks an item reminded only because its status reminds every day,
	// not because a 10/5/0/overdue mark fell today. Type still names an
	// existing audit_notification type for the send log, so the digest reads
	// this flag, not Type, to choose its wording.
	Daily bool
}
