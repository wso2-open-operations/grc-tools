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
	"strconv"
	"strings"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/aigateway"
)

// LikelihoodSuggestionService generates and records AI Likelihood
// suggestions — Gross (at risk creation) and Residual (at reassessment).
// Suggest is stateless, same reasoning as CategorySuggestionService.Suggest:
// nothing is persisted until the risk/assessment this suggestion is about
// actually has an id, which Suggest itself can't guarantee.
type LikelihoodSuggestionService interface {
	Suggest(ctx context.Context, req model.SuggestLikelihoodRequest) (*model.SuggestLikelihoodResponse, error)
	// RecordDecision persists the suggestion shown for riskID and immediately
	// marks it ACCEPTED or OVERRIDDEN by comparing suggestion.Score against
	// finalScore — the caller never asserts the decision directly, so it
	// can't drift from what was actually saved. No-op when suggestion is nil.
	RecordDecision(ctx context.Context, riskID int, suggestion *model.AILikelihoodSuggestion, finalScore int, decidedBy string) error
	// PendingSuggestion returns the most recent unresolved (SUGGESTED)
	// LIKELIHOOD suggestion for riskID, or nil if there isn't one — powers
	// the in-risk "may need reassessment" reminder.
	PendingSuggestion(ctx context.Context, riskID int) (*model.Suggestion, error)
	// CheckedThisQuarter reports whether riskID already has a LIKELIHOOD
	// suggestion (any outcome — SUGGESTED, ACCEPTED, or OVERRIDDEN all count)
	// created since quarterStart. Used by the quarterly re-check sweep to
	// avoid spending a live web-search call on a risk it's already checked
	// this quarter, so a missed/failed run on one day within the re-check
	// window doesn't cause a duplicate check on a later day within the same
	// window.
	CheckedThisQuarter(ctx context.Context, riskID int, quarterStart time.Time) (bool, error)
	// CreateUnresolvedSuggestion writes a bare SUGGESTED row for riskID — the
	// quarterly re-check sweep's one write when live evidence has moved since
	// the risk's current Gross score. Deliberately never decided by the
	// caller: only a human decides, later, via the existing reassessment flow
	// this row's existence nudges them toward (the risk detail page's
	// PendingLikelihoodSuggestion reminder).
	CreateUnresolvedSuggestion(ctx context.Context, riskID int, result *model.SuggestLikelihoodResponse) error
	// RecordNoChangeCheck writes a row for riskID marking it checked this
	// quarter with no change found — the quarterly re-check sweep's write when
	// live evidence still supports the risk's current EffectiveScore. Unlike
	// CreateUnresolvedSuggestion, it's immediately self-decided (ACCEPTED, by
	// a system sentinel, never a real user): there's nothing for a human to
	// act on, and leaving it SUGGESTED would wrongly surface the "may need
	// reassessment" in-risk reminder for a risk that hasn't actually changed.
	// It counts toward CheckedThisQuarter the same as any other outcome,
	// which is the point — without this, an unchanged risk writes no row and
	// gets re-checked (and re-billed) on every remaining day of the recheck
	// window instead of once per quarter.
	RecordNoChangeCheck(ctx context.Context, riskID int, result *model.SuggestLikelihoodResponse) error
}

// systemDecider marks a risk_ai_suggestion row as decided by the quarterly
// re-check sweep itself, not a human — decided_by has no FK constraint (see
// risk_schema.sql), so a descriptive sentinel is safe and self-documenting
// for anyone auditing the column later.
const systemDecider = "system:likelihood-recheck-job"

type likelihoodSuggestionService struct {
	gateway        *aigateway.Client
	categoryRepo   repository.RiskCategoryRepository
	teamRepo       repository.TeamRepository
	complianceRepo repository.ComplianceReferenceRepository
	suggestionRepo repository.SuggestionRepository
}

// NewLikelihoodSuggestionService constructs a LikelihoodSuggestionService.
func NewLikelihoodSuggestionService(
	gateway *aigateway.Client,
	categoryRepo repository.RiskCategoryRepository,
	teamRepo repository.TeamRepository,
	complianceRepo repository.ComplianceReferenceRepository,
	suggestionRepo repository.SuggestionRepository,
) LikelihoodSuggestionService {
	return &likelihoodSuggestionService{
		gateway:        gateway,
		categoryRepo:   categoryRepo,
		teamRepo:       teamRepo,
		complianceRepo: complianceRepo,
		suggestionRepo: suggestionRepo,
	}
}

func (s *likelihoodSuggestionService) Suggest(ctx context.Context, req model.SuggestLikelihoodRequest) (*model.SuggestLikelihoodResponse, error) {
	categoryName, err := resolveCategoryName(ctx, s.categoryRepo, req.CategoryID)
	if err != nil {
		return nil, fmt.Errorf("suggest likelihood: resolve category: %w", err)
	}
	sourceRegisterName, err := s.resolveTeamName(ctx, req.SourceRegisterID)
	if err != nil {
		return nil, fmt.Errorf("suggest likelihood: resolve source register: %w", err)
	}
	refNames, err := resolveComplianceReferenceNames(ctx, s.complianceRepo, req.ComplianceReferenceIDs)
	if err != nil {
		return nil, fmt.Errorf("suggest likelihood: resolve compliance references: %w", err)
	}

	result, err := s.gateway.SuggestLikelihood(ctx, aigateway.SuggestLikelihoodRequest{
		Title:                    req.Title,
		Description:              req.Description,
		ImpactDescription:        req.ImpactDescription,
		ComplianceReferenceNames: refNames,
		CategoryName:             categoryName,
		SourceRegisterName:       sourceRegisterName,
	})
	if err != nil {
		return nil, fmt.Errorf("suggest likelihood: %w", err)
	}
	if result.Score < 1 || result.Score > 3 {
		return nil, fmt.Errorf("suggest likelihood: model returned out-of-range score %d", result.Score)
	}

	token, err := s.gateway.SignSuggestion(model.SuggestionFeatureLikelihood, strconv.Itoa(result.Score), result.Reason, result.Confidence)
	if err != nil {
		return nil, fmt.Errorf("suggest likelihood: sign suggestion: %w", err)
	}

	return &model.SuggestLikelihoodResponse{
		Score:      result.Score,
		Reason:     result.Reason,
		Confidence: result.Confidence,
		Token:      token,
	}, nil
}

func (s *likelihoodSuggestionService) resolveTeamName(ctx context.Context, id int) (string, error) {
	if id == 0 {
		return "", nil
	}
	teams, err := s.teamRepo.List(ctx, model.ListTeamsFilter{IncludeInactive: true})
	if err != nil {
		return "", err
	}
	for _, t := range teams {
		if t.ID == id {
			return t.Name, nil
		}
	}
	return "", nil
}

func (s *likelihoodSuggestionService) RecordDecision(ctx context.Context, riskID int, suggestion *model.AILikelihoodSuggestion, finalScore int, decidedBy string) error {
	// Resolve any suggestion left SUGGESTED by an earlier call first — most
	// notably one written by the quarterly re-check sweep
	// (CreateUnresolvedSuggestion), which is never decided by its own writer,
	// only by a human later via this exact reassessment flow. This runs
	// whether or not a *new* suggestion was shown during this save (below):
	// the save itself — reassessing the risk — is the human response the
	// pending row exists to prompt, so it must clear the reminder either way.
	// A no-op at risk creation, since a brand-new risk id can't have an
	// existing pending row yet.
	if err := s.resolvePendingSuggestion(ctx, riskID, finalScore, decidedBy); err != nil {
		return fmt.Errorf("record likelihood suggestion: resolve pending: %w", err)
	}

	if suggestion == nil {
		return nil
	}

	payload, err := s.gateway.VerifySuggestion(suggestion.Token, model.SuggestionFeatureLikelihood)
	if err != nil {
		return fmt.Errorf("record likelihood suggestion: %w", err)
	}
	score, err := strconv.Atoi(payload.Value)
	if err != nil {
		return fmt.Errorf("record likelihood suggestion: suggested score %q: %w", payload.Value, err)
	}

	confidence := strings.ToUpper(payload.Confidence)
	id, err := s.suggestionRepo.Create(ctx, model.CreateSuggestionRequest{
		RiskID:          riskID,
		Feature:         model.SuggestionFeatureLikelihood,
		SuggestedValue:  payload.Value,
		SuggestedReason: payload.Reason,
		Confidence:      confidence,
	})
	if err != nil {
		return fmt.Errorf("record likelihood suggestion: create: %w", err)
	}

	status := model.SuggestionStatusOverridden
	if score == finalScore {
		status = model.SuggestionStatusAccepted
	}

	if err := s.suggestionRepo.Decide(ctx, id, model.DecideSuggestionRequest{
		Status: status,
	}, decidedBy); err != nil {
		return fmt.Errorf("record likelihood suggestion: decide: %w", err)
	}
	return nil
}

// resolvePendingSuggestion decides any existing unresolved (SUGGESTED)
// LIKELIHOOD row for riskID against finalScore — ACCEPTED if the pending
// suggestion's score matches what was actually saved, OVERRIDDEN otherwise.
// No-op if there isn't one. This is what makes PendingSuggestion (the
// in-risk reminder) eventually clear: without it, a row written by
// CreateUnresolvedSuggestion stays SUGGESTED forever, since nothing else
// ever decides it.
func (s *likelihoodSuggestionService) resolvePendingSuggestion(ctx context.Context, riskID int, finalScore int, decidedBy string) error {
	pending, err := s.PendingSuggestion(ctx, riskID)
	if err != nil {
		return err
	}
	if pending == nil {
		return nil
	}

	status := model.SuggestionStatusOverridden
	if pendingScore, err := strconv.Atoi(pending.SuggestedValue); err == nil && pendingScore == finalScore {
		status = model.SuggestionStatusAccepted
	}

	return s.suggestionRepo.Decide(ctx, pending.ID, model.DecideSuggestionRequest{
		Status: status,
	}, decidedBy)
}

func (s *likelihoodSuggestionService) PendingSuggestion(ctx context.Context, riskID int) (*model.Suggestion, error) {
	list, err := s.suggestionRepo.ListByRisk(ctx, riskID, model.SuggestionFeatureLikelihood, string(model.SuggestionStatusSuggested))
	if err != nil {
		return nil, fmt.Errorf("pending likelihood suggestion: %w", err)
	}
	if len(list) == 0 {
		return nil, nil
	}
	// ListByRisk returns newest first.
	return &list[0], nil
}

func (s *likelihoodSuggestionService) CheckedThisQuarter(ctx context.Context, riskID int, quarterStart time.Time) (bool, error) {
	list, err := s.suggestionRepo.ListByRisk(ctx, riskID, model.SuggestionFeatureLikelihood, "")
	if err != nil {
		return false, fmt.Errorf("checked this quarter: %w", err)
	}
	for _, sug := range list {
		if !sug.CreatedAt.Before(quarterStart) {
			return true, nil
		}
	}
	return false, nil
}

func (s *likelihoodSuggestionService) CreateUnresolvedSuggestion(ctx context.Context, riskID int, result *model.SuggestLikelihoodResponse) error {
	_, err := s.suggestionRepo.Create(ctx, model.CreateSuggestionRequest{
		RiskID:          riskID,
		Feature:         model.SuggestionFeatureLikelihood,
		SuggestedValue:  strconv.Itoa(result.Score),
		SuggestedReason: result.Reason,
		Confidence:      strings.ToUpper(result.Confidence),
	})
	if err != nil {
		return fmt.Errorf("create unresolved likelihood suggestion: %w", err)
	}
	return nil
}

func (s *likelihoodSuggestionService) RecordNoChangeCheck(ctx context.Context, riskID int, result *model.SuggestLikelihoodResponse) error {
	id, err := s.suggestionRepo.Create(ctx, model.CreateSuggestionRequest{
		RiskID:          riskID,
		Feature:         model.SuggestionFeatureLikelihood,
		SuggestedValue:  strconv.Itoa(result.Score),
		SuggestedReason: result.Reason,
		Confidence:      strings.ToUpper(result.Confidence),
	})
	if err != nil {
		return fmt.Errorf("record no-change likelihood check: create: %w", err)
	}
	if err := s.suggestionRepo.Decide(ctx, id, model.DecideSuggestionRequest{
		Status: model.SuggestionStatusAccepted,
	}, systemDecider); err != nil {
		return fmt.Errorf("record no-change likelihood check: decide: %w", err)
	}
	return nil
}
