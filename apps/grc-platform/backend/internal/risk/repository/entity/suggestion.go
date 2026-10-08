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

package entity

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
)

type suggestionRepository struct{ c *entityclient.Client }

// NewSuggestionRepository creates a Compliance Entity-backed
// repository.SuggestionRepository.
func NewSuggestionRepository(c *entityclient.Client) repository.SuggestionRepository {
	return &suggestionRepository{c: c}
}

type entSuggestion struct {
	ID int `json:"id"`
}

// Create persists a freshly-generated suggestion via the entity's
// POST /risk/ai-suggestions, returning its new id.
func (r *suggestionRepository) Create(ctx context.Context, req model.CreateSuggestionRequest) (int, error) {
	body := map[string]any{
		"riskId":          req.RiskID,
		"feature":         req.Feature,
		"suggestedValue":  req.SuggestedValue,
		"suggestedReason": req.SuggestedReason,
		"confidence":      req.Confidence,
	}
	var s entSuggestion
	if err := r.c.Post(ctx, "/risk/ai-suggestions", body, &s); err != nil {
		return 0, fmt.Errorf("create risk ai suggestion: %w", err)
	}
	return s.ID, nil
}

// Decide records the user's accept/override decision via the entity's
// PATCH /risk/ai-suggestions/{id}/decide.
func (r *suggestionRepository) Decide(ctx context.Context, id int, req model.DecideSuggestionRequest, decidedBy string) error {
	body := map[string]any{
		"status":         req.Status,
		"overrideReason": req.OverrideReason,
		"decidedBy":      decidedBy,
	}
	if err := r.c.Patch(ctx, fmt.Sprintf("/risk/ai-suggestions/%d/decide", id), body, nil); err != nil {
		return fmt.Errorf("decide risk ai suggestion %d: %w", id, err)
	}
	return nil
}

type entSuggestionDetail struct {
	ID              int     `json:"id"`
	RiskID          int     `json:"riskId"`
	Feature         string  `json:"feature"`
	SuggestedValue  string  `json:"suggestedValue"`
	SuggestedReason *string `json:"suggestedReason"`
	Confidence      *string `json:"confidence"`
	Status          string  `json:"status"`
	CreatedAt       string  `json:"createdAt"`
	DecidedAt       *string `json:"decidedAt"`
}

type listSuggestionsResponse struct {
	Suggestions []entSuggestionDetail `json:"suggestions"`
}

// ListByRisk fetches every suggestion for riskID + feature via the entity's
// GET /risk/ai-suggestions, optionally filtered to one status.
func (r *suggestionRepository) ListByRisk(ctx context.Context, riskID int, feature string, status string) ([]model.Suggestion, error) {
	q := url.Values{}
	q.Set("riskId", fmt.Sprintf("%d", riskID))
	q.Set("feature", feature)
	if status != "" {
		q.Set("status", status)
	}

	var resp listSuggestionsResponse
	if err := r.c.Get(ctx, "/risk/ai-suggestions?"+q.Encode(), &resp); err != nil {
		return nil, fmt.Errorf("list risk ai suggestions (risk=%d, feature=%s): %w", riskID, feature, err)
	}

	out := make([]model.Suggestion, 0, len(resp.Suggestions))
	for _, s := range resp.Suggestions {
		m := model.Suggestion{
			ID:             s.ID,
			RiskID:         s.RiskID,
			Feature:        s.Feature,
			SuggestedValue: s.SuggestedValue,
			Status:         s.Status,
		}
		if s.SuggestedReason != nil {
			m.SuggestedReason = *s.SuggestedReason
		}
		if s.Confidence != nil {
			m.Confidence = *s.Confidence
		}
		if t, err := time.Parse(time.RFC3339, s.CreatedAt); err == nil {
			m.CreatedAt = t
		}
		if s.DecidedAt != nil {
			if t, err := time.Parse(time.RFC3339, *s.DecidedAt); err == nil {
				m.DecidedAt = &t
			}
		}
		out = append(out, m)
	}
	return out, nil
}
