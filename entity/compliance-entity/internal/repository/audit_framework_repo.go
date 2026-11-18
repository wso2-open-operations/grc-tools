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
	// DeleteAuditFramework soft-deletes a framework by setting its status to
	// INACTIVE, recording deletedBy. Returns ConflictError while any non-removed
	// audit or any framework control still points at it, and NotFoundError for
	// an unknown or already-deleted id.
	DeleteAuditFramework(ctx context.Context, id int, deletedBy string) error
}

// Framework statuses. ARCHIVED hides a framework from new audits but keeps it
// and its audits; INACTIVE means deleted and is treated as not found everywhere.
const (
	frameworkActive   = "ACTIVE"
	frameworkArchived = "ARCHIVED"
	frameworkDeleted  = "INACTIVE"
)

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
	} else {
		// No status filter means every framework that has not been deleted.
		where += " AND status <> ?"
		args = append(args, frameworkDeleted)
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
	f, err := r.getAnyFrameworkByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if f.Status == frameworkDeleted {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("framework %d not found", id)}
	}
	return f, nil
}

// getAnyFrameworkByID reads a framework whatever its status, deleted included.
func (r *auditFrameworkRepo) getAnyFrameworkByID(ctx context.Context, id int) (*domain.AuditFramework, error) {
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
		status = frameworkActive
	}

	// The name is unique across every status. A deleted framework still holds
	// its name, so creating the same name brings that row back instead of failing.
	var existingID int
	var existingStatus string
	err := r.db.QueryRowContext(ctx,
		"SELECT id, status FROM audit_framework WHERE name = ? LIMIT 1", req.Name).Scan(&existingID, &existingStatus)
	switch {
	case err == nil && existingStatus == frameworkDeleted:
		if _, err := r.db.ExecContext(ctx,
			"UPDATE audit_framework SET name = ?, status = ?, updated_by = ? WHERE id = ? AND status = ?",
			req.Name, status, req.CreatedBy, existingID, frameworkDeleted); err != nil {
			if isDuplicateKey(err) {
				return nil, &apierror.ConflictError{Msg: "a framework with this name already exists"}
			}
			return nil, fmt.Errorf("audit_framework.Create revive(%d): %w", existingID, err)
		}
		return r.GetAuditFrameworkByID(ctx, existingID)
	case err == nil:
		return nil, &apierror.ConflictError{Msg: nameTakenMessage(existingStatus)}
	case !errors.Is(err, sql.ErrNoRows):
		return nil, fmt.Errorf("audit_framework.Create name check: %w", err)
	}

	res, err := r.db.ExecContext(ctx,
		"INSERT INTO audit_framework (name, status, created_by, updated_by) VALUES (?, ?, ?, ?)",
		req.Name, status, req.CreatedBy, req.CreatedBy)
	if err != nil {
		if isDuplicateKey(err) {
			return nil, &apierror.ConflictError{Msg: "a framework with this name already exists"}
		}
		return nil, fmt.Errorf("audit_framework.Create: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.GetAuditFrameworkByID(ctx, int(id))
}

func (r *auditFrameworkRepo) UpdateAuditFramework(ctx context.Context, id int, req domain.UpdateAuditFrameworkRequest) (*domain.AuditFramework, error) {
	// A deleted framework is not found, so it cannot be renamed or restored here.
	if _, err := r.GetAuditFrameworkByID(ctx, id); err != nil {
		return nil, err
	}
	sets := []string{}
	args := []any{}

	if req.Name != nil {
		var clash int
		var clashStatus string
		err := r.db.QueryRowContext(ctx,
			"SELECT id, status FROM audit_framework WHERE name = ? AND id <> ? LIMIT 1", *req.Name, id).Scan(&clash, &clashStatus)
		if err == nil {
			return nil, &apierror.ConflictError{Msg: nameTakenMessage(clashStatus)}
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

func (r *auditFrameworkRepo) DeleteAuditFramework(ctx context.Context, id int, deletedBy string) error {
	if _, err := r.GetAuditFrameworkByID(ctx, id); err != nil {
		return err
	}
	var audits, controls int
	// Removed audits are soft-deleted and no longer count.
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM audit WHERE framework_id = ? AND status <> 'REMOVED'", id).Scan(&audits); err != nil {
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
	// The NOT EXISTS guards repeat the checks above, so an audit created in the
	// gap between them cannot end up under a deleted framework.
	res, err := r.db.ExecContext(ctx,
		`UPDATE audit_framework SET status = ?, updated_by = ?
		 WHERE id = ? AND status <> ?
		   AND NOT EXISTS (SELECT 1 FROM audit WHERE framework_id = ? AND status <> 'REMOVED')
		   AND NOT EXISTS (SELECT 1 FROM audit_framework_control WHERE framework_id = ?)`,
		frameworkDeleted, deletedBy, id, frameworkDeleted, id, id)
	if err != nil {
		return fmt.Errorf("audit_framework.Delete(%d): %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return &apierror.ConflictError{Msg: "this framework is still in use and cannot be deleted; archive it instead"}
	}
	return nil
}

// nameTakenMessage explains a name clash in terms of the status of the framework
// holding the name, so a hidden (archived or deleted) one is not a mystery.
func nameTakenMessage(holderStatus string) string {
	switch holderStatus {
	case frameworkArchived:
		return "a framework with this name is archived; restore it instead, or choose another name"
	case frameworkDeleted:
		return "this name belongs to a deleted framework; choose another name, or create a framework with this name to bring it back"
	default:
		return "a framework with this name already exists"
	}
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
