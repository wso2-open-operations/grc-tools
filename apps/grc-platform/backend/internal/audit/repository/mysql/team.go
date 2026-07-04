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

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
)

type teamRepository struct{ db *sql.DB }

// NewTeamRepository creates a MySQL-backed repository.TeamRepository.
func NewTeamRepository(db *sql.DB) repository.TeamRepository {
	return &teamRepository{db: db}
}

func (r *teamRepository) List(ctx context.Context) ([]*model.AuditTeam, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name
		FROM audit_team
		WHERE status = 'ACTIVE'
		ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("team.List: %w", err)
	}
	defer rows.Close()

	var teams []*model.AuditTeam
	for rows.Next() {
		var t model.AuditTeam
		if err := rows.Scan(&t.ID, &t.Name); err != nil {
			return nil, fmt.Errorf("team.List scan: %w", err)
		}
		teams = append(teams, &t)
	}
	return teams, rows.Err()
}
