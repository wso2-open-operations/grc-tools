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

type userRepository struct{ db *sql.DB }

// NewUserRepository creates a MySQL-backed repository.UserRepository.
// Reads from the shared `user` table (defined in shared.sql).
func NewUserRepository(db *sql.DB) repository.UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) List(ctx context.Context) ([]*model.UserRef, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, display_name, email
		FROM `+"`user`"+`
		WHERE status = 'ACTIVE'
		ORDER BY display_name`)
	if err != nil {
		return nil, fmt.Errorf("user.List: %w", err)
	}
	defer rows.Close()

	var users []*model.UserRef
	for rows.Next() {
		var (
			id          int
			displayName string
			email       string
		)
		if err := rows.Scan(&id, &displayName, &email); err != nil {
			return nil, fmt.Errorf("user.List scan: %w", err)
		}
		users = append(users, &model.UserRef{
			ID:          id,
			DisplayName: displayName,
			Email:       email,
			ProfileURL:  nil, // populated from Asgardeo when integrated
		})
	}
	return users, rows.Err()
}
