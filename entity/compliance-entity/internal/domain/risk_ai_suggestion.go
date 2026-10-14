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

package domain

import "time"

// RiskAISuggestionFeature is which of the AI features produced a suggestion
// row. CATEGORY, LIKELIHOOD, and ACTION_PLAN are all written by code today —
// see risk_ai_suggestion's schema in
// apps/grc-platform/backend/Resources/risk_schema.sql.
type RiskAISuggestionFeature string

const (
	RiskAISuggestionFeatureCategory   RiskAISuggestionFeature = "CATEGORY"
	RiskAISuggestionFeatureLikelihood RiskAISuggestionFeature = "LIKELIHOOD"
	RiskAISuggestionFeatureActionPlan RiskAISuggestionFeature = "ACTION_PLAN"
)

// RiskAISuggestionStatus is what happened to a suggestion after it was shown.
type RiskAISuggestionStatus string

const (
	RiskAISuggestionStatusSuggested  RiskAISuggestionStatus = "SUGGESTED"
	RiskAISuggestionStatusAccepted   RiskAISuggestionStatus = "ACCEPTED"
	RiskAISuggestionStatusOverridden RiskAISuggestionStatus = "OVERRIDDEN"
)

// RiskAISuggestionConfidence is the model's own self-reported confidence —
// not a calibrated probability.
type RiskAISuggestionConfidence string

const (
	RiskAISuggestionConfidenceHigh   RiskAISuggestionConfidence = "HIGH"
	RiskAISuggestionConfidenceMedium RiskAISuggestionConfidence = "MEDIUM"
	RiskAISuggestionConfidenceLow    RiskAISuggestionConfidence = "LOW"
)

// RiskAISuggestion represents one row in risk_ai_suggestion —
// one AI suggestion shown against a risk. One row per suggestion shown, not
// per risk: a risk may accumulate several over time.
type RiskAISuggestion struct {
	ID              int                         `json:"id"`
	RiskID          int                         `json:"riskId"`
	Feature         RiskAISuggestionFeature     `json:"feature"`
	SuggestedValue  string                      `json:"suggestedValue"`
	SuggestedReason *string                     `json:"suggestedReason"`
	Confidence      *RiskAISuggestionConfidence `json:"confidence"`
	Status          RiskAISuggestionStatus      `json:"status"`
	OverrideReason  *string                     `json:"overrideReason"`
	DecidedBy       *string                     `json:"decidedBy"`
	DecidedAt       *time.Time                  `json:"decidedAt"`
	CreatedAt       time.Time                   `json:"createdAt"`
}

// CreateRiskAISuggestionRequest is the payload for
// POST /risk/ai-suggestions — written the moment a suggestion is generated
// and shown to a user, before they've decided anything (Status starts at
// SUGGESTED).
type CreateRiskAISuggestionRequest struct {
	RiskID          int                         `json:"riskId"`
	Feature         RiskAISuggestionFeature     `json:"feature"`
	SuggestedValue  string                      `json:"suggestedValue"`
	SuggestedReason *string                     `json:"suggestedReason"`
	Confidence      *RiskAISuggestionConfidence `json:"confidence"`
}

// DecideRiskAISuggestionRequest is the payload for
// PATCH /risk/ai-suggestions/{id}/decide — recorded once the user accepts the
// suggestion as-is, or picks something else (OverrideReason optional either
// way; enforced as required for LIKELIHOOD at the service layer that calls
// this, not here, since that rule may change and shouldn't cost a schema or
// API-contract change when it does).
type DecideRiskAISuggestionRequest struct {
	Status         RiskAISuggestionStatus `json:"status"`
	OverrideReason *string                `json:"overrideReason"`
	DecidedBy      string                 `json:"decidedBy"`
}
