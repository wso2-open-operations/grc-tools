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

package entity

import (
	"context"
	"fmt"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
)

type deadlineDaysRepo struct{ c *entityclient.Client }

// NewDeadlineDaysRepository constructs a DeadlineDaysRepository backed by the Compliance Entity.
func NewDeadlineDaysRepository(c *entityclient.Client) repository.DeadlineDaysRepository {
	return &deadlineDaysRepo{c: c}
}

func (r *deadlineDaysRepo) Get(ctx context.Context, auditID int) (*model.DeadlineDays, error) {
	var out model.DeadlineDays
	if err := r.c.Get(ctx, fmt.Sprintf("/audits/%d/deadline-days", auditID), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *deadlineDaysRepo) Put(ctx context.Context, auditID int, rows []model.DeadlineDaysRow, updatedBy string) (*model.DeadlineDays, error) {
	body := map[string]any{"days": rows, "updatedBy": updatedBy}
	var out model.DeadlineDays
	if err := r.c.Put(ctx, fmt.Sprintf("/audits/%d/deadline-days", auditID), body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
