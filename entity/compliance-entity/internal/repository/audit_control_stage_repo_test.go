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
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/deadline"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

func stageNow() time.Time {
	return time.Date(2026, time.November, 10, 8, 30, 0, 0, time.UTC) // 14:00 in Colombo
}

func TestOpenFirstStageFreezesPlannedDates(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("FROM audit_deadline_days")).
		WithArgs(7).
		WillReturnRows(sqlmock.NewRows([]string{"phase", "cycle_type", "handled_by", "allowed_days"}))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO audit_control_stage")).
		WithArgs(42, "EVIDENCE", "OWNER", "INITIAL", stageNow(), "2026-12-10", "2026-12-20", "2026-12-30").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, _ := db.Begin()
	if err := openFirstStage(context.Background(), tx, 7, 42, false, stageNow()); err != nil {
		t.Fatal(err)
	}
	_ = tx.Commit()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestOpenFirstStageUsesSavedAuditDays(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("FROM audit_deadline_days")).
		WithArgs(7).
		WillReturnRows(sqlmock.NewRows([]string{"phase", "cycle_type", "handled_by", "allowed_days"}).
			AddRow("EVIDENCE", "INITIAL", "COMPLIANCE", 12).AddRow("EVIDENCE", "INITIAL", "AUDITOR", 5))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO audit_control_stage")).
		WithArgs(42, "EVIDENCE", "OWNER", "INITIAL", stageNow(), "2026-12-10", "2026-12-22", "2026-12-27").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, _ := db.Begin()
	if err := openFirstStage(context.Background(), tx, 7, 42, false, stageNow()); err != nil {
		t.Fatal(err)
	}
	_ = tx.Commit()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyStageTransitionHandsOffToGRC(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT requirement_type FROM audit_control")).
		WithArgs(42).WillReturnRows(sqlmock.NewRows([]string{"requirement_type"}).AddRow("DESIGN"))
	mock.ExpectQuery(regexp.QuoteMeta("FROM audit_control_stage")).
		WithArgs(42).
		WillReturnRows(sqlmock.NewRows([]string{"id", "phase", "handled_by", "cycle_type", "ended_at"}).AddRow(5, "EVIDENCE", "OWNER", "INITIAL", nil))
	mock.ExpectQuery(regexp.QuoteMeta("FROM audit_deadline_days")).
		WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"phase", "cycle_type", "handled_by", "allowed_days"}))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE audit_control_stage SET ended_at")).
		WithArgs(stageNow(), "owner@wso2.com", int64(5)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO audit_control_stage")).
		WithArgs(42, "EVIDENCE", "COMPLIANCE", "INITIAL", stageNow(), "2026-11-20", nil, nil).WillReturnResult(sqlmock.NewResult(6, 1))
	mock.ExpectCommit()

	tx, _ := db.Begin()
	if err := applyStageTransition(context.Background(), tx, 7, 42,
		"EVIDENCE_PENDING", "EVIDENCE_INTERNAL_REVIEW", "owner@wso2.com", false, stageNow()); err != nil {
		t.Fatal(err)
	}
	_ = tx.Commit()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyStageTransitionOEPopulationHandOff(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT requirement_type FROM audit_control")).
		WithArgs(42).WillReturnRows(sqlmock.NewRows([]string{"requirement_type"}).AddRow("OE"))
	mock.ExpectQuery(regexp.QuoteMeta("FROM audit_control_stage")).
		WithArgs(42).
		WillReturnRows(sqlmock.NewRows([]string{"id", "phase", "handled_by", "cycle_type", "ended_at"}).AddRow(5, "POPULATION", "OWNER", "INITIAL", nil))
	mock.ExpectQuery(regexp.QuoteMeta("FROM audit_deadline_days")).
		WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"phase", "cycle_type", "handled_by", "allowed_days"}))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE audit_control_stage SET ended_at")).
		WithArgs(stageNow(), "owner@wso2.com", int64(5)).WillReturnResult(sqlmock.NewResult(0, 1))
	// GRC has 3 population days from the 10th.
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO audit_control_stage")).
		WithArgs(42, "POPULATION", "COMPLIANCE", "INITIAL", stageNow(), "2026-11-13", nil, nil).WillReturnResult(sqlmock.NewResult(6, 1))
	mock.ExpectCommit()

	tx, _ := db.Begin()
	if err := applyStageTransition(context.Background(), tx, 7, 42,
		"POPULATION_PENDING", "POPULATION_INTERNAL_REVIEW", "owner@wso2.com", false, stageNow()); err != nil {
		t.Fatal(err)
	}
	_ = tx.Commit()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// When EY submits the sample the Owner starts the evidence phase: their first
// time there, so 5 initial days, and the phase's plan is frozen on that row
// (GRC 2 days later, EY 2 more) even though the Owner held the population.
func TestApplyStageTransitionOEEvidenceStartsFreshWithPlan(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT requirement_type FROM audit_control")).
		WithArgs(42).WillReturnRows(sqlmock.NewRows([]string{"requirement_type"}).AddRow("OE"))
	mock.ExpectQuery(regexp.QuoteMeta("FROM audit_control_stage")).
		WithArgs(42).
		WillReturnRows(sqlmock.NewRows([]string{"id", "phase", "handled_by", "cycle_type", "ended_at"}).
			AddRow(8, "POPULATION", "AUDITOR", "INITIAL", nil).
			AddRow(7, "POPULATION", "COMPLIANCE", "INITIAL", stageNow()).
			AddRow(6, "POPULATION", "OWNER", "INITIAL", stageNow()))
	mock.ExpectQuery(regexp.QuoteMeta("FROM audit_deadline_days")).
		WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"phase", "cycle_type", "handled_by", "allowed_days"}))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE audit_control_stage SET ended_at")).
		WithArgs(stageNow(), "ey@wso2.com", int64(8)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO audit_control_stage")).
		WithArgs(42, "OE_EVIDENCE", "OWNER", "INITIAL", stageNow(), "2026-11-15", "2026-11-17", "2026-11-19").WillReturnResult(sqlmock.NewResult(9, 1))
	mock.ExpectCommit()

	tx, _ := db.Begin()
	if err := applyStageTransition(context.Background(), tx, 7, 42,
		"AWAITING_SAMPLE", "SUBMITTED_SAMPLE", "ey@wso2.com", false, stageNow()); err != nil {
		t.Fatal(err)
	}
	_ = tx.Commit()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// The new OE control starts in the population phase with the Owner's 4 days.
func TestOpenFirstStageOEStartsInPopulation(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("FROM audit_deadline_days")).
		WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"phase", "cycle_type", "handled_by", "allowed_days"}))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO audit_control_stage")).
		WithArgs(42, "POPULATION", "OWNER", "INITIAL", stageNow(), "2026-11-14", "2026-11-17", "2026-11-20").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, _ := db.Begin()
	if err := openFirstStage(context.Background(), tx, 7, 42, true, stageNow()); err != nil {
		t.Fatal(err)
	}
	_ = tx.Commit()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// A control that predates the feature has no stage rows: it starts being
// tracked from the hand-off, with nothing to close.
func TestApplyStageTransitionStartsTrackingLegacyControl(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT requirement_type FROM audit_control")).
		WithArgs(42).WillReturnRows(sqlmock.NewRows([]string{"requirement_type"}).AddRow("DESIGN"))
	mock.ExpectQuery(regexp.QuoteMeta("FROM audit_control_stage")).
		WithArgs(42).WillReturnRows(sqlmock.NewRows([]string{"id", "phase", "handled_by", "cycle_type", "ended_at"}))
	mock.ExpectQuery(regexp.QuoteMeta("FROM audit_deadline_days")).
		WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"phase", "cycle_type", "handled_by", "allowed_days"}))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO audit_control_stage")).
		WithArgs(42, "EVIDENCE", "AUDITOR", "INITIAL", stageNow(), "2026-11-20", nil, nil).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, _ := db.Begin()
	if err := applyStageTransition(context.Background(), tx, 7, 42,
		"EVIDENCE_INTERNAL_REVIEW", "EVIDENCE_UNDER_VALIDATION", "grc@wso2.com", false, stageNow()); err != nil {
		t.Fatal(err)
	}
	_ = tx.Commit()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyStageTransitionClosesOnComplete(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT requirement_type FROM audit_control")).
		WithArgs(42).WillReturnRows(sqlmock.NewRows([]string{"requirement_type"}).AddRow("DESIGN"))
	mock.ExpectQuery(regexp.QuoteMeta("FROM audit_control_stage")).
		WithArgs(42).
		WillReturnRows(sqlmock.NewRows([]string{"id", "phase", "handled_by", "cycle_type", "ended_at"}).AddRow(9, "EVIDENCE", "AUDITOR", "INITIAL", nil))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE audit_control_stage SET ended_at")).
		WithArgs(stageNow(), "ey@wso2.com", int64(9)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, _ := db.Begin()
	if err := applyStageTransition(context.Background(), tx, 7, 42,
		"EVIDENCE_UNDER_VALIDATION", "COMPLETE", "ey@wso2.com", false, stageNow()); err != nil {
		t.Fatal(err)
	}
	_ = tx.Commit()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestBuildControlFiltersCurrentHandlers(t *testing.T) {
	where, args := buildControlFilters("WHERE c.audit_id = ?", []any{7}, domainSearchWithHandlers("OWNER", "AUDITOR"))
	if !regexp.MustCompile(`EXISTS \(SELECT 1 FROM audit_control_stage st WHERE st.control_id = c.id AND st.ended_at IS NULL AND st.handled_by IN \(\?,\?\)\)`).MatchString(where) {
		t.Fatalf("where = %s", where)
	}
	if len(args) < 3 || args[1] != "OWNER" || args[2] != "AUDITOR" {
		t.Fatalf("args = %v", args)
	}
}

// GRC passes a control on after a rejection round. EY has never held it, so EY
// gets the initial 10 days, not the revision 5.
func TestApplyStageTransitionFirstTimeAuditorAfterRevisionGetsInitialDays(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT requirement_type FROM audit_control")).
		WithArgs(42).WillReturnRows(sqlmock.NewRows([]string{"requirement_type"}).AddRow("DESIGN"))
	mock.ExpectQuery(regexp.QuoteMeta("FROM audit_control_stage")).
		WithArgs(42).
		WillReturnRows(sqlmock.NewRows([]string{"id", "phase", "handled_by", "cycle_type", "ended_at"}).
			AddRow(9, "EVIDENCE", "COMPLIANCE", "REVISION", nil).
			AddRow(8, "EVIDENCE", "OWNER", "REVISION", stageNow()).
			AddRow(7, "EVIDENCE", "COMPLIANCE", "INITIAL", stageNow()).
			AddRow(6, "EVIDENCE", "OWNER", "INITIAL", stageNow()))
	mock.ExpectQuery(regexp.QuoteMeta("FROM audit_deadline_days")).
		WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"phase", "cycle_type", "handled_by", "allowed_days"}))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE audit_control_stage SET ended_at")).
		WithArgs(stageNow(), "grc@wso2.com", int64(9)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO audit_control_stage")).
		WithArgs(42, "EVIDENCE", "AUDITOR", "INITIAL", stageNow(), "2026-11-20", nil, nil).WillReturnResult(sqlmock.NewResult(10, 1))
	mock.ExpectCommit()

	tx, _ := db.Begin()
	if err := applyStageTransition(context.Background(), tx, 7, 42,
		"EVIDENCE_INTERNAL_REVIEW", "EVIDENCE_UNDER_VALIDATION", "grc@wso2.com", false, stageNow()); err != nil {
		t.Fatal(err)
	}
	_ = tx.Commit()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// ---- applying changed days to open stages ----

func recalcRepo(t *testing.T) (*stageRepo, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	// 8 Nov in Sri Lanka.
	now := time.Date(2026, time.November, 8, 12, 0, 0, 0, deadline.Location)
	return &stageRepo{db: db, now: func() time.Time { return now }}, mock, func() { db.Close() }
}

func utcDay(m time.Month, d int) time.Time { return time.Date(2026, m, d, 0, 0, 0, 0, time.UTC) }

func complianceRow(days int) domain.DeadlineDaysRow {
	return domain.DeadlineDaysRow{Phase: "EVIDENCE", CycleType: "INITIAL", HandledBy: "COMPLIANCE", AllowedDays: days}
}

func expectOpenStages(mock sqlmock.Sqlmock) {
	started := time.Date(2026, time.November, 1, 10, 0, 0, 0, deadline.Location)
	mock.ExpectQuery(regexp.QuoteMeta("FROM audit_control_stage s")).
		WithArgs(7).
		WillReturnRows(sqlmock.NewRows([]string{"id", "phase", "cycle_type", "handled_by", "started_at", "due_date"}).
			AddRow(10, "EVIDENCE", "INITIAL", "COMPLIANCE", started, utcDay(time.November, 11)).
			AddRow(11, "EVIDENCE", "REVISION", "COMPLIANCE", started, utcDay(time.November, 6)).
			AddRow(12, "EVIDENCE", "INITIAL", "AUDITOR", started, utcDay(time.November, 11)))
}

func emptyDays(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(regexp.QuoteMeta("FROM audit_deadline_days")).
		WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"phase", "cycle_type", "handled_by", "allowed_days"}))
}

func TestPutDeadlineDaysWithoutApplyOnlySavesTheDays(t *testing.T) {
	r, mock, done := recalcRepo(t)
	defer done()
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO audit_deadline_days")).
		WithArgs(7, "EVIDENCE", "INITIAL", "COMPLIANCE", 15).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	sum, err := r.PutDeadlineDays(context.Background(), 7, []domain.DeadlineDaysRow{complianceRow(15)}, DeadlineDaysOptions{})
	if err != nil || sum.StagesUpdated != 0 {
		t.Fatalf("sum=%+v err=%v", sum, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// 10 -> 15: the open initial Compliance stage that started 1 Nov moves from 11 Nov to
// 16 Nov. The revision stage and the Auditor's stage are not governed by the change.
func TestPutDeadlineDaysExtendsOpenStagesOfTheChangedCycleOnly(t *testing.T) {
	r, mock, done := recalcRepo(t)
	defer done()
	mock.ExpectBegin()
	emptyDays(mock)
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO audit_deadline_days")).
		WithArgs(7, "EVIDENCE", "INITIAL", "COMPLIANCE", 15).WillReturnResult(sqlmock.NewResult(0, 1))
	expectOpenStages(mock)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE audit_control_stage SET due_date")).
		WithArgs("2026-11-16", int64(10)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	sum, err := r.PutDeadlineDays(context.Background(), 7, []domain.DeadlineDaysRow{complianceRow(15)}, DeadlineDaysOptions{ApplyToOpenStages: true})
	if err != nil {
		t.Fatal(err)
	}
	if sum.StagesUpdated != 1 || sum.NewlyOverdue != 0 {
		t.Fatalf("sum = %+v", sum)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// 10 -> 5: that stage's deadline becomes 6 Nov, before today (8 Nov): newly overdue.
func TestPutDeadlineDaysShorteningCountsNewlyOverdue(t *testing.T) {
	r, mock, done := recalcRepo(t)
	defer done()
	mock.ExpectBegin()
	emptyDays(mock)
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO audit_deadline_days")).
		WithArgs(7, "EVIDENCE", "INITIAL", "COMPLIANCE", 5).WillReturnResult(sqlmock.NewResult(0, 1))
	expectOpenStages(mock)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE audit_control_stage SET due_date")).
		WithArgs("2026-11-06", int64(10)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	sum, err := r.PutDeadlineDays(context.Background(), 7, []domain.DeadlineDaysRow{complianceRow(5)}, DeadlineDaysOptions{ApplyToOpenStages: true})
	if err != nil || sum.StagesUpdated != 1 || sum.NewlyOverdue != 1 {
		t.Fatalf("sum=%+v err=%v", sum, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// Sending the number the audit already uses changes nothing, so no stage is touched.
func TestPutDeadlineDaysUnchangedValueMovesNothing(t *testing.T) {
	r, mock, done := recalcRepo(t)
	defer done()
	mock.ExpectBegin()
	emptyDays(mock)
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO audit_deadline_days")).
		WithArgs(7, "EVIDENCE", "INITIAL", "COMPLIANCE", 10).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	sum, err := r.PutDeadlineDays(context.Background(), 7, []domain.DeadlineDaysRow{complianceRow(10)}, DeadlineDaysOptions{ApplyToOpenStages: true})
	if err != nil || sum.StagesUpdated != 0 {
		t.Fatalf("sum=%+v err=%v", sum, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// A dry run works everything out and then rolls back: nothing is committed.
func TestPutDeadlineDaysDryRunRollsBack(t *testing.T) {
	r, mock, done := recalcRepo(t)
	defer done()
	mock.ExpectBegin()
	emptyDays(mock)
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO audit_deadline_days")).
		WithArgs(7, "EVIDENCE", "INITIAL", "COMPLIANCE", 5).WillReturnResult(sqlmock.NewResult(0, 1))
	expectOpenStages(mock)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE audit_control_stage SET due_date")).
		WithArgs("2026-11-06", int64(10)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()
	sum, err := r.PutDeadlineDays(context.Background(), 7, []domain.DeadlineDaysRow{complianceRow(5)}, DeadlineDaysOptions{DryRun: true})
	if err != nil || sum.StagesUpdated != 1 || sum.NewlyOverdue != 1 {
		t.Fatalf("sum=%+v err=%v", sum, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
