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

func newRiskLookupRepoMock(t *testing.T, kind LookupKind) (*riskLookupRepo, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return &riskLookupRepo{db: db, kind: kind}, mock
}

// TestUpdateRiskLookup_CustomerCodeFrozenWhenInUse: once any risk uses a
// customer, its code is part of issued risk codes, so changing it is a 409
// and nothing is written.
func TestUpdateRiskLookup_CustomerCodeFrozenWhenInUse(t *testing.T) {
	repo, mock := newRiskLookupRepoMock(t, LookupCustomer)

	mock.ExpectBegin()
	mock.ExpectQuery(re("FROM risk_customer l WHERE l.id = ? FOR UPDATE")).
		WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"in_use"}).AddRow(true))
	mock.ExpectQuery(re("SELECT code FROM risk_customer WHERE id = ?")).
		WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"code"}).AddRow("BANKONESUB"))
	mock.ExpectRollback()

	_, err := repo.UpdateRiskLookup(context.Background(), 7,
		domain.UpdateRiskLookupRequest{Code: strPtr("BANKONE"), UpdatedBy: "admin"})

	var conflict *apierror.ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("err = %v, want ConflictError", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// TestUpdateRiskLookup_SameCodeAllowedWhenInUse: a form that re-posts every
// field sends the unchanged code; that must not trip the freeze, and the
// UPDATE must not touch code.
func TestUpdateRiskLookup_SameCodeAllowedWhenInUse(t *testing.T) {
	repo, mock := newRiskLookupRepoMock(t, LookupCustomer)

	mock.ExpectBegin()
	mock.ExpectQuery(re("FOR UPDATE")).WithArgs(7).
		WillReturnRows(sqlmock.NewRows([]string{"in_use"}).AddRow(true))
	mock.ExpectQuery(re("SELECT code FROM risk_customer WHERE id = ?")).WithArgs(7).
		WillReturnRows(sqlmock.NewRows([]string{"code"}).AddRow("BANKONESUB"))
	mock.ExpectExec(re("UPDATE risk_customer SET name = ?, updated_by = ? WHERE id = ?")).
		WithArgs("Bank One Subsidiary", "admin", 7).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(re("FROM risk_customer l WHERE l.id = ?")).WithArgs(7).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "code", "parent_id", "status", "in_use"}).
			AddRow(7, "Bank One Subsidiary", "BANKONESUB", nil, "ACTIVE", true))

	got, err := repo.UpdateRiskLookup(context.Background(), 7, domain.UpdateRiskLookupRequest{
		Name: strPtr("Bank One Subsidiary"), Code: strPtr("BANKONESUB"), UpdatedBy: "admin"})
	if err != nil {
		t.Fatalf("UpdateRiskLookup: %v", err)
	}
	if got.Name != "Bank One Subsidiary" || got.Code == nil || *got.Code != "BANKONESUB" || !got.InUse {
		t.Errorf("got %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// TestDeleteRiskLookup_InUseRefused: a used value is refused with a 409 and
// the DELETE never runs.
func TestDeleteRiskLookup_InUseRefused(t *testing.T) {
	repo, mock := newRiskLookupRepoMock(t, LookupProduct)

	mock.ExpectBegin()
	mock.ExpectQuery(re("FROM risk_product l WHERE l.id = ? FOR UPDATE")).WithArgs(3).
		WillReturnRows(sqlmock.NewRows([]string{"in_use"}).AddRow(true))
	mock.ExpectRollback()

	err := repo.DeleteRiskLookup(context.Background(), 3)
	var conflict *apierror.ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("err = %v, want ConflictError", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// TestDeleteRiskLookup_Unused deletes a value nothing references.
func TestDeleteRiskLookup_Unused(t *testing.T) {
	repo, mock := newRiskLookupRepoMock(t, LookupPlatform)

	mock.ExpectBegin()
	mock.ExpectQuery(re("FROM risk_platform l WHERE l.id = ? FOR UPDATE")).WithArgs(4).
		WillReturnRows(sqlmock.NewRows([]string{"in_use"}).AddRow(false))
	mock.ExpectExec(re("DELETE FROM risk_platform WHERE id = ?")).WithArgs(4).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := repo.DeleteRiskLookup(context.Background(), 4); err != nil {
		t.Fatalf("DeleteRiskLookup: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// TestDeleteRiskLookup_NotFound maps a missing row to a 404.
func TestDeleteRiskLookup_NotFound(t *testing.T) {
	repo, mock := newRiskLookupRepoMock(t, LookupDeploymentType)

	mock.ExpectBegin()
	mock.ExpectQuery(re("FOR UPDATE")).WithArgs(99).
		WillReturnRows(sqlmock.NewRows([]string{"in_use"}))
	mock.ExpectRollback()

	err := repo.DeleteRiskLookup(context.Background(), 99)
	var nf *apierror.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("err = %v, want NotFoundError", err)
	}
}

// TestRiskTeamUpdate_TemplateLockedOnceTeamHasRisks: changing a team's
// template once any risk uses it as the source register is a 409 and nothing
// is written. A risk routed to the team as its assignment team doesn't count.
func TestRiskTeamUpdate_TemplateLockedOnceTeamHasRisks(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	repo := &riskTeamRepo{db: db}

	mock.ExpectBegin()
	mock.ExpectQuery(re("SELECT register_template FROM risk_team WHERE id = ? FOR UPDATE")).WithArgs(5).
		WillReturnRows(sqlmock.NewRows([]string{"register_template"}).AddRow("STANDARD"))
	mock.ExpectQuery(re("SELECT EXISTS(SELECT 1 FROM risk WHERE source_register_id = ?)")).
		WithArgs(5).WillReturnRows(sqlmock.NewRows([]string{"e"}).AddRow(true))
	mock.ExpectRollback()

	_, err = repo.UpdateRiskTeam(context.Background(), 5, domain.UpdateRiskTeamRequest{
		RegisterTemplate: strPtr("MANAGED_SERVICES"), UpdatedBy: "admin"})
	var conflict *apierror.ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("err = %v, want ConflictError", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// TestRiskTeamUpdate_SameTemplateSkipsLockCheck: re-sending the current
// template is not a change, so a register with risks can still be renamed
// by a form that posts every field.
func TestRiskTeamUpdate_SameTemplateSkipsLockCheck(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	repo := &riskTeamRepo{db: db}

	mock.ExpectBegin()
	mock.ExpectQuery(re("FOR UPDATE")).WithArgs(5).
		WillReturnRows(sqlmock.NewRows([]string{"register_template"}).AddRow("STANDARD"))
	mock.ExpectExec(re("UPDATE risk_team SET name = ?, updated_by = ? WHERE id = ?")).
		WithArgs("Asgardeo IAM", "admin", 5).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(re("FROM risk_team WHERE id = ?")).WithArgs(5).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "code", "description", "team_type",
			"register_template", "has_risks", "status", "created_at", "updated_at"}).
			AddRow(5, "Asgardeo IAM", "ASG", nil, "BOTH", "STANDARD", true, "ACTIVE", time.Now(), time.Now()))
	mock.ExpectQuery(re("FROM risk_team_platform")).WithArgs(5).
		WillReturnRows(sqlmock.NewRows([]string{"risk_team_id", "platform_id"}))

	got, err := repo.UpdateRiskTeam(context.Background(), 5, domain.UpdateRiskTeamRequest{
		Name: strPtr("Asgardeo IAM"), RegisterTemplate: strPtr("STANDARD"), UpdatedBy: "admin"})
	if err != nil {
		t.Fatalf("UpdateRiskTeam: %v", err)
	}
	// HasRisks is what the Admin Console uses to disable the Template select.
	if !got.HasRisks {
		t.Error("HasRisks not read back from the team row")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
