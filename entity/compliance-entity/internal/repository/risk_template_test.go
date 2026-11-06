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

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

// msFields is a complete, valid set of Managed Services fields.
func msFields() domain.CreateRiskRequest {
	return domain.CreateRiskRequest{
		CustomerID:       intPtr(1),
		DeploymentTypeID: intPtr(2),
		ProductIDs:       []int{3},
		Environments:     []string{"PRODUCTION"},
	}
}

func TestCheckTemplateFields(t *testing.T) {
	cases := []struct {
		name     string
		template string
		req      func() domain.CreateRiskRequest
		wantErr  bool
	}{
		{"standard, nothing extra", TemplateStandard, func() domain.CreateRiskRequest {
			return domain.CreateRiskRequest{ComplianceReferenceIDs: []int{1}}
		}, false},
		{"standard rejects platform", TemplateStandard, func() domain.CreateRiskRequest {
			return domain.CreateRiskRequest{PlatformIDs: []int{1}}
		}, true},
		{"standard rejects customer", TemplateStandard, func() domain.CreateRiskRequest {
			return domain.CreateRiskRequest{CustomerID: intPtr(1)}
		}, true},

		{"aggregated with platforms and compliance refs", TemplateAggregated, func() domain.CreateRiskRequest {
			return domain.CreateRiskRequest{PlatformIDs: []int{1, 2}, ComplianceReferenceIDs: []int{1}}
		}, false},
		{"aggregated requires a platform", TemplateAggregated, func() domain.CreateRiskRequest {
			return domain.CreateRiskRequest{}
		}, true},
		{"aggregated rejects MS fields", TemplateAggregated, func() domain.CreateRiskRequest {
			r := msFields()
			r.PlatformIDs = []int{1}
			return r
		}, true},

		{"managed services complete", TemplateManagedServices, msFields, false},
		{"managed services requires customer", TemplateManagedServices, func() domain.CreateRiskRequest {
			r := msFields()
			r.CustomerID = nil
			return r
		}, true},
		{"managed services requires deployment type", TemplateManagedServices, func() domain.CreateRiskRequest {
			r := msFields()
			r.DeploymentTypeID = nil
			return r
		}, true},
		{"managed services requires a product", TemplateManagedServices, func() domain.CreateRiskRequest {
			r := msFields()
			r.ProductIDs = nil
			return r
		}, true},
		{"managed services requires an environment", TemplateManagedServices, func() domain.CreateRiskRequest {
			r := msFields()
			r.Environments = nil
			return r
		}, true},
		{"managed services rejects compliance refs", TemplateManagedServices, func() domain.CreateRiskRequest {
			r := msFields()
			r.ComplianceReferenceIDs = []int{1}
			return r
		}, true},
		{"managed services rejects platforms", TemplateManagedServices, func() domain.CreateRiskRequest {
			r := msFields()
			r.PlatformIDs = []int{1}
			return r
		}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkTemplateFields(tc.template, tc.req())
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("unexpected err: %v", err)
				}
				return
			}
			var ve *apierror.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("err = %v, want ValidationError", err)
			}
		})
	}
}

// An unknown template is a code/schema mismatch, not a client error: it must
// not come back as a 400 the caller could "fix".
func TestCheckTemplateFields_UnknownTemplateIsInternal(t *testing.T) {
	err := checkTemplateFields("PLATFORM", domain.CreateRiskRequest{})
	var ve *apierror.ValidationError
	if err == nil || errors.As(err, &ve) {
		t.Fatalf("err = %v, want a non-validation error", err)
	}
}

// templateTx opens a sqlmock transaction whose first query reports the risk's
// template and current assignment team.
func templateTx(t *testing.T, template string, team int) (*sql.Tx, sqlmock.Sqlmock) {
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
	mock.ExpectQuery(re("SELECT t.register_template, r.assignment_team_id")).WithArgs(9).
		WillReturnRows(sqlmock.NewRows([]string{"t", "team", "register"}).AddRow(template, team, 3))
	return tx, mock
}

func TestCheckTemplateUpdate_Rejections(t *testing.T) {
	cases := []struct {
		name     string
		template string
		req      domain.UpdateRiskRequest
	}{
		{"MS risk gets compliance refs", TemplateManagedServices, domain.UpdateRiskRequest{ComplianceReferenceIDs: []int{1}}},
		{"aggregated platforms emptied", TemplateAggregated, domain.UpdateRiskRequest{PlatformIDs: []int{}}},
		{"MS environments emptied", TemplateManagedServices, domain.UpdateRiskRequest{Environments: []string{}}},
		{"standard risk gets products", TemplateStandard, domain.UpdateRiskRequest{ProductIDs: []int{1}}},
		{"MS risk gets platforms", TemplateManagedServices, domain.UpdateRiskRequest{PlatformIDs: []int{1}}},
		{"aggregated risk gets deployment type", TemplateAggregated, domain.UpdateRiskRequest{DeploymentTypeID: intPtr(1)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tx, mock := templateTx(t, tc.template, 5)
			err := checkTemplateUpdate(context.Background(), tx, 9, tc.req)
			var ve *apierror.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("err = %v, want ValidationError", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Error(err)
			}
		})
	}
}

// An edit form that re-posts the risk's current assignment team must not be
// refused: only a change of team is checked. sqlmock fails the test if the team
// lookup runs.
func TestCheckTemplateUpdate_UnchangedTeamNotRechecked(t *testing.T) {
	tx, mock := templateTx(t, TemplateManagedServices, 5)
	if err := checkTemplateUpdate(context.Background(), tx, 9,
		domain.UpdateRiskRequest{AssignmentTeamID: intPtr(5)}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// An update that touches nothing template-related reads nothing.
func TestCheckTemplateUpdate_UntouchedSkipsRead(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	mock.ExpectBegin()
	tx, _ := db.Begin()
	title := "renamed"
	if err := checkTemplateUpdate(context.Background(), tx, 9,
		domain.UpdateRiskRequest{RiskTitle: &title, ComplianceReferenceIDs: []int{}}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// A Managed Services risk with no detail row would have its deployment type
// "updated" by a plain UPDATE that changes nothing and reports nothing. The
// check turns that into an error. sqlmock fails the test if the update path runs.
func TestCheckTemplateUpdate_MissingManagedServiceDetailIsAnError(t *testing.T) {
	tx, mock := templateTx(t, TemplateManagedServices, 5)
	mock.ExpectQuery(re("SELECT EXISTS(SELECT 1 FROM risk_managed_service_detail WHERE risk_id = ?)")).WithArgs(9).
		WillReturnRows(sqlmock.NewRows([]string{"e"}).AddRow(false))

	err := checkTemplateUpdate(context.Background(), tx, 9, domain.UpdateRiskRequest{DeploymentTypeID: intPtr(2)})
	var ve *apierror.ValidationError
	if err == nil || errors.As(err, &ve) {
		t.Fatalf("err = %v, want a non-validation (data integrity) error", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// The assignment team is read FOR SHARE so an admin cannot delete it between
// this check and the commit of the risk that references it. A plain SELECT
// would pass the check yet leave that window open, so the lock is part of what
// is asserted: sqlmock matches the query text.
func TestCheckAssignmentTeam_ReadsTheTeamUnderASharedLock(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	mock.ExpectBegin()
	tx, _ := db.Begin()
	mock.ExpectQuery(re("SELECT 1 FROM risk_team WHERE id = ? FOR SHARE")).WithArgs(20).
		WillReturnRows(sqlmock.NewRows([]string{"one"}).AddRow(1))

	if err := checkAssignmentTeam(context.Background(), tx, 20); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestCheckAssignmentTeam_MissingTeamIsAValidationError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	mock.ExpectBegin()
	tx, _ := db.Begin()
	mock.ExpectQuery(re("SELECT 1 FROM risk_team WHERE id = ? FOR SHARE")).WithArgs(20).
		WillReturnRows(sqlmock.NewRows([]string{"one"}))

	err = checkAssignmentTeam(context.Background(), tx, 20)
	var ve *apierror.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v, want ValidationError", err)
	}
}
