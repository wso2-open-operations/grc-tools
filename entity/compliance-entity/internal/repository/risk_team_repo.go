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

// teamHasRisks is the SELECT expression behind RiskTeam.HasRisks: any risk
// using the team as its source register. It also drives the template lock in
// UpdateRiskTeam, so the two cannot disagree. A risk routed to the team as its
// assignment team doesn't count: the template only decides which fields a
// register's risks carry.
const teamHasRisks = "EXISTS(SELECT 1 FROM risk r WHERE r.source_register_id = risk_team.id)"

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
		p := "%" + likeEscape(req.SearchQuery) + "%"
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
	if len(req.RegisterTemplateKeys) > 0 {
		ph := strings.Repeat("?,", len(req.RegisterTemplateKeys))
		ph = ph[:len(ph)-1]
		where += " AND register_template IN (" + ph + ")"
		for _, t := range req.RegisterTemplateKeys {
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
		"SELECT id, name, code, description, team_type, register_template, "+teamHasRisks+", status, created_at, updated_at "+
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
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("risk_team.Search rows: %w", err)
	}
	if err := attachTeamPlatforms(ctx, r.db, teams); err != nil {
		return nil, 0, err
	}
	return teams, total, nil
}

func (r *riskTeamRepo) GetRiskTeamByID(ctx context.Context, id int) (*domain.RiskTeam, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT id, name, code, description, team_type, register_template, "+teamHasRisks+", status, created_at, updated_at FROM risk_team WHERE id = ?", id)
	t, err := scanRiskTeam(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("risk team %d not found", id)}
	}
	if err != nil {
		return nil, fmt.Errorf("risk_team.GetByID(%d): %w", id, err)
	}
	one := []domain.RiskTeam{*t}
	if err := attachTeamPlatforms(ctx, r.db, one); err != nil {
		return nil, err
	}
	return &one[0], nil
}

// The only unique key on risk_team besides the id is the code, so a duplicate
// entry on a save is always a code another team already uses.
var errTeamCodeTaken = &apierror.ConflictError{Msg: "another team already uses that code, so choose a different one"}

func (r *riskTeamRepo) CreateRiskTeam(ctx context.Context, req domain.CreateRiskTeamRequest) (*domain.RiskTeam, error) {
	status := req.Status
	if status == "" {
		status = "ACTIVE"
	}
	template := req.RegisterTemplate
	if template == "" {
		template = "STANDARD"
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("risk_team.Create begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	res, err := tx.ExecContext(ctx,
		"INSERT INTO risk_team (name, code, description, team_type, register_template, status, created_by, updated_by) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		req.Name, nullableString(req.Code), nullableString(req.Description),
		req.TeamType, template, status, req.CreatedBy, req.CreatedBy)
	if err != nil {
		if isDuplicateKey(err) {
			return nil, errTeamCodeTaken
		}
		return nil, fmt.Errorf("risk_team.Create: %w", err)
	}
	id, _ := res.LastInsertId()
	if len(req.PlatformIDs) > 0 {
		if err := replaceTeamPlatforms(ctx, tx, int(id), template, req.PlatformIDs); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("risk_team.Create commit: %w", err)
	}
	return r.GetRiskTeamByID(ctx, int(id))
}

// UpdateRiskTeam applies a partial update. A change of register_template is
// refused once any risk uses this team as its source register: those risks
// would carry fields the new template lacks (or lack ones it requires). The
// team row is locked first, and CreateRisk reads the template under a shared
// lock on the same row, so a risk cannot be created between the check and the
// change.
func (r *riskTeamRepo) UpdateRiskTeam(ctx context.Context, id int, req domain.UpdateRiskTeamRequest) (*domain.RiskTeam, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("risk_team.Update(%d) begin: %w", id, err)
	}
	defer tx.Rollback() //nolint:errcheck

	var currentTemplate string
	if err := tx.QueryRowContext(ctx,
		"SELECT register_template FROM risk_team WHERE id = ? FOR UPDATE", id).Scan(&currentTemplate); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("risk team %d not found", id)}
		}
		return nil, fmt.Errorf("risk_team.Update(%d) lock: %w", id, err)
	}

	sets := []string{}
	args := []any{}
	// templateAfter is the register's template once this update lands: the
	// platform list below is only valid for AGGREGATED.
	templateAfter := currentTemplate

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
	if req.RegisterTemplate != nil && *req.RegisterTemplate != currentTemplate {
		var hasRisks bool
		if err := tx.QueryRowContext(ctx,
			"SELECT EXISTS(SELECT 1 FROM risk WHERE source_register_id = ?)", id).Scan(&hasRisks); err != nil {
			return nil, fmt.Errorf("risk_team.Update(%d) has-risks check: %w", id, err)
		}
		if hasRisks {
			return nil, &apierror.ConflictError{
				Msg: "this team already has risks, so its template can no longer be changed"}
		}
		sets = append(sets, "register_template = ?")
		args = append(args, *req.RegisterTemplate)
		templateAfter = *req.RegisterTemplate
	}
	if req.Status != nil {
		sets = append(sets, "status = ?")
		args = append(args, *req.Status)
	}
	sets = append(sets, "updated_by = ?")
	args = append(args, req.UpdatedBy)
	args = append(args, id)

	if _, err := tx.ExecContext(ctx,
		"UPDATE risk_team SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...); err != nil { // #nosec G202
		if isDuplicateKey(err) {
			return nil, errTeamCodeTaken
		}
		return nil, fmt.Errorf("risk_team.Update(%d): %w", id, err)
	}
	// Leaving the AGGREGATED template (possible only before the register has
	// risks) drops its platform list; otherwise a sent list replaces it, and an
	// absent one leaves it alone.
	switch {
	case templateAfter != TemplateAggregated && currentTemplate == TemplateAggregated && req.PlatformIDs == nil:
		if err := replaceTeamPlatforms(ctx, tx, id, templateAfter, nil); err != nil {
			return nil, err
		}
	case req.PlatformIDs != nil:
		if err := replaceTeamPlatforms(ctx, tx, id, templateAfter, req.PlatformIDs); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("risk_team.Update(%d) commit: %w", id, err)
	}
	return r.GetRiskTeamByID(ctx, id)
}

func scanRiskTeam(s scanner) (*domain.RiskTeam, error) {
	var t domain.RiskTeam
	var code, description sql.NullString
	if err := s.Scan(&t.ID, &t.Name, &code, &description, &t.TeamType, &t.RegisterTemplate, &t.HasRisks, &t.Status, &t.CreatedOn, &t.UpdatedOn); err != nil {
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
