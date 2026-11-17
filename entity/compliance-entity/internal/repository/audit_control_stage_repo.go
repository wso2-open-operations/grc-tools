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

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/deadline"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

// StageRepository reads the hand-off history (audit_control_stage) and an
// audit's allowed days (audit_deadline_days). Writing stages happens inside
// the control repository's own transactions, next to the status write.
type StageRepository interface {
	// ListStagesByControls returns each control's stages in every phase, oldest first.
	// Controls with no recorded hand-offs are absent from the map.
	ListStagesByControls(ctx context.Context, controlIDs []int) (map[int][]deadline.Stage, error)
	// GetDeadlineDays returns the allowed days saved for the audit. Anything not
	// saved is left for deadline.PhaseDays.For to default.
	GetDeadlineDays(ctx context.Context, auditID int) (deadline.PhaseDays, error)
	// PutDeadlineDays upserts the given rows in one transaction. With
	// ApplyToOpenStages it also moves the deadline of every open stage that a
	// changed row governs, in the same transaction; with DryRun it does all of
	// that and rolls back, returning only what would have changed.
	PutDeadlineDays(ctx context.Context, auditID int, rows []domain.DeadlineDaysRow, opts DeadlineDaysOptions) (domain.RecalcSummary, error)
}

// DeadlineDaysOptions says what saving allowed days should also do.
type DeadlineDaysOptions struct {
	ApplyToOpenStages bool
	DryRun            bool // implies working out ApplyToOpenStages, and saves nothing
}

type stageRepo struct {
	db  *sql.DB
	now func() time.Time
}

// NewStageRepository constructs a StageRepository.
func NewStageRepository(db *sql.DB) StageRepository { return &stageRepo{db: db, now: time.Now} }

// stageDB is the part of *sql.DB / *sql.Tx the stage helpers need, so the same
// code runs inside a control transaction or on its own.
type stageDB interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func (r *stageRepo) ListStagesByControls(ctx context.Context, controlIDs []int) (map[int][]deadline.Stage, error) {
	out := map[int][]deadline.Stage{}
	if len(controlIDs) == 0 {
		return out, nil
	}
	ph := strings.Repeat("?,", len(controlIDs))
	args := make([]any, 0, len(controlIDs))
	for _, id := range controlIDs {
		args = append(args, id)
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, control_id, phase, handled_by, cycle_type, started_at, due_date,
		        planned_compliance_date, planned_auditor_date, ended_at, updated_by
		 FROM audit_control_stage
		 WHERE control_id IN (`+ph[:len(ph)-1]+`)
		 ORDER BY control_id, started_at, id`, args...)
	if err != nil {
		return nil, fmt.Errorf("stage.ListByControls: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			s                                 deadline.Stage
			due                               time.Time
			plannedCompliance, plannedAuditor sql.NullTime
			ended                             sql.NullTime
			updatedBy                         sql.NullString
		)
		if err := rows.Scan(&s.ID, &s.ControlID, &s.Phase, &s.HandledBy, &s.CycleType, &s.StartedAt, &due,
			&plannedCompliance, &plannedAuditor, &ended, &updatedBy); err != nil {
			return nil, fmt.Errorf("stage.ListByControls scan: %w", err)
		}
		s.DueDate = deadline.DayFromDB(due)
		if plannedCompliance.Valid {
			d := deadline.DayFromDB(plannedCompliance.Time)
			s.PlannedComplianceDate = &d
		}
		if plannedAuditor.Valid {
			d := deadline.DayFromDB(plannedAuditor.Time)
			s.PlannedAuditorDate = &d
		}
		if ended.Valid {
			t := ended.Time
			s.EndedAt = &t
		}
		if updatedBy.Valid {
			u := updatedBy.String
			s.UpdatedBy = &u
		}
		out[s.ControlID] = append(out[s.ControlID], s)
	}
	return out, rows.Err()
}

func (r *stageRepo) GetDeadlineDays(ctx context.Context, auditID int) (deadline.PhaseDays, error) {
	return loadDeadlineDays(ctx, r.db, auditID)
}

func (r *stageRepo) PutDeadlineDays(ctx context.Context, auditID int, rows []domain.DeadlineDaysRow, opts DeadlineDaysOptions) (domain.RecalcSummary, error) {
	var sum domain.RecalcSummary
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return sum, fmt.Errorf("stage.PutDeadlineDays begin: %w", err)
	}
	defer tx.Rollback()

	// The values that actually change, by phase|cycle|group: a row sent with the
	// number the audit already uses moves nothing.
	changed := map[string]int{}
	if opts.ApplyToOpenStages || opts.DryRun {
		before, err := loadDeadlineDays(ctx, tx, auditID)
		if err != nil {
			return sum, err
		}
		for _, row := range rows {
			p := rowPhase(row)
			if row.AllowedDays != before.For(p, row.CycleType, row.HandledBy) {
				changed[stageKey(p, row.CycleType, row.HandledBy)] = row.AllowedDays
			}
		}
	}
	for _, row := range rows {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO audit_deadline_days (audit_id, phase, cycle_type, handled_by, allowed_days)
			 VALUES (?, ?, ?, ?, ?)
			 ON DUPLICATE KEY UPDATE allowed_days = VALUES(allowed_days)`,
			auditID, rowPhase(row), row.CycleType, row.HandledBy, row.AllowedDays); err != nil {
			return sum, fmt.Errorf("stage.PutDeadlineDays(%d): %w", auditID, err)
		}
	}
	if len(changed) > 0 {
		if sum, err = recalcOpenStages(ctx, tx, auditID, changed, r.now()); err != nil {
			return sum, err
		}
	}
	if opts.DryRun {
		return sum, nil // the deferred rollback discards everything
	}
	if err := tx.Commit(); err != nil {
		return sum, fmt.Errorf("stage.PutDeadlineDays commit: %w", err)
	}
	return sum, nil
}

func stageKey(phase, cycle, handler string) string { return phase + "|" + cycle + "|" + handler }

// recalcOpenStages moves the deadline of each open stage in the audit that a
// changed value governs to its start day plus the new days. Only the stage's
// own deadline moves: the control's ultimate deadline and the planned GRC and EY
// dates frozen on a phase's first stage are never touched. The Owner's planned
// date is that first stage's deadline, so it follows while the stage is open.
func recalcOpenStages(ctx context.Context, tx stageDB, auditID int, changed map[string]int, now time.Time) (domain.RecalcSummary, error) {
	var sum domain.RecalcSummary
	rows, err := tx.QueryContext(ctx,
		`SELECT s.id, s.phase, s.cycle_type, s.handled_by, s.started_at, s.due_date
		 FROM audit_control_stage s
		 WHERE s.ended_at IS NULL AND s.control_id IN (SELECT id FROM audit_control WHERE audit_id = ?)
		 FOR UPDATE`, auditID)
	if err != nil {
		return sum, fmt.Errorf("stage.recalc(%d) load: %w", auditID, err)
	}
	type move struct {
		id     int64
		newDue time.Time
	}
	var moves []move
	today := deadline.DayOf(now)
	for rows.Next() {
		var (
			id                    int64
			phase, cycle, handler string
			started, due          time.Time
		)
		if err := rows.Scan(&id, &phase, &cycle, &handler, &started, &due); err != nil {
			rows.Close()
			return sum, fmt.Errorf("stage.recalc(%d) scan: %w", auditID, err)
		}
		days, ok := changed[stageKey(phase, cycle, handler)]
		if !ok {
			continue
		}
		newDue := deadline.AddDays(deadline.DayOf(started), days)
		oldDue := deadline.DayFromDB(due)
		if newDue.Equal(oldDue) {
			continue
		}
		moves = append(moves, move{id, newDue})
		sum.StagesUpdated++
		if newDue.Before(today) && !oldDue.Before(today) {
			sum.NewlyOverdue++
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return sum, fmt.Errorf("stage.recalc(%d): %w", auditID, err)
	}
	rows.Close()
	for _, m := range moves {
		if _, err := tx.ExecContext(ctx,
			"UPDATE audit_control_stage SET due_date = ? WHERE id = ?", deadline.FormatDay(m.newDue), m.id); err != nil {
			return sum, fmt.Errorf("stage.recalc(%d) update %d: %w", auditID, m.id, err)
		}
	}
	return sum, nil
}

// rowPhase is the phase a saved row belongs to; a row from a client that
// predates OE has none and means Design evidence.
func rowPhase(row domain.DeadlineDaysRow) string {
	if row.Phase == "" {
		return deadline.PhaseEvidence
	}
	return row.Phase
}

// loadDeadlineDays reads the allowed days saved for an audit, for every phase.
func loadDeadlineDays(ctx context.Context, q stageDB, auditID int) (deadline.PhaseDays, error) {
	days := deadline.PhaseDays{}
	rows, err := q.QueryContext(ctx,
		`SELECT phase, cycle_type, handled_by, allowed_days FROM audit_deadline_days
		 WHERE audit_id = ?`, auditID)
	if err != nil {
		return nil, fmt.Errorf("deadlineDays.load(%d): %w", auditID, err)
	}
	defer rows.Close()
	for rows.Next() {
		var phase, cycle, handler string
		var n int
		if err := rows.Scan(&phase, &cycle, &handler, &n); err != nil {
			return nil, fmt.Errorf("deadlineDays.load scan: %w", err)
		}
		days.Set(phase, cycle, handler, n)
	}
	return days, rows.Err()
}

// dayString trims a date or datetime string to YYYY-MM-DD.
func dayString(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 10 {
		return s[:10]
	}
	return s
}

// openFirstStage records the owner's opening stage for a new control: Design
// evidence for a DESIGN control, population for an OE control. The owner's
// deadline is the creation day plus the audit's Owner initial days for that
// phase; the control's own due date is the ultimate deadline and plays no part
// in the plan. The original planned GRC and EY dates are frozen on the row.
func openFirstStage(ctx context.Context, tx stageDB, auditID, controlID int, oe bool, now time.Time) error {
	days, err := loadDeadlineDays(ctx, tx, auditID)
	if err != nil {
		return err
	}
	phase := deadline.FirstPhase(oe)
	ownerDue := deadline.AddDays(deadline.DayOf(now), days.For(phase, deadline.Initial, deadline.Owner))
	compliance, auditor := deadline.PlannedDates(phase, ownerDue, days)
	first := deadline.FirstStage(phase, ownerDue, now)
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO audit_control_stage
		 (control_id, phase, handled_by, cycle_type, started_at, due_date, planned_compliance_date, planned_auditor_date)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		controlID, first.Phase, first.HandledBy, first.CycleType, first.StartedAt,
		deadline.FormatDay(first.DueDate), deadline.FormatDay(compliance), deadline.FormatDay(auditor)); err != nil {
		return fmt.Errorf("stage.openFirst(%d): %w", controlID, err)
	}
	return nil
}

// applyStageTransition closes and opens stages for a status change on a
// control. It runs inside the transaction that writes the new status, so the
// two can never disagree. A control with no stage history yet (it predates the
// feature) simply starts being tracked from this hand-off.
func applyStageTransition(ctx context.Context, tx stageDB, auditID, controlID int, from, to, actor string, override bool, now time.Time) error {
	from, to = strings.ToUpper(from), strings.ToUpper(to)
	if from == to {
		return nil
	}
	if _, _, tracked := deadline.StageFor(to, true); !tracked && to != "COMPLETE" {
		return nil
	}
	var requirementType string
	if err := tx.QueryRowContext(ctx,
		"SELECT requirement_type FROM audit_control WHERE id = ?", controlID).Scan(&requirementType); err != nil {
		return fmt.Errorf("stage.transition(%d) requirement type: %w", controlID, err)
	}
	oe := strings.EqualFold(requirementType, "OE")

	// Every stage the control has had, newest first: the latest says who holds it
	// now, and the groups seen anywhere in the list, per phase, have held it before.
	rows, err := tx.QueryContext(ctx,
		`SELECT id, phase, handled_by, cycle_type, ended_at FROM audit_control_stage
		 WHERE control_id = ?
		 ORDER BY started_at DESC, id DESC FOR UPDATE`, controlID)
	if err != nil {
		return fmt.Errorf("stage.transition(%d) history: %w", controlID, err)
	}
	var open *deadline.Stage
	heldBefore := map[string]bool{}
	first := true
	for rows.Next() {
		var (
			stageID               int64
			phase, handler, cycle string
			ended                 sql.NullTime
		)
		if err := rows.Scan(&stageID, &phase, &handler, &cycle, &ended); err != nil {
			rows.Close()
			return fmt.Errorf("stage.transition(%d) history scan: %w", controlID, err)
		}
		heldBefore[deadline.HeldKey(phase, handler)] = true
		if first && !ended.Valid {
			open = &deadline.Stage{ID: stageID, Phase: phase, HandledBy: handler, CycleType: cycle}
		}
		first = false
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("stage.transition(%d) history: %w", controlID, err)
	}
	rows.Close()

	days := deadline.PhaseDays{}
	if to != "COMPLETE" {
		if days, err = loadDeadlineDays(ctx, tx, auditID); err != nil {
			return err
		}
	}
	change := deadline.PlanTransition(deadline.TransitionInput{
		From: from, To: to, OE: oe, Override: override, Now: now, OpenStage: open, HeldBefore: heldBefore, Days: days,
	})
	if change.CloseOpen && open != nil {
		if _, err := tx.ExecContext(ctx,
			"UPDATE audit_control_stage SET ended_at = ?, updated_by = ? WHERE id = ?", now, actor, open.ID); err != nil {
			return fmt.Errorf("stage.transition(%d) close: %w", controlID, err)
		}
	}
	if change.Open != nil {
		o := change.Open
		var plannedCompliance, plannedAuditor any
		if o.PlannedComplianceDate != nil {
			plannedCompliance = deadline.FormatDay(*o.PlannedComplianceDate)
		}
		if o.PlannedAuditorDate != nil {
			plannedAuditor = deadline.FormatDay(*o.PlannedAuditorDate)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO audit_control_stage
			 (control_id, phase, handled_by, cycle_type, started_at, due_date, planned_compliance_date, planned_auditor_date)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			controlID, o.Phase, o.HandledBy, o.CycleType, o.StartedAt,
			deadline.FormatDay(o.DueDate), plannedCompliance, plannedAuditor); err != nil {
			return fmt.Errorf("stage.transition(%d) open: %w", controlID, err)
		}
	}
	return nil
}

// resetStagesForRequirementType rebuilds a control's stages after it switches
// between DESIGN and OE (only possible before work has started): the control
// starts a fresh first stage in the first phase of its new type.
func resetStagesForRequirementType(ctx context.Context, tx stageDB, auditID, controlID int, newType string, now time.Time) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM audit_control_stage WHERE control_id = ?", controlID); err != nil {
		return fmt.Errorf("stage.reset(%d) clear: %w", controlID, err)
	}
	return openFirstStage(ctx, tx, auditID, controlID, strings.EqualFold(newType, "OE"), now)
}
