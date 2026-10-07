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
	"strings"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/aigateway"
)

// ActionPlanSuggestionService generates and records AI action plan
// suggestions. Suggest is stateless — same reasoning as
// CategorySuggestionService.Suggest: nothing is persisted until
// RecordDecision, which runs at save time, once a risk id definitely exists.
//
// Unlike Category/Likelihood, there is no fixed suggested value to compare
// against what was finally saved — "accepting" this suggestion means copying
// free text into a field the user may then edit further, so RecordDecision
// does not compare anything; it takes the frontend's explicit Used flag
// directly. See model.AIActionPlanSuggestion's doc comment.
type ActionPlanSuggestionService interface {
	Suggest(ctx context.Context, req model.SuggestActionPlanRequest) (*model.SuggestActionPlanResponse, error)
	// RecordDecision persists the suggestion that was shown for riskID and
	// marks it ACCEPTED when suggestion.Used is true, OVERRIDDEN otherwise —
	// no-op (returns nil) when suggestion is nil, i.e. the user never
	// requested one for this save.
	RecordDecision(ctx context.Context, riskID int, suggestion *model.AIActionPlanSuggestion, decidedBy string) error
}

type actionPlanSuggestionService struct {
	gateway        *aigateway.Client
	categoryRepo   repository.RiskCategoryRepository
	complianceRepo repository.ComplianceReferenceRepository
	suggestionRepo repository.SuggestionRepository
}

// NewActionPlanSuggestionService constructs an ActionPlanSuggestionService.
func NewActionPlanSuggestionService(
	gateway *aigateway.Client,
	categoryRepo repository.RiskCategoryRepository,
	complianceRepo repository.ComplianceReferenceRepository,
	suggestionRepo repository.SuggestionRepository,
) ActionPlanSuggestionService {
	return &actionPlanSuggestionService{
		gateway:        gateway,
		categoryRepo:   categoryRepo,
		complianceRepo: complianceRepo,
		suggestionRepo: suggestionRepo,
	}
}

func (s *actionPlanSuggestionService) Suggest(ctx context.Context, req model.SuggestActionPlanRequest) (*model.SuggestActionPlanResponse, error) {
	categoryName, err := resolveCategoryName(ctx, s.categoryRepo, req.CategoryID)
	if err != nil {
		return nil, fmt.Errorf("suggest action plan: resolve category: %w", err)
	}
	refNames, err := resolveComplianceReferenceNames(ctx, s.complianceRepo, req.ComplianceReferenceIDs)
	if err != nil {
		return nil, fmt.Errorf("suggest action plan: resolve compliance references: %w", err)
	}

	result, err := s.gateway.SuggestActionPlan(ctx, aigateway.SuggestActionPlanRequest{
		Title:                    req.Title,
		Description:              req.Description,
		CategoryName:             categoryName,
		ComplianceReferenceNames: refNames,
		TreatmentStrategy:        req.TreatmentStrategy,
	})
	if err != nil {
		return nil, fmt.Errorf("suggest action plan: %w", err)
	}

	return &model.SuggestActionPlanResponse{
		Description: result.Description,
		Reason:      result.Reason,
		Confidence:  result.Confidence,
	}, nil
}

func (s *actionPlanSuggestionService) RecordDecision(ctx context.Context, riskID int, suggestion *model.AIActionPlanSuggestion, decidedBy string) error {
	if suggestion == nil {
		return nil
	}

	confidence := strings.ToUpper(suggestion.Confidence)
	id, err := s.suggestionRepo.Create(ctx, model.CreateSuggestionRequest{
		RiskID:          riskID,
		Feature:         model.SuggestionFeatureActionPlan,
		SuggestedValue:  suggestion.Description,
		SuggestedReason: suggestion.Reason,
		Confidence:      confidence,
	})
	if err != nil {
		return fmt.Errorf("record action plan suggestion: create: %w", err)
	}

	// No comparison here, unlike Category/Likelihood — there is no fixed
	// suggested value to diff against what was finally saved; the frontend
	// tells us directly whether the user used the suggestion.
	status := model.SuggestionStatusOverridden
	if suggestion.Used {
		status = model.SuggestionStatusAccepted
	}

	if err := s.suggestionRepo.Decide(ctx, id, model.DecideSuggestionRequest{
		Status: status,
	}, decidedBy); err != nil {
		return fmt.Errorf("record action plan suggestion: decide: %w", err)
	}
	return nil
}
