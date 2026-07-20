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

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/user"
)

type repository struct{ db *sql.DB }

// NewRepository creates a MySQL-backed user.Repository.
func NewRepository(db *sql.DB) user.Repository {
	return &repository{db: db}
}

func (r *repository) GetByEmail(ctx context.Context, email string) (*user.User, error) {
	u := &user.User{}
	err := r.db.QueryRowContext(ctx,
		"SELECT id, display_name, email, status FROM user WHERE email = ? AND status != 'REMOVED'",
		email,
	).Scan(&u.ID, &u.DisplayName, &u.Email, &u.Status)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return u, nil
}

func (r *repository) GetByID(ctx context.Context, id int) (*user.User, error) {
	u := &user.User{}
	err := r.db.QueryRowContext(ctx,
		"SELECT id, display_name, email, status FROM user WHERE id = ? AND status != 'REMOVED'",
		id,
	).Scan(&u.ID, &u.DisplayName, &u.Email, &u.Status)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return u, nil
}

// Upsert creates a user row for email if one doesn't exist yet, or refreshes
// display_name if it does. Used to provision an account for a
// employee (from an HR entity search) picked as e.g. a risk's Action Owner,
// who may never have logged into grc-platform before.
//
// TODO(user-management): these created rows only have email/display_name/
// status set — once admin user management exists, give an admin a way to view
// and edit them (team assignment, status, etc.) directly.
func (r *repository) Upsert(ctx context.Context, email, displayName string) (*user.User, error) {
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO user (email, display_name, status) VALUES (?, ?, 'ACTIVE')
		 ON DUPLICATE KEY UPDATE display_name = VALUES(display_name), id = LAST_INSERT_ID(id)`,
		email, displayName,
	)
	if err != nil {
		return nil, fmt.Errorf("upsert user: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("get upserted user id: %w", err)
	}
	return r.GetByID(ctx, int(id))
}

func (r *repository) List(ctx context.Context) ([]*user.User, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, display_name, email, status FROM user WHERE status = 'ACTIVE' ORDER BY display_name",
	)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []*user.User
	for rows.Next() {
		u := &user.User{}
		if err := rows.Scan(&u.ID, &u.DisplayName, &u.Email, &u.Status); err != nil {
			return nil, fmt.Errorf("scan user row: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}
