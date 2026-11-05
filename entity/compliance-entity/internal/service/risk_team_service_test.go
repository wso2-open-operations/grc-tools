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

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/repository"
)

// fakeTeamRepo records whether a write reached the repository. Validation
// failures must be caught before that.
type fakeTeamRepo struct {
	repository.RiskTeamRepository
	created *domain.CreateRiskTeamRequest
	updated *domain.UpdateRiskTeamRequest
}

func (f *fakeTeamRepo) CreateRiskTeam(_ context.Context, req domain.CreateRiskTeamRequest) (*domain.RiskTeam, error) {
	f.created = &req
	return &domain.RiskTeam{ID: 1, Name: req.Name, PlatformIDs: req.PlatformIDs}, nil
}

func (f *fakeTeamRepo) UpdateRiskTeam(_ context.Context, id int, req domain.UpdateRiskTeamRequest) (*domain.RiskTeam, error) {
	f.updated = &req
	return &domain.RiskTeam{ID: id, PlatformIDs: req.PlatformIDs}, nil
}

func TestCheckPlatformIDs(t *testing.T) {
	cases := []struct {
		name    string
		ids     []int
		wantErr bool
	}{
		{"nil", nil, false},
		{"empty", []int{}, false},
		{"valid", []int{7, 9, 10}, false},
		{"zero", []int{7, 0}, true},
		{"negative", []int{-1}, true},
		{"repeated", []int{7, 9, 7}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkPlatformIDs(tc.ids)
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

// A bad platform list is a 400 before anything is written, on create and update.
func TestRiskTeamService_RejectsBadPlatformIDsBeforeWriting(t *testing.T) {
	repo := &fakeTeamRepo{}
	svc := NewRiskTeamService(repo)
	var ve *apierror.ValidationError

	_, err := svc.CreateRiskTeam(context.Background(), domain.CreateRiskTeamRequest{
		Name: "Integration Platform", TeamType: "BOTH", RegisterTemplate: "AGGREGATED",
		PlatformIDs: []int{7, 7}, CreatedBy: "admin"})
	if !errors.As(err, &ve) {
		t.Fatalf("create err = %v, want ValidationError", err)
	}
	if repo.created != nil {
		t.Error("an invalid create reached the repository")
	}

	_, err = svc.UpdateRiskTeam(context.Background(), 5, domain.UpdateRiskTeamRequest{
		PlatformIDs: []int{-3}, UpdatedBy: "admin"})
	if !errors.As(err, &ve) {
		t.Fatalf("update err = %v, want ValidationError", err)
	}
	if repo.updated != nil {
		t.Error("an invalid update reached the repository")
	}
}

// An absent list and an empty one mean different things on update: absent
// leaves the stored list alone, empty clears it. Both must reach the
// repository exactly as sent.
func TestRiskTeamService_UpdatePassesAbsentAndEmptyListsThrough(t *testing.T) {
	repo := &fakeTeamRepo{}
	svc := NewRiskTeamService(repo)

	if _, err := svc.UpdateRiskTeam(context.Background(), 5, domain.UpdateRiskTeamRequest{UpdatedBy: "admin"}); err != nil {
		t.Fatalf("absent: %v", err)
	}
	if repo.updated.PlatformIDs != nil {
		t.Errorf("an absent list reached the repository as %v", repo.updated.PlatformIDs)
	}

	if _, err := svc.UpdateRiskTeam(context.Background(), 5,
		domain.UpdateRiskTeamRequest{PlatformIDs: []int{}, UpdatedBy: "admin"}); err != nil {
		t.Fatalf("empty: %v", err)
	}
	if repo.updated.PlatformIDs == nil || len(repo.updated.PlatformIDs) != 0 {
		t.Errorf("an empty list reached the repository as %#v, want empty non-nil", repo.updated.PlatformIDs)
	}
}

func TestRiskLookupService_ParentOnlyForPlatforms(t *testing.T) {
	parent := 7
	for _, tc := range []struct {
		name    string
		kind    repository.LookupKind
		parent  *int
		wantErr bool
	}{
		{"sub-platform", repository.LookupPlatform, &parent, false},
		{"top-level platform", repository.LookupPlatform, nil, false},
		{"product with a parent", repository.LookupProduct, &parent, true},
		{"non-positive parent", repository.LookupPlatform, intPtrT(0), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeLookupRepo{}
			svc := NewRiskLookupService(repo, tc.kind)
			_, err := svc.CreateRiskLookup(context.Background(),
				domain.CreateRiskLookupRequest{Name: "Ballerina", ParentID: tc.parent, CreatedBy: "admin"})
			var ve *apierror.ValidationError
			if tc.wantErr {
				if !errors.As(err, &ve) {
					t.Fatalf("err = %v, want ValidationError", err)
				}
				if repo.created != nil {
					t.Error("invalid input reached the repository")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if repo.created.ParentID != tc.parent {
				t.Errorf("ParentID = %v, want %v", repo.created.ParentID, tc.parent)
			}
		})
	}
}

func intPtrT(v int) *int { return &v }
