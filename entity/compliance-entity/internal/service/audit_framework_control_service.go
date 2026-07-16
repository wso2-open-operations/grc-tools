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
	"fmt"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/repository"
)

type frameworkControlService struct {
	repo repository.FrameworkControlRepository
}

// NewFrameworkControlService constructs a FrameworkControlService.
func NewFrameworkControlService(repo repository.FrameworkControlRepository) FrameworkControlService {
	return &frameworkControlService{repo: repo}
}

func (s *frameworkControlService) ListCurrentControls(ctx context.Context, frameworkID int) (domain.ListFrameworkControlsResponse, error) {
	controls, err := s.repo.ListCurrentControls(ctx, frameworkID)
	if err != nil {
		return domain.ListFrameworkControlsResponse{}, err
	}
	if controls == nil {
		controls = []domain.AuditFrameworkControl{}
	}
	return domain.ListFrameworkControlsResponse{Controls: controls, Total: len(controls)}, nil
}

func (s *frameworkControlService) ListAllVersions(ctx context.Context, frameworkID int, controlNumber string) ([]domain.AuditFrameworkControl, error) {
	versions, err := s.repo.ListAllVersions(ctx, frameworkID, controlNumber)
	if err != nil {
		return nil, err
	}
	if versions == nil {
		versions = []domain.AuditFrameworkControl{}
	}
	return versions, nil
}

func (s *frameworkControlService) GetByID(ctx context.Context, id int) (domain.AuditFrameworkControl, error) {
	c, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return domain.AuditFrameworkControl{}, err
	}
	return *c, nil
}

func (s *frameworkControlService) Create(ctx context.Context, frameworkID int, req domain.CreateFrameworkControlRequest) (domain.AuditFrameworkControl, error) {
	if req.ControlNumber == "" {
		return domain.AuditFrameworkControl{}, &apierror.ValidationError{Msg: "controlNumber is required"}
	}
	if req.Description == "" {
		return domain.AuditFrameworkControl{}, &apierror.ValidationError{Msg: "description is required"}
	}
	if !validRequirementType(req.RequirementType) {
		return domain.AuditFrameworkControl{}, &apierror.ValidationError{Msg: fmt.Sprintf("invalid requirementType %q (must be DESIGN or OE)", req.RequirementType)}
	}
	if !validControlType(req.ControlType) {
		return domain.AuditFrameworkControl{}, &apierror.ValidationError{Msg: fmt.Sprintf("invalid controlType %q (must be CONFIG or NON_CONFIG)", req.ControlType)}
	}
	if !validScope(req.Scope) {
		return domain.AuditFrameworkControl{}, &apierror.ValidationError{Msg: fmt.Sprintf("invalid scope %q (must be COMMON or PRODUCT_SPECIFIC)", req.Scope)}
	}
	if req.CreatedBy == "" {
		return domain.AuditFrameworkControl{}, &apierror.ValidationError{Msg: "createdBy is required"}
	}
	c, err := s.repo.Create(ctx, frameworkID, req)
	if err != nil {
		return domain.AuditFrameworkControl{}, err
	}
	return *c, nil
}

func (s *frameworkControlService) NewVersion(ctx context.Context, id int, req domain.UpdateFrameworkControlRequest) (domain.AuditFrameworkControl, error) {
	if req.RequirementType != nil && !validRequirementType(*req.RequirementType) {
		return domain.AuditFrameworkControl{}, &apierror.ValidationError{Msg: fmt.Sprintf("invalid requirementType %q", *req.RequirementType)}
	}
	if req.ControlType != nil && !validControlType(*req.ControlType) {
		return domain.AuditFrameworkControl{}, &apierror.ValidationError{Msg: fmt.Sprintf("invalid controlType %q", *req.ControlType)}
	}
	if req.Scope != nil && !validScope(*req.Scope) {
		return domain.AuditFrameworkControl{}, &apierror.ValidationError{Msg: fmt.Sprintf("invalid scope %q", *req.Scope)}
	}
	if req.UpdatedBy == "" {
		return domain.AuditFrameworkControl{}, &apierror.ValidationError{Msg: "updatedBy is required"}
	}
	c, err := s.repo.NewVersion(ctx, id, req)
	if err != nil {
		return domain.AuditFrameworkControl{}, err
	}
	return *c, nil
}

func validRequirementType(v string) bool { return v == "DESIGN" || v == "OE" }
func validControlType(v string) bool     { return v == "CONFIG" || v == "NON_CONFIG" }
func validScope(v string) bool           { return v == "COMMON" || v == "PRODUCT_SPECIFIC" }
