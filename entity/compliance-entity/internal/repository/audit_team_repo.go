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
	"fmt"
	"strings"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

// AuditTeamRepository defines persistence operations for the audit_team table.
type AuditTeamRepository interface {
	SearchAuditTeams(ctx context.Context, req domain.SearchAuditTeamsRequest) ([]domain.AuditTeam, int, error)
	GetAuditTeamByID(ctx context.Context, id int) (*domain.AuditTeam, error)
	CreateAuditTeam(ctx context.Context, req domain.CreateAuditTeamRequest) (*domain.AuditTeam, error)
	UpdateAuditTeam(ctx context.Context, id int, req domain.UpdateAuditTeamRequest) (*domain.AuditTeam, error)
}

type auditTeamRepo struct{ db *sql.DB }

// NewAuditTeamRepository constructs an AuditTeamRepository backed by the given pool.
func NewAuditTeamRepository(db *sql.DB) AuditTeamRepository { return &auditTeamRepo{db: db} }

func (r *auditTeamRepo) SearchAuditTeams(ctx context.Context, req domain.SearchAuditTeamsRequest) ([]domain.AuditTeam, int, error) {
	args := []any{}
	where := "WHERE 1=1"

	if req.SearchQuery != "" {
		where += " AND name LIKE ?"
		args = append(args, "%"+req.SearchQuery+"%")
	}
	if req.StatusKey != "" {
		where += " AND status = ?"
		args = append(args, req.StatusKey)
	}

	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_team "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("audit_team.Search count: %w", err)
	}

	dataArgs := append(append([]any{}, args...), req.Pagination.Limit, req.Pagination.Offset)
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, name, status, created_at, updated_at FROM audit_team "+where+" ORDER BY name LIMIT ? OFFSET ?",
		dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("audit_team.Search query: %w", err)
	}
	defer rows.Close()

	var teams []domain.AuditTeam
	for rows.Next() {
		var t domain.AuditTeam
		if err := rows.Scan(&t.ID, &t.Name, &t.Status, &t.CreatedOn, &t.UpdatedOn); err != nil {
			return nil, 0, fmt.Errorf("audit_team.Search scan: %w", err)
		}
		teams = append(teams, t)
	}
	return teams, total, rows.Err()
}

func (r *auditTeamRepo) GetAuditTeamByID(ctx context.Context, id int) (*domain.AuditTeam, error) {
	var t domain.AuditTeam
	err := r.db.QueryRowContext(ctx,
		"SELECT id, name, status, created_at, updated_at FROM audit_team WHERE id = ?", id).
		Scan(&t.ID, &t.Name, &t.Status, &t.CreatedOn, &t.UpdatedOn)
	if err == sql.ErrNoRows {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("audit team %d not found", id)}
	}
	if err != nil {
		return nil, fmt.Errorf("audit_team.GetByID(%d): %w", id, err)
	}
	return &t, nil
}

func (r *auditTeamRepo) CreateAuditTeam(ctx context.Context, req domain.CreateAuditTeamRequest) (*domain.AuditTeam, error) {
	status := req.Status
	if status == "" {
		status = "ACTIVE"
	}
	res, err := r.db.ExecContext(ctx,
		"INSERT INTO audit_team (name, status, created_by, updated_by) VALUES (?, ?, ?, ?)",
		req.Name, status, req.CreatedBy, req.CreatedBy)
	if err != nil {
		return nil, fmt.Errorf("audit_team.Create: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.GetAuditTeamByID(ctx, int(id))
}

func (r *auditTeamRepo) UpdateAuditTeam(ctx context.Context, id int, req domain.UpdateAuditTeamRequest) (*domain.AuditTeam, error) {
	sets := []string{}
	args := []any{}

	if req.Name != nil {
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
		"UPDATE audit_team SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...); err != nil { // #nosec G202
		return nil, fmt.Errorf("audit_team.Update(%d): %w", id, err)
	}
	return r.GetAuditTeamByID(ctx, id)
}
