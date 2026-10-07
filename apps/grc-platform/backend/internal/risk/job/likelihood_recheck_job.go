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

// recheckWindowDays is how many days before each quarter's end the sweep
// actively re-checks risks — proposed default, pure config/logic, adjustable
// without any schema or deployment change (see
// docs/plans/auto-categorisation-plan.md, "The quarterly re-check").
const recheckWindowDays = 14

// riskDetailGetter fetches the full detail a Likelihood suggestion needs
// (Title, Description, Impact Description, Category, Compliance References,
// Source Register, current effective score) — RiskListItem (riskLister's
// result) doesn't carry enough.
type riskDetailGetter interface {
	GetByID(ctx context.Context, id int) (*model.RiskDetail, error)
}

// likelihoodChecker is the subset of riskservice.LikelihoodSuggestionService
// this job needs, satisfied structurally — same reasoning as reminderClaimer:
// this package stays decoupled from the service layer's concrete types.
// CreateUnresolvedSuggestion is the one write this job performs — a bare
// SUGGESTED row, never decided by the job itself (only a human decides, via
// the existing reassessment flow this row's existence nudges them toward).
type likelihoodChecker interface {
	Suggest(ctx context.Context, req model.SuggestLikelihoodRequest) (*model.SuggestLikelihoodResponse, error)
	CheckedThisQuarter(ctx context.Context, riskID int, quarterStart time.Time) (bool, error)
	CreateUnresolvedSuggestion(ctx context.Context, riskID int, result *model.SuggestLikelihoodResponse) error
}

// LikelihoodRecheckJob is the quarterly re-check sweep: for every
// IN_REMEDIATION risk, inside the last recheckWindowDays of each quarter,
// re-run the same live Likelihood check used for the original suggestion and
// compare it against the risk's current EFFECTIVE score — the latest
// reassessment's Residual when one exists, else Gross. Comparing against
// Gross unconditionally would flag a risk as "changed" just because an
// earlier, already-reviewed reassessment moved it away from Gross on
// purpose; EffectiveScore is what the risk's owner actually believes today.
// A changed result writes a new SUGGESTED row — nothing is decided
// automatically, and nothing is emailed; the risk's own detail page
// surfaces the unresolved row as an in-page reminder (handler.handleGetRisk,
// PendingLikelihoodSuggestion).
//
// Registered as an ordinary daily scheduler.Sweep (internal/scheduler) — not
// a new cadence. The window-gating below is what makes "quarterly" work on
// top of a scheduler that only ever ticks once a day: every day this job
// runs, it checks "am I inside the window," and does nothing on every day
// it isn't.
type LikelihoodRecheckJob struct {
	risks   riskLister
	detail  riskDetailGetter
	check   likelihoodChecker
	enabled func() bool
	running atomic.Bool
}

// NewLikelihoodRecheckJob constructs a LikelihoodRecheckJob. enabled is
// checked at the start of every run — a plain function, not a bool captured
// at startup, so flipping AI_LIKELIHOOD_ENABLED takes effect without a
// restart, matching how the rest of this feature is gated.
func NewLikelihoodRecheckJob(
	risks riskLister,
	detail riskDetailGetter,
	check likelihoodChecker,
	enabled func() bool,
) *LikelihoodRecheckJob {
	return &LikelihoodRecheckJob{risks: risks, detail: detail, check: check, enabled: enabled}
}

// RunOnce runs the sweep synchronously and returns its error, if any.
func (j *LikelihoodRecheckJob) RunOnce(ctx context.Context) error {
	if !j.running.CompareAndSwap(false, true) {
		return errors.New("likelihood recheck job: a sweep is already running")
	}
	defer j.running.Store(false)
	return j.runOnce(ctx)
}

func (j *LikelihoodRecheckJob) runOnce(parent context.Context) (runErr error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("likelihood recheck job: recovered from panic", "panic", r, "stack", string(debug.Stack()))
			runErr = fmt.Errorf("likelihood recheck job panic: %v", r)
		}
	}()

	if !j.enabled() {
		return nil
	}

	now := time.Now().UTC()
	quarterStart, quarterEnd := quarterBounds(now)
	if !inRecheckWindow(now, quarterEnd) {
		return nil
	}

	ctx, cancel := context.WithTimeout(parent, runTimeout)
	defer cancel()

	checked, changed, skippedAlready, skippedErr := 0, 0, 0, 0
	offset := 0
	for {
		page, err := j.risks.List(ctx, model.ListRisksFilter{
			Statuses: []string{model.StatusInRemediation},
			Limit:    pageLimit,
			Offset:   offset,
		})
		if err != nil {
			slog.Error("likelihood recheck job: list IN_REMEDIATION risks", "err", err)
			return fmt.Errorf("list risks: %w", err)
		}
		if len(page.Items) == 0 {
			break
		}

		for _, r := range page.Items {
			if r == nil {
				continue
			}

			already, err := j.check.CheckedThisQuarter(ctx, r.ID, quarterStart)
			if err != nil {
				slog.Warn("likelihood recheck job: already-checked lookup failed, skipping this risk",
					"riskId", r.ID, "err", err)
				skippedErr++
				continue
			}
			if already {
				skippedAlready++
				continue
			}

			detail, err := j.detail.GetByID(ctx, r.ID)
			if err != nil {
				slog.Warn("likelihood recheck job: fetch risk detail failed, skipping this risk",
					"riskId", r.ID, "err", err)
				skippedErr++
				continue
			}
			// EffectiveScore, not GrossScore: the baseline to compare fresh
			// evidence against is whatever the risk's current standing
			// actually is — the latest reassessment's Residual when one
			// exists, else Gross (same "effective score" convention the
			// dashboard/analytics repositories already use). Comparing
			// against Gross unconditionally would flag a risk as "changed"
			// just because an earlier reassessment (e.g. after compensating
			// controls landed) moved it away from Gross on purpose — that's
			// not new evidence, it's a decision already reviewed and on
			// record.
			if detail.EffectiveScore == nil {
				// Not expected for an IN_REMEDIATION risk, but fail safe
				// rather than crash the sweep.
				skippedErr++
				continue
			}

			req, ok := buildSuggestRequest(detail)
			if !ok {
				// Missing one of the required inputs (e.g. no category or
				// compliance reference yet) — nothing to check this risk
				// against; not an error, just not ready.
				continue
			}

			result, err := j.check.Suggest(ctx, req)
			if err != nil {
				slog.Warn("likelihood recheck job: suggest call failed, skipping this risk",
					"riskId", r.ID, "err", err)
				skippedErr++
				continue
			}
			checked++

			if result.Score == detail.EffectiveScore.Likelihood {
				continue
			}

			if err := j.check.CreateUnresolvedSuggestion(ctx, r.ID, result); err != nil {
				slog.Warn("likelihood recheck job: write suggestion failed",
					"riskId", r.ID, "err", err)
				skippedErr++
				continue
			}
			changed++
		}

		offset += len(page.Items)
		if offset >= page.Total {
			break
		}
	}

	slog.Info("likelihood recheck job: run complete",
		"checked", checked, "changed", changed,
		"skippedAlreadyThisQuarter", skippedAlready, "skippedErr", skippedErr)
	return nil
}

// buildSuggestRequest maps a risk's current detail to a suggestion request.
// ok is false when a required input (Category, Compliance Reference) isn't
// set yet — Title/Description/Impact Description are already mandatory on
// every risk, so only these two are ever actually missing in practice.
func buildSuggestRequest(detail *model.RiskDetail) (model.SuggestLikelihoodRequest, bool) {
	if len(detail.RiskCategories) == 0 {
		return model.SuggestLikelihoodRequest{}, false
	}
	impactDescription := ""
	if detail.ImpactDescription != nil {
		impactDescription = *detail.ImpactDescription
	}
	refIDs := make([]int, 0, len(detail.ComplianceReferences))
	for _, ref := range detail.ComplianceReferences {
		refIDs = append(refIDs, ref.ID)
	}
	return model.SuggestLikelihoodRequest{
		Title:                  detail.RiskTitle,
		Description:            detail.RiskDescription,
		ImpactDescription:      impactDescription,
		ComplianceReferenceIDs: refIDs,
		// Single-select today even though the schema is many-to-many — same
		// convention CreateRiskRequest.RiskCategoryIDs already follows.
		CategoryID:       detail.RiskCategories[0].ID,
		SourceRegisterID: detail.SourceRegisterID,
	}, true
}

// quarterBounds returns [start, end) of the calendar quarter t falls in,
// both at UTC midnight. Calendar-aligned (Jan-Mar, Apr-Jun, Jul-Sep, Oct-Dec),
// matching risk.risk_quarter's own Q1-Q4 convention.
func quarterBounds(t time.Time) (start, end time.Time) {
	qStartMonth := time.Month(((int(t.Month())-1)/3)*3 + 1)
	start = time.Date(t.Year(), qStartMonth, 1, 0, 0, 0, 0, time.UTC)
	end = start.AddDate(0, 3, 0)
	return start, end
}

// inRecheckWindow reports whether now falls in the last recheckWindowDays
// before quarterEnd.
func inRecheckWindow(now, quarterEnd time.Time) bool {
	windowStart := quarterEnd.AddDate(0, 0, -recheckWindowDays)
	return !now.Before(windowStart) && now.Before(quarterEnd)
}
