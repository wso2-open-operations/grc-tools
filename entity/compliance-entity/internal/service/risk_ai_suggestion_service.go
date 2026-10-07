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

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/repository"
)

type riskAISuggestionService struct {
	repo repository.RiskAISuggestionRepository
}

// NewRiskAISuggestionService constructs a RiskAISuggestionService.
func NewRiskAISuggestionService(repo repository.RiskAISuggestionRepository) RiskAISuggestionService {
	return &riskAISuggestionService{repo: repo}
}

func (s *riskAISuggestionService) CreateRiskAISuggestion(ctx context.Context, req domain.CreateRiskAISuggestionRequest) (domain.RiskAISuggestion, error) {
	if req.RiskID <= 0 {
		return domain.RiskAISuggestion{}, &apierror.ValidationError{Msg: "riskId must be a positive integer"}
	}
	if req.Feature != domain.RiskAISuggestionFeatureCategory && req.Feature != domain.RiskAISuggestionFeatureLikelihood && req.Feature != domain.RiskAISuggestionFeatureActionPlan {
		return domain.RiskAISuggestion{}, &apierror.ValidationError{Msg: "feature must be CATEGORY, LIKELIHOOD, or ACTION_PLAN"}
	}
	if req.SuggestedValue == "" {
		return domain.RiskAISuggestion{}, &apierror.ValidationError{Msg: "suggestedValue is required"}
	}
	s2, err := s.repo.CreateRiskAISuggestion(ctx, req)
	if err != nil {
		return domain.RiskAISuggestion{}, err
	}
	return *s2, nil
}

func (s *riskAISuggestionService) DecideRiskAISuggestion(ctx context.Context, id int, req domain.DecideRiskAISuggestionRequest) (domain.RiskAISuggestion, error) {
	if id <= 0 {
		return domain.RiskAISuggestion{}, &apierror.ValidationError{Msg: "suggestion id must be a positive integer"}
	}
	if req.Status != domain.RiskAISuggestionStatusAccepted && req.Status != domain.RiskAISuggestionStatusOverridden {
		return domain.RiskAISuggestion{}, &apierror.ValidationError{Msg: "status must be ACCEPTED or OVERRIDDEN"}
	}
	if req.DecidedBy == "" {
		return domain.RiskAISuggestion{}, &apierror.ValidationError{Msg: "decidedBy is required"}
	}
	s2, err := s.repo.DecideRiskAISuggestion(ctx, id, req)
	if err != nil {
		return domain.RiskAISuggestion{}, err
	}
	return *s2, nil
}

func (s *riskAISuggestionService) ListRiskAISuggestionsByRisk(ctx context.Context, riskID int, feature domain.RiskAISuggestionFeature, status *domain.RiskAISuggestionStatus) ([]domain.RiskAISuggestion, error) {
	if riskID <= 0 {
		return nil, &apierror.ValidationError{Msg: "riskId must be a positive integer"}
	}
	if feature != domain.RiskAISuggestionFeatureCategory && feature != domain.RiskAISuggestionFeatureLikelihood && feature != domain.RiskAISuggestionFeatureActionPlan {
		return nil, &apierror.ValidationError{Msg: "feature must be CATEGORY, LIKELIHOOD, or ACTION_PLAN"}
	}
	if status != nil && *status != domain.RiskAISuggestionStatusSuggested &&
		*status != domain.RiskAISuggestionStatusAccepted && *status != domain.RiskAISuggestionStatusOverridden {
		return nil, &apierror.ValidationError{Msg: "status must be SUGGESTED, ACCEPTED, or OVERRIDDEN"}
	}
	list, err := s.repo.ListRiskAISuggestionsByRisk(ctx, riskID, feature, status)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []domain.RiskAISuggestion{}
	}
	return list, nil
}
