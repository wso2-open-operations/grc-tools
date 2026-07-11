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

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
)

type dashboardRepository struct{ db *sql.DB }

// NewDashboardRepository creates a MySQL-backed repository.DashboardRepository.
func NewDashboardRepository(db *sql.DB) repository.DashboardRepository {
	return &dashboardRepository{db: db}
}

// resolveScope returns a WHERE fragment (starting with "AND") that restricts
// audit_control rows to those the user may see, plus any args to bind.
func (r *dashboardRepository) resolveScope(ctx context.Context, f model.DashboardFilter) (string, []any, error) {
	role := f.PrimaryRole()

	switch role {
	case model.RoleComplianceAdmin, model.RoleComplianceTeam, model.RoleManagement, "":
		// Full visibility — all controls across active audits. No extra filter.
		return "", nil, nil

	case model.RoleInternalTeam:
		// Scope to the user's team.
		var teamID sql.NullInt64
		err := r.db.QueryRowContext(ctx,
			"SELECT audit_team_id FROM `user` WHERE email = ?", f.UserEmail,
		).Scan(&teamID)
		if err != nil || !teamID.Valid {
			return " AND 1=0", nil, nil // no team → no controls
		}
		return " AND c.team_id = ?", []any{teamID.Int64}, nil

	case model.RoleExternalAuditor:
		// Scope to audits where the user is assigned + controls assigned to them directly.
		var userID sql.NullInt64
		err := r.db.QueryRowContext(ctx,
			"SELECT id FROM `user` WHERE email = ?", f.UserEmail,
		).Scan(&userID)
		if err != nil || !userID.Valid {
			return " AND 1=0", nil, nil
		}
		// Controls where auditor_id matches OR audit is assigned to this auditor.
		return ` AND (c.auditor_id = ? OR c.audit_id IN (
			SELECT audit_id FROM audit_auditor_assignment
			WHERE user_id = ? AND status = 'ACTIVE'
		))`, []any{userID.Int64, userID.Int64}, nil

	default:
		return "", nil, nil
	}
}

func (r *dashboardRepository) Get(ctx context.Context, f model.DashboardFilter) (*model.DashboardData, error) {
	scope, args, err := r.resolveScope(ctx, f)
	if err != nil {
		return nil, err
	}

	baseWhere := "WHERE a.status = 'ACTIVE'" + scope

	// ── Status distribution ──────────────────────────────────────────────────
	statusRows, err := r.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT c.status, COUNT(*) AS cnt
		FROM audit_control c
		JOIN audit a ON a.id = c.audit_id
		%s
		GROUP BY c.status`, baseWhere), args...)
	if err != nil {
		return nil, err
	}
	defer statusRows.Close()

	var statusDist []model.StatusCount
	totalControls := 0
	completedControls := 0
	for statusRows.Next() {
		var sc model.StatusCount
		if err := statusRows.Scan(&sc.Status, &sc.Count); err != nil {
			return nil, err
		}
		statusDist = append(statusDist, sc)
		totalControls += sc.Count
		if sc.Status == "COMPLETE" {
			completedControls = sc.Count
		}
	}
	if err := statusRows.Err(); err != nil {
		return nil, err
	}
	if statusDist == nil {
		statusDist = []model.StatusCount{}
	}

	// ── Team completion ──────────────────────────────────────────────────────
	teamRows, err := r.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT
			COALESCE(t.name, 'Unassigned') AS team,
			COUNT(*) AS total,
			SUM(c.status = 'COMPLETE') AS completed
		FROM audit_control c
		JOIN audit a ON a.id = c.audit_id
		LEFT JOIN audit_team t ON t.id = c.team_id
		%s
		GROUP BY c.team_id, t.name
		ORDER BY total DESC
		LIMIT 10`, baseWhere), args...)
	if err != nil {
		return nil, err
	}
	defer teamRows.Close()

	var teamCompletion []model.TeamCompletion
	for teamRows.Next() {
		var tc model.TeamCompletion
		if err := teamRows.Scan(&tc.Team, &tc.Total, &tc.Completed); err != nil {
			return nil, err
		}
		teamCompletion = append(teamCompletion, tc)
	}
	if err := teamRows.Err(); err != nil {
		return nil, err
	}
	if teamCompletion == nil {
		teamCompletion = []model.TeamCompletion{}
	}

	// ── Overdue count ────────────────────────────────────────────────────────
	var overdueCount int
	overdueArgs := append([]any{}, args...)
	err = r.db.QueryRowContext(ctx, fmt.Sprintf(`
		SELECT COUNT(*)
		FROM audit_control c
		JOIN audit a ON a.id = c.audit_id
		%s
		AND c.due_date IS NOT NULL AND c.due_date < CURDATE() AND c.status != 'COMPLETE'`,
		baseWhere), overdueArgs...).Scan(&overdueCount)
	if err != nil {
		return nil, err
	}

	// ── Evidence required count ──────────────────────────────────────────────
	var evidenceReqCount int
	evidenceArgs := append([]any{}, args...)
	err = r.db.QueryRowContext(ctx, fmt.Sprintf(`
		SELECT COUNT(*)
		FROM audit_control c
		JOIN audit a ON a.id = c.audit_id
		%s
		AND c.status IN ('EVIDENCE_PENDING','SUBMITTED_SAMPLE','EVIDENCE_NEED_CLARIFICATION')`,
		baseWhere), evidenceArgs...).Scan(&evidenceReqCount)
	if err != nil {
		return nil, err
	}

	// ── Audit-level stats ────────────────────────────────────────────────────
	auditStats, err := r.queryAuditStats(ctx)
	if err != nil {
		return nil, err
	}

	// ── Action items (role-specific) ─────────────────────────────────────────
	actionItems, err := r.queryActionItems(ctx, f, baseWhere, args)
	if err != nil {
		return nil, err
	}

	// ── Overdue controls list ────────────────────────────────────────────────
	overdueControls, err := r.queryOverdueControls(ctx, baseWhere, args)
	if err != nil {
		return nil, err
	}

	// ── Assemble stats ────────────────────────────────────────────────────────
	completionPct := 0.0
	if totalControls > 0 {
		completionPct = float64(completedControls) / float64(totalControls) * 100
	}

	return &model.DashboardData{
		AuditStats: auditStats,
		Stats: model.DashboardStats{
			TotalControls:            totalControls,
			CompletedControls:        completedControls,
			OverdueControls:          overdueCount,
			EvidenceRequiredControls: evidenceReqCount,
			CompletionPercent:        completionPct,
		},
		StatusDistribution: statusDist,
		TeamCompletion:     teamCompletion,
		ActionItems:        actionItems,
		OverdueControls:    overdueControls,
	}, nil
}

func (r *dashboardRepository) queryAuditStats(ctx context.Context) (model.AuditStats, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT status, COUNT(*) FROM audit
		WHERE status IN ('ACTIVE','COMPLETED','ARCHIVED')
		GROUP BY status`)
	if err != nil {
		return model.AuditStats{}, err
	}
	defer rows.Close()

	var s model.AuditStats
	for rows.Next() {
		var status string
		var cnt int
		if err := rows.Scan(&status, &cnt); err != nil {
			return model.AuditStats{}, err
		}
		s.TotalAudits += cnt
		switch status {
		case "ACTIVE":
			s.ActiveAudits = cnt
		case "COMPLETED":
			s.CompletedAudits = cnt
		case "ARCHIVED":
			s.ArchivedAudits = cnt
		}
	}
	return s, rows.Err()
}

func (r *dashboardRepository) queryActionItems(
	ctx context.Context,
	f model.DashboardFilter,
	baseWhere string,
	scopeArgs []any,
) ([]model.ActionItem, error) {
	role := f.PrimaryRole()

	var statusFilter string
	switch role {
	case model.RoleInternalTeam:
		statusFilter = "c.status IN ('EVIDENCE_PENDING','SUBMITTED_SAMPLE','EVIDENCE_NEED_CLARIFICATION','POPULATION_PENDING','POPULATION_NEED_CLARIFICATION')"
	case model.RoleComplianceAdmin, model.RoleComplianceTeam:
		statusFilter = "c.status IN ('EVIDENCE_INTERNAL_REVIEW','POPULATION_INTERNAL_REVIEW')"
	case model.RoleExternalAuditor:
		statusFilter = "c.status IN ('EVIDENCE_UNDER_VALIDATION','POPULATION_UNDER_VALIDATION','POPULATION_COMPLETE','AWAITING_SAMPLE')"
	case model.RoleManagement:
		return []model.ActionItem{}, nil
	default:
		statusFilter = "c.status IN ('EVIDENCE_INTERNAL_REVIEW','POPULATION_INTERNAL_REVIEW')"
	}

	extraArgs := append([]any{}, scopeArgs...)
	q := fmt.Sprintf(`
		SELECT c.id, c.audit_id, a.name, c.control_number, c.description, c.status,
		       COALESCE(DATE_FORMAT(c.due_date, '%%Y-%%m-%%d'), '') AS due_date
		FROM audit_control c
		JOIN audit a ON a.id = c.audit_id
		%s AND %s
		ORDER BY c.due_date ASC, c.id ASC
		LIMIT 20`, baseWhere, statusFilter)

	rows, err := r.db.QueryContext(ctx, q, extraArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []model.ActionItem
	for rows.Next() {
		var ai model.ActionItem
		if err := rows.Scan(&ai.ControlID, &ai.AuditID, &ai.AuditName, &ai.ControlNumber, &ai.Description, &ai.Status, &ai.DueDate); err != nil {
			return nil, err
		}
		items = append(items, ai)
	}
	if items == nil {
		items = []model.ActionItem{}
	}
	return items, rows.Err()
}

func (r *dashboardRepository) queryOverdueControls(
	ctx context.Context,
	baseWhere string,
	scopeArgs []any,
) ([]model.OverdueControl, error) {
	extraArgs := append([]any{}, scopeArgs...)
	q := fmt.Sprintf(`
		SELECT c.id, c.audit_id, a.name, c.control_number, c.description, c.status,
		       DATE_FORMAT(c.due_date, '%%Y-%%m-%%d') AS due_date
		FROM audit_control c
		JOIN audit a ON a.id = c.audit_id
		%s
		AND c.due_date IS NOT NULL AND c.due_date < CURDATE() AND c.status != 'COMPLETE'
		ORDER BY c.due_date ASC
		LIMIT 20`, baseWhere)

	rows, err := r.db.QueryContext(ctx, q, extraArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.OverdueControl
	for rows.Next() {
		var oc model.OverdueControl
		if err := rows.Scan(&oc.ControlID, &oc.AuditID, &oc.AuditName, &oc.ControlNumber, &oc.Description, &oc.Status, &oc.DueDate); err != nil {
			return nil, err
		}
		list = append(list, oc)
	}
	if list == nil {
		list = []model.OverdueControl{}
	}
	return list, rows.Err()

}
