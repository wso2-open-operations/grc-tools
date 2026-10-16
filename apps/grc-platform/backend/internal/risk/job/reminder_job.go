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
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
)

// reminderClaimer is the sweep's de-dup gate, structurally satisfied by
// repository.ReminderRepository without importing it. The claim is what stops
// the several backend replicas — each running this same sweep every morning —
// from all emailing the same risk.
type reminderClaimer interface {
	Claim(ctx context.Context, riskID int, reminderType, dueDateSnapshot string) (claimed bool, reminderID int64, err error)
	ReleaseClaim(ctx context.Context, reminderID int64) error
}

// releaseTimeout bounds a claim release, on its own short deadline instead of
// the sweep's ctx — a release is often needed because that ctx just expired,
// and reusing it would fail immediately. Same reasoning as the audit reminder
// job's releaseTimeout.
const releaseTimeout = 10 * time.Second

// ReminderJob emails a risk's Assigner, Action Owners and compliance
// oversight as its implementation date approaches: 15 days out, 5 days out and
// on the day itself, with how many of those a risk earns depending on its
// effective level (see model.ReminderTierFor).
//
// It stops at the due date. The day after, EscalationJob takes over: the risk
// is flipped to ESCALATED and a wider list is told it was missed.
type ReminderJob struct {
	risks riskLister
	claim reminderClaimer
	// notify sends one risk's reminder synchronously and reports whether it
	// actually went out — the claim must be released when it didn't, so a
	// fire-and-forget send would leave the reminder claimed with nobody told.
	// A plain function rather than a handler dependency, so this package never
	// imports internal/risk/handler (which imports services, and would cycle);
	// wired to handler.Deps.SendDueReminderSync at startup, exactly as
	// EscalationJob.notify is wired to NotifyEscalationSync.
	notify func(ctx context.Context, riskID int, tier, dueDate string) error
	// running serializes RunOnce against itself: the scheduler's daily tick
	// and the manual-trigger endpoint both call it on this same instance.
	// This guards a same-process overlap; the claim above is what makes the
	// de-dup correct across replicas too.
	running atomic.Bool
}

// NewReminderJob constructs a ReminderJob.
func NewReminderJob(
	risks riskLister,
	claim reminderClaimer,
	notify func(ctx context.Context, riskID int, tier, dueDate string) error,
) *ReminderJob {
	return &ReminderJob{risks: risks, claim: claim, notify: notify}
}

// RunOnce runs the sweep synchronously and returns its error, if any. Two
// callers share it: the scheduler's daily tick (internal/scheduler) and the
// manual-trigger endpoint (POST /api/v1/risks/reminders/run), which lets
// QA/ops fire a sweep without waiting for the fixed daily time.
func (j *ReminderJob) RunOnce(ctx context.Context) error {
	if !j.running.CompareAndSwap(false, true) {
		return errors.New("reminder job: a sweep is already running")
	}
	defer j.running.Store(false)
	return j.runOnce(ctx)
}

// runOnce is one sweep: page through APPROVED risks due within the
// reminder horizon, and for each one whose tier falls today, claim it, send
// it, and release the claim if the send failed. RunOnce owns the
// single-flight guard; this owns the work.
func (j *ReminderJob) runOnce(parent context.Context) (runErr error) {
	// This can execute in a bare goroutine (the scheduler runs each sweep in
	// its own), where an unrecovered panic would take the whole process down.
	defer func() {
		if r := recover(); r != nil {
			slog.Error("risk reminder job: recovered from panic", "panic", r, "stack", string(debug.Stack()))
			runErr = fmt.Errorf("risk reminder job panic: %v", r)
		}
	}()

	ctx, cancel := context.WithTimeout(parent, runTimeout)
	defer cancel()

	today := dateOnly(time.Now().UTC())
	// The query is narrowed to the window the tiers can possibly fall in —
	// today through the furthest lead time — so the sweep pages over the
	// handful of risks approaching their date rather than every risk in
	// remediation. Which of those days actually fires is then per-risk, from
	// its level.
	horizon := today.AddDate(0, 0, model.MaxReminderLeadDays)

	sent, notifyFailed := 0, 0
	skippedDup, skippedErr, skippedNoTier := 0, 0, 0
	offset := 0
	for {
		page, err := j.risks.List(ctx, model.ListRisksFilter{
			Statuses: []string{model.StatusApproved},
			DueFrom:  today.Format(dateLayout),
			DueTo:    horizon.Format(dateLayout),
			Limit:    pageLimit,
			// Unlike EscalationJob, this sweep mutates nothing, so rows never
			// drop out of the result set underneath it and paging forward is
			// safe (and necessary — re-querying from 0 would loop forever).
			Offset: offset,
		})
		if err != nil {
			slog.Error("risk reminder job: list risks approaching their due date", "err", err)
			return fmt.Errorf("list risks: %w", err)
		}
		if len(page.Items) == 0 {
			break
		}

		for _, r := range page.Items {
			if r == nil || r.ImplementationDate == nil {
				continue
			}
			due, err := parseDueDate(*r.ImplementationDate)
			if err != nil {
				slog.Warn("risk reminder job: unparseable implementation date, skipping",
					"riskId", r.ID, "implementationDate", *r.ImplementationDate, "err", err)
				skippedNoTier++
				continue
			}
			// RiskListItem.RiskLevel is the EFFECTIVE (residual) level — the
			// entity resolves it from the latest reassessment, falling back to
			// gross — which is the level this schedule is defined on.
			tier := model.ReminderTierFor(r.RiskLevel, daysBetween(today, due))
			if tier == "" {
				skippedNoTier++
				continue
			}
			dueDate := due.Format(dateLayout)

			claimed, reminderID, err := j.claim.Claim(ctx, r.ID, tier, dueDate)
			if err != nil {
				// Fail CLOSED: a claim error means we can't tell whether we
				// hold it, so sending anyway risks a duplicate. Skipping costs
				// this tier for this risk, since it is never sent late.
				slog.Warn("risk reminder job: claim failed, skipping this risk (fail closed)",
					"riskId", r.ID, "tier", tier, "err", err)
				skippedErr++
				continue
			}
			if !claimed {
				// Another replica's sweep already sent it. Expected, not a problem.
				skippedDup++
				continue
			}

			if err := j.notify(ctx, r.ID, tier, dueDate); err != nil {
				slog.Warn("risk reminder job: notification failed, releasing claim",
					"riskId", r.ID, "tier", tier, "err", err)
				j.release(ctx, reminderID, r.ID, tier)
				notifyFailed++
				continue
			}
			sent++
		}

		offset += len(page.Items)
		if offset >= page.Total {
			break
		}
	}

	slog.Info("risk reminder job: run complete",
		"sent", sent, "notifyFailed", notifyFailed,
		"skippedDup", skippedDup, "skippedErr", skippedErr, "skippedNoTier", skippedNoTier)
	return nil
}

// release gives one claim back so a later run the same day retries it. If the
// release itself fails the claim IS stuck, and the reminder is lost: logged
// loud (Error, not Warn) since that is the one failure this mechanism doesn't
// heal from on its own.
func (j *ReminderJob) release(ctx context.Context, reminderID int64, riskID int, tier string) {
	// Detached from ctx's deadline (values kept) on its own short timeout, so
	// a release triggered by the sweep's own ctx expiring isn't doomed for the
	// same reason.
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), releaseTimeout)
	defer cancel()
	if err := j.claim.ReleaseClaim(rctx, reminderID); err != nil {
		slog.Error("risk reminder job: failed to release claim — this reminder will NOT be retried",
			"reminderId", reminderID, "riskId", riskID, "tier", tier, "err", err)
	}
}

const dateLayout = "2006-01-02"

// dateOnly truncates t to midnight UTC, so the tier arithmetic works in
// whole days.
func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// daysBetween counts whole days from one date-only day to another, which is
// what the tiers are defined in: 0 is the due date itself.
func daysBetween(from, to time.Time) int {
	return int(to.Sub(from).Hours() / 24)
}

// parseDueDate accepts both shapes an implementation date arrives in: the
// RFC3339 the risk list returns (the entity layer widens the DATE on the way
// through) and the bare YYYY-MM-DD everything else speaks. Either way the
// result is a UTC date-only day, so day arithmetic can't be thrown off by a
// time component.
func parseDueDate(s string) (time.Time, error) {
	if t, err := time.Parse(dateLayout, s); err == nil {
		return dateOnly(t.UTC()), nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse due date %q: %w", s, err)
	}
	return dateOnly(t.UTC()), nil
}
