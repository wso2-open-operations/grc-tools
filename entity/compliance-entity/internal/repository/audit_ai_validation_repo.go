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

// AIValidationRepository defines persistence for audit_ai_validation_log
// (append-only). A row belongs to either an evidence round or a population
// round — never both (chk_ai_owner) — so creation and listing are split into
// an evidence-keyed and a population-keyed variant.
type AIValidationRepository interface {
	CreateValidation(ctx context.Context, evidenceID int, req domain.CreateAuditAIValidationLogRequest) (*domain.AuditAIValidationLog, error)
	ListValidationsByEvidence(ctx context.Context, evidenceID int) ([]domain.AuditAIValidationLog, error)
	CreateValidationForPopulation(ctx context.Context, populationID int, req domain.CreateAuditAIValidationLogRequest) (*domain.AuditAIValidationLog, error)
	ListValidationsByPopulation(ctx context.Context, populationID int) ([]domain.AuditAIValidationLog, error)
}

type aiValidationRepo struct{ db *sql.DB }

// NewAIValidationRepository constructs an AIValidationRepository.
func NewAIValidationRepository(db *sql.DB) AIValidationRepository { return &aiValidationRepo{db: db} }

func (r *aiValidationRepo) CreateValidation(ctx context.Context, evidenceID int, req domain.CreateAuditAIValidationLogRequest) (*domain.AuditAIValidationLog, error) {
	return r.create(ctx, &evidenceID, nil, req)
}

func (r *aiValidationRepo) CreateValidationForPopulation(ctx context.Context, populationID int, req domain.CreateAuditAIValidationLogRequest) (*domain.AuditAIValidationLog, error) {
	return r.create(ctx, nil, &populationID, req)
}

func (r *aiValidationRepo) create(ctx context.Context, evidenceID, populationID *int, req domain.CreateAuditAIValidationLogRequest) (*domain.AuditAIValidationLog, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO audit_ai_validation_log
		 (evidence_id, population_id, control_id, result, gaps_found, summary, created_by,
		  input_tokens, output_tokens, cache_read_input_tokens, cache_creation_input_tokens)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		nullableInt(evidenceID),
		nullableInt(populationID),
		req.ControlID,
		req.Result,
		nullableString(req.GapsFound),
		nullableString(req.Summary),
		nullableString(&req.CreatedBy),
		nullableInt64(req.InputTokens),
		nullableInt64(req.OutputTokens),
		nullableInt64(req.CacheReadInputTokens),
		nullableInt64(req.CacheCreationInputTokens),
	)
	if err != nil {
		return nil, fmt.Errorf("ai_validation.Create: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.getValidationByID(ctx, id)
}

func (r *aiValidationRepo) getValidationByID(ctx context.Context, id int64) (*domain.AuditAIValidationLog, error) {
	return scanAIValidation(r.db.QueryRowContext(ctx,
		`SELECT id, evidence_id, population_id, control_id, result, gaps_found, summary,
		        created_by, created_at,
		        input_tokens, output_tokens, cache_read_input_tokens, cache_creation_input_tokens
		 FROM audit_ai_validation_log WHERE id = ?`, id))
}

func (r *aiValidationRepo) ListValidationsByEvidence(ctx context.Context, evidenceID int) ([]domain.AuditAIValidationLog, error) {
	return r.list(ctx, "evidence_id", evidenceID)
}

func (r *aiValidationRepo) ListValidationsByPopulation(ctx context.Context, populationID int) ([]domain.AuditAIValidationLog, error) {
	return r.list(ctx, "population_id", populationID)
}

// list scans rows matching one owner column (evidence_id or population_id) —
// always the same one, never client-provided, so no injection risk from the
// column name.
func (r *aiValidationRepo) list(ctx context.Context, ownerColumn string, ownerID int) ([]domain.AuditAIValidationLog, error) {
	rows, err := r.db.QueryContext(ctx,
		fmt.Sprintf(`SELECT id, evidence_id, population_id, control_id, result, gaps_found, summary,
		        created_by, created_at,
		        input_tokens, output_tokens, cache_read_input_tokens, cache_creation_input_tokens
		 FROM audit_ai_validation_log WHERE %s = ? ORDER BY id DESC`, ownerColumn),
		ownerID)
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
	var evidenceID, populationID sql.NullInt64
	var gaps, summary, createdBy sql.NullString
	var inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens sql.NullInt64
	err := s.Scan(
		&l.ID, &evidenceID, &populationID, &l.ControlID, &l.Result,
		&gaps, &summary, &createdBy, &l.CreatedOn,
		&inputTokens, &outputTokens, &cacheReadTokens, &cacheCreationTokens,
	)
	if err != nil {
		return nil, err
	}
	if evidenceID.Valid {
		v := int(evidenceID.Int64)
		l.EvidenceID = &v
	}
	if populationID.Valid {
		v := int(populationID.Int64)
		l.PopulationID = &v
	}
	if gaps.Valid {
		l.GapsFound = &gaps.String
	}
	if summary.Valid {
		l.Summary = &summary.String
	}
	if createdBy.Valid {
		l.CreatedBy = &createdBy.String
	}
	if inputTokens.Valid {
		l.InputTokens = &inputTokens.Int64
	}
	if outputTokens.Valid {
		l.OutputTokens = &outputTokens.Int64
	}
	if cacheReadTokens.Valid {
		l.CacheReadInputTokens = &cacheReadTokens.Int64
	}
	if cacheCreationTokens.Valid {
		l.CacheCreationInputTokens = &cacheCreationTokens.Int64
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

// nullableInt64 converts a *int64 to sql.NullInt64 for optional BIGINT columns.
func nullableInt64(v *int64) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *v, Valid: true}
}
