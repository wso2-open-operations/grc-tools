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
	"errors"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/repository"
)

// errNotImplemented is returned by repository stubs that are not yet implemented.
// Callers receive a clear error rather than a silent nil/empty result.
var errNotImplemented = errors.New("not implemented")

type actionPlanRepository struct{ db *sql.DB }

// NewActionPlanRepository creates a MySQL-backed repository.ActionPlanRepository.
func NewActionPlanRepository(db *sql.DB) repository.ActionPlanRepository {
	return &actionPlanRepository{db: db}
}

func (r *actionPlanRepository) List(ctx context.Context, riskID int) ([]*model.ActionPlan, error) {
	// TODO: implement
	return nil, errNotImplemented
}

func (r *actionPlanRepository) GetByID(ctx context.Context, planID int) (*model.ActionPlan, error) {
	// TODO: implement
	return nil, errNotImplemented
}

func (r *actionPlanRepository) Create(ctx context.Context, riskID int, req model.CreateActionPlanRequest, createdBy string) (*model.ActionPlan, error) {
	// TODO: implement
	return nil, errNotImplemented
}

func (r *actionPlanRepository) Update(ctx context.Context, planID int, req model.UpdateActionPlanRequest, updatedBy string) error {
	// TODO: implement
	return errNotImplemented
}

func (r *actionPlanRepository) ListSteps(ctx context.Context, planID int) ([]*model.ActionPlanStep, error) {
	// TODO: implement
	return nil, errNotImplemented
}

func (r *actionPlanRepository) AddStep(ctx context.Context, planID, stepNo int, req model.AddActionPlanStepRequest, createdBy string) (*model.ActionPlanStep, error) {
	// TODO: implement
	return nil, errNotImplemented
}

func (r *actionPlanRepository) UpdateStep(ctx context.Context, stepID int, req model.UpdateActionPlanStepRequest, updatedBy string) error {
	// TODO: implement
	return errNotImplemented
}
