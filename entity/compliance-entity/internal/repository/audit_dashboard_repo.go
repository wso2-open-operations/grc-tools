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

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

// DashboardRepository aggregates the audit dashboard from the audit tables.
type DashboardRepository interface {
	Get(ctx context.Context, req domain.AuditDashboardRequest) (*domain.DashboardData, error)
}

type dashboardRepo struct{ db *sql.DB }

// NewDashboardRepository constructs a DashboardRepository.
func NewDashboardRepository(db *sql.DB) DashboardRepository { return &dashboardRepo{db: db} }

// resolveScope returns a WHERE fragment (starting with "AND"), any args to bind,
// and an error. Only sql.ErrNoRows and a NULL team/user column are mapped to
// " AND 1=0" (legitimate no-data cases); any other DB error is propagated so
// callers return 500 instead of a silent empty dashboard.
func (r *dashboardRepo) resolveScope(ctx context.Context, req domain.AuditDashboardRequest) (string, []any, error) {
	switch req.PrimaryRole() {
	case domain.RoleComplianceAdmin, domain.RoleComplianceTeam, domain.RoleManagement:
		return "", nil, nil
	case domain.RoleInternalTeam:
		var teamID sql.NullInt64
		err := r.db.QueryRowContext(ctx, "SELECT audit_team_id FROM `user` WHERE email = ?", req.UserEmail).Scan(&teamID)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && !teamID.Valid) {
			return " AND 1=0", nil, nil
		}
		if err != nil {
			return "", nil, fmt.Errorf("dashboard.resolveScope: lookup team for %q: %w", req.UserEmail, err)
		}
		return " AND c.team_id = ?", []any{teamID.Int64}, nil
	case domain.RoleExternalAuditor:
		var userID sql.NullInt64
		err := r.db.QueryRowContext(ctx, "SELECT id FROM `user` WHERE email = ?", req.UserEmail).Scan(&userID)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && !userID.Valid) {
			return " AND 1=0", nil, nil
		}
		if err != nil {
			return "", nil, fmt.Errorf("dashboard.resolveScope: lookup user for %q: %w", req.UserEmail, err)
		}
		return " AND c.auditor_id = ?", []any{userID.Int64}, nil
	default:
		return " AND 1=0", nil, nil
	}
}

func (r *dashboardRepo) Get(ctx context.Context, req domain.AuditDashboardRequest) (*domain.DashboardData, error) {
	scope, args, err := r.resolveScope(ctx, req)
	if err != nil {
		return nil, err
	}
	baseWhere := "WHERE a.status = 'ACTIVE'" + scope

	// Status distribution.
	statusRows, err := r.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT c.status, COUNT(*) FROM audit_control c
		JOIN audit a ON a.id = c.audit_id %s GROUP BY c.status`, baseWhere), args...) // #nosec G201
	if err != nil {
		return nil, err
	}
	defer statusRows.Close()
	statusDist := []domain.StatusCount{}
	totalControls, completedControls := 0, 0
	for statusRows.Next() {
		var sc domain.StatusCount
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

	// Team completion.
	teamRows, err := r.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT COALESCE(t.name,'Unassigned'), COUNT(*), SUM(c.status='COMPLETE')
		FROM audit_control c JOIN audit a ON a.id = c.audit_id
		LEFT JOIN audit_team t ON t.id = c.team_id %s
		GROUP BY c.team_id, t.name ORDER BY COUNT(*) DESC LIMIT 10`, baseWhere), args...) // #nosec G201
	if err != nil {
		return nil, err
	}
	defer teamRows.Close()
	teamCompletion := []domain.TeamCompletion{}
	for teamRows.Next() {
		var tc domain.TeamCompletion
		if err := teamRows.Scan(&tc.Team, &tc.Total, &tc.Completed); err != nil {
			return nil, err
		}
		teamCompletion = append(teamCompletion, tc)
	}
	if err := teamRows.Err(); err != nil {
		return nil, err
	}

	// Overdue + evidence-required counts.
	var overdueCount, evidenceReqCount int
	if err := r.db.QueryRowContext(ctx, fmt.Sprintf(`
		SELECT COUNT(*) FROM audit_control c JOIN audit a ON a.id = c.audit_id %s
		AND c.due_date IS NOT NULL AND c.due_date < CURDATE() AND c.status != 'COMPLETE'`, baseWhere),
		args...).Scan(&overdueCount); err != nil { // #nosec G201
		return nil, err
	}
	if err := r.db.QueryRowContext(ctx, fmt.Sprintf(`
		SELECT COUNT(*) FROM audit_control c JOIN audit a ON a.id = c.audit_id %s
		AND c.status IN ('EVIDENCE_PENDING','SUBMITTED_SAMPLE','EVIDENCE_NEED_CLARIFICATION')`, baseWhere),
		args...).Scan(&evidenceReqCount); err != nil { // #nosec G201
		return nil, err
	}

	auditStats, err := r.queryAuditStats(ctx)
	if err != nil {
		return nil, err
	}
	actionItems, err := r.queryActionItems(ctx, req, baseWhere, args)
	if err != nil {
		return nil, err
	}
	overdueControls, err := r.queryOverdueControls(ctx, baseWhere, args)
	if err != nil {
		return nil, err
	}

	completionPct := 0.0
	if totalControls > 0 {
		completionPct = float64(completedControls) / float64(totalControls) * 100
	}
	return &domain.DashboardData{
		AuditStats: auditStats,
		Stats: domain.DashboardStats{
			TotalControls: totalControls, CompletedControls: completedControls,
			OverdueControls: overdueCount, EvidenceRequiredControls: evidenceReqCount,
			CompletionPercent: completionPct,
		},
		StatusDistribution: statusDist,
		TeamCompletion:     teamCompletion,
		ActionItems:        actionItems,
		OverdueControls:    overdueControls,
	}, nil
}

func (r *dashboardRepo) queryAuditStats(ctx context.Context) (domain.AuditStats, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT status, COUNT(*) FROM audit WHERE status IN ('ACTIVE','COMPLETED','ARCHIVED') GROUP BY status`)
	if err != nil {
		return domain.AuditStats{}, err
	}
	defer rows.Close()
	var s domain.AuditStats
	for rows.Next() {
		var status string
		var cnt int
		if err := rows.Scan(&status, &cnt); err != nil {
			return domain.AuditStats{}, err
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

func (r *dashboardRepo) queryActionItems(ctx context.Context, req domain.AuditDashboardRequest, baseWhere string, scopeArgs []any) ([]domain.DashboardControlItem, error) {
	var statusFilter string
	switch req.PrimaryRole() {
	case domain.RoleInternalTeam:
		statusFilter = "c.status IN ('EVIDENCE_PENDING','SUBMITTED_SAMPLE','EVIDENCE_NEED_CLARIFICATION','POPULATION_PENDING','POPULATION_NEED_CLARIFICATION')"
	case domain.RoleComplianceAdmin, domain.RoleComplianceTeam:
		statusFilter = "c.status IN ('EVIDENCE_INTERNAL_REVIEW','POPULATION_INTERNAL_REVIEW')"
	case domain.RoleExternalAuditor:
		statusFilter = "c.status IN ('EVIDENCE_UNDER_VALIDATION','POPULATION_UNDER_VALIDATION','POPULATION_COMPLETE','AWAITING_SAMPLE')"
	case domain.RoleManagement:
		return []domain.DashboardControlItem{}, nil
	default:
		statusFilter = "c.status IN ('EVIDENCE_INTERNAL_REVIEW','POPULATION_INTERNAL_REVIEW')"
	}
	q := fmt.Sprintf(`
		SELECT c.id, c.audit_id, a.name,
		       COALESCE(c.control_number, fc.control_number, ''),
		       COALESCE(c.description, fc.description, ''),
		       c.status,
		       COALESCE(DATE_FORMAT(c.due_date,'%%Y-%%m-%%d'),'')
		FROM audit_control c JOIN audit a ON a.id = c.audit_id
		LEFT JOIN audit_framework_control fc ON fc.id = c.framework_control_id
		%s AND %s ORDER BY c.due_date ASC, c.id ASC LIMIT 20`, baseWhere, statusFilter) // #nosec G201
	rows, err := r.db.QueryContext(ctx, q, scopeArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.DashboardControlItem{}
	for rows.Next() {
		var item domain.DashboardControlItem
		if err := rows.Scan(&item.ControlID, &item.AuditID, &item.AuditName, &item.ControlNumber, &item.Description, &item.Status, &item.DueDate); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *dashboardRepo) queryOverdueControls(ctx context.Context, baseWhere string, scopeArgs []any) ([]domain.DashboardControlItem, error) {
	q := fmt.Sprintf(`
		SELECT c.id, c.audit_id, a.name,
		       COALESCE(c.control_number, fc.control_number, ''),
		       COALESCE(c.description, fc.description, ''),
		       c.status,
		       DATE_FORMAT(c.due_date,'%%Y-%%m-%%d')
		FROM audit_control c JOIN audit a ON a.id = c.audit_id
		LEFT JOIN audit_framework_control fc ON fc.id = c.framework_control_id
		%s AND c.due_date IS NOT NULL AND c.due_date < CURDATE() AND c.status != 'COMPLETE'
		ORDER BY c.due_date ASC LIMIT 20`, baseWhere) // #nosec G201
	rows, err := r.db.QueryContext(ctx, q, scopeArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []domain.DashboardControlItem{}
	for rows.Next() {
		var item domain.DashboardControlItem
		if err := rows.Scan(&item.ControlID, &item.AuditID, &item.AuditName, &item.ControlNumber, &item.Description, &item.Status, &item.DueDate); err != nil {
			return nil, err
		}
		list = append(list, item)
	}
	return list, rows.Err()
}
