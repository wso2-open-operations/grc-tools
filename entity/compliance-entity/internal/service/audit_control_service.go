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
	"strings"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/repository"
)

type controlService struct{ repo repository.ControlRepository }

// NewControlService constructs a ControlService.
func NewControlService(repo repository.ControlRepository) ControlService {
	return &controlService{repo: repo}
}

// validControlStatuses mirrors the audit_control.status ENUM in audit_schema.sql
// exactly (12 statuses). Keep in sync with the schema — any drift causes valid
// filters to 400 or invalid ones to reach the DB.
var validControlStatuses = map[string]bool{
	// OE — population phase
	"POPULATION_PENDING":            true,
	"POPULATION_INTERNAL_REVIEW":    true,
	"POPULATION_UNDER_VALIDATION":   true,
	"POPULATION_NEED_CLARIFICATION": true,
	"POPULATION_COMPLETE":           true,
	// OE — sample phase (between population approval and evidence)
	"AWAITING_SAMPLE":  true,
	"SUBMITTED_SAMPLE": true,
	// Evidence phase (Design default; OE after sample)
	"EVIDENCE_PENDING":            true,
	"EVIDENCE_INTERNAL_REVIEW":    true,
	"EVIDENCE_UNDER_VALIDATION":   true,
	"EVIDENCE_NEED_CLARIFICATION": true,
	// Terminal
	"COMPLETE": true,
}

var validRequirementTypes = map[string]bool{"DESIGN": true, "OE": true}

// validControlTypes / validScopes mirror the audit_control.control_type and
// audit_control.scope ENUMs in audit_schema.sql.
var validControlTypes = map[string]bool{"CONFIG": true, "NON_CONFIG": true}
var validScopes = map[string]bool{"COMMON": true, "PRODUCT_SPECIFIC": true}

func (s *controlService) SearchControls(ctx context.Context, auditID int, req domain.SearchControlsRequest) (domain.SearchControlsResponse, error) {
	if auditID <= 0 {
		return domain.SearchControlsResponse{}, &apierror.ValidationError{Msg: "auditId must be a positive integer"}
	}
	for _, sk := range req.StatusKeys {
		if !validControlStatuses[strings.ToUpper(sk)] {
			return domain.SearchControlsResponse{}, &apierror.ValidationError{Msg: "invalid statusKey: " + sk}
		}
	}
	for _, rt := range req.RequirementTypes {
		if !validRequirementTypes[strings.ToUpper(rt)] {
			return domain.SearchControlsResponse{}, &apierror.ValidationError{Msg: "invalid requirementType: " + rt + " (must be DESIGN or OE)"}
		}
	}
	normalizePagination(&req.Pagination)
	controls, total, err := s.repo.SearchControls(ctx, auditID, req)
	if err != nil {
		return domain.SearchControlsResponse{}, err
	}
	if controls == nil {
		controls = []domain.AuditControl{}
	}
	return domain.SearchControlsResponse{Controls: controls, Total: total, Limit: req.Pagination.Limit, Offset: req.Pagination.Offset}, nil
}

func (s *controlService) SearchControlsGlobal(ctx context.Context, req domain.SearchControlsRequest) (domain.SearchControlsResponse, error) {
	for _, sk := range req.StatusKeys {
		if !validControlStatuses[strings.ToUpper(sk)] {
			return domain.SearchControlsResponse{}, &apierror.ValidationError{Msg: "invalid statusKey: " + sk}
		}
	}
	for _, rt := range req.RequirementTypes {
		if !validRequirementTypes[strings.ToUpper(rt)] {
			return domain.SearchControlsResponse{}, &apierror.ValidationError{Msg: "invalid requirementType: " + rt + " (must be DESIGN or OE)"}
		}
	}
	normalizePagination(&req.Pagination)
	controls, total, err := s.repo.SearchControlsGlobal(ctx, req)
	if err != nil {
		return domain.SearchControlsResponse{}, err
	}
	if controls == nil {
		controls = []domain.AuditControl{}
	}
	return domain.SearchControlsResponse{Controls: controls, Total: total, Limit: req.Pagination.Limit, Offset: req.Pagination.Offset}, nil
}

func (s *controlService) BulkCreateControls(ctx context.Context, auditID int, req domain.BulkCreateControlsRequest) (domain.BulkCreateControlsResponse, error) {
	if auditID <= 0 {
		return domain.BulkCreateControlsResponse{}, &apierror.ValidationError{Msg: "auditId must be a positive integer"}
	}
	if len(req.Controls) == 0 {
		return domain.BulkCreateControlsResponse{}, &apierror.ValidationError{Msg: "controls must not be empty"}
	}
	for i, c := range req.Controls {
		if c.ControlNumber == "" {
			return domain.BulkCreateControlsResponse{}, &apierror.ValidationError{Msg: fmt.Sprintf("controls[%d]: controlNumber is required", i)}
		}
		if !validRequirementTypes[strings.ToUpper(c.RequirementType)] {
			return domain.BulkCreateControlsResponse{}, &apierror.ValidationError{Msg: fmt.Sprintf("controls[%d]: invalid requirementType %q", i, c.RequirementType)}
		}
		if !validControlTypes[strings.ToUpper(c.ControlType)] {
			return domain.BulkCreateControlsResponse{}, &apierror.ValidationError{Msg: fmt.Sprintf("controls[%d]: invalid controlType %q (must be CONFIG or NON_CONFIG)", i, c.ControlType)}
		}
		if !validScopes[strings.ToUpper(c.Scope)] {
			return domain.BulkCreateControlsResponse{}, &apierror.ValidationError{Msg: fmt.Sprintf("controls[%d]: invalid scope %q (must be COMMON or PRODUCT_SPECIFIC)", i, c.Scope)}
		}
		if c.CreatedBy == "" {
			return domain.BulkCreateControlsResponse{}, &apierror.ValidationError{Msg: fmt.Sprintf("controls[%d]: createdBy is required", i)}
		}
	}
	controls, err := s.repo.BulkCreateControls(ctx, auditID, req.Controls)
	if err != nil {
		return domain.BulkCreateControlsResponse{}, err
	}
	return domain.BulkCreateControlsResponse{Controls: controls, Created: len(controls)}, nil
}

func (s *controlService) DeleteControl(ctx context.Context, auditID, controlID int) error {
	if auditID <= 0 {
		return &apierror.ValidationError{Msg: "auditId must be a positive integer"}
	}
	if controlID <= 0 {
		return &apierror.ValidationError{Msg: "controlId must be a positive integer"}
	}
	return s.repo.DeleteControl(ctx, auditID, controlID)
}

func (s *controlService) GetControlByID(ctx context.Context, auditID, controlID int) (domain.AuditControl, error) {
	if auditID <= 0 {
		return domain.AuditControl{}, &apierror.ValidationError{Msg: "auditId must be a positive integer"}
	}
	if controlID <= 0 {
		return domain.AuditControl{}, &apierror.ValidationError{Msg: "controlId must be a positive integer"}
	}
	c, err := s.repo.GetControlByID(ctx, auditID, controlID)
	if err != nil {
		return domain.AuditControl{}, err
	}
	return *c, nil
}

func (s *controlService) CreateControl(ctx context.Context, auditID int, req domain.CreateControlRequest) (domain.AuditControl, error) {
	if auditID <= 0 {
		return domain.AuditControl{}, &apierror.ValidationError{Msg: "auditId must be a positive integer"}
	}
	if req.ControlNumber == "" {
		return domain.AuditControl{}, &apierror.ValidationError{Msg: "controlNumber is required"}
	}
	if req.Description == "" {
		return domain.AuditControl{}, &apierror.ValidationError{Msg: "description is required"}
	}
	if !validRequirementTypes[strings.ToUpper(req.RequirementType)] {
		return domain.AuditControl{}, &apierror.ValidationError{Msg: "requirementType must be DESIGN or OE"}
	}
	if !validControlTypes[strings.ToUpper(req.ControlType)] {
		return domain.AuditControl{}, &apierror.ValidationError{Msg: "controlType must be CONFIG or NON_CONFIG"}
	}
	if !validScopes[strings.ToUpper(req.Scope)] {
		return domain.AuditControl{}, &apierror.ValidationError{Msg: "scope must be COMMON or PRODUCT_SPECIFIC"}
	}
	if req.CreatedBy == "" {
		return domain.AuditControl{}, &apierror.ValidationError{Msg: "createdBy is required"}
	}
	c, err := s.repo.CreateControl(ctx, auditID, req)
	if err != nil {
		return domain.AuditControl{}, err
	}
	return *c, nil
}

func (s *controlService) UpdateControl(ctx context.Context, auditID, controlID int, req domain.UpdateControlRequest) (domain.AuditControl, error) {
	if auditID <= 0 {
		return domain.AuditControl{}, &apierror.ValidationError{Msg: "auditId must be a positive integer"}
	}
	if controlID <= 0 {
		return domain.AuditControl{}, &apierror.ValidationError{Msg: "controlId must be a positive integer"}
	}
	if req.UpdatedBy == "" {
		return domain.AuditControl{}, &apierror.ValidationError{Msg: "updatedBy is required"}
	}
	if req.Status != nil && !validControlStatuses[strings.ToUpper(*req.Status)] {
		return domain.AuditControl{}, &apierror.ValidationError{Msg: "invalid status: " + *req.Status}
	}
	c, err := s.repo.UpdateControl(ctx, auditID, controlID, req)
	if err != nil {
		return domain.AuditControl{}, err
	}
	return *c, nil
}
