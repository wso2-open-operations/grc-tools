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

type productRepository struct{ db *sql.DB }

// NewProductRepository creates a MySQL-backed repository.ProductRepository.
func NewProductRepository(db *sql.DB) repository.ProductRepository {
	return &productRepository{db: db}
}

func (r *productRepository) List(ctx context.Context) ([]*model.AuditProduct, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, status, created_at, updated_at
		FROM audit_product
		WHERE status = 'ACTIVE'
		ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("product.List: %w", err)
	}
	defer rows.Close()

	var products []*model.AuditProduct
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, fmt.Errorf("product.List scan: %w", err)
		}
		products = append(products, p)
	}
	return products, rows.Err()
}

func (r *productRepository) GetByID(ctx context.Context, id int) (*model.AuditProduct, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT id, name, status, created_at, updated_at FROM audit_product WHERE id = ?", id)
	p, err := scanProduct(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("product.GetByID(%d): %w", id, err)
	}
	return p, nil
}

func (r *productRepository) Create(ctx context.Context, req model.CreateProductRequest, createdBy string) (*model.AuditProduct, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO audit_product (name, status, created_by, updated_by)
		VALUES (?, 'ACTIVE', ?, ?)`,
		req.Name, createdBy, createdBy,
	)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return nil, &apierror.Error{StatusCode: http.StatusConflict, Body: "A product with this name already exists."}
		}
		return nil, fmt.Errorf("product.Create: %w", err)
	}
	id64, err := res.LastInsertId()
	if err != nil || id64 == 0 {
		return nil, fmt.Errorf("product.Create get insert id: %w", err)
	}
	return r.GetByID(ctx, int(id64))
}

func scanProduct(s scanner) (*model.AuditProduct, error) {
	var (
		id        int
		name      string
		status    string
		createdAt time.Time
		updatedAt time.Time
	)
	err := s.Scan(&id, &name, &status, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	return &model.AuditProduct{
		ID:        id,
		Name:      name,
		Status:    status,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil
}
