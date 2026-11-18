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
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package service

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
)

// stubFrameworkRepo serves one framework and records the last Update/Delete.
type stubFrameworkRepo struct {
	repository.FrameworkRepository
	fw        *model.AuditFramework
	updated   *model.UpdateFrameworkRequest
	deleted   bool
	deletedBy string
}

func (s *stubFrameworkRepo) GetByID(_ context.Context, id int) (*model.AuditFramework, error) {
	if s.fw != nil && s.fw.ID == id {
		return s.fw, nil
	}
	return nil, nil
}
func (s *stubFrameworkRepo) Update(_ context.Context, _ int, req model.UpdateFrameworkRequest, _ string) (*model.AuditFramework, error) {
	s.updated = &req
	return s.fw, nil
}
func (s *stubFrameworkRepo) Delete(_ context.Context, _ int, deletedBy string) error {
	s.deleted, s.deletedBy = true, deletedBy
	return nil
}

func newFrameworkSvc() (FrameworkService, *stubFrameworkRepo) {
	repo := &stubFrameworkRepo{fw: &model.AuditFramework{ID: 1, Name: "SOC2", Status: "ACTIVE"}}
	return NewFrameworkService(repo, nil, nil), repo
}

func TestUpdateFramework_Validation(t *testing.T) {
	svc, repo := newFrameworkSvc()
	blank, bad := "   ", "INACTIVE"
	cases := map[string]model.UpdateFrameworkRequest{
		"empty request": {},
		"blank name":    {Name: &blank},
		"bad status":    {Status: &bad},
	}
	for name, req := range cases {
		_, err := svc.UpdateFramework(context.Background(), 1, req, "a@b.c")
		if got := statusOf(err); got != http.StatusUnprocessableEntity {
			t.Errorf("%s: status = %d, want 422", name, got)
		}
	}
	if repo.updated != nil {
		t.Error("repo.Update must not run for an invalid request")
	}
}

func TestUpdateFramework_TrimsNameAndArchives(t *testing.T) {
	svc, repo := newFrameworkSvc()
	name, status := "  SOC 2  ", "ARCHIVED"
	if _, err := svc.UpdateFramework(context.Background(), 1, model.UpdateFrameworkRequest{Name: &name, Status: &status}, "a@b.c"); err != nil {
		t.Fatal(err)
	}
	if repo.updated == nil || *repo.updated.Name != "SOC 2" || *repo.updated.Status != "ARCHIVED" {
		t.Errorf("unexpected update: %+v", repo.updated)
	}
}

func TestUpdateAndDeleteFramework_NotFound(t *testing.T) {
	svc, repo := newFrameworkSvc()
	status := "ARCHIVED"
	_, err := svc.UpdateFramework(context.Background(), 99, model.UpdateFrameworkRequest{Status: &status}, "a@b.c")
	if got := statusOf(err); got != http.StatusNotFound {
		t.Errorf("update status = %d, want 404", got)
	}
	if got := statusOf(svc.DeleteFramework(context.Background(), 99, "a@b.c")); got != http.StatusNotFound {
		t.Errorf("delete status = %d, want 404", got)
	}
	if repo.deleted {
		t.Error("repo.Delete must not run for an unknown framework")
	}
}

func TestCreateAudit_RejectsArchivedFramework(t *testing.T) {
	fwRepo := &stubFrameworkRepo{fw: &model.AuditFramework{ID: 1, Name: "SOC2", Status: "ARCHIVED"}}
	svc := NewAuditService(&stubAuditRepo{}, fwRepo, nil, nil)
	_, err := svc.Create(context.Background(), model.CreateAuditRequest{
		FrameworkID: 1, ProductID: 1, PeriodStart: "2026-01-01", PeriodEnd: "2026-12-31",
	}, "a@b.c")
	if got := statusOf(err); got != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", got)
	}
}

func TestDeleteFramework_PassesActorAndRequiresOne(t *testing.T) {
	svc, repo := newFrameworkSvc()
	if got := statusOf(svc.DeleteFramework(context.Background(), 1, "")); got != http.StatusUnprocessableEntity {
		t.Errorf("missing actor: status = %d, want 422", got)
	}
	if repo.deleted {
		t.Error("repo.Delete must not run without an acting user")
	}
	if err := svc.DeleteFramework(context.Background(), 1, "a@b.c"); err != nil {
		t.Fatal(err)
	}
	if !repo.deleted || repo.deletedBy != "a@b.c" {
		t.Errorf("delete not recorded with the actor: %+v", repo)
	}
}

func TestFrameworkName_LengthCountsCharactersNotBytes(t *testing.T) {
	svc, repo := newFrameworkSvc()
	// 100 two-byte characters: 200 bytes, but exactly the column's 100 characters.
	fits := strings.Repeat("é", 100)
	if _, err := svc.UpdateFramework(context.Background(), 1, model.UpdateFrameworkRequest{Name: &fits}, "a@b.c"); err != nil {
		t.Fatalf("a 100-character name must be accepted: %v", err)
	}
	if repo.updated == nil || *repo.updated.Name != fits {
		t.Errorf("name not passed through: %+v", repo.updated)
	}
	tooLong := strings.Repeat("é", 101)
	if got := statusOf(func() error {
		_, err := svc.UpdateFramework(context.Background(), 1, model.UpdateFrameworkRequest{Name: &tooLong}, "a@b.c")
		return err
	}()); got != http.StatusUnprocessableEntity {
		t.Errorf("101 characters: status = %d, want 422", got)
	}
}

func TestCreateFramework_ValidatesName(t *testing.T) {
	svc, _ := newFrameworkSvc()
	for name, in := range map[string]string{"blank": "   ", "too long": strings.Repeat("a", 101)} {
		_, err := svc.CreateFramework(context.Background(), model.CreateFrameworkRequest{Name: in}, "a@b.c")
		if got := statusOf(err); got != http.StatusUnprocessableEntity {
			t.Errorf("%s: status = %d, want 422", name, got)
		}
	}
}
