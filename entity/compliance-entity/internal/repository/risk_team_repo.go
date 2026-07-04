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

// RiskTeamRepository defines persistence operations for the risk_team table.
type RiskTeamRepository interface {
	SearchRiskTeams(ctx context.Context, req domain.SearchRiskTeamsRequest) ([]domain.RiskTeam, int, error)
	GetRiskTeamByID(ctx context.Context, id int) (*domain.RiskTeam, error)
	CreateRiskTeam(ctx context.Context, req domain.CreateRiskTeamRequest) (*domain.RiskTeam, error)
	UpdateRiskTeam(ctx context.Context, id int, req domain.UpdateRiskTeamRequest) (*domain.RiskTeam, error)
}

type riskTeamRepo struct{ db *sql.DB }

// NewRiskTeamRepository constructs a RiskTeamRepository.
func NewRiskTeamRepository(db *sql.DB) RiskTeamRepository { return &riskTeamRepo{db: db} }

func (r *riskTeamRepo) SearchRiskTeams(ctx context.Context, req domain.SearchRiskTeamsRequest) ([]domain.RiskTeam, int, error) {
	args := []any{}
	where := "WHERE 1=1"

	if req.SearchQuery != "" {
		where += " AND (name LIKE ? OR code LIKE ?)"
		p := "%" + req.SearchQuery + "%"
		args = append(args, p, p)
	}
	if len(req.TeamTypeKeys) > 0 {
		ph := strings.Repeat("?,", len(req.TeamTypeKeys))
		ph = ph[:len(ph)-1]
		where += " AND team_type IN (" + ph + ")"
		for _, t := range req.TeamTypeKeys {
			args = append(args, t)
		}
	}
	if req.StatusKey != "" {
		where += " AND status = ?"
		args = append(args, req.StatusKey)
	}

	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM risk_team "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("risk_team.Search count: %w", err)
	}

	dataArgs := append(append([]any{}, args...), req.Pagination.Limit, req.Pagination.Offset)
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, name, code, description, team_type, status, created_at, updated_at "+
			"FROM risk_team "+where+" ORDER BY name LIMIT ? OFFSET ?",
		dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("risk_team.Search query: %w", err)
	}
	defer rows.Close()

	var teams []domain.RiskTeam
	for rows.Next() {
		t, err := scanRiskTeam(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("risk_team.Search scan: %w", err)
		}
		teams = append(teams, *t)
	}
	return teams, total, rows.Err()
}

func (r *riskTeamRepo) GetRiskTeamByID(ctx context.Context, id int) (*domain.RiskTeam, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT id, name, code, description, team_type, status, created_at, updated_at FROM risk_team WHERE id = ?", id)
	t, err := scanRiskTeam(row)
	if err == sql.ErrNoRows {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("risk team %d not found", id)}
	}
	if err != nil {
		return nil, fmt.Errorf("risk_team.GetByID(%d): %w", id, err)
	}
	return t, nil
}

func (r *riskTeamRepo) CreateRiskTeam(ctx context.Context, req domain.CreateRiskTeamRequest) (*domain.RiskTeam, error) {
	status := req.Status
	if status == "" {
		status = "ACTIVE"
	}
	res, err := r.db.ExecContext(ctx,
		"INSERT INTO risk_team (name, code, description, team_type, status, created_by, updated_by) VALUES (?, ?, ?, ?, ?, ?, ?)",
		req.Name, nullableString(req.Code), nullableString(req.Description),
		req.TeamType, status, req.CreatedBy, req.CreatedBy)
	if err != nil {
		return nil, fmt.Errorf("risk_team.Create: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.GetRiskTeamByID(ctx, int(id))
}

func (r *riskTeamRepo) UpdateRiskTeam(ctx context.Context, id int, req domain.UpdateRiskTeamRequest) (*domain.RiskTeam, error) {
	sets := []string{}
	args := []any{}

	if req.Name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *req.Name)
	}
	if req.Code != nil {
		sets = append(sets, "code = ?")
		args = append(args, *req.Code)
	}
	if req.Description != nil {
		sets = append(sets, "description = ?")
		args = append(args, *req.Description)
	}
	if req.TeamType != nil {
		sets = append(sets, "team_type = ?")
		args = append(args, *req.TeamType)
	}
	if req.Status != nil {
		sets = append(sets, "status = ?")
		args = append(args, *req.Status)
	}
	sets = append(sets, "updated_by = ?")
	args = append(args, req.UpdatedBy)
	args = append(args, id)

	if _, err := r.db.ExecContext(ctx,
		"UPDATE risk_team SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...); err != nil { // #nosec G202
		return nil, fmt.Errorf("risk_team.Update(%d): %w", id, err)
	}
	return r.GetRiskTeamByID(ctx, id)
}

func scanRiskTeam(s scanner) (*domain.RiskTeam, error) {
	var t domain.RiskTeam
	var code, description sql.NullString
	if err := s.Scan(&t.ID, &t.Name, &code, &description, &t.TeamType, &t.Status, &t.CreatedOn, &t.UpdatedOn); err != nil {
		return nil, err
	}
	if code.Valid {
		t.Code = &code.String
	}
	if description.Valid {
		t.Description = &description.String
	}
	return &t, nil
}
