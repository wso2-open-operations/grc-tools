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
	"strings"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

// RiskActionStepRepository defines persistence for risk_action_step.
type RiskActionStepRepository interface {
	CreateRiskActionStep(ctx context.Context, planID int, req domain.CreateRiskActionStepRequest) (*domain.RiskActionStep, error)
	GetRiskActionStepByID(ctx context.Context, planID, stepID int) (*domain.RiskActionStep, error)
	UpdateRiskActionStep(ctx context.Context, planID, stepID int, req domain.UpdateRiskActionStepRequest) (*domain.RiskActionStep, error)
	DeleteRiskActionStep(ctx context.Context, planID, stepID int) error
	ListRiskActionSteps(ctx context.Context, planID int) ([]domain.RiskActionStep, error)
}

type riskActionStepRepo struct{ db *sql.DB }

// NewRiskActionStepRepository constructs a RiskActionStepRepository.
func NewRiskActionStepRepository(db *sql.DB) RiskActionStepRepository {
	return &riskActionStepRepo{db: db}
}

func (r *riskActionStepRepo) CreateRiskActionStep(ctx context.Context, planID int, req domain.CreateRiskActionStepRequest) (*domain.RiskActionStep, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO risk_action_step (plan_id, step_no, description, status, created_by, updated_by)
		 VALUES (?, ?, ?, 'PENDING', ?, ?)`,
		planID, req.StepNo, nullableString(req.Description), req.CreatedBy, req.CreatedBy)
	if err != nil {
		return nil, fmt.Errorf("risk_action_step.Create: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.GetRiskActionStepByID(ctx, planID, int(id))
}

func (r *riskActionStepRepo) GetRiskActionStepByID(ctx context.Context, planID, stepID int) (*domain.RiskActionStep, error) {
	s, err := scanRiskActionStep(r.db.QueryRowContext(ctx,
		`SELECT id, plan_id, step_no, description, status,
		        DATE_FORMAT(completed_date,'%Y-%m-%d'), created_at, updated_at
		 FROM risk_action_step WHERE id = ? AND plan_id = ?`, stepID, planID))
	if err == sql.ErrNoRows {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("action step %d not found", stepID)}
	}
	if err != nil {
		return nil, fmt.Errorf("risk_action_step.GetByID(%d): %w", stepID, err)
	}
	return s, nil
}

func (r *riskActionStepRepo) UpdateRiskActionStep(ctx context.Context, planID, stepID int, req domain.UpdateRiskActionStepRequest) (*domain.RiskActionStep, error) {
	sets := []string{}
	args := []any{}

	if req.StepNo != nil {
		sets = append(sets, "step_no = ?")
		args = append(args, *req.StepNo)
	}
	if req.Description != nil {
		sets = append(sets, "description = ?")
		args = append(args, *req.Description)
	}
	if req.Status != nil {
		sets = append(sets, "status = ?")
		args = append(args, *req.Status)
	}
	if req.CompletedDate != nil {
		sets = append(sets, "completed_date = ?")
		args = append(args, *req.CompletedDate)
	}
	sets = append(sets, "updated_by = ?")
	args = append(args, req.UpdatedBy)
	args = append(args, stepID, planID)

	if _, err := r.db.ExecContext(ctx,
		"UPDATE risk_action_step SET "+strings.Join(sets, ", ")+" WHERE id = ? AND plan_id = ?", args...); err != nil { // #nosec G202
		return nil, fmt.Errorf("risk_action_step.Update(%d): %w", stepID, err)
	}
	return r.GetRiskActionStepByID(ctx, planID, stepID)
}

func (r *riskActionStepRepo) DeleteRiskActionStep(ctx context.Context, planID, stepID int) error {
	result, err := r.db.ExecContext(ctx,
		"DELETE FROM risk_action_step WHERE id = ? AND plan_id = ?", stepID, planID)
	if err != nil {
		return fmt.Errorf("risk_action_step.Delete(%d): %w", stepID, err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return &apierror.NotFoundError{Msg: fmt.Sprintf("action step %d not found", stepID)}
	}
	return nil
}

func (r *riskActionStepRepo) ListRiskActionSteps(ctx context.Context, planID int) ([]domain.RiskActionStep, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, plan_id, step_no, description, status,
		        DATE_FORMAT(completed_date,'%Y-%m-%d'), created_at, updated_at
		 FROM risk_action_step WHERE plan_id = ? ORDER BY step_no ASC`, planID)
	if err != nil {
		return nil, fmt.Errorf("risk_action_step.List: %w", err)
	}
	defer rows.Close()

	var steps []domain.RiskActionStep
	for rows.Next() {
		s, err := scanRiskActionStep(rows)
		if err != nil {
			return nil, fmt.Errorf("risk_action_step.List scan: %w", err)
		}
		steps = append(steps, *s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("risk_action_step.List rows: %w", err)
	}
	return steps, nil
}

func scanRiskActionStep(s scanner) (*domain.RiskActionStep, error) {
	var step domain.RiskActionStep
	var desc, completedDate sql.NullString
	err := s.Scan(
		&step.ID, &step.PlanID, &step.StepNo,
		&desc, &step.Status, &completedDate,
		&step.CreatedOn, &step.UpdatedOn,
	)
	if err != nil {
		return nil, err
	}
	if desc.Valid {
		step.Description = &desc.String
	}
	if completedDate.Valid {
		step.CompletedDate = &completedDate.String
	}
	return &step, nil
}
