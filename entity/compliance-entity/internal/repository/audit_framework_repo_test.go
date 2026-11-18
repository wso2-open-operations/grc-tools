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
	"strings"
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

func expectFrameworkRow(mock sqlmock.Sqlmock, id int, status string) {
	mock.ExpectQuery(re("FROM audit_framework WHERE id = ?")).WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "status", "created_at", "updated_at"}).
			AddRow(id, "SOC2", status, time.Now(), time.Now()))
}

func TestGetAuditFrameworkByID_DeletedIsNotFound(t *testing.T) {
	repo, mock := newFrameworkRepoMock(t)
	expectFrameworkRow(mock, 3, "INACTIVE")

	var nf *apierror.NotFoundError
	if _, err := repo.GetAuditFrameworkByID(context.Background(), 3); !errors.As(err, &nf) {
		t.Fatalf("want NotFoundError for a deleted framework, got %v", err)
	}
}

func TestGetAuditFrameworkByID_ArchivedIsReturned(t *testing.T) {
	repo, mock := newFrameworkRepoMock(t)
	expectFrameworkRow(mock, 3, "ARCHIVED")

	f, err := repo.GetAuditFrameworkByID(context.Background(), 3)
	if err != nil || f.Status != "ARCHIVED" {
		t.Fatalf("want the archived framework, got %v, %v", f, err)
	}
}

func TestSearchAuditFrameworks_DefaultExcludesDeleted(t *testing.T) {
	repo, mock := newFrameworkRepoMock(t)
	mock.ExpectQuery(re("SELECT COUNT(*) FROM audit_framework WHERE 1=1 AND status <> ?")).WithArgs("INACTIVE").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(0))
	mock.ExpectQuery(re("FROM audit_framework WHERE 1=1 AND status <> ?")).WithArgs("INACTIVE", 20, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "status", "created_at", "updated_at"}))

	req := domain.SearchAuditFrameworksRequest{Scope: domain.ScopeAll, Pagination: domain.Pagination{Limit: 20}}
	if _, _, err := repo.SearchAuditFrameworks(context.Background(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteAuditFramework_Empty(t *testing.T) {
	repo, mock := newFrameworkRepoMock(t)
	expectFrameworkRow(mock, 3, "ACTIVE")
	mock.ExpectQuery(re("SELECT COUNT(*) FROM audit WHERE framework_id = ? AND status <> 'REMOVED'")).WithArgs(3).
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(0))
	mock.ExpectQuery(re("SELECT COUNT(*) FROM audit_framework_control WHERE framework_id = ?")).WithArgs(3).
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(0))
	mock.ExpectExec(re("UPDATE audit_framework SET status = ?, updated_by = ?")).
		WithArgs("INACTIVE", "a@b.c", 3, "INACTIVE", 3, 3).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.DeleteAuditFramework(context.Background(), 3, "a@b.c"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteAuditFramework_AlreadyDeleted(t *testing.T) {
	repo, mock := newFrameworkRepoMock(t)
	expectFrameworkRow(mock, 3, "INACTIVE")

	var nf *apierror.NotFoundError
	if err := repo.DeleteAuditFramework(context.Background(), 3, "a@b.c"); !errors.As(err, &nf) {
		t.Fatalf("want NotFoundError, got %v", err)
	}
}

func TestDeleteAuditFramework_BlockedByAudits(t *testing.T) {
	repo, mock := newFrameworkRepoMock(t)
	expectFrameworkRow(mock, 3, "ACTIVE")
	mock.ExpectQuery(re("SELECT COUNT(*) FROM audit WHERE framework_id = ? AND status <> 'REMOVED'")).WithArgs(3).
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(2))

	err := repo.DeleteAuditFramework(context.Background(), 3, "a@b.c")
	var ce *apierror.ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("want ConflictError, got %v", err)
	}
	// No UPDATE was expected: sqlmock fails on any unexpected statement.
}

func TestDeleteAuditFramework_BlockedByControls(t *testing.T) {
	repo, mock := newFrameworkRepoMock(t)
	expectFrameworkRow(mock, 3, "ACTIVE")
	mock.ExpectQuery(re("SELECT COUNT(*) FROM audit WHERE framework_id = ? AND status <> 'REMOVED'")).WithArgs(3).
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(0))
	mock.ExpectQuery(re("SELECT COUNT(*) FROM audit_framework_control WHERE framework_id = ?")).WithArgs(3).
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(5))

	var ce *apierror.ConflictError
	if err := repo.DeleteAuditFramework(context.Background(), 3, "a@b.c"); !errors.As(err, &ce) {
		t.Fatalf("want ConflictError, got %v", err)
	}
}

func TestDeleteAuditFramework_RaceLeavesRowUntouched(t *testing.T) {
	repo, mock := newFrameworkRepoMock(t)
	expectFrameworkRow(mock, 3, "ACTIVE")
	mock.ExpectQuery(re("SELECT COUNT(*) FROM audit WHERE framework_id = ? AND status <> 'REMOVED'")).WithArgs(3).
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(0))
	mock.ExpectQuery(re("SELECT COUNT(*) FROM audit_framework_control WHERE framework_id = ?")).WithArgs(3).
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(0))
	// An audit appeared after the counts: the guarded UPDATE matches no row.
	mock.ExpectExec(re("UPDATE audit_framework SET status = ?, updated_by = ?")).
		WillReturnResult(sqlmock.NewResult(0, 0))

	var ce *apierror.ConflictError
	if err := repo.DeleteAuditFramework(context.Background(), 3, "a@b.c"); !errors.As(err, &ce) {
		t.Fatalf("want ConflictError, got %v", err)
	}
}

func TestCreateAuditFramework_RevivesDeletedName(t *testing.T) {
	repo, mock := newFrameworkRepoMock(t)
	mock.ExpectQuery(re("SELECT id, status FROM audit_framework WHERE name = ?")).WithArgs("HIPAA").
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow(7, "INACTIVE"))
	mock.ExpectExec(re("UPDATE audit_framework SET name = ?, status = ?, updated_by = ?")).
		WithArgs("HIPAA", "ACTIVE", "a@b.c", 7, "INACTIVE").
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectFrameworkRow(mock, 7, "ACTIVE")

	f, err := repo.CreateAuditFramework(context.Background(), domain.CreateAuditFrameworkRequest{Name: "HIPAA", CreatedBy: "a@b.c"})
	if err != nil || f.ID != 7 {
		t.Fatalf("want the revived framework 7, got %v, %v", f, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateAuditFramework_LiveDuplicateConflicts(t *testing.T) {
	repo, mock := newFrameworkRepoMock(t)
	mock.ExpectQuery(re("SELECT id, status FROM audit_framework WHERE name = ?")).WithArgs("HIPAA").
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow(7, "ARCHIVED"))

	var ce *apierror.ConflictError
	_, err := repo.CreateAuditFramework(context.Background(), domain.CreateAuditFrameworkRequest{Name: "HIPAA", CreatedBy: "a@b.c"})
	if !errors.As(err, &ce) {
		t.Fatalf("want ConflictError, got %v", err)
	}
	if !strings.Contains(ce.Msg, "archived") || !strings.Contains(ce.Msg, "restore") {
		t.Errorf("an archived holder should point at restore, got %q", ce.Msg)
	}
}

func TestCreateAuditFramework_NewName(t *testing.T) {
	repo, mock := newFrameworkRepoMock(t)
	mock.ExpectQuery(re("SELECT id, status FROM audit_framework WHERE name = ?")).WithArgs("PCI").
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}))
	mock.ExpectExec(re("INSERT INTO audit_framework")).WithArgs("PCI", "ACTIVE", "a@b.c", "a@b.c").
		WillReturnResult(sqlmock.NewResult(11, 1))
	expectFrameworkRow(mock, 11, "ACTIVE")

	if f, err := repo.CreateAuditFramework(context.Background(), domain.CreateAuditFrameworkRequest{Name: "PCI", CreatedBy: "a@b.c"}); err != nil || f.ID != 11 {
		t.Fatalf("want framework 11, got %v, %v", f, err)
	}
}

func TestUpdateAuditFramework_DeletedIsNotFound(t *testing.T) {
	repo, mock := newFrameworkRepoMock(t)
	expectFrameworkRow(mock, 3, "INACTIVE")

	status := "ACTIVE"
	var nf *apierror.NotFoundError
	if _, err := repo.UpdateAuditFramework(context.Background(), 3, domain.UpdateAuditFrameworkRequest{Status: &status, UpdatedBy: "a@b.c"}); !errors.As(err, &nf) {
		t.Fatalf("want NotFoundError, got %v", err)
	}
}

func TestUpdateAuditFramework_DuplicateName(t *testing.T) {
	repo, mock := newFrameworkRepoMock(t)
	name := "ISO 27001"
	expectFrameworkRow(mock, 3, "ACTIVE")
	mock.ExpectQuery(re("SELECT id, status FROM audit_framework WHERE name = ? AND id <> ?")).WithArgs(name, 3).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow(9, "ACTIVE"))

	var ce *apierror.ConflictError
	_, err := repo.UpdateAuditFramework(context.Background(), 3, domain.UpdateAuditFrameworkRequest{Name: &name, UpdatedBy: "a@b.c"})
	if !errors.As(err, &ce) {
		t.Fatalf("want ConflictError, got %v", err)
	}
	if ce.Msg != "a framework with this name already exists" {
		t.Errorf("unexpected message: %q", ce.Msg)
	}
}

// A name held by a framework the user cannot see must say why, and how to move on.
func TestUpdateAuditFramework_NameHeldByHiddenFramework(t *testing.T) {
	for holder, want := range map[string]string{
		"ARCHIVED": "archived",
		"INACTIVE": "deleted framework",
	} {
		repo, mock := newFrameworkRepoMock(t)
		name := "SOC2"
		expectFrameworkRow(mock, 3, "ACTIVE")
		mock.ExpectQuery(re("SELECT id, status FROM audit_framework WHERE name = ? AND id <> ?")).WithArgs(name, 3).
			WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow(9, holder))

		var ce *apierror.ConflictError
		_, err := repo.UpdateAuditFramework(context.Background(), 3, domain.UpdateAuditFrameworkRequest{Name: &name, UpdatedBy: "a@b.c"})
		if !errors.As(err, &ce) || !strings.Contains(ce.Msg, want) {
			t.Errorf("%s holder: want a message mentioning %q, got %v", holder, want, err)
		}
	}
}
