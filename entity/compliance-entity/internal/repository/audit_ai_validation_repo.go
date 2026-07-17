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
	"fmt"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

// AIValidationRepository defines persistence for audit_ai_validation_log (append-only).
type AIValidationRepository interface {
	CreateValidation(ctx context.Context, evidenceID int, req domain.CreateAuditAIValidationLogRequest) (*domain.AuditAIValidationLog, error)
	ListValidationsByEvidence(ctx context.Context, evidenceID int) ([]domain.AuditAIValidationLog, error)
}

type aiValidationRepo struct{ db *sql.DB }

// NewAIValidationRepository constructs an AIValidationRepository.
func NewAIValidationRepository(db *sql.DB) AIValidationRepository { return &aiValidationRepo{db: db} }

func (r *aiValidationRepo) CreateValidation(ctx context.Context, evidenceID int, req domain.CreateAuditAIValidationLogRequest) (*domain.AuditAIValidationLog, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO audit_ai_validation_log
		 (evidence_id, control_id, result, gaps_found, feedback, summary, confidence_score, created_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		evidenceID,
		req.ControlID,
		req.Result,
		nullableString(req.GapsFound),
		nullableString(req.Feedback),
		nullableString(req.Summary),
		nullableFloat(req.ConfidenceScore),
		nullableString(&req.CreatedBy),
	)
	if err != nil {
		return nil, fmt.Errorf("ai_validation.Create: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.getValidationByID(ctx, id)
}

func (r *aiValidationRepo) getValidationByID(ctx context.Context, id int64) (*domain.AuditAIValidationLog, error) {
	return scanAIValidation(r.db.QueryRowContext(ctx,
		`SELECT id, evidence_id, control_id, result, gaps_found, feedback, summary,
		        confidence_score, created_by, created_at
		 FROM audit_ai_validation_log WHERE id = ?`, id))
}

func (r *aiValidationRepo) ListValidationsByEvidence(ctx context.Context, evidenceID int) ([]domain.AuditAIValidationLog, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, evidence_id, control_id, result, gaps_found, feedback, summary,
		        confidence_score, created_by, created_at
		 FROM audit_ai_validation_log WHERE evidence_id = ? ORDER BY id DESC`,
		evidenceID)
	if err != nil {
		return nil, fmt.Errorf("ai_validation.List: %w", err)
	}
	defer rows.Close()

	var logs []domain.AuditAIValidationLog
	for rows.Next() {
		l, err := scanAIValidation(rows)
		if err != nil {
			return nil, fmt.Errorf("ai_validation.List scan: %w", err)
		}
		logs = append(logs, *l)
	}
	return logs, rows.Err()
}

func scanAIValidation(s scanner) (*domain.AuditAIValidationLog, error) {
	var l domain.AuditAIValidationLog
	var gaps, feedback, summary, createdBy sql.NullString
	var confidence sql.NullFloat64
	err := s.Scan(
		&l.ID, &l.EvidenceID, &l.ControlID, &l.Result,
		&gaps, &feedback, &summary, &confidence, &createdBy, &l.CreatedOn,
	)
	if err != nil {
		return nil, err
	}
	if gaps.Valid {
		l.GapsFound = &gaps.String
	}
	if feedback.Valid {
		l.Feedback = &feedback.String
	}
	if summary.Valid {
		l.Summary = &summary.String
	}
	if confidence.Valid {
		l.ConfidenceScore = &confidence.Float64
	}
	if createdBy.Valid {
		l.CreatedBy = &createdBy.String
	}
	return &l, nil
}

// nullableFloat converts a *float64 to sql.NullFloat64 for optional DECIMAL columns.
func nullableFloat(v *float64) sql.NullFloat64 {
	if v == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: *v, Valid: true}
}
