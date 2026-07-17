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

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
)

type trailRepo struct{ c *entityclient.Client }

// NewTrailRepository returns an entity-backed TrailRepository.
func NewTrailRepository(c *entityclient.Client) repository.TrailRepository {
	return &trailRepo{c: c}
}

func (r *trailRepo) Create(ctx context.Context, auditID int, controlID, evidenceID *int, action, details, createdBy string) error {
	body := map[string]any{
		"controlId":  controlID,
		"evidenceId": evidenceID,
		"action":     action,
		"createdBy":  createdBy,
	}
	if details != "" {
		body["details"] = details
	}
	return r.c.Post(ctx, fmt.Sprintf("/audits/%d/trail", auditID), body, nil)
}
