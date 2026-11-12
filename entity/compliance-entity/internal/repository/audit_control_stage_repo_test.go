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
		WillReturnRows(sqlmock.NewRows([]string{"cycle_type", "handled_by", "allowed_days"}))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO audit_control_stage")).
		WithArgs(42, "OWNER", "INITIAL", stageNow(), "2026-11-07", "2026-11-17", "2026-11-27").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, _ := db.Begin()
	if err := openFirstStage(context.Background(), tx, 7, 42, "2026-11-07", stageNow()); err != nil {
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
		WillReturnRows(sqlmock.NewRows([]string{"cycle_type", "handled_by", "allowed_days"}).
			AddRow("INITIAL", "COMPLIANCE", 12).AddRow("INITIAL", "AUDITOR", 5))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO audit_control_stage")).
		WithArgs(42, "OWNER", "INITIAL", stageNow(), "2026-11-07", "2026-11-19", "2026-11-24").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, _ := db.Begin()
	if err := openFirstStage(context.Background(), tx, 7, 42, "2026-11-07", stageNow()); err != nil {
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
		WillReturnRows(sqlmock.NewRows([]string{"id", "handled_by", "cycle_type", "ended_at"}).AddRow(5, "OWNER", "INITIAL", nil))
	mock.ExpectQuery(regexp.QuoteMeta("FROM audit_deadline_days")).
		WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"cycle_type", "handled_by", "allowed_days"}))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE audit_control_stage SET ended_at")).
		WithArgs(stageNow(), "owner@wso2.com", int64(5)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO audit_control_stage")).
		WithArgs(42, "COMPLIANCE", "INITIAL", stageNow(), "2026-11-20").WillReturnResult(sqlmock.NewResult(6, 1))
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

func TestApplyStageTransitionSkipsOEControls(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT requirement_type FROM audit_control")).
		WithArgs(42).WillReturnRows(sqlmock.NewRows([]string{"requirement_type"}).AddRow("OE"))
	mock.ExpectCommit()

	tx, _ := db.Begin()
	if err := applyStageTransition(context.Background(), tx, 7, 42,
		"EVIDENCE_PENDING", "EVIDENCE_INTERNAL_REVIEW", "x", false, stageNow()); err != nil {
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
		WithArgs(42).WillReturnRows(sqlmock.NewRows([]string{"id", "handled_by", "cycle_type", "ended_at"}))
	mock.ExpectQuery(regexp.QuoteMeta("FROM audit_deadline_days")).
		WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"cycle_type", "handled_by", "allowed_days"}))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO audit_control_stage")).
		WithArgs(42, "AUDITOR", "INITIAL", stageNow(), "2026-11-20").WillReturnResult(sqlmock.NewResult(1, 1))
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
		WillReturnRows(sqlmock.NewRows([]string{"id", "handled_by", "cycle_type", "ended_at"}).AddRow(9, "AUDITOR", "INITIAL", nil))
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
	if !regexp.MustCompile(`EXISTS \(SELECT 1 FROM audit_control_stage st WHERE st.control_id = c.id AND st.phase = 'EVIDENCE' AND st.ended_at IS NULL AND st.handled_by IN \(\?,\?\)\)`).MatchString(where) {
		t.Fatalf("where = %s", where)
	}
	if len(args) < 3 || args[1] != "OWNER" || args[2] != "AUDITOR" {
		t.Fatalf("args = %v", args)
	}
}
