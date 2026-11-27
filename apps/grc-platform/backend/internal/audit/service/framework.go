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
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
)

// maxFrameworkNameLength mirrors audit_framework.name VARCHAR(100), which counts
// characters, not bytes.
const maxFrameworkNameLength = 100

// validateFrameworkName trims name and checks it is present and fits the column.
func validateFrameworkName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "name is required"}
	}
	if utf8.RuneCountInString(name) > maxFrameworkNameLength {
		return "", &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "name must be 100 characters or fewer"}
	}
	return name, nil
}

// FrameworkService defines business operations for audit frameworks, products,
// and the versioned framework control library.
type FrameworkService interface {
	// ListFrameworks returns frameworks visible to the caller. status is
	// "ACTIVE" (what dropdowns want), "ARCHIVED", or "" for every framework that
	// has not been deleted.
	ListFrameworks(ctx context.Context, scope model.Scope, userID int, scopeTeamIDs []int, status string) ([]*model.AuditFramework, error)
	CreateFramework(ctx context.Context, req model.CreateFrameworkRequest, createdBy string) (*model.AuditFramework, error)
	// UpdateFramework renames and/or archives (status ARCHIVED) / restores
	// (status ACTIVE) a framework. Existing audits are never touched.
	UpdateFramework(ctx context.Context, id int, req model.UpdateFrameworkRequest, updatedBy string) (*model.AuditFramework, error)
	// DeleteFramework soft-deletes (status INACTIVE) a framework that has no
	// audits and no controls; otherwise the data service answers 409 and the
	// caller should archive.
	DeleteFramework(ctx context.Context, id int, deletedBy string) error
	ListProducts(ctx context.Context, scope model.Scope, userID int, scopeTeamIDs []int) ([]*model.AuditProduct, error)
	CreateProduct(ctx context.Context, req model.CreateProductRequest, createdBy string) (*model.AuditProduct, error)
	// FrameworkInScope reports whether frameworkID has at least one audit
	// visible to the caller — used to gate ListFrameworkControls.
	FrameworkInScope(ctx context.Context, frameworkID int, scope model.Scope, userID int, scopeTeamIDs []int) (bool, error)
	ListFrameworkControls(ctx context.Context, frameworkID int) ([]*model.AuditFrameworkControl, error)
	CreateFrameworkControl(ctx context.Context, frameworkID int, req model.CreateFrameworkControlRequest, createdBy string) (*model.AuditFrameworkControl, error)
	// BulkCreateFrameworkControls validates every row (and rejects control
	// numbers repeated in the file or already in the framework) before writing
	// any, then creates them in order.
	BulkCreateFrameworkControls(ctx context.Context, frameworkID int, reqs []model.CreateFrameworkControlRequest, createdBy string) ([]*model.AuditFrameworkControl, error)
}

type frameworkService struct {
	frameworkRepo        repository.FrameworkRepository
	productRepo          repository.ProductRepository
	frameworkControlRepo repository.FrameworkControlRepository
}

func NewFrameworkService(frameworkRepo repository.FrameworkRepository, productRepo repository.ProductRepository, frameworkControlRepo repository.FrameworkControlRepository) FrameworkService {
	return &frameworkService{frameworkRepo: frameworkRepo, productRepo: productRepo, frameworkControlRepo: frameworkControlRepo}
}

func (s *frameworkService) ListFrameworks(ctx context.Context, scope model.Scope, userID int, scopeTeamIDs []int, status string) ([]*model.AuditFramework, error) {
	return s.frameworkRepo.List(ctx, scope, userID, scopeTeamIDs, status)
}

// FrameworkInScope reuses List (frameworks are a small reference table) rather
// than adding a dedicated single-ID repo endpoint.
func (s *frameworkService) FrameworkInScope(ctx context.Context, frameworkID int, scope model.Scope, userID int, scopeTeamIDs []int) (bool, error) {
	frameworks, err := s.frameworkRepo.List(ctx, scope, userID, scopeTeamIDs, "")
	if err != nil {
		return false, err
	}
	for _, f := range frameworks {
		if f.ID == frameworkID {
			return true, nil
		}
	}
	return false, nil
}

func (s *frameworkService) CreateFramework(ctx context.Context, req model.CreateFrameworkRequest, createdBy string) (*model.AuditFramework, error) {
	name, err := validateFrameworkName(req.Name)
	if err != nil {
		return nil, err
	}
	req.Name = name
	return s.frameworkRepo.Create(ctx, req, createdBy)
}

func (s *frameworkService) UpdateFramework(ctx context.Context, id int, req model.UpdateFrameworkRequest, updatedBy string) (*model.AuditFramework, error) {
	if req.Name == nil && req.Status == nil {
		return nil, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "nothing to update"}
	}
	if req.Name != nil {
		name, err := validateFrameworkName(*req.Name)
		if err != nil {
			return nil, err
		}
		req.Name = &name
	}
	if req.Status != nil && *req.Status != "ACTIVE" && *req.Status != "ARCHIVED" {
		return nil, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "status must be ACTIVE or ARCHIVED"}
	}
	if updatedBy == "" {
		return nil, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "authenticated user email is missing from token — check Asgardeo app email scope"}
	}
	fw, err := s.frameworkRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if fw == nil {
		return nil, &apierror.Error{StatusCode: http.StatusNotFound, Body: "framework not found"}
	}
	return s.frameworkRepo.Update(ctx, id, req, updatedBy)
}

func (s *frameworkService) DeleteFramework(ctx context.Context, id int, deletedBy string) error {
	if deletedBy == "" {
		return &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "authenticated user email is missing from token — check Asgardeo app email scope"}
	}
	fw, err := s.frameworkRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if fw == nil {
		return &apierror.Error{StatusCode: http.StatusNotFound, Body: "framework not found"}
	}
	return s.frameworkRepo.Delete(ctx, id, deletedBy)
}

func (s *frameworkService) ListProducts(ctx context.Context, scope model.Scope, userID int, scopeTeamIDs []int) ([]*model.AuditProduct, error) {
	return s.productRepo.List(ctx, scope, userID, scopeTeamIDs)
}

func (s *frameworkService) CreateProduct(ctx context.Context, req model.CreateProductRequest, createdBy string) (*model.AuditProduct, error) {
	if req.Name == "" {
		return nil, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "name is required"}
	}
	return s.productRepo.Create(ctx, req, createdBy)
}

func (s *frameworkService) ListFrameworkControls(ctx context.Context, frameworkID int) ([]*model.AuditFrameworkControl, error) {
	return s.frameworkControlRepo.ListCurrent(ctx, frameworkID)
}

// Column limits of audit_framework_control (see Resources/audit_schema.sql):
// control_number is VARCHAR(60), which counts characters; description and
// evidence_requirement are TEXT, which holds 65,535 bytes. Checked up front so a
// too-long value is a clear 422 rather than a failed insert, which in a bulk
// request would leave earlier rows saved.
const (
	maxFrameworkControlNumberLength = 60
	maxFrameworkControlTextBytes    = 65535
)

// maxBulkFrameworkControls bounds one bulk request. Rows are saved one at a time
// through the data service, so this is kept well under maxBulkControls.
const maxBulkFrameworkControls = 200

// validateFrameworkControl checks one control definition and returns the first
// problem as a plain message, or "" when valid.
func validateFrameworkControl(req model.CreateFrameworkControlRequest) string {
	switch {
	case strings.TrimSpace(req.ControlNumber) == "":
		return "controlNumber is required"
	case utf8.RuneCountInString(strings.TrimSpace(req.ControlNumber)) > maxFrameworkControlNumberLength:
		return fmt.Sprintf("controlNumber must be at most %d characters", maxFrameworkControlNumberLength)
	case strings.TrimSpace(req.Description) == "":
		return "description is required"
	case len(req.Description) > maxFrameworkControlTextBytes:
		return "description is too long"
	case req.EvidenceRequirement != nil && len(*req.EvidenceRequirement) > maxFrameworkControlTextBytes:
		return "evidenceRequirement is too long"
	case req.RequirementType != "DESIGN" && req.RequirementType != "OE":
		return "requirementType must be DESIGN or OE"
	case req.ControlType != "CONFIG" && req.ControlType != "NON_CONFIG":
		return "controlType must be CONFIG or NON_CONFIG"
	case req.Scope != "COMMON" && req.Scope != "PRODUCT_SPECIFIC":
		return "scope must be COMMON or PRODUCT_SPECIFIC"
	}
	return ""
}

func (s *frameworkService) CreateFrameworkControl(ctx context.Context, frameworkID int, req model.CreateFrameworkControlRequest, createdBy string) (*model.AuditFrameworkControl, error) {
	if msg := validateFrameworkControl(req); msg != "" {
		return nil, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: msg}
	}
	return s.frameworkControlRepo.Create(ctx, frameworkID, req, createdBy)
}

func (s *frameworkService) BulkCreateFrameworkControls(ctx context.Context, frameworkID int, reqs []model.CreateFrameworkControlRequest, createdBy string) ([]*model.AuditFrameworkControl, error) {
	unprocessable := func(msg string) error {
		return &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: msg}
	}
	if len(reqs) == 0 {
		return nil, unprocessable("controls must not be empty")
	}
	if len(reqs) > maxBulkFrameworkControls {
		return nil, unprocessable(fmt.Sprintf("controls must not exceed %d items per request", maxBulkFrameworkControls))
	}
	fw, err := s.frameworkRepo.GetByID(ctx, frameworkID)
	if err != nil {
		return nil, err
	}
	if fw == nil {
		return nil, &apierror.Error{StatusCode: http.StatusNotFound, Body: "framework not found"}
	}
	existing, err := s.frameworkControlRepo.ListCurrent(ctx, frameworkID)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(existing)+len(reqs))
	for _, c := range existing {
		seen[strings.ToLower(strings.TrimSpace(c.ControlNumber))] = struct{}{}
	}
	var problems []string
	for i := range reqs {
		reqs[i].ControlNumber = strings.TrimSpace(reqs[i].ControlNumber)
		reqs[i].Description = strings.TrimSpace(reqs[i].Description)
		if msg := validateFrameworkControl(reqs[i]); msg != "" {
			problems = append(problems, fmt.Sprintf("row %d: %s", i+1, msg))
			continue
		}
		key := strings.ToLower(reqs[i].ControlNumber)
		if _, dup := seen[key]; dup {
			problems = append(problems, fmt.Sprintf("row %d: control %q already exists in this framework or earlier in the file", i+1, reqs[i].ControlNumber))
			continue
		}
		seen[key] = struct{}{}
	}
	if len(problems) > 0 {
		const maxShown = 10
		shown := problems
		if len(shown) > maxShown {
			shown = append(shown[:maxShown:maxShown], fmt.Sprintf("…and %d more", len(problems)-maxShown))
		}
		return nil, unprocessable(strings.Join(shown, "; "))
	}
	created := make([]*model.AuditFrameworkControl, 0, len(reqs))
	for i, req := range reqs {
		fc, err := s.frameworkControlRepo.Create(ctx, frameworkID, req, createdBy)
		if err != nil {
			if len(created) == 0 {
				return nil, err // nothing saved: the caller maps it (logged, generic 500)
			}
			// Earlier rows are already saved, so say how many, but keep the
			// data service's error text out of the response.
			slog.ErrorContext(ctx, "bulk framework control create failed part-way",
				"frameworkId", frameworkID, "row", i+1, "added", len(created), "err", err)
			return nil, &apierror.Error{
				StatusCode: http.StatusConflict,
				Body: fmt.Sprintf("%d of %d controls were added before an error stopped the upload. Reload the list to see them, then upload the remaining rows.",
					len(created), len(reqs)),
			}
		}
		created = append(created, fc)
	}
	return created, nil
}
