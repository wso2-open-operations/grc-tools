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

func newFrameworkRepoMock(t *testing.T) (*auditFrameworkRepo, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return &auditFrameworkRepo{db: db}, mock
}

func expectFrameworkRow(mock sqlmock.Sqlmock, id int) {
	mock.ExpectQuery(re("FROM audit_framework WHERE id = ?")).WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "status", "created_at", "updated_at"}).
			AddRow(id, "SOC2", "ACTIVE", time.Now(), time.Now()))
}

func TestDeleteAuditFramework_Empty(t *testing.T) {
	repo, mock := newFrameworkRepoMock(t)
	expectFrameworkRow(mock, 3)
	mock.ExpectQuery(re("SELECT COUNT(*) FROM audit WHERE framework_id = ?")).WithArgs(3).
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(0))
	mock.ExpectQuery(re("SELECT COUNT(*) FROM audit_framework_control WHERE framework_id = ?")).WithArgs(3).
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(0))
	mock.ExpectExec(re("DELETE FROM audit_framework WHERE id = ?")).WithArgs(3).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.DeleteAuditFramework(context.Background(), 3); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteAuditFramework_BlockedByAudits(t *testing.T) {
	repo, mock := newFrameworkRepoMock(t)
	expectFrameworkRow(mock, 3)
	mock.ExpectQuery(re("SELECT COUNT(*) FROM audit WHERE framework_id = ?")).WithArgs(3).
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(2))

	err := repo.DeleteAuditFramework(context.Background(), 3)
	var ce *apierror.ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("want ConflictError, got %v", err)
	}
	// No DELETE was expected: sqlmock fails on any unexpected statement.
}

func TestDeleteAuditFramework_BlockedByControls(t *testing.T) {
	repo, mock := newFrameworkRepoMock(t)
	expectFrameworkRow(mock, 3)
	mock.ExpectQuery(re("SELECT COUNT(*) FROM audit WHERE framework_id = ?")).WithArgs(3).
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(0))
	mock.ExpectQuery(re("SELECT COUNT(*) FROM audit_framework_control WHERE framework_id = ?")).WithArgs(3).
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(5))

	var ce *apierror.ConflictError
	if err := repo.DeleteAuditFramework(context.Background(), 3); !errors.As(err, &ce) {
		t.Fatalf("want ConflictError, got %v", err)
	}
}

func TestUpdateAuditFramework_DuplicateName(t *testing.T) {
	repo, mock := newFrameworkRepoMock(t)
	name := "ISO 27001"
	mock.ExpectQuery(re("SELECT id FROM audit_framework WHERE name = ? AND id <> ?")).WithArgs(name, 3).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(9))

	var ce *apierror.ConflictError
	_, err := repo.UpdateAuditFramework(context.Background(), 3, domain.UpdateAuditFrameworkRequest{Name: &name, UpdatedBy: "a@b.c"})
	if !errors.As(err, &ce) {
		t.Fatalf("want ConflictError, got %v", err)
	}
}
