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

package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

// AuditFrameworkRepository defines persistence operations for the audit_framework table.
type AuditFrameworkRepository interface {
	SearchAuditFrameworks(ctx context.Context, req domain.SearchAuditFrameworksRequest) ([]domain.AuditFramework, int, error)
	GetAuditFrameworkByID(ctx context.Context, id int) (*domain.AuditFramework, error)
	CreateAuditFramework(ctx context.Context, req domain.CreateAuditFrameworkRequest) (*domain.AuditFramework, error)
	UpdateAuditFramework(ctx context.Context, id int, req domain.UpdateAuditFrameworkRequest) (*domain.AuditFramework, error)
	// DeleteAuditFramework hard-deletes a framework that nothing references.
	// Returns ConflictError while any audit (of any status, REMOVED included —
	// the FK counts soft-deleted rows too) or framework control still points at
	// it, and NotFoundError for an unknown id.
	DeleteAuditFramework(ctx context.Context, id int) error
}

type auditFrameworkRepo struct{ db *sql.DB }

// NewAuditFrameworkRepository constructs an AuditFrameworkRepository.
func NewAuditFrameworkRepository(db *sql.DB) AuditFrameworkRepository {
	return &auditFrameworkRepo{db: db}
}

func (r *auditFrameworkRepo) SearchAuditFrameworks(ctx context.Context, req domain.SearchAuditFrameworksRequest) ([]domain.AuditFramework, int, error) {
	args := []any{}
	where := "WHERE 1=1"

	if req.SearchQuery != "" {
		where += " AND name LIKE ?"
		args = append(args, "%"+likeEscape(req.SearchQuery)+"%")
	}
	if req.StatusKey != "" {
		where += " AND status = ?"
		args = append(args, req.StatusKey)
	}
	scopeClause, scopeArgs := frameworkScopeWhere(req.Scope, req.UserID, req.ScopeTeamIDs)
	where += scopeClause
	args = append(args, scopeArgs...)

	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_framework "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("audit_framework.Search count: %w", err)
	}

	dataArgs := append(append([]any{}, args...), req.Pagination.Limit, req.Pagination.Offset)
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, name, status, created_at, updated_at FROM audit_framework "+where+" ORDER BY name LIMIT ? OFFSET ?",
		dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("audit_framework.Search query: %w", err)
	}
	defer rows.Close()

	var frameworks []domain.AuditFramework
	for rows.Next() {
		f, err := scanFramework(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("audit_framework.Search scan: %w", err)
		}
		frameworks = append(frameworks, *f)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("audit_framework.Search rows: %w", err)
	}
	return frameworks, total, nil
}

func (r *auditFrameworkRepo) GetAuditFrameworkByID(ctx context.Context, id int) (*domain.AuditFramework, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT id, name, status, created_at, updated_at FROM audit_framework WHERE id = ?", id)
	f, err := scanFramework(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("framework %d not found", id)}
	}
	if err != nil {
		return nil, fmt.Errorf("audit_framework.GetByID(%d): %w", id, err)
	}
	return f, nil
}

func (r *auditFrameworkRepo) CreateAuditFramework(ctx context.Context, req domain.CreateAuditFrameworkRequest) (*domain.AuditFramework, error) {
	status := req.Status
	if status == "" {
		status = "ACTIVE"
	}
	res, err := r.db.ExecContext(ctx,
		"INSERT INTO audit_framework (name, status, created_by, updated_by) VALUES (?, ?, ?, ?)",
		req.Name, status, req.CreatedBy, req.CreatedBy)
	if err != nil {
		return nil, fmt.Errorf("audit_framework.Create: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.GetAuditFrameworkByID(ctx, int(id))
}

func (r *auditFrameworkRepo) UpdateAuditFramework(ctx context.Context, id int, req domain.UpdateAuditFrameworkRequest) (*domain.AuditFramework, error) {
	sets := []string{}
	args := []any{}

	if req.Name != nil {
		var clash int
		err := r.db.QueryRowContext(ctx,
			"SELECT id FROM audit_framework WHERE name = ? AND id <> ? LIMIT 1", *req.Name, id).Scan(&clash)
		if err == nil {
			return nil, &apierror.ConflictError{Msg: "a framework with this name already exists"}
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("audit_framework.Update(%d) name check: %w", id, err)
		}
		sets = append(sets, "name = ?")
		args = append(args, *req.Name)
	}
	if req.Status != nil {
		sets = append(sets, "status = ?")
		args = append(args, *req.Status)
	}
	sets = append(sets, "updated_by = ?")
	args = append(args, req.UpdatedBy)
	args = append(args, id)

	if _, err := r.db.ExecContext(ctx,
		"UPDATE audit_framework SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...); err != nil { // #nosec G202
		if isDuplicateKey(err) {
			return nil, &apierror.ConflictError{Msg: "a framework with this name already exists"}
		}
		return nil, fmt.Errorf("audit_framework.Update(%d): %w", id, err)
	}
	return r.GetAuditFrameworkByID(ctx, id)
}

func (r *auditFrameworkRepo) DeleteAuditFramework(ctx context.Context, id int) error {
	if _, err := r.GetAuditFrameworkByID(ctx, id); err != nil {
		return err
	}
	var audits, controls int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit WHERE framework_id = ?", id).Scan(&audits); err != nil {
		return fmt.Errorf("audit_framework.Delete(%d) count audits: %w", id, err)
	}
	if audits > 0 {
		return &apierror.ConflictError{Msg: "this framework has audits and cannot be deleted; archive it instead"}
	}
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_framework_control WHERE framework_id = ?", id).Scan(&controls); err != nil {
		return fmt.Errorf("audit_framework.Delete(%d) count controls: %w", id, err)
	}
	if controls > 0 {
		return &apierror.ConflictError{Msg: "this framework has controls in its library and cannot be deleted; archive it instead"}
	}
	// The FKs are ON DELETE RESTRICT: a row created between the counts above
	// and this DELETE trips 1451, mapped to the same 409.
	if _, err := r.db.ExecContext(ctx, "DELETE FROM audit_framework WHERE id = ?", id); err != nil {
		if isFKViolation(err) {
			return &apierror.ConflictError{Msg: "this framework is still in use and cannot be deleted; archive it instead"}
		}
		return fmt.Errorf("audit_framework.Delete(%d): %w", id, err)
	}
	return nil
}

// frameworkScopeWhere returns a WHERE fragment (starting with "AND") and its
// bind args for the given row scope. Frameworks have no team/owner/auditor of
// their own (only their audits' controls do), so — one level deeper than
// auditScopeWhere's "audit qualifies if any control does" — a framework
// qualifies if any of its audits has any control in scope.
//
// An unset (zero-value "") scope is NOT treated as ScopeAll: it falls through
// to the default deny-all case below, matching scopeWhere's behaviour. A
// caller that genuinely wants every row (an internal existence/uniqueness
// check, never an external request) must pass domain.ScopeAll explicitly.
func frameworkScopeWhere(scope domain.Scope, userID int, scopeTeamIDs []int) (string, []any) {
	switch scope {
	case domain.ScopeAll:
		return "", nil
	case domain.ScopeOwned:
		return ` AND EXISTS (SELECT 1 FROM audit a JOIN audit_control c ON c.audit_id = a.id
			WHERE a.framework_id = audit_framework.id AND c.owner_id = ?)`,
			[]any{userID}
	case domain.ScopeAssigned:
		return ` AND EXISTS (SELECT 1 FROM audit a JOIN audit_control c ON c.audit_id = a.id
			WHERE a.framework_id = audit_framework.id AND c.auditor_id = ?)`,
			[]any{userID}
	case domain.ScopeTeam:
		pred, args := teamScopePredicate("c", scopeTeamIDs, userID)
		return ` AND EXISTS (SELECT 1 FROM audit a JOIN audit_control c ON c.audit_id = a.id
			WHERE a.framework_id = audit_framework.id AND ` + pred + `)`,
			args
	default: // ScopeNone and any unrecognized value scope to nothing.
		return " AND 1=0", nil
	}
}

func scanFramework(s scanner) (*domain.AuditFramework, error) {
	var f domain.AuditFramework
	if err := s.Scan(&f.ID, &f.Name, &f.Status, &f.CreatedOn, &f.UpdatedOn); err != nil {
		return nil, err
	}
	return &f, nil
}

// nullableString converts *string to sql.NullString for optional columns.
func nullableString(v *string) sql.NullString {
	if v == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *v, Valid: true}
}
