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

// Package model defines the domain types for the Audit Hub module.
package model

import "time"

// AIValidationLog is one AI validation run against an evidence or population
// submission, mirrored from the Compliance Entity's audit_ai_validation_log.
// It is advisory only — the frontend renders the latest row as a review hint
// and never gates the workflow on it. Exactly one of EvidenceID /
// PopulationID is set. Result is PASS | FAIL | UNCERTAIN | PENDING | ERROR |
// SKIPPED.
type AIValidationLog struct {
	ID           int64     `json:"id"`
	EvidenceID   *int      `json:"evidenceId"`
	PopulationID *int      `json:"populationId"`
	ControlID    int       `json:"controlId"`
	Result       string    `json:"result"`
	GapsFound    *string   `json:"gapsFound"` // JSON array of gap objects
	Summary      *string   `json:"summary"`
	CreatedBy    *string   `json:"createdBy"`
	CreatedOn    time.Time `json:"createdOn"`
	// Anthropic token accounting for this call (nil on lifecycle rows that
	// never reached a completed LLM response). CacheReadInputTokens vs
	// InputTokens shows how much of the static system prompt was served
	// from Anthropic's prompt cache rather than billed at full price.
	InputTokens              *int64 `json:"inputTokens,omitempty"`
	OutputTokens             *int64 `json:"outputTokens,omitempty"`
	CacheReadInputTokens     *int64 `json:"cacheReadInputTokens,omitempty"`
	CacheCreationInputTokens *int64 `json:"cacheCreationInputTokens,omitempty"`
}

// AIValidationListResponse is the payload of
// GET /api/v1/audits/{id}/controls/{controlId}/evidence/{evidenceId}/ai-validations
// and GET /api/v1/audits/{id}/controls/{controlId}/population/{populationId}/ai-validations
// (latest row first).
type AIValidationListResponse struct {
	Validations []*AIValidationLog `json:"validations"`
}

// CreateAIValidationLogRequest is the body posted to the entity's
// POST .../ai-validations routes (evidence and population share this shape;
// the owning id is in the path, not here — see repository/entity/aivalidation.go).
type CreateAIValidationLogRequest struct {
	ControlID int     `json:"controlId"`
	Result    string  `json:"result"` // PASS | FAIL | UNCERTAIN | PENDING | ERROR | SKIPPED
	GapsFound *string `json:"gapsFound"`
	Summary   *string `json:"summary"`
	CreatedBy string  `json:"createdBy"`
	// Anthropic token accounting for this call; left nil on lifecycle rows
	// (PENDING/SKIPPED/ERROR) that never reached a completed LLM response.
	InputTokens              *int64 `json:"inputTokens,omitempty"`
	OutputTokens             *int64 `json:"outputTokens,omitempty"`
	CacheReadInputTokens     *int64 `json:"cacheReadInputTokens,omitempty"`
	CacheCreationInputTokens *int64 `json:"cacheCreationInputTokens,omitempty"`
}
