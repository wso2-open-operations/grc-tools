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

package model

import "time"

// SuggestionFeature values — mirrors the entity's RiskAISuggestionFeature
// ENUM.
const (
	SuggestionFeatureCategory   = "CATEGORY"
	SuggestionFeatureLikelihood = "LIKELIHOOD"
	SuggestionFeatureActionPlan = "ACTION_PLAN"
)

// Suggestion is the read model for one risk_ai_suggestion row, returned by
// repository.SuggestionRepository.ListByRisk — powers the
// in-risk "may need reassessment" reminder and the quarterly sweep's
// already-checked-this-quarter guard.
type Suggestion struct {
	ID              int        `json:"id"`
	RiskID          int        `json:"risk_id"`
	Feature         string     `json:"feature"`
	SuggestedValue  string     `json:"suggested_value"`
	SuggestedReason string     `json:"suggested_reason"`
	Confidence      string     `json:"confidence"`
	Status          string     `json:"status"`
	CreatedAt       time.Time  `json:"created_at"`
	DecidedAt       *time.Time `json:"decided_at"`
}

// SuggestCategoryRequest is the payload for
// POST /api/v1/risks/categories/suggest. Stateless — nothing is persisted by
// this call. This matters because on Add Risk, the risk the suggestion is
// about doesn't have an id yet (it isn't created until the form is
// submitted); persisting at suggest-time would have nothing to attach the
// row to. See AICategorySuggestion below for where it actually gets recorded.
type SuggestCategoryRequest struct {
	Title                  string `json:"title"`
	Description            string `json:"description"`
	ComplianceReferenceIDs []int  `json:"compliance_reference_ids"`
}

// SuggestCategoryResponse is what the form shows inline and pre-fills the
// Risk Category select with.
type SuggestCategoryResponse struct {
	CategoryID   int    `json:"category_id"`
	CategoryName string `json:"category_name"`
	Reason       string `json:"reason"`
	Confidence   string `json:"confidence"`
}

// AICategorySuggestion is the suggestion snapshot the frontend includes in
// the risk Create/Update request body when a suggestion was shown — whether
// the risk is being created for the first time (no risk id existed when the
// suggestion was generated) or edited (one did). Nil when the user never
// clicked "Suggest category" at all, in which case nothing is written to
// risk_ai_suggestion for this save.
//
// The service compares CategoryID against the risk's final, actually-chosen
// category to decide ACCEPTED vs OVERRIDDEN — the frontend does not send that
// decision directly, so it can't be spoofed or drift out of sync with what
// was actually saved.
type AICategorySuggestion struct {
	CategoryID int    `json:"category_id"`
	Reason     string `json:"reason"`
	Confidence string `json:"confidence"`
}

// SuggestionStatus mirrors the entity's RiskAISuggestionStatus — only the two
// values a human decision can produce; a fresh suggestion starts at
// "SUGGESTED" server-side (repository.SuggestionRepository.Create) and is
// never set to that from here.
type SuggestionStatus string

const (
	SuggestionStatusAccepted   SuggestionStatus = "ACCEPTED"
	SuggestionStatusOverridden SuggestionStatus = "OVERRIDDEN"
	// SuggestionStatusSuggested is never set from here — a fresh suggestion
	// starts at this status server-side (compliance-entity's Create). It
	// exists on this side purely to query by it (repository.SuggestionRepository.
	// ListByRisk's status filter), e.g. PendingSuggestion's "is there an
	// unresolved one" check.
	SuggestionStatusSuggested SuggestionStatus = "SUGGESTED"
)

// DecideSuggestionRequest is the internal (not HTTP-facing) payload the risk
// service builds to record a decision, immediately after persisting a fresh
// suggestion row at risk-save time — see recordCategorySuggestion.
type DecideSuggestionRequest struct {
	Status         SuggestionStatus
	OverrideReason *string
}

// CreateSuggestionRequest is the internal (not HTTP-facing) payload the risk
// service builds from an AI Gateway result (held in the save request as
// AICategorySuggestion) to persist it — the caller never constructs this
// directly.
type CreateSuggestionRequest struct {
	RiskID          int
	Feature         string // "CATEGORY" | "LIKELIHOOD"
	SuggestedValue  string
	SuggestedReason string
	Confidence      string // "HIGH" | "MEDIUM" | "LOW"
}

// SuggestLikelihoodRequest is the payload for
// POST /api/v1/risks/likelihood/suggest. Stateless, same reasoning as
// SuggestCategoryRequest — nothing is persisted until the risk/assessment
// itself is saved. Used for both the Gross suggestion (risk creation) and the
// Residual re-check (reassessment) — same six inputs either way.
type SuggestLikelihoodRequest struct {
	Title                  string `json:"title"`
	Description            string `json:"description"`
	ImpactDescription      string `json:"impact_description"`
	ComplianceReferenceIDs []int  `json:"compliance_reference_ids"`
	CategoryID             int    `json:"category_id"`
	SourceRegisterID       int    `json:"source_register_id"`
}

// SuggestLikelihoodResponse is what the form shows inline and highlights the
// matrix row for.
type SuggestLikelihoodResponse struct {
	Score      int    `json:"score"`
	Reason     string `json:"reason"`
	Confidence string `json:"confidence"`
}

// AILikelihoodSuggestion is the suggestion snapshot the frontend includes in
// the risk-create payload (Gross) or the reassessment payload (Residual) —
// same pattern as AICategorySuggestion, same reason: the risk/assessment row
// this attaches to doesn't exist yet at suggest-time. Nil when the user never
// clicked "Suggest Likelihood".
type AILikelihoodSuggestion struct {
	Score      int    `json:"score"`
	Reason     string `json:"reason"`
	Confidence string `json:"confidence"`
}

// SuggestActionPlanRequest is the payload for
// POST /api/v1/risks/action-plans/suggest. Stateless, same reasoning as
// SuggestCategoryRequest/SuggestLikelihoodRequest — nothing is persisted
// until the risk or action plan itself is saved. Used both on the Add Risk
// wizard's Action Plan step and the standalone "add another action plan"
// dialog for an already-in-remediation risk.
type SuggestActionPlanRequest struct {
	Title                  string `json:"title"`
	Description            string `json:"description"`
	CategoryID             int    `json:"category_id"`
	ComplianceReferenceIDs []int  `json:"compliance_reference_ids"`
	TreatmentStrategy      string `json:"treatment_strategy"`
}

// SuggestActionPlanResponse is what the form shows inline and offers to copy
// into the Action Plan Description field.
type SuggestActionPlanResponse struct {
	Description string `json:"description"`
	Reason      string `json:"reason"`
	Confidence  string `json:"confidence"`
}

// AIActionPlanSuggestion is the suggestion snapshot the frontend includes in
// the risk-create payload or the create-action-plan payload when a
// suggestion was shown — same pattern as AICategorySuggestion/
// AILikelihoodSuggestion, nil when the user never clicked "Suggest action
// plan".
//
// Unlike Category/Likelihood, there is no fixed suggested value to compare
// against what was finally saved here — "accepting" this suggestion means
// copying free text into a field, which the user may then edit further, so
// there's nothing for the service to diff. Instead the frontend tracks
// directly whether the user clicked "use this suggestion" and sends that as
// Used — the service sets ACCEPTED/OVERRIDDEN straight from that flag, no
// comparison logic (see ActionPlanSuggestionService.RecordDecision).
type AIActionPlanSuggestion struct {
	Description string `json:"description"`
	Reason      string `json:"reason"`
	Confidence  string `json:"confidence"`
	Used        bool   `json:"used"`
}
