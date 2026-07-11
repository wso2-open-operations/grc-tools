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

type auditTeamService struct {
	repo repository.AuditTeamRepository
}

// NewAuditTeamService constructs an AuditTeamService.
func NewAuditTeamService(repo repository.AuditTeamRepository) AuditTeamService {
	return &auditTeamService{repo: repo}
}

var validAuditTeamStatuses = map[string]bool{"ACTIVE": true, "INACTIVE": true}

func (s *auditTeamService) SearchAuditTeams(ctx context.Context, req domain.SearchAuditTeamsRequest) (domain.SearchAuditTeamsResponse, error) {
	if req.StatusKey != "" && !validAuditTeamStatuses[strings.ToUpper(req.StatusKey)] {
		return domain.SearchAuditTeamsResponse{}, &apierror.ValidationError{Msg: "invalid statusKey: must be ACTIVE or INACTIVE"}
	}
	req.StatusKey = strings.ToUpper(req.StatusKey)
	normalizePagination(&req.Pagination)
	teams, total, err := s.repo.SearchAuditTeams(ctx, req)
	if err != nil {
		return domain.SearchAuditTeamsResponse{}, err
	}
	if teams == nil {
		teams = []domain.AuditTeam{}
	}
	return domain.SearchAuditTeamsResponse{Teams: teams, Total: total, Limit: req.Pagination.Limit, Offset: req.Pagination.Offset}, nil
}

func (s *auditTeamService) GetAuditTeamByID(ctx context.Context, id int) (domain.AuditTeam, error) {
	if id <= 0 {
		return domain.AuditTeam{}, &apierror.ValidationError{Msg: "team id must be a positive integer"}
	}
	t, err := s.repo.GetAuditTeamByID(ctx, id)
	if err != nil {
		return domain.AuditTeam{}, err
	}
	return *t, nil
}

func (s *auditTeamService) CreateAuditTeam(ctx context.Context, req domain.CreateAuditTeamRequest) (domain.AuditTeam, error) {
	if req.Name == "" {
		return domain.AuditTeam{}, &apierror.ValidationError{Msg: "name is required"}
	}
	if req.Status != "" && !validAuditTeamStatuses[strings.ToUpper(req.Status)] {
		return domain.AuditTeam{}, &apierror.ValidationError{Msg: "invalid status: must be ACTIVE or INACTIVE"}
	}
	req.Status = strings.ToUpper(req.Status)
	if req.CreatedBy == "" {
		return domain.AuditTeam{}, &apierror.ValidationError{Msg: "createdBy is required"}
	}
	t, err := s.repo.CreateAuditTeam(ctx, req)
	if err != nil {
		return domain.AuditTeam{}, err
	}
	return *t, nil
}

func (s *auditTeamService) UpdateAuditTeam(ctx context.Context, id int, req domain.UpdateAuditTeamRequest) (domain.AuditTeam, error) {
	if id <= 0 {
		return domain.AuditTeam{}, &apierror.ValidationError{Msg: "team id must be a positive integer"}
	}
	if req.Status != nil && !validAuditTeamStatuses[strings.ToUpper(*req.Status)] {
		return domain.AuditTeam{}, &apierror.ValidationError{Msg: "invalid status: must be ACTIVE or INACTIVE"}
	}
	if req.Status != nil {
		up := strings.ToUpper(*req.Status)
		req.Status = &up
	}
	if req.UpdatedBy == "" {
		return domain.AuditTeam{}, &apierror.ValidationError{Msg: "updatedBy is required"}
	}
	t, err := s.repo.UpdateAuditTeam(ctx, id, req)
	if err != nil {
		return domain.AuditTeam{}, err
	}
	return *t, nil
}
