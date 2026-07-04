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
	"strings"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/repository"
)

type auditService struct{ repo repository.AuditRepository }

// NewAuditService constructs an AuditService.
func NewAuditService(repo repository.AuditRepository) AuditService {
	return &auditService{repo: repo}
}

var validAuditStatuses = map[string]bool{
	"ACTIVE": true, "COMPLETED": true, "ARCHIVED": true, "REMOVED": true,
}

func (s *auditService) SearchAudits(ctx context.Context, req domain.SearchAuditsRequest) (domain.SearchAuditsResponse, error) {
	for _, sk := range req.StatusKeys {
		if !validAuditStatuses[strings.ToUpper(sk)] {
			return domain.SearchAuditsResponse{}, &apierror.ValidationError{Msg: "invalid statusKey: " + sk}
		}
	}
	normalizePagination(&req.Pagination)
	audits, total, err := s.repo.SearchAudits(ctx, req)
	if err != nil {
		return domain.SearchAuditsResponse{}, err
	}
	if audits == nil {
		audits = []domain.Audit{}
	}
	return domain.SearchAuditsResponse{Audits: audits, Total: total, Limit: req.Pagination.Limit, Offset: req.Pagination.Offset}, nil
}

func (s *auditService) GetAuditByID(ctx context.Context, id int) (domain.Audit, error) {
	if id <= 0 {
		return domain.Audit{}, &apierror.ValidationError{Msg: "audit id must be a positive integer"}
	}
	a, err := s.repo.GetAuditByID(ctx, id)
	if err != nil {
		return domain.Audit{}, err
	}
	return *a, nil
}

func (s *auditService) CreateAudit(ctx context.Context, req domain.CreateAuditRequest) (domain.Audit, error) {
	if req.Name == "" {
		return domain.Audit{}, &apierror.ValidationError{Msg: "name is required"}
	}
	if req.FrameworkID <= 0 {
		return domain.Audit{}, &apierror.ValidationError{Msg: "frameworkId is required"}
	}
	if req.ProductID <= 0 {
		return domain.Audit{}, &apierror.ValidationError{Msg: "productId is required"}
	}
	if req.PeriodStart == "" {
		return domain.Audit{}, &apierror.ValidationError{Msg: "periodStart is required"}
	}
	if req.PeriodEnd == "" {
		return domain.Audit{}, &apierror.ValidationError{Msg: "periodEnd is required"}
	}
	if req.CreatedBy == "" {
		return domain.Audit{}, &apierror.ValidationError{Msg: "createdBy is required"}
	}
	a, err := s.repo.CreateAudit(ctx, req)
	if err != nil {
		return domain.Audit{}, err
	}
	return *a, nil
}

func (s *auditService) DeleteAudit(ctx context.Context, id int, deletedBy string) error {
	if id <= 0 {
		return &apierror.ValidationError{Msg: "audit id must be a positive integer"}
	}
	if deletedBy == "" {
		return &apierror.ValidationError{Msg: "deletedBy is required"}
	}
	return s.repo.DeleteAudit(ctx, id, deletedBy)
}

func (s *auditService) UpdateAudit(ctx context.Context, id int, req domain.UpdateAuditRequest) (domain.Audit, error) {
	if id <= 0 {
		return domain.Audit{}, &apierror.ValidationError{Msg: "audit id must be a positive integer"}
	}
	if req.UpdatedBy == "" {
		return domain.Audit{}, &apierror.ValidationError{Msg: "updatedBy is required"}
	}
	if req.Status != nil && !validAuditStatuses[strings.ToUpper(*req.Status)] {
		return domain.Audit{}, &apierror.ValidationError{Msg: "invalid status: " + *req.Status}
	}
	a, err := s.repo.UpdateAudit(ctx, id, req)
	if err != nil {
		return domain.Audit{}, err
	}
	return *a, nil
}
