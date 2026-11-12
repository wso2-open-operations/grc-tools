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
	"errors"
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
	// ListStagesByControls returns each control's evidence stages, oldest first.
	// Controls with no recorded hand-offs are absent from the map.
	ListStagesByControls(ctx context.Context, controlIDs []int) (map[int][]deadline.Stage, error)
	// GetDeadlineDays returns the allowed days saved for the audit. Anything not
	// saved is left for deadline.Days.For to default.
	GetDeadlineDays(ctx context.Context, auditID int) (deadline.Days, error)
	// PutDeadlineDays upserts the given rows in one transaction.
	PutDeadlineDays(ctx context.Context, auditID int, rows []domain.DeadlineDaysRow) error
}

type stageRepo struct{ db *sql.DB }

// NewStageRepository constructs a StageRepository.
func NewStageRepository(db *sql.DB) StageRepository { return &stageRepo{db: db} }

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
		 WHERE phase = 'EVIDENCE' AND control_id IN (`+ph[:len(ph)-1]+`)
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

func (r *stageRepo) GetDeadlineDays(ctx context.Context, auditID int) (deadline.Days, error) {
	return loadDeadlineDays(ctx, r.db, auditID)
}

func (r *stageRepo) PutDeadlineDays(ctx context.Context, auditID int, rows []domain.DeadlineDaysRow) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("stage.PutDeadlineDays begin: %w", err)
	}
	defer tx.Rollback()
	for _, row := range rows {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO audit_deadline_days (audit_id, phase, cycle_type, handled_by, allowed_days)
			 VALUES (?, 'EVIDENCE', ?, ?, ?)
			 ON DUPLICATE KEY UPDATE allowed_days = VALUES(allowed_days)`,
			auditID, row.CycleType, row.HandledBy, row.AllowedDays); err != nil {
			return fmt.Errorf("stage.PutDeadlineDays(%d): %w", auditID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("stage.PutDeadlineDays commit: %w", err)
	}
	return nil
}

// loadDeadlineDays reads the evidence-phase allowed days saved for an audit.
func loadDeadlineDays(ctx context.Context, q stageDB, auditID int) (deadline.Days, error) {
	days := deadline.Days{}
	rows, err := q.QueryContext(ctx,
		`SELECT cycle_type, handled_by, allowed_days FROM audit_deadline_days
		 WHERE audit_id = ? AND phase = 'EVIDENCE'`, auditID)
	if err != nil {
		return nil, fmt.Errorf("deadlineDays.load(%d): %w", auditID, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cycle, handler string
		var n int
		if err := rows.Scan(&cycle, &handler, &n); err != nil {
			return nil, fmt.Errorf("deadlineDays.load scan: %w", err)
		}
		days.Set(cycle, handler, n)
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

// openFirstStage records the owner's opening stage for a new DESIGN control and
// freezes the original planned GRC and EY dates on it.
func openFirstStage(ctx context.Context, tx stageDB, auditID, controlID int, dueDate string, now time.Time) error {
	ownerDue, err := deadline.ParseDay(dayString(dueDate))
	if err != nil {
		return fmt.Errorf("stage.openFirst: bad due date %q: %w", dueDate, err)
	}
	days, err := loadDeadlineDays(ctx, tx, auditID)
	if err != nil {
		return err
	}
	compliance, auditor := deadline.PlannedDates(ownerDue, days)
	first := deadline.FirstStage(ownerDue, now)
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO audit_control_stage
		 (control_id, phase, handled_by, cycle_type, started_at, due_date, planned_compliance_date, planned_auditor_date)
		 VALUES (?, 'EVIDENCE', ?, ?, ?, ?, ?, ?)`,
		controlID, first.HandledBy, first.CycleType, first.StartedAt,
		deadline.FormatDay(first.DueDate), deadline.FormatDay(compliance), deadline.FormatDay(auditor)); err != nil {
		return fmt.Errorf("stage.openFirst(%d): %w", controlID, err)
	}
	return nil
}

// applyStageTransition closes and opens stages for a status change on a DESIGN
// control. It runs inside the transaction that writes the new status, so the
// two can never disagree. A control with no stage history yet (it predates the
// feature) simply starts being tracked from this hand-off.
func applyStageTransition(ctx context.Context, tx stageDB, auditID, controlID int, from, to, actor string, override bool, now time.Time) error {
	from, to = strings.ToUpper(from), strings.ToUpper(to)
	if from == to {
		return nil
	}
	if _, tracked := deadline.HandlerForStatus(to); !tracked && to != "COMPLETE" {
		return nil
	}
	var requirementType string
	if err := tx.QueryRowContext(ctx,
		"SELECT requirement_type FROM audit_control WHERE id = ?", controlID).Scan(&requirementType); err != nil {
		return fmt.Errorf("stage.transition(%d) requirement type: %w", controlID, err)
	}
	if requirementType != "DESIGN" {
		return nil
	}

	var (
		open      *deadline.Stage
		prevCycle string
		stageID   int64
		handler   string
		cycle     string
		ended     sql.NullTime
	)
	err := tx.QueryRowContext(ctx,
		`SELECT id, handled_by, cycle_type, ended_at FROM audit_control_stage
		 WHERE control_id = ? AND phase = 'EVIDENCE'
		 ORDER BY started_at DESC, id DESC LIMIT 1 FOR UPDATE`, controlID).Scan(&stageID, &handler, &cycle, &ended)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return fmt.Errorf("stage.transition(%d) latest: %w", controlID, err)
	default:
		prevCycle = cycle
		if !ended.Valid {
			open = &deadline.Stage{ID: stageID, HandledBy: handler, CycleType: cycle}
		}
	}

	days := deadline.Days{}
	if to != "COMPLETE" {
		if days, err = loadDeadlineDays(ctx, tx, auditID); err != nil {
			return err
		}
	}
	change := deadline.PlanTransition(deadline.TransitionInput{
		From: from, To: to, Override: override, Now: now, OpenStage: open, PrevCycle: prevCycle, Days: days,
	})
	if change.CloseOpen && open != nil {
		if _, err := tx.ExecContext(ctx,
			"UPDATE audit_control_stage SET ended_at = ?, updated_by = ? WHERE id = ?", now, actor, open.ID); err != nil {
			return fmt.Errorf("stage.transition(%d) close: %w", controlID, err)
		}
	}
	if change.Open != nil {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO audit_control_stage (control_id, phase, handled_by, cycle_type, started_at, due_date)
			 VALUES (?, 'EVIDENCE', ?, ?, ?, ?)`,
			controlID, change.Open.HandledBy, change.Open.CycleType, change.Open.StartedAt,
			deadline.FormatDay(change.Open.DueDate)); err != nil {
			return fmt.Errorf("stage.transition(%d) open: %w", controlID, err)
		}
	}
	return nil
}

// syncFirstStageDueDate follows an edit of the control's due date while the
// owner is still on their first stage: the stage deadline moves and the
// planned GRC and EY dates are replanned. Once the control has moved on the
// first stage is closed and never changes, so this finds nothing.
func syncFirstStageDueDate(ctx context.Context, tx stageDB, auditID, controlID int, dueDate string) error {
	var stageID int64
	err := tx.QueryRowContext(ctx,
		`SELECT id FROM audit_control_stage
		 WHERE control_id = ? AND phase = 'EVIDENCE' AND ended_at IS NULL
		   AND handled_by = 'OWNER' AND cycle_type = 'INITIAL' AND planned_compliance_date IS NOT NULL
		 ORDER BY started_at, id LIMIT 1 FOR UPDATE`, controlID).Scan(&stageID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stage.syncDue(%d): %w", controlID, err)
	}
	ownerDue, err := deadline.ParseDay(dayString(dueDate))
	if err != nil {
		return fmt.Errorf("stage.syncDue: bad due date %q: %w", dueDate, err)
	}
	days, err := loadDeadlineDays(ctx, tx, auditID)
	if err != nil {
		return err
	}
	compliance, auditor := deadline.PlannedDates(ownerDue, days)
	if _, err := tx.ExecContext(ctx,
		`UPDATE audit_control_stage SET due_date = ?, planned_compliance_date = ?, planned_auditor_date = ? WHERE id = ?`,
		deadline.FormatDay(ownerDue), deadline.FormatDay(compliance), deadline.FormatDay(auditor), stageID); err != nil {
		return fmt.Errorf("stage.syncDue(%d) update: %w", controlID, err)
	}
	return nil
}

// resetStagesForRequirementType rebuilds a control's stages after it switches
// between DESIGN and OE (only possible before work has started): OE controls
// are not tracked, a DESIGN control starts a fresh first stage.
func resetStagesForRequirementType(ctx context.Context, tx stageDB, auditID, controlID int, newType string, now time.Time) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM audit_control_stage WHERE control_id = ?", controlID); err != nil {
		return fmt.Errorf("stage.reset(%d) clear: %w", controlID, err)
	}
	if newType != "DESIGN" {
		return nil
	}
	var due sql.NullString
	if err := tx.QueryRowContext(ctx,
		"SELECT DATE_FORMAT(due_date, '%Y-%m-%d') FROM audit_control WHERE id = ?", controlID).Scan(&due); err != nil {
		return fmt.Errorf("stage.reset(%d) due date: %w", controlID, err)
	}
	if !due.Valid || due.String == "" {
		return nil
	}
	return openFirstStage(ctx, tx, auditID, controlID, due.String, now)
}
