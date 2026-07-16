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

type riskTeamService struct{ repo repository.RiskTeamRepository }

// NewRiskTeamService constructs a RiskTeamService.
func NewRiskTeamService(repo repository.RiskTeamRepository) RiskTeamService {
	return &riskTeamService{repo: repo}
}

var validRiskTeamTypes = map[string]bool{"SOURCE_REGISTER": true, "ASSIGNMENT": true, "BOTH": true}
var validRiskTeamStatuses = map[string]bool{"ACTIVE": true, "INACTIVE": true, "REMOVED": true}

func (s *riskTeamService) SearchRiskTeams(ctx context.Context, req domain.SearchRiskTeamsRequest) (domain.SearchRiskTeamsResponse, error) {
	for _, tt := range req.TeamTypeKeys {
		if !validRiskTeamTypes[strings.ToUpper(tt)] {
			return domain.SearchRiskTeamsResponse{}, &apierror.ValidationError{Msg: "invalid teamTypeKey: " + tt + " (must be SOURCE_REGISTER, ASSIGNMENT, or BOTH)"}
		}
	}
	if req.StatusKey != "" && !validRiskTeamStatuses[strings.ToUpper(req.StatusKey)] {
		return domain.SearchRiskTeamsResponse{}, &apierror.ValidationError{Msg: "invalid statusKey: must be ACTIVE, INACTIVE, or REMOVED"}
	}
	normalizePagination(&req.Pagination)
	teams, total, err := s.repo.SearchRiskTeams(ctx, req)
	if err != nil {
		return domain.SearchRiskTeamsResponse{}, err
	}
	if teams == nil {
		teams = []domain.RiskTeam{}
	}
	return domain.SearchRiskTeamsResponse{Teams: teams, Total: total, Limit: req.Pagination.Limit, Offset: req.Pagination.Offset}, nil
}

func (s *riskTeamService) GetRiskTeamByID(ctx context.Context, id int) (domain.RiskTeam, error) {
	if id <= 0 {
		return domain.RiskTeam{}, &apierror.ValidationError{Msg: "team id must be a positive integer"}
	}
	t, err := s.repo.GetRiskTeamByID(ctx, id)
	if err != nil {
		return domain.RiskTeam{}, err
	}
	return *t, nil
}

func (s *riskTeamService) CreateRiskTeam(ctx context.Context, req domain.CreateRiskTeamRequest) (domain.RiskTeam, error) {
	if req.Name == "" {
		return domain.RiskTeam{}, &apierror.ValidationError{Msg: "name is required"}
	}
	if req.TeamType == "" {
		return domain.RiskTeam{}, &apierror.ValidationError{Msg: "teamType is required"}
	}
	if !validRiskTeamTypes[strings.ToUpper(req.TeamType)] {
		return domain.RiskTeam{}, &apierror.ValidationError{Msg: "invalid teamType: must be SOURCE_REGISTER, ASSIGNMENT, or BOTH"}
	}
	if req.CreatedBy == "" {
		return domain.RiskTeam{}, &apierror.ValidationError{Msg: "createdBy is required"}
	}
	t, err := s.repo.CreateRiskTeam(ctx, req)
	if err != nil {
		return domain.RiskTeam{}, err
	}
	return *t, nil
}

func (s *riskTeamService) UpdateRiskTeam(ctx context.Context, id int, req domain.UpdateRiskTeamRequest) (domain.RiskTeam, error) {
	if id <= 0 {
		return domain.RiskTeam{}, &apierror.ValidationError{Msg: "team id must be a positive integer"}
	}
	if req.UpdatedBy == "" {
		return domain.RiskTeam{}, &apierror.ValidationError{Msg: "updatedBy is required"}
	}
	if req.TeamType != nil && !validRiskTeamTypes[strings.ToUpper(*req.TeamType)] {
		return domain.RiskTeam{}, &apierror.ValidationError{Msg: "invalid teamType"}
	}
	t, err := s.repo.UpdateRiskTeam(ctx, id, req)
	if err != nil {
		return domain.RiskTeam{}, err
	}
	return *t, nil
}
