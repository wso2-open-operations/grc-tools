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

package entity

import (
	"context"
	"fmt"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
)

type aiValidationRepo struct{ c *entityclient.Client }

// NewAIValidationRepository returns an entity-backed AIValidationLogRepository.
func NewAIValidationRepository(c *entityclient.Client) repository.AIValidationLogRepository {
	return &aiValidationRepo{c: c}
}

func (r *aiValidationRepo) ListByEvidence(ctx context.Context, evidenceID int) ([]*model.AIValidationLog, error) {
	var resp struct {
		Validations []*model.AIValidationLog `json:"validations"`
	}
	if err := r.c.Get(ctx, fmt.Sprintf("/evidence/%d/ai-validations", evidenceID), &resp); err != nil {
		return nil, err
	}
	return resp.Validations, nil
}
