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
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

func platformTx(t *testing.T) (*sql.Tx, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	mock.ExpectBegin()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	return tx, mock
}

func platformRows(pairs ...[2]any) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"id", "parent_id"})
	for _, p := range pairs {
		rows.AddRow(p[0], p[1])
	}
	return rows
}

// Integration Platform is 7; Ballerina 9 and Micro Integrator 10 sit under it;
// Core Platform is 4.
func TestCheckRegisterPlatforms(t *testing.T) {
	offeredIntegration := func() *sqlmock.Rows { return platformRows([2]any{7, nil}, [2]any{9, 7}) }
	cases := []struct {
		name    string
		want    []int
		added   []int
		chosen  *sqlmock.Rows        // the chosen platforms with their parents
		offered func() *sqlmock.Rows // no rows: the register offers every platform
		wantErr bool
	}{
		{"no restriction: any platform", []int{4}, []int{4},
			platformRows([2]any{4, nil}), func() *sqlmock.Rows { return platformRows() }, false},
		{"restricted: offered platform with offered sub-platform", []int{7, 9}, []int{7, 9},
			platformRows([2]any{7, nil}, [2]any{9, 7}), offeredIntegration, false},
		{"restricted: platform not offered", []int{4}, []int{4},
			platformRows([2]any{4, nil}), offeredIntegration, true},
		{"restricted: sub-platform not offered", []int{7, 10}, []int{7, 10},
			platformRows([2]any{7, nil}, [2]any{10, 7}), offeredIntegration, true},
		{"restricted: offered sub-platforms are required", []int{7}, []int{7},
			platformRows([2]any{7, nil}), offeredIntegration, true},
		{"restricted: an inactive offered sub-platform is not required", []int{7}, []int{7},
			platformRows([2]any{7, nil}), offeredIntegration, false},
		{"sub-platform without its platform", []int{9}, []int{9},
			platformRows([2]any{9, 7}), nil, true},
		{"unchanged platform no longer offered is not re-checked", []int{4}, nil,
			platformRows([2]any{4, nil}), offeredIntegration, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tx, mock := platformTx(t)
			mock.ExpectQuery(re("FROM risk_platform WHERE id IN")).WillReturnRows(tc.chosen)
			if tc.name != "sub-platform without its platform" {
				mock.ExpectQuery(re("FROM risk_team_platform")).WithArgs(3).WillReturnRows(tc.offered())
				if tc.name != "no restriction: any platform" {
					active := tc.offered()
					if tc.name == "restricted: an inactive offered sub-platform is not required" {
						active = platformRows([2]any{7, nil})
					}
					mock.ExpectQuery(re("p.status = 'ACTIVE'")).WithArgs(3).WillReturnRows(active)
				}
			}
			err := checkRegisterPlatforms(context.Background(), tx, 3, tc.want, tc.added)
			var ve *apierror.ValidationError
			if tc.wantErr && !errors.As(err, &ve) {
				t.Fatalf("err = %v, want ValidationError", err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
		})
	}
}

// A list is only valid on an AGGREGATED register, and nothing is queried or
// written for any other template.
func TestReplaceTeamPlatforms_OnlyAggregated(t *testing.T) {
	tx, mock := platformTx(t)
	err := replaceTeamPlatforms(context.Background(), tx, 14, TemplateStandard, []int{7})
	var ve *apierror.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v, want ValidationError", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestReplaceTeamPlatforms_SubPlatformNeedsItsParent(t *testing.T) {
	tx, mock := platformTx(t)
	mock.ExpectQuery(re("FROM risk_platform WHERE id IN")).WithArgs(9).
		WillReturnRows(sqlmock.NewRows([]string{"id", "parent_id", "status"}).AddRow(9, 7, "ACTIVE"))
	mock.ExpectQuery(re("FROM risk_team_platform")).WithArgs(14).
		WillReturnRows(sqlmock.NewRows([]string{"risk_team_id", "platform_id"}))

	err := replaceTeamPlatforms(context.Background(), tx, 14, TemplateAggregated, []int{9})
	var ve *apierror.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v, want ValidationError", err)
	}
}

// A platform already on the list may stay after it is deactivated, so a form
// that re-posts the whole list still saves; a newly listed inactive one may not.
func TestReplaceTeamPlatforms_InactiveOnlyIfAlreadyListed(t *testing.T) {
	run := func(listed bool) error {
		tx, mock := platformTx(t)
		mock.ExpectQuery(re("FROM risk_platform WHERE id IN")).WithArgs(4).
			WillReturnRows(sqlmock.NewRows([]string{"id", "parent_id", "status"}).AddRow(4, nil, "INACTIVE"))
		current := sqlmock.NewRows([]string{"risk_team_id", "platform_id"})
		if listed {
			current.AddRow(14, 4)
		}
		mock.ExpectQuery(re("FROM risk_team_platform")).WithArgs(14).WillReturnRows(current)
		if listed {
			mock.ExpectExec(re("DELETE FROM risk_team_platform")).WithArgs(14).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec(re("INSERT INTO risk_team_platform")).WithArgs(14, 4).WillReturnResult(sqlmock.NewResult(0, 1))
		}
		return replaceTeamPlatforms(context.Background(), tx, 14, TemplateAggregated, []int{4})
	}
	if err := run(true); err != nil {
		t.Errorf("already listed: unexpected err %v", err)
	}
	var ve *apierror.ValidationError
	if err := run(false); !errors.As(err, &ve) {
		t.Errorf("newly listed: err = %v, want ValidationError", err)
	}
}

// One level only, under an active top-level parent.
func TestCreateRiskLookup_SubPlatformRules(t *testing.T) {
	cases := []struct {
		name    string
		parent  func(*sqlmock.Rows) *sqlmock.Rows
		wantErr bool
	}{
		{"ok", func(r *sqlmock.Rows) *sqlmock.Rows { return r.AddRow(nil, "ACTIVE") }, false},
		{"parent is itself a sub-platform", func(r *sqlmock.Rows) *sqlmock.Rows { return r.AddRow(7, "ACTIVE") }, true},
		{"parent inactive", func(r *sqlmock.Rows) *sqlmock.Rows { return r.AddRow(nil, "INACTIVE") }, true},
		{"parent missing", func(r *sqlmock.Rows) *sqlmock.Rows { return r }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo, mock := newRiskLookupRepoMock(t, LookupPlatform)
			mock.ExpectBegin()
			mock.ExpectQuery(re("SELECT parent_id, status FROM risk_platform WHERE id = ? FOR SHARE")).WithArgs(7).
				WillReturnRows(tc.parent(sqlmock.NewRows([]string{"parent_id", "status"})))
			if !tc.wantErr {
				mock.ExpectExec(re("INSERT INTO risk_platform (name, parent_id, created_by, updated_by)")).
					WithArgs("Ballerina", 7, "admin", "admin").WillReturnResult(sqlmock.NewResult(9, 1))
				mock.ExpectCommit()
				mock.ExpectQuery(re("FROM risk_platform l WHERE l.id = ?")).WithArgs(9).
					WillReturnRows(sqlmock.NewRows([]string{"id", "name", "code", "parent_id", "status", "in_use"}).
						AddRow(9, "Ballerina", nil, 7, "ACTIVE", false))
			} else {
				mock.ExpectRollback()
			}
			got, err := repo.CreateRiskLookup(context.Background(),
				domain.CreateRiskLookupRequest{Name: "Ballerina", ParentID: intPtr(7), CreatedBy: "admin"})
			if tc.wantErr {
				var ve *apierror.ValidationError
				if !errors.As(err, &ve) {
					t.Fatalf("err = %v, want ValidationError", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("CreateRiskLookup: %v", err)
			}
			if got.ParentID == nil || *got.ParentID != 7 {
				t.Errorf("ParentID = %v, want 7", got.ParentID)
			}
		})
	}
}

func teamRow(id int, template string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "name", "code", "description", "team_type",
		"register_template", "has_risks", "status", "created_at", "updated_at"}).
		AddRow(id, "Integration Platform", "INT", nil, "BOTH", template, false, "ACTIVE", time.Now(), time.Now())
}

// Saving a register writes the team and its platform list in one transaction,
// and the list comes back on the register.
func TestCreateRiskTeam_WritesPlatformListWithTheTeam(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	repo := &riskTeamRepo{db: db}

	mock.ExpectBegin()
	mock.ExpectExec(re("INSERT INTO risk_team")).
		WithArgs("Integration Platform", sqlmock.AnyArg(), sqlmock.AnyArg(), "BOTH", "AGGREGATED", "ACTIVE", "admin", "admin").
		WillReturnResult(sqlmock.NewResult(14, 1))
	mock.ExpectQuery(re("FROM risk_platform WHERE id IN")).WithArgs(7, 9).
		WillReturnRows(sqlmock.NewRows([]string{"id", "parent_id", "status"}).
			AddRow(7, nil, "ACTIVE").AddRow(9, 7, "ACTIVE"))
	mock.ExpectQuery(re("FROM risk_team_platform")).WithArgs(14).
		WillReturnRows(sqlmock.NewRows([]string{"risk_team_id", "platform_id"}))
	mock.ExpectExec(re("DELETE FROM risk_team_platform")).WithArgs(14).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(re("INSERT INTO risk_team_platform")).WithArgs(14, 7).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(re("INSERT INTO risk_team_platform")).WithArgs(14, 9).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(re("FROM risk_team WHERE id = ?")).WithArgs(14).WillReturnRows(teamRow(14, "AGGREGATED"))
	mock.ExpectQuery(re("FROM risk_team_platform")).WithArgs(14).
		WillReturnRows(sqlmock.NewRows([]string{"risk_team_id", "platform_id"}).AddRow(14, 7).AddRow(14, 9))

	got, err := repo.CreateRiskTeam(context.Background(), domain.CreateRiskTeamRequest{
		Name: "Integration Platform", Code: strPtr("INT"), TeamType: "BOTH", RegisterTemplate: "AGGREGATED",
		PlatformIDs: []int{7, 9}, CreatedBy: "admin"})
	if err != nil {
		t.Fatalf("CreateRiskTeam: %v", err)
	}
	if len(got.PlatformIDs) != 2 || got.PlatformIDs[0] != 7 || got.PlatformIDs[1] != 9 {
		t.Errorf("PlatformIDs = %v, want [7 9]", got.PlatformIDs)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// A list on a non-Aggregated register fails the whole create: the team row is
// rolled back, not left behind without its list.
func TestCreateRiskTeam_PlatformListOnStandardRegisterRollsBack(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	repo := &riskTeamRepo{db: db}

	mock.ExpectBegin()
	mock.ExpectExec(re("INSERT INTO risk_team")).WillReturnResult(sqlmock.NewResult(14, 1))
	mock.ExpectRollback()

	_, err = repo.CreateRiskTeam(context.Background(), domain.CreateRiskTeamRequest{
		Name: "Choreo", TeamType: "BOTH", PlatformIDs: []int{7}, CreatedBy: "admin"})
	var ve *apierror.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v, want ValidationError", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestUpdateRiskTeam_PlatformList(t *testing.T) {
	cases := []struct {
		name    string
		current string
		req     domain.UpdateRiskTeamRequest
		expect  func(mock sqlmock.Sqlmock)
		wantErr bool
	}{
		{
			name:    "an absent list leaves the stored one alone",
			current: "AGGREGATED",
			req:     domain.UpdateRiskTeamRequest{UpdatedBy: "admin"},
			expect:  func(sqlmock.Sqlmock) {},
		},
		{
			name:    "an empty list clears it",
			current: "AGGREGATED",
			req:     domain.UpdateRiskTeamRequest{PlatformIDs: []int{}, UpdatedBy: "admin"},
			expect: func(m sqlmock.Sqlmock) {
				m.ExpectExec(re("DELETE FROM risk_team_platform")).WithArgs(5).WillReturnResult(sqlmock.NewResult(0, 2))
			},
		},
		{
			name:    "leaving the Aggregated template drops the list",
			current: "AGGREGATED",
			req:     domain.UpdateRiskTeamRequest{RegisterTemplate: strPtr("STANDARD"), UpdatedBy: "admin"},
			expect: func(m sqlmock.Sqlmock) {
				m.ExpectExec(re("DELETE FROM risk_team_platform")).WithArgs(5).WillReturnResult(sqlmock.NewResult(0, 2))
			},
		},
		{
			name:    "a list on a Standard register is refused",
			current: "STANDARD",
			req:     domain.UpdateRiskTeamRequest{PlatformIDs: []int{7}, UpdatedBy: "admin"},
			expect:  func(sqlmock.Sqlmock) {},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New: %v", err)
			}
			defer db.Close()
			repo := &riskTeamRepo{db: db}

			mock.ExpectBegin()
			mock.ExpectQuery(re("SELECT register_template FROM risk_team WHERE id = ? FOR UPDATE")).WithArgs(5).
				WillReturnRows(sqlmock.NewRows([]string{"register_template"}).AddRow(tc.current))
			// The register row is updated first; every case below passes a field
			// set that produces this statement.
			if tc.req.RegisterTemplate != nil {
				mock.ExpectQuery(re("SELECT EXISTS(SELECT 1 FROM risk WHERE source_register_id = ?)")).
					WithArgs(5).WillReturnRows(sqlmock.NewRows([]string{"e"}).AddRow(false))
				mock.ExpectExec(re("UPDATE risk_team SET register_template = ?, updated_by = ? WHERE id = ?")).
					WithArgs(*tc.req.RegisterTemplate, "admin", 5).WillReturnResult(sqlmock.NewResult(0, 1))
			} else {
				mock.ExpectExec(re("UPDATE risk_team SET updated_by = ? WHERE id = ?")).
					WithArgs("admin", 5).WillReturnResult(sqlmock.NewResult(0, 1))
			}
			tc.expect(mock)
			if tc.wantErr {
				mock.ExpectRollback()
			} else {
				mock.ExpectCommit()
				template := tc.current
				if tc.req.RegisterTemplate != nil {
					template = *tc.req.RegisterTemplate
				}
				mock.ExpectQuery(re("FROM risk_team WHERE id = ?")).WithArgs(5).WillReturnRows(teamRow(5, template))
				mock.ExpectQuery(re("FROM risk_team_platform")).WithArgs(5).
					WillReturnRows(sqlmock.NewRows([]string{"risk_team_id", "platform_id"}))
			}

			_, err = repo.UpdateRiskTeam(context.Background(), 5, tc.req)
			var ve *apierror.ValidationError
			if tc.wantErr && !errors.As(err, &ve) {
				t.Fatalf("err = %v, want ValidationError", err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Error(err)
			}
		})
	}
}
