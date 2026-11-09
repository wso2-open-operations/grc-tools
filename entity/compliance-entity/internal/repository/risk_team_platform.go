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

// queryer is the read side shared by *sql.DB and *sql.Tx.
type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// loadTeamPlatforms returns the platform ids each register offers, keyed by
// register. A register with no rows (every register that predates the table,
// and every one an admin has not narrowed) is absent from the map: it offers
// every active platform.
func loadTeamPlatforms(ctx context.Context, q queryer, teamIDs []int) (map[int][]int, error) {
	out := map[int][]int{}
	if len(teamIDs) == 0 {
		return out, nil
	}
	ph := strings.Repeat("?,", len(teamIDs))
	args := make([]any, len(teamIDs))
	for i, id := range teamIDs {
		args[i] = id
	}
	rows, err := q.QueryContext(ctx,
		"SELECT risk_team_id, platform_id FROM risk_team_platform WHERE risk_team_id IN ("+ph[:len(ph)-1]+") ORDER BY platform_id", // #nosec G202 -- placeholders only
		args...)
	if err != nil {
		return nil, fmt.Errorf("risk_team_platform.Load: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var teamID, platformID int
		if err := rows.Scan(&teamID, &platformID); err != nil {
			return nil, fmt.Errorf("risk_team_platform.Load scan: %w", err)
		}
		out[teamID] = append(out[teamID], platformID)
	}
	return out, rows.Err()
}

// attachTeamPlatforms fills PlatformIDs on every team, empty rather than nil
// when a register offers every platform.
func attachTeamPlatforms(ctx context.Context, q queryer, teams []domain.RiskTeam) error {
	ids := make([]int, len(teams))
	for i, t := range teams {
		ids[i] = t.ID
	}
	byTeam, err := loadTeamPlatforms(ctx, q, ids)
	if err != nil {
		return err
	}
	for i := range teams {
		teams[i].PlatformIDs = byTeam[teams[i].ID]
		if teams[i].PlatformIDs == nil {
			teams[i].PlatformIDs = []int{}
		}
	}
	return nil
}

// replaceTeamPlatforms makes ids the register's platform list, inside tx. The
// register row must already be locked by the caller. Rules:
//   - only an AGGREGATED register has a list;
//   - every id must name a platform, and a newly listed one must be ACTIVE
//     (one already listed may stay even if since deactivated, so a form that
//     re-posts the whole list still saves);
//   - a sub-platform may be listed only with its parent.
//
// An empty ids clears the list, which restores "offers every active platform".
func replaceTeamPlatforms(ctx context.Context, tx *sql.Tx, teamID int, template string, ids []int) error {
	if len(ids) > 0 && template != TemplateAggregated {
		return &apierror.ValidationError{
			Msg: fmt.Sprintf("platformIds not allowed for a register on the %s template", template)}
	}

	if len(ids) > 0 {
		ph := strings.Repeat("?,", len(ids))
		args := make([]any, len(ids))
		for i, id := range ids {
			args[i] = id
		}
		rows, err := tx.QueryContext(ctx,
			"SELECT id, parent_id, status FROM risk_platform WHERE id IN ("+ph[:len(ph)-1]+") FOR SHARE", args...) // #nosec G202 -- placeholders only
		if err != nil {
			return fmt.Errorf("risk_team_platform.Replace(%d) read platforms: %w", teamID, err)
		}
		type platform struct {
			parent sql.NullInt64
			status string
		}
		found := map[int]platform{}
		for rows.Next() {
			var id int
			var p platform
			if err := rows.Scan(&id, &p.parent, &p.status); err != nil {
				rows.Close()
				return fmt.Errorf("risk_team_platform.Replace(%d) scan: %w", teamID, err)
			}
			found[id] = p
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return fmt.Errorf("risk_team_platform.Replace(%d) rows: %w", teamID, err)
		}
		rows.Close()

		current, err := loadTeamPlatforms(ctx, tx, []int{teamID})
		if err != nil {
			return err
		}
		listed := map[int]bool{}
		for _, id := range current[teamID] {
			listed[id] = true
		}
		want := map[int]bool{}
		for _, id := range ids {
			want[id] = true
		}
		for _, id := range ids {
			p, ok := found[id]
			if !ok {
				return &apierror.ValidationError{Msg: fmt.Sprintf("platform %d not found", id)}
			}
			if p.status != "ACTIVE" && !listed[id] {
				return &apierror.ValidationError{Msg: fmt.Sprintf("platform %d is no longer active", id)}
			}
			if p.parent.Valid && !want[int(p.parent.Int64)] {
				return &apierror.ValidationError{
					Msg: fmt.Sprintf("sub-platform %d needs its platform %d in the list too", id, p.parent.Int64)}
			}
		}
	}

	if _, err := tx.ExecContext(ctx, "DELETE FROM risk_team_platform WHERE risk_team_id = ?", teamID); err != nil {
		return fmt.Errorf("risk_team_platform.Replace(%d) clear: %w", teamID, err)
	}
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO risk_team_platform (risk_team_id, platform_id) VALUES (?, ?)", teamID, id); err != nil {
			return fmt.Errorf("risk_team_platform.Replace(%d) add %d: %w", teamID, id, err)
		}
	}
	return nil
}
