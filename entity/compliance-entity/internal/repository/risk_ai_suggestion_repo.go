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

package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

// RiskAISuggestionRepository defines persistence operations for the
// risk_ai_suggestion table.
type RiskAISuggestionRepository interface {
	CreateRiskAISuggestion(ctx context.Context, req domain.CreateRiskAISuggestionRequest) (*domain.RiskAISuggestion, error)
	GetRiskAISuggestionByID(ctx context.Context, id int) (*domain.RiskAISuggestion, error)
	DecideRiskAISuggestion(ctx context.Context, id int, req domain.DecideRiskAISuggestionRequest) (*domain.RiskAISuggestion, error)
	// ListRiskAISuggestionsByRisk returns every suggestion for riskID + feature,
	// newest first. status filters to that status only when non-nil — used
	// both for "does this risk have an unresolved suggestion" (status=SUGGESTED,
	// the in-risk reminder) and "was this risk already checked this quarter"
	// (status=nil — any outcome still counts as "checked", not just a pending
	// one, so an ACCEPTED or OVERRIDDEN row from earlier in the quarter
	// correctly blocks a duplicate live-search call too).
	ListRiskAISuggestionsByRisk(ctx context.Context, riskID int, feature domain.RiskAISuggestionFeature, status *domain.RiskAISuggestionStatus) ([]domain.RiskAISuggestion, error)
}

type riskAISuggestionRepo struct{ db *sql.DB }

// NewRiskAISuggestionRepository constructs a RiskAISuggestionRepository.
func NewRiskAISuggestionRepository(db *sql.DB) RiskAISuggestionRepository {
	return &riskAISuggestionRepo{db: db}
}

const riskAISuggestionColumns = `
	id, risk_id, feature, suggested_value, suggested_reason, confidence,
	status, override_reason, decided_by, decided_at, created_at`

// CreateRiskAISuggestion inserts a new suggestion row, Status always starting
// at SUGGESTED — a suggestion is only ever created the moment it's generated
// and shown, before anyone has decided anything about it.
func (r *riskAISuggestionRepo) CreateRiskAISuggestion(ctx context.Context, req domain.CreateRiskAISuggestionRequest) (*domain.RiskAISuggestion, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO risk_ai_suggestion
			(risk_id, feature, suggested_value, suggested_reason, confidence, status)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		req.RiskID, req.Feature, req.SuggestedValue,
		nullableString(req.SuggestedReason), nullableConfidence(req.Confidence),
		domain.RiskAISuggestionStatusSuggested)
	if err != nil {
		return nil, fmt.Errorf("risk_ai_suggestion.Create: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.GetRiskAISuggestionByID(ctx, int(id))
}

// GetRiskAISuggestionByID returns one suggestion row.
func (r *riskAISuggestionRepo) GetRiskAISuggestionByID(ctx context.Context, id int) (*domain.RiskAISuggestion, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT "+riskAISuggestionColumns+" FROM risk_ai_suggestion WHERE id = ?", id)
	s, err := scanRiskAISuggestion(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("risk AI suggestion %d not found", id)}
	}
	if err != nil {
		return nil, fmt.Errorf("risk_ai_suggestion.GetByID(%d): %w", id, err)
	}
	return s, nil
}

// DecideRiskAISuggestion records that a user accepted or overrode the
// suggestion — the only update this row ever receives; everything else about
// it is immutable once created. The WHERE clause enforces that invariant:
// scoped to status = SUGGESTED, so a row that was already decided (e.g. by a
// concurrent reassessment racing this one) can't be silently overwritten.
func (r *riskAISuggestionRepo) DecideRiskAISuggestion(ctx context.Context, id int, req domain.DecideRiskAISuggestionRequest) (*domain.RiskAISuggestion, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE risk_ai_suggestion
		 SET status = ?, override_reason = ?, decided_by = ?, decided_at = NOW()
		 WHERE id = ? AND status = ?`,
		req.Status, nullableString(req.OverrideReason), req.DecidedBy, id, domain.RiskAISuggestionStatusSuggested)
	if err != nil {
		return nil, fmt.Errorf("risk_ai_suggestion.Decide(%d): %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		// Distinguish "no such row" from "row exists but was already decided"
		// (or never SUGGESTED) — GetRiskAISuggestionByID returns NotFoundError
		// for the former, which we pass straight through.
		if _, getErr := r.GetRiskAISuggestionByID(ctx, id); getErr != nil {
			return nil, getErr
		}
		return nil, &apierror.ConflictError{Msg: fmt.Sprintf("risk AI suggestion %d was already decided", id)}
	}
	return r.GetRiskAISuggestionByID(ctx, id)
}

// ListRiskAISuggestionsByRisk returns every suggestion for riskID + feature,
// newest first, optionally filtered to one status.
func (r *riskAISuggestionRepo) ListRiskAISuggestionsByRisk(ctx context.Context, riskID int, feature domain.RiskAISuggestionFeature, status *domain.RiskAISuggestionStatus) ([]domain.RiskAISuggestion, error) {
	query := "SELECT " + riskAISuggestionColumns + " FROM risk_ai_suggestion WHERE risk_id = ? AND feature = ?"
	args := []any{riskID, feature}
	if status != nil {
		query += " AND status = ?"
		args = append(args, *status)
	}
	query += " ORDER BY created_at DESC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("risk_ai_suggestion.ListByRisk(%d, %s): %w", riskID, feature, err)
	}
	defer rows.Close()

	var out []domain.RiskAISuggestion
	for rows.Next() {
		s, err := scanRiskAISuggestion(rows)
		if err != nil {
			return nil, fmt.Errorf("risk_ai_suggestion.ListByRisk(%d, %s) scan: %w", riskID, feature, err)
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

func scanRiskAISuggestion(s scanner) (*domain.RiskAISuggestion, error) {
	var out domain.RiskAISuggestion
	var suggestedReason, confidence, overrideReason, decidedBy sql.NullString
	var decidedAt sql.NullTime
	if err := s.Scan(
		&out.ID, &out.RiskID, &out.Feature, &out.SuggestedValue, &suggestedReason, &confidence,
		&out.Status, &overrideReason, &decidedBy, &decidedAt, &out.CreatedAt,
	); err != nil {
		return nil, err
	}
	if suggestedReason.Valid {
		out.SuggestedReason = &suggestedReason.String
	}
	if confidence.Valid {
		c := domain.RiskAISuggestionConfidence(confidence.String)
		out.Confidence = &c
	}
	if overrideReason.Valid {
		out.OverrideReason = &overrideReason.String
	}
	if decidedBy.Valid {
		out.DecidedBy = &decidedBy.String
	}
	if decidedAt.Valid {
		out.DecidedAt = &decidedAt.Time
	}
	return &out, nil
}

func nullableConfidence(c *domain.RiskAISuggestionConfidence) any {
	if c == nil {
		return nil
	}
	return *c
}
