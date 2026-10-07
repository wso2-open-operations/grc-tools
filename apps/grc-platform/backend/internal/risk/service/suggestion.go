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
	"slices"
	"strings"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/aigateway"
)

// CategorySuggestionService generates and records AI category suggestions.
// Suggest is stateless — see model.SuggestCategoryRequest's doc comment for
// why nothing is persisted until RecordDecision, which runs at risk-save
// time, once a risk id definitely exists.
type CategorySuggestionService interface {
	Suggest(ctx context.Context, req model.SuggestCategoryRequest) (*model.SuggestCategoryResponse, error)
	// RecordDecision persists the suggestion that was shown for riskID and
	// immediately marks it ACCEPTED or OVERRIDDEN by comparing suggestion's
	// CategoryID against finalCategoryIDs — the caller never asserts the
	// decision directly, so it can't drift from what was actually saved.
	// No-op (returns nil) when suggestion is nil, i.e. the user never
	// requested one for this save.
	RecordDecision(ctx context.Context, riskID int, suggestion *model.AICategorySuggestion, finalCategoryIDs []int, decidedBy string) error
}

type categorySuggestionService struct {
	gateway        *aigateway.Client
	categoryRepo   repository.RiskCategoryRepository
	complianceRepo repository.ComplianceReferenceRepository
	suggestionRepo repository.SuggestionRepository
}

// NewCategorySuggestionService constructs a CategorySuggestionService.
func NewCategorySuggestionService(
	gateway *aigateway.Client,
	categoryRepo repository.RiskCategoryRepository,
	complianceRepo repository.ComplianceReferenceRepository,
	suggestionRepo repository.SuggestionRepository,
) CategorySuggestionService {
	return &categorySuggestionService{
		gateway:        gateway,
		categoryRepo:   categoryRepo,
		complianceRepo: complianceRepo,
		suggestionRepo: suggestionRepo,
	}
}

func (s *categorySuggestionService) Suggest(ctx context.Context, req model.SuggestCategoryRequest) (*model.SuggestCategoryResponse, error) {
	cats, err := s.categoryRepo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("suggest category: list categories: %w", err)
	}
	options := make([]aigateway.CategoryOption, 0, len(cats))
	byID := make(map[int]*model.RiskCategory, len(cats))
	for _, c := range cats {
		desc := ""
		if c.Description != nil {
			desc = *c.Description
		}
		options = append(options, aigateway.CategoryOption{ID: c.ID, Name: c.Name, Description: desc})
		byID[c.ID] = c
	}

	refNames, err := resolveComplianceReferenceNames(ctx, s.complianceRepo, req.ComplianceReferenceIDs)
	if err != nil {
		return nil, fmt.Errorf("suggest category: resolve compliance references: %w", err)
	}

	result, err := s.gateway.SuggestCategory(ctx, aigateway.SuggestCategoryRequest{
		Title:                    req.Title,
		Description:              req.Description,
		ComplianceReferenceNames: refNames,
		Categories:               options,
	})
	if err != nil {
		return nil, fmt.Errorf("suggest category: %w", err)
	}

	cat, ok := byID[result.CategoryID]
	if !ok {
		return nil, fmt.Errorf("suggest category: model returned unknown category id %d", result.CategoryID)
	}

	return &model.SuggestCategoryResponse{
		CategoryID:   cat.ID,
		CategoryName: cat.Name,
		Reason:       result.Reason,
		Confidence:   result.Confidence,
	}, nil
}

func (s *categorySuggestionService) RecordDecision(ctx context.Context, riskID int, suggestion *model.AICategorySuggestion, finalCategoryIDs []int, decidedBy string) error {
	if suggestion == nil {
		return nil
	}

	confidence := strings.ToUpper(suggestion.Confidence)
	id, err := s.suggestionRepo.Create(ctx, model.CreateSuggestionRequest{
		RiskID:          riskID,
		Feature:         "CATEGORY",
		SuggestedValue:  fmt.Sprintf("%d", suggestion.CategoryID),
		SuggestedReason: suggestion.Reason,
		Confidence:      confidence,
	})
	if err != nil {
		return fmt.Errorf("record category suggestion: create: %w", err)
	}

	status := model.SuggestionStatusOverridden
	var overrideReason *string
	if slices.Contains(finalCategoryIDs, suggestion.CategoryID) {
		status = model.SuggestionStatusAccepted
	}
	// Override reason is optional for CATEGORY (unlike LIKELIHOOD, later) —
	// the frontend doesn't send one separately from the risk form today, so
	// this is always nil for now; left as a field on DecideSuggestionRequest
	// so a future "why did you change it" prompt has somewhere to go without
	// another schema or API-contract change.
	_ = overrideReason

	if err := s.suggestionRepo.Decide(ctx, id, model.DecideSuggestionRequest{
		Status:         status,
		OverrideReason: overrideReason,
	}, decidedBy); err != nil {
		return fmt.Errorf("record category suggestion: decide: %w", err)
	}
	return nil
}
