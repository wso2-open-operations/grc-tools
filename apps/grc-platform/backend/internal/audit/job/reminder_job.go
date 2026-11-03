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

// Package job runs the audit module's daily due-date reminder digest.
package job

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
)

// auditLister and controlLister are narrow local interfaces, not the full
// repository types — importing those would cycle back through
// internal/audit/handler. Same reasoning as internal/risk/job.
type auditLister interface {
	List(ctx context.Context) ([]*model.Audit, error)
}

type controlLister interface {
	ListAllForReminders(ctx context.Context) ([]*model.AuditControl, error)
}

// claimer is the reminder job's de-dup gate, structurally satisfied by
// NotificationService.Claim/ReleaseClaim without importing that package.
// Claim is the atomic de-dup decision; ReleaseClaim undoes it on a failed send.
type claimer interface {
	Claim(ctx context.Context, recipientID, auditID int, notifType string, controlID, populationID *int, dueDateSnapshot *string) (claimed bool, notificationID int64, err error)
	ReleaseClaim(ctx context.Context, notificationID int64) error
}

const (
	// runTimeout bounds a single sweep.
	runTimeout = 30 * time.Minute
	// releaseTimeout bounds a claim release, on its own short deadline instead
	// of the sweep's ctx — a release is often needed because runTimeout just
	// expired, and reusing that same expired ctx would fail immediately.
	releaseTimeout = 10 * time.Second
)

// ReminderJob sweeps every control daily and emails whoever each one is
// waiting on — owner, admins or auditor, by status — one combined digest of
// everything due in 10 days, due in 5 days, due today or overdue.
type ReminderJob struct {
	audits   auditLister
	controls controlLister
	claim    claimer
	// notify delivers one recipient's full daily digest synchronously — a plain
	// function, not a handler dependency, to avoid an import cycle. Wired to
	// handler.Deps.SendReminderDigestSync; doesn't log on success since Claim already did.
	notify func(ctx context.Context, recipientUserID int, items []model.ReminderItem) error
	// admins resolves the admin recipient set once per sweep; notifyAdmin
	// sends one admin's digest of overdue items in one audit. Both nil unless
	// WithAdminAlerts wired them — wired to ReminderAdminIDs / SendOverdueAdminDigestSync.
	admins      func(ctx context.Context) ([]int, error)
	notifyAdmin func(ctx context.Context, adminUserID int, items []model.ReminderItem) error
	// resolveUserNames looks up a batch of user ids in one call, so the
	// escalation resolves each person an item is waiting on once per sweep instead of once per
	// (admin, item) email — wired to handler.Deps.ResolveUserNames.
	resolveUserNames func(ctx context.Context, userIDs []int) map[int]string
	// leads resolves each overdue owner's line-manager email once per sweep;
	// notifyLead sends that owner's overdue items to them. Both nil unless
	// WithLeadAlerts wired them.
	leads      func(ctx context.Context, ownerIDs []int) map[int]string
	notifyLead func(ctx context.Context, ownerUserID int, leadEmail string, items []model.ReminderItem) error
	// ccEmails turns the admin ids resolved for the sweep into the addresses
	// copied on every email except the admins' own. notifyCC and notifyLeadCC
	// are notify and notifyLead with that CC added. All nil unless WithAdminCC
	// wired them, in which case nothing is ever copied.
	ccEmails     func(ctx context.Context, adminIDs []int) []string
	notifyCC     func(ctx context.Context, recipientUserID int, items []model.ReminderItem, cc []string) (delivered bool, err error)
	notifyLeadCC func(ctx context.Context, ownerUserID int, leadEmail string, items []model.ReminderItem, cc []string) error
	// daily turns on the every-day reminder for dailyStatuses and the
	// dailyRouteOverrides. Off, the sweep only reminds on the 10/5/0/overdue marks.
	daily bool
	// running serializes runOnce against itself: Start's daily ticker and the
	// manual-trigger endpoint (handler.reminderJobHandler.run) both end up
	// calling runOnce on this same instance. This guards a same-process
	// overlap cheaply; claimer's DB-level unique constraint is what makes the
	// de-dup correct across processes/replicas too.
	running atomic.Bool
}

// NewReminderJob constructs a ReminderJob.
func NewReminderJob(
	audits auditLister,
	controls controlLister,
	claim claimer,
	notify func(ctx context.Context, recipientUserID int, items []model.ReminderItem) error,
) *ReminderJob {
	return &ReminderJob{audits: audits, controls: controls, claim: claim, notify: notify}
}

// WithAdminAlerts wires the admin recipient set: every admin `admins` returns
// gets one escalation digest per audit with overdue items, and the reminders
// for items waiting on the admins. Without it those items reach nobody.
func (j *ReminderJob) WithAdminAlerts(
	admins func(ctx context.Context) ([]int, error),
	notifyAdmin func(ctx context.Context, adminUserID int, items []model.ReminderItem) error,
	resolveUserNames func(ctx context.Context, userIDs []int) map[int]string,
) *ReminderJob {
	j.admins = admins
	j.notifyAdmin = notifyAdmin
	j.resolveUserNames = resolveUserNames
	return j
}

// WithLeadAlerts turns on the overdue lead escalation: each owner's line
// manager gets a digest of that owner's overdue items. Opt-in setter (nil
// functions skip it) so a disabled deployment resolves no leads at all.
func (j *ReminderJob) WithLeadAlerts(
	leads func(ctx context.Context, ownerIDs []int) map[int]string,
	notifyLead func(ctx context.Context, ownerUserID int, leadEmail string, items []model.ReminderItem) error,
) *ReminderJob {
	j.leads = leads
	j.notifyLead = notifyLead
	return j
}

// WithAdminCC copies the compliance admins on the emails to an item's
// assignee and to that assignee's lead. Opt-in: unless all three functions are
// given it stays off, so a half-wired job never copies some emails and not
// others, and a disabled deployment resolves no CC list at all. The admins are
// the ones WithAdminAlerts resolves, so it must be wired as well.
//
// notifyCC reports whether the email went out, because it can be skipped
// without an error (an inactive recipient, or one with no address). The CC then
// reached nobody, so the overdue admin digest covers those items after all. While it is on and resolves
// at least one address, the CC stands in for the overdue admin digest on items
// that have an assignee; items nobody holds keep the digest.
func (j *ReminderJob) WithAdminCC(
	ccEmails func(ctx context.Context, adminIDs []int) []string,
	notifyCC func(ctx context.Context, recipientUserID int, items []model.ReminderItem, cc []string) (delivered bool, err error),
	notifyLeadCC func(ctx context.Context, ownerUserID int, leadEmail string, items []model.ReminderItem, cc []string) error,
) *ReminderJob {
	if ccEmails == nil || notifyCC == nil || notifyLeadCC == nil {
		return j
	}
	j.ccEmails = ccEmails
	j.notifyCC = notifyCC
	j.notifyLeadCC = notifyLeadCC
	return j
}

// WithDailyReminders makes the dailyStatuses remind every day instead of only
// on the 10/5/0/overdue marks, and sends a submitted sample to the admins.
func (j *ReminderJob) WithDailyReminders() *ReminderJob {
	j.daily = true
	return j
}

// overdueOnly returns the entries of a digest that escalate to a lead: overdue
// and still waiting on an owner's submission.
func overdueOnly(items []model.ReminderItem) []model.ReminderItem {
	out := make([]model.ReminderItem, 0, len(items))
	for _, it := range items {
		if it.Type == "REMINDER_OVERDUE" && it.EscalatesToLead {
			out = append(out, it)
		}
	}
	return out
}

// ccFor returns the admin CC for one recipient's digest: none when every item
// in it is waiting on the admins themselves, since they are then the To and
// copying the rest of the team on it adds nothing.
func ccFor(cc []string, items []model.ReminderItem) []string {
	for _, it := range items {
		if it.WaitingOnUserID > 0 {
			return cc
		}
	}
	return nil
}

// hasOverdue answers overdueOnly's question without building the slice, for
// the pre-pass that decides which owners are worth an HR lookup.
func hasOverdue(items []model.ReminderItem) bool {
	for _, it := range items {
		if it.Type == "REMINDER_OVERDUE" && it.EscalatesToLead {
			return true
		}
	}
	return false
}

type actorKind int

const (
	actorOwner actorKind = iota
	actorAdmins
	actorAuditor
)

const (
	waitingOnAdmins     = "Compliance Admins"
	waitingOnUnassigned = "Unassigned"
	// waitingOnUnresolved stands in when an assigned person's name can't be looked up.
	waitingOnUnresolved = "Name unavailable"
)

var unassignedNotes = map[actorKind]string{
	actorOwner:   "No owner assigned",
	actorAuditor: "No auditor assigned",
}

// statusRoute is everything the sweep reads off a control's status: who it is
// waiting on, which due date it is measured against, and its label. One table,
// so the recipient and the date can never describe different phases.
type statusRoute struct {
	waitsOn actorKind
	// population: measured against the population's due date and owner
	// rather than the control's own.
	population bool
	label      string
}

// statusRoutes mirrors the dashboard's action queue split. A status absent
// here (COMPLETE) reminds nobody.
var statusRoutes = map[string]statusRoute{
	"POPULATION_PENDING":            {actorOwner, true, "Population Pending"},
	"POPULATION_NEED_CLARIFICATION": {actorOwner, true, "Population Need Clarification"},
	"POPULATION_INTERNAL_REVIEW":    {actorAdmins, true, "Population Internal Review"},
	"POPULATION_UNDER_VALIDATION":   {actorAuditor, true, "Population Under Validation"},
	"POPULATION_COMPLETE":           {actorAuditor, false, "Population Complete"},
	"AWAITING_SAMPLE":               {actorAuditor, false, "Awaiting Sample"},
	"SUBMITTED_SAMPLE":              {actorOwner, false, "Submitted Sample"},
	"EVIDENCE_PENDING":              {actorOwner, false, "Evidence Pending"},
	"EVIDENCE_NEED_CLARIFICATION":   {actorOwner, false, "Evidence Need Clarification"},
	"EVIDENCE_INTERNAL_REVIEW":      {actorAdmins, false, "Evidence Internal Review"},
	"EVIDENCE_UNDER_VALIDATION":     {actorAuditor, false, "Evidence Under Validation"},
}

// dailyStatuses remind every day, not only on the due-date marks — the
// statuses waiting on the compliance team or the auditor.
var dailyStatuses = map[string]bool{
	"POPULATION_INTERNAL_REVIEW":  true,
	"EVIDENCE_INTERNAL_REVIEW":    true,
	"SUBMITTED_SAMPLE":            true,
	"POPULATION_UNDER_VALIDATION": true,
	"EVIDENCE_UNDER_VALIDATION":   true,
	"POPULATION_COMPLETE":         true,
	"AWAITING_SAMPLE":             true,
}

// dailyRouteOverrides changes who a status waits on once the daily reminders
// are on: a submitted sample is the compliance team's to review, not the owner's.
var dailyRouteOverrides = map[string]actorKind{
	"SUBMITTED_SAMPLE": actorAdmins,
}

// dailyTierLabel heads the Status column of an item reminded only because its
// status reminds every day.
const dailyTierLabel = "Awaiting action"

// assignee is the person the control is waiting on, nil when that role is the
// admins as a group or nobody holds it.
func (r statusRoute) assignee(c *model.AuditControl) *int {
	switch r.waitsOn {
	case actorOwner:
		if r.population {
			return c.PopulationOwnerID
		}
		return c.OwnerID
	case actorAuditor:
		return c.AuditorID
	default:
		return nil
	}
}

// adminAuditKey groups escalated items by (admin, audit) — one digest email
// per key, since the digest's subject names a single audit.
type adminAuditKey struct {
	adminID int
	auditID int
}

// RunOnce runs the sweep synchronously and returns its error, if any. Two
// callers share it: the scheduler's daily tick (internal/scheduler) and the
// manual-trigger endpoint (POST /api/v1/audits/reminders/run), which lets
// QA/ops fire a sweep without waiting for the fixed daily time.
func (j *ReminderJob) RunOnce(ctx context.Context) error {
	return j.runOnce(ctx)
}

// reminderTier returns "REMINDER_DUE_10", "REMINDER_DUE_5",
// "REMINDER_DUE_TODAY", "REMINDER_OVERDUE", or "" (no tier applies today)
// for dueDate ("YYYY-MM-DD") relative to today (UTC, date-only comparison).
func reminderTier(dueDate string, today time.Time) string {
	due, err := time.Parse("2006-01-02", dueDate)
	if err != nil {
		return ""
	}
	daysUntil := int(due.Sub(dateOnly(today)).Hours() / 24)
	switch {
	case daysUntil < 0:
		return "REMINDER_OVERDUE"
	case daysUntil == 0:
		return "REMINDER_DUE_TODAY"
	case daysUntil == 5:
		return "REMINDER_DUE_5"
	case daysUntil == 10:
		return "REMINDER_DUE_10"
	default:
		return ""
	}
}

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// releaseCtx detaches from ctx's deadline/cancellation (but keeps its values)
// and applies releaseTimeout instead, so a release triggered by the sweep's
// own ctx expiring isn't doomed to fail for the same reason.
func releaseCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), releaseTimeout)
}

// tierLabel renders a reminderTier value for the email body.
func tierLabel(tier string) string {
	switch tier {
	case "REMINDER_DUE_10":
		return "Due in 10 days"
	case "REMINDER_DUE_5":
		return "Due in 5 days"
	case "REMINDER_DUE_TODAY":
		return "Due today"
	case "REMINDER_OVERDUE":
		return "Overdue"
	default:
		return ""
	}
}

func (j *ReminderJob) runOnce(parent context.Context) (runErr error) {
	if !j.running.CompareAndSwap(false, true) {
		return errors.New("reminder job: a sweep is already running")
	}
	defer j.running.Store(false)
	defer func() {
		if r := recover(); r != nil {
			slog.Error("reminder job: recovered from panic", "panic", r, "stack", string(debug.Stack()))
			runErr = fmt.Errorf("reminder job panic: %v", r)
		}
	}()

	ctx, cancel := context.WithTimeout(parent, runTimeout)
	defer cancel()

	audits, err := j.audits.List(ctx)
	if err != nil {
		slog.Error("reminder job: list audits", "err", err)
		return fmt.Errorf("list audits: %w", err)
	}
	// ACTIVE and ARCHIVED audits both stay in scope; only COMPLETED/REMOVED
	// are excluded — an archived audit can still be ongoing.
	activeAuditIDs := make(map[int]bool, len(audits))
	// Names come from this same already-fetched list, so the overdue admin
	// alert can head each email with its audit without a lookup per email.
	auditNames := make(map[int]string, len(audits))
	for _, a := range audits {
		if a.IsOngoing() {
			activeAuditIDs[a.ID] = true
		}
		auditNames[a.ID] = a.EffectiveName()
	}

	// Resolved once per sweep, not per control. A failure here is logged and
	// treated as "no admins": the reminders to owners and auditors below must
	// not be lost to a failed grant lookup.
	var adminIDs []int
	if j.admins != nil && j.notifyAdmin != nil {
		adminIDs, err = j.admins(ctx)
		if err != nil {
			slog.Warn("reminder job: failed to resolve admin recipients, skipping escalations and admin reminders this run", "err", err)
			adminIDs = nil
		} else if len(adminIDs) == 0 {
			slog.Warn("reminder job: no admin recipients, escalations and admin reminders reach nobody this run")
		}
	}

	// The CC list is resolved once per sweep. Empty means no CC is sent and
	// the overdue admin digest keeps covering assigned items too.
	var cc []string
	if j.ccEmails != nil && len(adminIDs) > 0 {
		cc = j.ccEmails(ctx, adminIDs)
		if len(cc) == 0 {
			slog.Warn("reminder job: no admin CC addresses, emails go out without a CC and the overdue admin digest stays on this run")
		}
	}
	ccActive := len(cc) > 0

	controls, err := j.controls.ListAllForReminders(ctx)
	if err != nil {
		slog.Error("reminder job: list controls", "err", err)
		return fmt.Errorf("list controls: %w", err)
	}

	today := time.Now().UTC()
	todayStr := today.Format("2006-01-02")
	byRecipient := map[int][]model.ReminderItem{}
	byAdmin := map[adminAuditKey][]model.ReminderItem{}
	queued, skippedDup, skippedErr := 0, 0, 0

	// release gives one claim back so a later run retries the item. If the
	// release itself fails, the claim IS stuck: logged loud (Error, not Warn)
	// since that's the one failure mode this mechanism doesn't self-heal from.
	// why names the path that triggered it, for the log line.
	release := func(notificationID int64, recipientID int, notifType, why string) {
		rctx, cancel := releaseCtx(ctx)
		relErr := j.claim.ReleaseClaim(rctx, notificationID)
		cancel()
		if relErr != nil {
			slog.Error("reminder job: failed to release claim — item will NOT retry until this is fixed",
				"reason", why, "notificationId", notificationID, "recipientId", recipientID, "type", notifType, "err", relErr)
		}
	}
	// Releases every still-pending claim before a panic reaches the top-level
	// recover, so nothing stays claimed forever with nothing sent. Whatever
	// remains in byRecipient/byAdmin here is exactly the unresolved set.
	defer func() {
		if r := recover(); r != nil {
			for recipientID, items := range byRecipient {
				for _, it := range items {
					release(it.NotificationID, recipientID, it.Type, "panic")
				}
			}
			for key, items := range byAdmin {
				for _, it := range items {
					release(it.NotificationID, key.adminID, it.Type, "panic")
				}
			}
			panic(r)
		}
	}()

	queue := func(recipientID int, item model.ReminderItem, controlID, populationID *int) {
		claimed, notificationID, err := j.claim.Claim(ctx, recipientID, item.AuditID, item.Type, controlID, populationID, &item.DedupSnapshot)
		if err != nil {
			// Fail CLOSED: a claim error means we can't tell if we hold it, so
			// sending anyway risks a duplicate. Skipping costs one day's delay.
			slog.Warn("reminder job: claim failed, skipping this run (fail closed)", "recipientId", recipientID, "type", item.Type, "err", err)
			skippedErr++
			return
		}
		if !claimed {
			skippedDup++
			return
		}
		item.NotificationID = notificationID
		byRecipient[recipientID] = append(byRecipient[recipientID], item)
		queued++
	}

	// queueAdmins escalates one overdue item to every admin (grouped into
	// that admin's digest per audit), claiming per admin per item. An admin
	// the item is waiting on is skipped — they already hear about it in their own digest.
	queueAdmins := func(item model.ReminderItem, controlID, populationID *int) {
		for _, adminID := range adminIDs {
			if adminID == item.WaitingOnUserID {
				continue
			}
			claimed, notificationID, err := j.claim.Claim(ctx, adminID, item.AuditID, item.Type, controlID, populationID, &item.DedupSnapshot)
			if err != nil {
				// Fail closed, exactly as queue does.
				slog.Warn("reminder job: admin claim failed, skipping this run (fail closed)", "adminId", adminID, "type", item.Type, "err", err)
				skippedErr++
				continue
			}
			if !claimed {
				skippedDup++
				continue
			}
			item.NotificationID = notificationID
			key := adminAuditKey{adminID: adminID, auditID: item.AuditID}
			byAdmin[key] = append(byAdmin[key], item)
			queued++
		}
	}

	for _, c := range controls {
		if c == nil || !activeAuditIDs[c.AuditID] {
			continue
		}
		route, ok := statusRoutes[c.Status]
		if !ok {
			continue
		}
		if j.daily {
			if kind, ok := dailyRouteOverrides[c.Status]; ok {
				route.waitsOn = kind
			}
		}
		populationPhase := route.population && c.PopulationID != nil
		dueDate := c.DueDate
		if populationPhase && c.PopulationDueDate != nil {
			dueDate = c.PopulationDueDate
		}
		if dueDate == nil {
			continue
		}
		due := *dueDate
		tier := reminderTier(due, today)
		daily := false
		if tier == "" {
			if !j.daily || !dailyStatuses[c.Status] {
				continue
			}
			// No mark falls today, but this status reminds every day. It logs
			// under an existing type so no new audit_notification type is
			// needed; the digest reads Daily for its wording. A due-today item
			// uses the same type and today's date either way, so it cannot send twice.
			tier, daily = "REMINDER_DUE_TODAY", true
		}
		actorID := route.assignee(c)
		overdue := tier == "REMINDER_OVERDUE"
		dedupSnapshot := due
		if overdue || daily {
			dedupSnapshot = todayStr // re-fires daily — see model.ReminderItem.DedupSnapshot
		}
		tierText := tierLabel(tier)
		if daily {
			tierText = dailyTierLabel
		}
		// audit_notification treats control_id/population_id as mutually exclusive.
		controlID, populationID := &c.ID, (*int)(nil)
		requirementType := "Evidence Requirement"
		if populationPhase {
			controlID, populationID = nil, c.PopulationID
			requirementType = "Population Requirement"
		}
		item := model.ReminderItem{
			AuditID:         c.AuditID,
			ControlID:       controlID,
			PopulationID:    populationID,
			Type:            tier,
			ControlNumber:   c.ControlNumber,
			Description:     c.Description,
			DueDate:         due,
			Tier:            tierText,
			RequirementType: requirementType,
			DedupSnapshot:   dedupSnapshot,
			Daily:           daily,
			AuditName:       auditNames[c.AuditID],
			LinkControlID:   c.ID,
			Status:          route.label,
		}

		if actorID != nil {
			item.WaitingOnUserID = *actorID
			item.EscalatesToLead = route.waitsOn == actorOwner
			queue(*actorID, item, controlID, populationID)
			// With a CC on the assignee's email the admins already hear about
			// this item, so the separate digest would only repeat it.
			if overdue && !ccActive {
				queueAdmins(item, controlID, populationID)
			}
			continue
		}

		// Nobody to remind: the admins are the reviewers, or stand in for a
		// missing owner/auditor since only they can assign one.
		if route.waitsOn == actorAdmins {
			item.WaitingOn = waitingOnAdmins
		} else {
			item.WaitingOn, item.UnassignedNote = waitingOnUnassigned, unassignedNotes[route.waitsOn]
		}
		if overdue {
			// Only the escalation digest, never both emails for one item.
			queueAdmins(item, controlID, populationID)
			continue
		}
		for _, adminID := range adminIDs {
			queue(adminID, item, controlID, populationID)
		}
	}

	// Lead recipients, resolved once per sweep and only for owners who
	// actually have overdue items. Stays nil when the escalation is disabled,
	// and a nil map reads as "no lead" for every owner below.
	var ownerLeads map[int]string
	if j.leads != nil && j.notifyLead != nil {
		overdueOwners := make([]int, 0, len(byRecipient))
		for recipientID, items := range byRecipient {
			if hasOverdue(items) {
				overdueOwners = append(overdueOwners, recipientID)
			}
		}
		if len(overdueOwners) > 0 {
			ownerLeads = j.leads(ctx, overdueOwners)
		}
	}

	sent, notifyFailed := 0, 0
	leadSent, leadFailed := 0, 0
	for recipientID, items := range byRecipient {
		var err error
		if rcc := ccFor(cc, items); len(rcc) > 0 {
			var delivered bool
			delivered, err = j.notifyCC(ctx, recipientID, items, rcc)
			if !delivered || err != nil {
				// The admins were counting on this email's CC for their overdue
				// items. It was skipped (inactive, no address) or failed, so
				// they get the digest after all.
				for _, it := range items {
					if it.Type == "REMINDER_OVERDUE" && it.WaitingOnUserID > 0 {
						queueAdmins(it, it.ControlID, it.PopulationID)
					}
				}
			}
		} else {
			err = j.notify(ctx, recipientID, items)
		}
		if err != nil {
			slog.Warn("reminder job: notification failed", "recipientId", recipientID, "items", len(items), "err", err)
			// The digest failed, so every item it covered must give up its
			// claim — otherwise it stays claimed forever with nothing sent.
			for _, it := range items {
				release(it.NotificationID, recipientID, it.Type, "failed send")
			}
			notifyFailed++
			delete(byRecipient, recipientID) // resolved (failed+released) — the panic-recovery defer above must not also release it
			continue
		}
		sent++
		delete(byRecipient, recipientID) // resolved (sent) — must never be released, even if a later owner's notify panics
		// Strictly after the delete above: the digest is out and its claims are
		// spent, so neither a failure nor a panic here may release them —
		// doing so would re-send the owner's own digest tomorrow for an item
		// they were already told about.
		if leadEmail := ownerLeads[recipientID]; leadEmail != "" {
			if overdue := overdueOnly(items); len(overdue) > 0 {
				var err error
				if ccActive {
					err = j.notifyLeadCC(ctx, recipientID, leadEmail, overdue, cc)
				} else {
					err = j.notifyLead(ctx, recipientID, leadEmail, overdue)
				}
				if err != nil {
					slog.Warn("reminder job: lead escalation failed", "recipientId", recipientID, "items", len(overdue), "err", err)
					leadFailed++
				} else {
					leadSent++
				}
			}
		}
	}

	// Resolved once per sweep, deduped across every escalated item, so an
	// owner with several overdue items (or several admins) is looked up once
	// instead of once per email.
	if len(byAdmin) > 0 && j.resolveUserNames != nil {
		userIDSet := map[int]bool{}
		for _, items := range byAdmin {
			for _, it := range items {
				if it.WaitingOnUserID > 0 {
					userIDSet[it.WaitingOnUserID] = true
				}
			}
		}
		userIDs := make([]int, 0, len(userIDSet))
		for id := range userIDSet {
			userIDs = append(userIDs, id)
		}
		userNames := j.resolveUserNames(ctx, userIDs)
		for _, items := range byAdmin {
			for i := range items {
				if id := items[i].WaitingOnUserID; id > 0 {
					items[i].WaitingOn = userNames[id]
					if items[i].WaitingOn == "" {
						items[i].WaitingOn = waitingOnUnresolved
					}
				}
			}
		}
	}

	// Overdue escalations: one digest email per (admin, audit). Each stands
	// alone, so unlike the owner loop above a failure releases only its own
	// group's claims and every other digest still goes out.
	adminSent, adminFailed := 0, 0
	for key, items := range byAdmin {
		if err := j.notifyAdmin(ctx, key.adminID, items); err != nil {
			slog.Warn("reminder job: overdue admin digest failed",
				"adminId", key.adminID, "auditId", key.auditID, "items", len(items), "err", err)
			for _, it := range items {
				release(it.NotificationID, key.adminID, it.Type, "failed admin send")
			}
			adminFailed++
			delete(byAdmin, key) // resolved (failed+released) — the panic-recovery defer above must not also release it
			continue
		}
		adminSent++
		delete(byAdmin, key) // resolved (sent) — must never be released, even if a later digest's send panics
	}

	slog.Info("reminder job: run complete",
		"owners", sent, "notifyFailed", notifyFailed,
		"adminDigestsSent", adminSent, "adminDigestsFailed", adminFailed,
		"leadDigestsSent", leadSent, "leadDigestsFailed", leadFailed,
		"itemsQueued", queued, "itemsSkippedDup", skippedDup, "itemsSkippedErr", skippedErr)
	return nil
}
