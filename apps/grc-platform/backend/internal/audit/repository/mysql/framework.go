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
	"fmt"
	"net/http"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
)

type frameworkRepository struct{ db *sql.DB }

// NewFrameworkRepository creates a MySQL-backed repository.FrameworkRepository.
func NewFrameworkRepository(db *sql.DB) repository.FrameworkRepository {
	return &frameworkRepository{db: db}
}

func (r *frameworkRepository) List(ctx context.Context) ([]*model.AuditFramework, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, version, status, created_at, updated_at
		FROM audit_framework
		WHERE status = 'ACTIVE'
		ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("framework.List: %w", err)
	}
	defer rows.Close()

	var frameworks []*model.AuditFramework
	for rows.Next() {
		f, err := scanFramework(rows)
		if err != nil {
			return nil, fmt.Errorf("framework.List scan: %w", err)
		}
		frameworks = append(frameworks, f)
	}
	return frameworks, rows.Err()
}

func (r *frameworkRepository) GetByID(ctx context.Context, id int) (*model.AuditFramework, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT id, name, version, status, created_at, updated_at FROM audit_framework WHERE id = ?", id)
	f, err := scanFramework(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("framework.GetByID(%d): %w", id, err)
	}
	return f, nil
}

func (r *frameworkRepository) Create(ctx context.Context, req model.CreateFrameworkRequest, createdBy string) (*model.AuditFramework, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO audit_framework (name, version, status, created_by, updated_by)
		VALUES (?, ?, 'ACTIVE', ?, ?)`,
		req.Name, stringPtrVal(req.Version), createdBy, createdBy,
	)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return nil, &apierror.Error{StatusCode: http.StatusConflict, Body: "A framework with this name already exists."}
		}
		return nil, fmt.Errorf("framework.Create: %w", err)
	}
	id64, err := res.LastInsertId()
	if err != nil || id64 == 0 {
		return nil, fmt.Errorf("framework.Create get insert id: %w", err)
	}
	return r.GetByID(ctx, int(id64))
}

func scanFramework(s scanner) (*model.AuditFramework, error) {
	var (
		id        int
		name      string
		version   sql.NullString
		status    string
		createdAt time.Time
		updatedAt time.Time
	)
	err := s.Scan(&id, &name, &version, &status, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	return &model.AuditFramework{
		ID:        id,
		Name:      name,
		Version:   nullStringPtr(version),
		Status:    status,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil
}
