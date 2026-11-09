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
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

// TestQueryAssigneeStatusDistribution_RolesAndArgs: the three role views come
// from four queries (auditor, owner, owner-overdue, compliance), each bound
// with the scope arg then the audit-filter arg, and rows carry the right role,
// the OVERDUE pseudo-status, and the team name as the compliance assignee.
func TestQueryAssigneeStatusDistribution_RolesAndArgs(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	repo := &dashboardRepo{db: db}

	// What Get builds for owned scope (user 9) filtered to audit 4.
	scope, args := repo.scopeWhere(domain.ScopeOwned, 9, nil)
	auditSQL, auditArgs := buildIntInFilter("c.audit_id", []int{4})
	baseWhere := "WHERE a.status = 'ACTIVE'" + scope + auditSQL
	args = append(args, auditArgs...)

	mock.ExpectQuery(re("LEFT JOIN `user` u ON u.id = c.auditor_id")).WithArgs(9, 4).
		WillReturnRows(sqlmock.NewRows([]string{"uuid", "type", "status", "n"}).
			AddRow("u-a", "INTERNAL", "EVIDENCE_UNDER_VALIDATION", 3))
	mock.ExpectQuery(re("LEFT JOIN `user` u ON u.id = c.owner_id")).WithArgs(9, 4).
		WillReturnRows(sqlmock.NewRows([]string{"uuid", "type", "status", "n"}).
			AddRow("u-o", "INTERNAL", "EVIDENCE_PENDING", 5))
	mock.ExpectQuery(re("< CURDATE() AND c.status != 'COMPLETE'")).WithArgs(9, 4).
		WillReturnRows(sqlmock.NewRows([]string{"uuid", "type", "n"}).AddRow("u-o", "INTERNAL", 2))
	mock.ExpectQuery(re("LEFT JOIN audit_team t ON t.id = c.team_id")).WithArgs(9, 4).
		WillReturnRows(sqlmock.NewRows([]string{"team", "status", "n"}).
			AddRow("Platform", "EVIDENCE_INTERNAL_REVIEW", 4))

	got, err := repo.queryAssigneeStatusDistribution(context.Background(), baseWhere, args)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	want := []domain.AssigneeStatusCount{
		{Role: "auditor", AssigneeUUID: "u-a", AssigneeUserType: "INTERNAL", Status: "EVIDENCE_UNDER_VALIDATION", Count: 3},
		{Role: "owner", AssigneeUUID: "u-o", AssigneeUserType: "INTERNAL", Status: "EVIDENCE_PENDING", Count: 5},
		{Role: "owner", AssigneeUUID: "u-o", AssigneeUserType: "INTERNAL", Status: "OVERDUE", Count: 2},
		{Role: "compliance", Assignee: "Platform", Status: "EVIDENCE_INTERNAL_REVIEW", Count: 4},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// TestQueryAssigneeStatusDistribution_AuditorIncludesPopulationComplete: the
// auditor view's SQL filter must list POPULATION_COMPLETE so the Auditor Team
// card can show those controls, and the row passes through with its status.
func TestQueryAssigneeStatusDistribution_AuditorIncludesPopulationComplete(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	repo := &dashboardRepo{db: db}

	mock.ExpectQuery(re("c.status IN ('EVIDENCE_UNDER_VALIDATION','POPULATION_UNDER_VALIDATION','POPULATION_COMPLETE')")).
		WillReturnRows(sqlmock.NewRows([]string{"uuid", "type", "status", "n"}).
			AddRow("u-a", "INTERNAL", "POPULATION_COMPLETE", 2))
	mock.ExpectQuery(re("LEFT JOIN `user` u ON u.id = c.owner_id")).
		WillReturnRows(sqlmock.NewRows([]string{"uuid", "type", "status", "n"}))
	mock.ExpectQuery(re("< CURDATE() AND c.status != 'COMPLETE'")).
		WillReturnRows(sqlmock.NewRows([]string{"uuid", "type", "n"}))
	mock.ExpectQuery(re("LEFT JOIN audit_team t ON t.id = c.team_id")).
		WillReturnRows(sqlmock.NewRows([]string{"team", "status", "n"}))

	got, err := repo.queryAssigneeStatusDistribution(context.Background(), "WHERE a.status = 'ACTIVE'", nil)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	want := domain.AssigneeStatusCount{Role: "auditor", AssigneeUUID: "u-a", AssigneeUserType: "INTERNAL", Status: "POPULATION_COMPLETE", Count: 2}
	if len(got) != 1 || got[0] != want {
		t.Errorf("got %+v, want [%+v]", got, want)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
