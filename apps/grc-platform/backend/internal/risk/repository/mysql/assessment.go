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

package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"

	"github.com/wso2-open-operations/grc-platform/backend/internal/apierror"
	"github.com/wso2-open-operations/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-platform/backend/internal/risk/repository"
)

type assessmentRepository struct{ db *sql.DB }

// NewAssessmentRepository creates a MySQL-backed repository.RiskAssessmentRepository.
func NewAssessmentRepository(db *sql.DB) repository.RiskAssessmentRepository {
	return &assessmentRepository{db: db}
}

func (r *assessmentRepository) Create(ctx context.Context, riskID int, req model.CreateAssessmentRequest, assessedBy string) (*model.RiskAssessment, error) {
	var exists int
	err := r.db.QueryRowContext(ctx, "SELECT 1 FROM risk WHERE id = ?", riskID).Scan(&exists)
	if err == sql.ErrNoRows {
		return nil, &apierror.Error{StatusCode: http.StatusNotFound, Body: fmt.Sprintf("risk %d not found", riskID)}
	}
	if err != nil {
		return nil, fmt.Errorf("check risk exists: %w", err)
	}

	var scoreID int
	err = r.db.QueryRowContext(ctx,
		"SELECT id FROM risk_score WHERE likelihood = ? AND impact = ?",
		req.Likelihood, req.Impact,
	).Scan(&scoreID)
	if err != nil {
		return nil, fmt.Errorf("resolve residual score: %w", err)
	}

	res, err := r.db.ExecContext(ctx, `
		INSERT INTO risk_assessment (risk_id, score_id, progress, reassessment_date, assessed_by, created_by)
		VALUES (?, ?, ?, ?, ?, ?)`,
		riskID, scoreID, req.Progress, req.ReassessmentDate, assessedBy, assessedBy,
	)
	if err != nil {
		return nil, fmt.Errorf("insert risk_assessment: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("get assessment id: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT ra.id, ra.risk_id, ra.score_id, ra.progress, ra.reassessment_date,
		       ra.assessed_by, ra.created_at,
		       rs.likelihood, rs.impact, rs.risk_rating, rs.risk_level, rs.color_code
		FROM risk_assessment ra
		JOIN risk_score rs ON rs.id = ra.score_id
		WHERE ra.id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("fetch created assessment: %w", err)
	}
	defer rows.Close()

	if rows.Next() {
		a, err := scanAssessment(rows)
		if err != nil {
			return nil, err
		}
		return a, nil
	}
	return nil, fmt.Errorf("assessment not found after insert")
}

func (r *assessmentRepository) ListByRiskID(ctx context.Context, riskID int) ([]model.RiskAssessment, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT ra.id, ra.risk_id, ra.score_id, ra.progress, ra.reassessment_date,
		       ra.assessed_by, ra.created_at,
		       rs.likelihood, rs.impact, rs.risk_rating, rs.risk_level, rs.color_code
		FROM risk_assessment ra
		JOIN risk_score rs ON rs.id = ra.score_id
		WHERE ra.risk_id = ?
		ORDER BY ra.created_at DESC`, riskID)
	if err != nil {
		return nil, fmt.Errorf("list assessments: %w", err)
	}
	defer rows.Close()

	var out []model.RiskAssessment
	for rows.Next() {
		a, err := scanAssessment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func scanAssessment(rows *sql.Rows) (*model.RiskAssessment, error) {
	var a model.RiskAssessment
	err := rows.Scan(
		&a.ID, &a.RiskID, &a.ScoreID, &a.Progress, &a.ReassessmentDate,
		&a.AssessedBy, &a.CreatedAt,
		&a.ResidualLikelihood, &a.ResidualImpact, &a.ResidualRating,
		&a.ResidualLevel, &a.ResidualColorCode,
	)
	if err != nil {
		return nil, fmt.Errorf("scan assessment: %w", err)
	}
	return &a, nil
}
