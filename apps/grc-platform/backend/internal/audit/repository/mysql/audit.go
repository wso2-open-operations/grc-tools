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
	"strings"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
)

type auditRepository struct{ db *sql.DB }

// NewAuditRepository creates a MySQL-backed repository.AuditRepository.
func NewAuditRepository(db *sql.DB) repository.AuditRepository {
	return &auditRepository{db: db}
}

// listQuery joins audit with framework, product, and a control-count subquery.
// Returns all non-REMOVED audits ordered by created_at DESC.
const listQuery = `
SELECT
  a.id, a.name,
  a.framework_id, f.name AS framework_name, f.version AS framework_version,
  a.product_id, p.name AS product_name,
  DATE_FORMAT(a.period_start, '%Y-%m-%d') AS period_start,
  DATE_FORMAT(a.period_end,   '%Y-%m-%d') AS period_end,
  a.status, a.scope_description,
  COALESCE(cc.total, 0)    AS total_controls,
  COALESCE(cc.approved, 0) AS approved_controls,
  COALESCE(cc.overdue, 0)  AS overdue_controls,
  a.created_at, a.updated_at
FROM audit a
JOIN audit_framework f ON f.id = a.framework_id
JOIN audit_product   p ON p.id = a.product_id
LEFT JOIN (
  SELECT audit_id,
    COUNT(*) AS total,
    SUM(status = 'COMPLETE') AS approved,
    SUM(due_date IS NOT NULL AND due_date < CURDATE() AND status != 'COMPLETE') AS overdue
  FROM audit_control
  GROUP BY audit_id
) cc ON cc.audit_id = a.id
WHERE a.status != 'REMOVED'`

func (r *auditRepository) List(ctx context.Context) ([]*model.Audit, error) {
	rows, err := r.db.QueryContext(ctx, listQuery+" ORDER BY a.created_at DESC")
	if err != nil {
		return nil, fmt.Errorf("audit.List: %w", err)
	}
	defer rows.Close()

	var audits []*model.Audit
	for rows.Next() {
		a, err := scanAudit(rows)
		if err != nil {
			return nil, fmt.Errorf("audit.List scan: %w", err)
		}
		audits = append(audits, a)
	}
	return audits, rows.Err()
}

func (r *auditRepository) GetByID(ctx context.Context, id int) (*model.Audit, error) {
	row := r.db.QueryRowContext(ctx, listQuery+" AND a.id = ?", id)
	a, err := scanAudit(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("audit.GetByID(%d): %w", id, err)
	}
	return a, nil
}

func (r *auditRepository) Create(ctx context.Context, req model.CreateAuditRequest, createdBy string) (*model.Audit, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO audit (name, framework_id, product_id, period_start, period_end, status, scope_description, created_by, updated_by)
		VALUES (?, ?, ?, ?, ?, 'ACTIVE', ?, ?, ?)`,
		req.Name, req.FrameworkID, req.ProductID,
		req.PeriodStart, req.PeriodEnd,
		stringPtrVal(req.ScopeDescription),
		createdBy, createdBy,
	)
	if err != nil {
		return nil, fmt.Errorf("audit.Create: %w", err)
	}
	id64, err := res.LastInsertId()
	if err != nil || id64 == 0 {
		return nil, fmt.Errorf("audit.Create get insert id: %w", err)
	}
	return r.GetByID(ctx, int(id64))
}

func (r *auditRepository) Update(ctx context.Context, id int, req model.UpdateAuditRequest, updatedBy string) error {
	setParts := []string{"updated_by = ?", "updated_at = NOW()"}
	args := []any{updatedBy}

	if req.Name != nil {
		setParts = append([]string{"name = ?"}, setParts...)
		args = append([]any{*req.Name}, args...)
	}
	if req.PeriodStart != nil {
		setParts = append([]string{"period_start = ?"}, setParts...)
		args = append([]any{*req.PeriodStart}, args...)
	}
	if req.PeriodEnd != nil {
		setParts = append([]string{"period_end = ?"}, setParts...)
		args = append([]any{*req.PeriodEnd}, args...)
	}
	if req.ScopeDescription != nil {
		setParts = append([]string{"scope_description = ?"}, setParts...)
		args = append([]any{*req.ScopeDescription}, args...)
	}
	if req.Status != nil {
		setParts = append([]string{"status = ?"}, setParts...)
		args = append([]any{*req.Status}, args...)
	}

	args = append(args, id)
	_, err := r.db.ExecContext(ctx,
		"UPDATE audit SET "+strings.Join(setParts, ", ")+" WHERE id = ? AND status != 'REMOVED'",
		args...)
	if err != nil {
		return fmt.Errorf("audit.Update(%d): %w", id, err)
	}
	return nil
}

func (r *auditRepository) Delete(ctx context.Context, id int, deletedBy string) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE audit SET status = 'REMOVED', updated_by = ?, updated_at = NOW() WHERE id = ? AND status != 'REMOVED'",
		deletedBy, id)
	return err
}

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanAudit(s scanner) (*model.Audit, error) {
	var (
		id               int
		name             string
		frameworkID      int
		frameworkName    string
		frameworkVersion sql.NullString
		productID        int
		productName      string
		periodStart      string
		periodEnd        string
		status           string
		scopeDesc        sql.NullString
		totalControls    int
		approvedControls int
		overdueControls  int
		createdAt        time.Time
		updatedAt        time.Time
	)
	err := s.Scan(
		&id, &name,
		&frameworkID, &frameworkName, &frameworkVersion,
		&productID, &productName,
		&periodStart, &periodEnd,
		&status, &scopeDesc,
		&totalControls, &approvedControls, &overdueControls,
		&createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &model.Audit{
		ID:   id,
		Name: name,
		Framework: model.AuditFrameworkRef{
			ID:      frameworkID,
			Name:    frameworkName,
			Version: nullStringPtr(frameworkVersion),
		},
		Product: model.AuditProductRef{
			ID:   productID,
			Name: productName,
		},
		PeriodStart:      periodStart,
		PeriodEnd:        periodEnd,
		Status:           status,
		ScopeDescription: nullStringPtr(scopeDesc),
		ControlCounts: model.ControlCounts{
			Total:    totalControls,
			Approved: approvedControls,
			Overdue:  overdueControls,
		},
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil
}
