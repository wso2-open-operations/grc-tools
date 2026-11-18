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

type auditFrameworkService struct {
	repo repository.AuditFrameworkRepository
}

// NewAuditFrameworkService constructs an AuditFrameworkService.
func NewAuditFrameworkService(repo repository.AuditFrameworkRepository) AuditFrameworkService {
	return &auditFrameworkService{repo: repo}
}

// Statuses that can be searched on: ACTIVE, ARCHIVED, or INACTIVE (deleted).
var validAuditFrameworkStatuses = map[string]bool{"ACTIVE": true, "ARCHIVED": true, "INACTIVE": true}

// Statuses a caller may set on create/update. INACTIVE (deleted) is reached only
// through DELETE, so a framework cannot be deleted by a status patch.
var settableAuditFrameworkStatuses = map[string]bool{"ACTIVE": true, "ARCHIVED": true}

func (s *auditFrameworkService) SearchAuditFrameworks(ctx context.Context, req domain.SearchAuditFrameworksRequest) (domain.SearchAuditFrameworksResponse, error) {
	if req.StatusKey != "" && !validAuditFrameworkStatuses[strings.ToUpper(req.StatusKey)] {
		return domain.SearchAuditFrameworksResponse{}, &apierror.ValidationError{Msg: "invalid statusKey: must be ACTIVE, ARCHIVED or INACTIVE"}
	}
	req.StatusKey = strings.ToUpper(req.StatusKey)
	normalizePagination(&req.Pagination)
	frameworks, total, err := s.repo.SearchAuditFrameworks(ctx, req)
	if err != nil {
		return domain.SearchAuditFrameworksResponse{}, err
	}
	if frameworks == nil {
		frameworks = []domain.AuditFramework{}
	}
	return domain.SearchAuditFrameworksResponse{Frameworks: frameworks, Total: total, Limit: req.Pagination.Limit, Offset: req.Pagination.Offset}, nil
}

func (s *auditFrameworkService) GetAuditFrameworkByID(ctx context.Context, id int) (domain.AuditFramework, error) {
	if id <= 0 {
		return domain.AuditFramework{}, &apierror.ValidationError{Msg: "framework id must be a positive integer"}
	}
	f, err := s.repo.GetAuditFrameworkByID(ctx, id)
	if err != nil {
		return domain.AuditFramework{}, err
	}
	return *f, nil
}

func (s *auditFrameworkService) CreateAuditFramework(ctx context.Context, req domain.CreateAuditFrameworkRequest) (domain.AuditFramework, error) {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return domain.AuditFramework{}, &apierror.ValidationError{Msg: "name is required"}
	}
	if req.Status != "" && !settableAuditFrameworkStatuses[strings.ToUpper(req.Status)] {
		return domain.AuditFramework{}, &apierror.ValidationError{Msg: "invalid status: must be ACTIVE or ARCHIVED"}
	}
	req.Status = strings.ToUpper(req.Status)
	if req.CreatedBy == "" {
		return domain.AuditFramework{}, &apierror.ValidationError{Msg: "createdBy is required"}
	}
	f, err := s.repo.CreateAuditFramework(ctx, req)
	if err != nil {
		return domain.AuditFramework{}, err
	}
	return *f, nil
}

func (s *auditFrameworkService) UpdateAuditFramework(ctx context.Context, id int, req domain.UpdateAuditFrameworkRequest) (domain.AuditFramework, error) {
	if id <= 0 {
		return domain.AuditFramework{}, &apierror.ValidationError{Msg: "framework id must be a positive integer"}
	}
	if req.Status != nil && !settableAuditFrameworkStatuses[strings.ToUpper(*req.Status)] {
		return domain.AuditFramework{}, &apierror.ValidationError{Msg: "invalid status: must be ACTIVE or ARCHIVED"}
	}
	if req.Status != nil {
		up := strings.ToUpper(*req.Status)
		req.Status = &up
	}
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if trimmed == "" {
			return domain.AuditFramework{}, &apierror.ValidationError{Msg: "name must not be empty"}
		}
		req.Name = &trimmed
	}
	if req.UpdatedBy == "" {
		return domain.AuditFramework{}, &apierror.ValidationError{Msg: "updatedBy is required"}
	}
	f, err := s.repo.UpdateAuditFramework(ctx, id, req)
	if err != nil {
		return domain.AuditFramework{}, err
	}
	return *f, nil
}

func (s *auditFrameworkService) DeleteAuditFramework(ctx context.Context, id int, deletedBy string) error {
	if id <= 0 {
		return &apierror.ValidationError{Msg: "framework id must be a positive integer"}
	}
	if strings.TrimSpace(deletedBy) == "" {
		return &apierror.ValidationError{Msg: "deletedBy is required"}
	}
	return s.repo.DeleteAuditFramework(ctx, id, deletedBy)
}
