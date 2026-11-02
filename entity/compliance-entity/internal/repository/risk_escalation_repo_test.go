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
	"errors"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

var escalationCols = []string{
	"id", "risk_id", "new_treatment_strategy", "action_plan_id", "decision",
	"assigner_lead_uuid", "action_owner_lead_uuid", "status", "created_by", "updated_by",
	"created_at", "updated_at",
}

func newEscalationRepoMock(t *testing.T) (*riskEscalationRepo, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return &riskEscalationRepo{db: db}, mock
}

const resolveAnsweredSQL = "UPDATE risk_escalation SET status = 'RESOLVED'"

// A manual re-escalation resolves the already-commented OPEN escalation, then
// flips the risk and inserts the new one, all in one transaction.
func TestEscalate_Reescalate_ResolvesAnsweredThenEscalates(t *testing.T) {
	r, mock := newEscalationRepoMock(t)
	mock.ExpectBegin()
	mock.ExpectExec(re(resolveAnsweredSQL)).WithArgs("by-1", 63).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(re("SELECT COUNT(*) FROM risk_escalation WHERE risk_id = ? AND status = 'OPEN'")).
		WithArgs(63).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))
	mock.ExpectExec(re("UPDATE risk SET workflow_status = 'ESCALATED'")).
		WithArgs("by-1", 63).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(re("INSERT INTO risk_escalation")).WillReturnResult(sqlmock.NewResult(27, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(re("FROM risk_escalation WHERE id = ? AND risk_id = ?")).
		WithArgs(27, 63).
		WillReturnRows(sqlmock.NewRows(escalationCols).
			AddRow(27, 63, nil, nil, nil, nil, nil, "OPEN", "by-1", "by-1", time.Now(), time.Now()))

	e, err := r.Escalate(context.Background(), 63, domain.CreateRiskEscalationRequest{CreatedBy: "by-1", Reescalate: true})
	if err != nil {
		t.Fatalf("Escalate: %v", err)
	}
	if e.ID != 27 || e.Status != "OPEN" {
		t.Errorf("got escalation %+v, want new OPEN #27", e)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// Still-unanswered OPEN escalation: the resolve step skips it, the guard
// fires, and the transaction rolls back without touching the risk.
func TestEscalate_Reescalate_UnansweredStillConflicts(t *testing.T) {
	r, mock := newEscalationRepoMock(t)
	mock.ExpectBegin()
	mock.ExpectExec(re(resolveAnsweredSQL)).WithArgs("by-1", 63).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(re("SELECT COUNT(*) FROM risk_escalation")).
		WithArgs(63).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(1))
	mock.ExpectRollback()

	_, err := r.Escalate(context.Background(), 63, domain.CreateRiskEscalationRequest{CreatedBy: "by-1", Reescalate: true})
	var conflict *apierror.ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("err = %v, want ConflictError", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// Without Reescalate (the daily job) nothing is resolved: an open escalation,
// answered or not, still blocks.
func TestEscalate_NoReescalate_DoesNotResolveAnswered(t *testing.T) {
	r, mock := newEscalationRepoMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery(re("SELECT COUNT(*) FROM risk_escalation")).
		WithArgs(63).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(1))
	mock.ExpectRollback()

	_, err := r.Escalate(context.Background(), 63, domain.CreateRiskEscalationRequest{CreatedBy: "job"})
	var conflict *apierror.ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("err = %v, want ConflictError", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
