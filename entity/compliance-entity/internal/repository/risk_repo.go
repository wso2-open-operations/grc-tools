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

// RiskRepository defines persistence operations for the risk table.
type RiskRepository interface {
	SearchRisks(ctx context.Context, req domain.SearchRisksRequest) ([]domain.Risk, int, error)
	GetRiskByID(ctx context.Context, id int) (*domain.Risk, error)
	CreateRisk(ctx context.Context, req domain.CreateRiskRequest) (*domain.Risk, error)
	UpdateRisk(ctx context.Context, id int, req domain.UpdateRiskRequest) (*domain.Risk, error)
}

type riskRepo struct{ db *sql.DB }

// NewRiskRepository constructs a RiskRepository.
func NewRiskRepository(db *sql.DB) RiskRepository { return &riskRepo{db: db} }

const riskSelectCols = `
  r.id, r.risk_code, r.risk_year, r.risk_quarter, r.risk_title, r.risk_description,
  r.source_register_id,  src.name  AS source_register_name,
  r.assignment_team_id,  asgn.name AS assignment_team_name,
  r.assigner_id,         u_asgn.display_name AS assigner_name,
  r.owner_id,            u_own.display_name  AS owner_name,
  r.workflow_status, r.treatment_strategy,
  r.gross_score_id, rs.risk_level AS gross_risk_level,
  DATE_FORMAT(r.implementation_date, '%Y-%m-%d'),
  DATE_FORMAT(r.reassessment_date,   '%Y-%m-%d'),
  r.created_at, r.updated_at`

const riskFromClause = `
FROM risk r
JOIN  risk_team  src   ON src.id   = r.source_register_id
JOIN  risk_team  asgn  ON asgn.id  = r.assignment_team_id
JOIN  ` + "`user`" + ` u_asgn ON u_asgn.id = r.assigner_id
JOIN  ` + "`user`" + ` u_own  ON u_own.id  = r.owner_id
LEFT JOIN risk_score rs ON rs.id   = r.gross_score_id`

func (r *riskRepo) SearchRisks(ctx context.Context, req domain.SearchRisksRequest) ([]domain.Risk, int, error) {
	args := []any{}
	where := "WHERE 1=1"

	if req.SearchQuery != "" {
		where += " AND (r.risk_code LIKE ? OR r.risk_title LIKE ?)"
		p := "%" + likeEscape(req.SearchQuery) + "%"
		args = append(args, p, p)
	}
	if len(req.WorkflowStatusKeys) > 0 {
		ph := strings.Repeat("?,", len(req.WorkflowStatusKeys))
		ph = ph[:len(ph)-1]
		where += " AND r.workflow_status IN (" + ph + ")"
		for _, s := range req.WorkflowStatusKeys {
			args = append(args, s)
		}
	}
	if len(req.SourceRegisterIDs) > 0 {
		ph := strings.Repeat("?,", len(req.SourceRegisterIDs))
		ph = ph[:len(ph)-1]
		where += " AND r.source_register_id IN (" + ph + ")"
		for _, id := range req.SourceRegisterIDs {
			args = append(args, id)
		}
	}
	if len(req.AssignmentTeamIDs) > 0 {
		ph := strings.Repeat("?,", len(req.AssignmentTeamIDs))
		ph = ph[:len(ph)-1]
		where += " AND r.assignment_team_id IN (" + ph + ")"
		for _, id := range req.AssignmentTeamIDs {
			args = append(args, id)
		}
	}
	if len(req.RiskYears) > 0 {
		ph := strings.Repeat("?,", len(req.RiskYears))
		ph = ph[:len(ph)-1]
		where += " AND r.risk_year IN (" + ph + ")"
		for _, y := range req.RiskYears {
			args = append(args, y)
		}
	}
	if len(req.RiskQuarterKeys) > 0 {
		ph := strings.Repeat("?,", len(req.RiskQuarterKeys))
		ph = ph[:len(ph)-1]
		where += " AND r.risk_quarter IN (" + ph + ")"
		for _, q := range req.RiskQuarterKeys {
			args = append(args, q)
		}
	}

	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) "+riskFromClause+" "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("risk.Search count: %w", err)
	}

	dataArgs := append(append([]any{}, args...), req.Pagination.Limit, req.Pagination.Offset)
	rows, err := r.db.QueryContext(ctx,
		"SELECT"+riskSelectCols+riskFromClause+" "+where+" ORDER BY r.created_at DESC LIMIT ? OFFSET ?",
		dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("risk.Search query: %w", err)
	}
	defer rows.Close()

	var risks []domain.Risk
	for rows.Next() {
		risk, err := scanRisk(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("risk.Search scan: %w", err)
		}
		risks = append(risks, *risk)
	}
	return risks, total, rows.Err()
}

func (r *riskRepo) GetRiskByID(ctx context.Context, id int) (*domain.Risk, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT"+riskSelectCols+riskFromClause+" WHERE r.id = ?", id)
	risk, err := scanRisk(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("risk %d not found", id)}
	}
	if err != nil {
		return nil, fmt.Errorf("risk.GetByID(%d): %w", id, err)
	}
	return risk, nil
}

// CreateRisk generates a unique risk code using the risk_register_sequence table
// (atomically incremented per team) and inserts the new risk row.
func (r *riskRepo) CreateRisk(ctx context.Context, req domain.CreateRiskRequest) (*domain.Risk, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("risk.Create begin: %w", err)
	}
	defer tx.Rollback()

	// Upsert sequence row and atomically increment.
	_, err = tx.ExecContext(ctx,
		`INSERT INTO risk_register_sequence (risk_team_id, last_sequence_number) VALUES (?, 1)
		 ON DUPLICATE KEY UPDATE last_sequence_number = last_sequence_number + 1`,
		req.AssignmentTeamID)
	if err != nil {
		return nil, fmt.Errorf("risk.Create seq upsert: %w", err)
	}

	var seqNum int
	if err = tx.QueryRowContext(ctx,
		"SELECT last_sequence_number FROM risk_register_sequence WHERE risk_team_id = ?",
		req.AssignmentTeamID).Scan(&seqNum); err != nil {
		return nil, fmt.Errorf("risk.Create seq read: %w", err)
	}

	// Use the team code (or name as fallback) for the risk code segment.
	var teamCode string
	if err = tx.QueryRowContext(ctx,
		"SELECT COALESCE(code, name) FROM risk_team WHERE id = ?",
		req.AssignmentTeamID).Scan(&teamCode); err != nil {
		return nil, fmt.Errorf("risk.Create team code: %w", err)
	}

	riskCode := fmt.Sprintf("%d-%s-%s-%04d", req.RiskYear, teamCode, req.RiskQuarter, seqNum)

	res, err := tx.ExecContext(ctx,
		`INSERT INTO risk (
			risk_code, risk_year, risk_quarter, risk_title, risk_description,
			source_register_id, assignment_team_id, assigner_id, owner_id,
			treatment_strategy, gross_score_id,
			implementation_date, reassessment_date,
			impact_description, risk_identified_date,
			identified_by_type, identified_by_user_id, identified_by_name,
			git_issue_url, email_subject, remarks,
			workflow_status, created_by, updated_by
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'PENDING_RISK_OWNER_APPROVAL', ?, ?)`,
		riskCode, req.RiskYear, req.RiskQuarter, req.RiskTitle, req.RiskDescription,
		req.SourceRegisterID, req.AssignmentTeamID, req.AssignerID, req.OwnerID,
		req.TreatmentStrategy, nullableInt(req.GrossScoreID),
		req.ImplementationDate, req.ReassessmentDate,
		req.ImpactDescription, req.RiskIdentifiedDate,
		req.IdentifiedByType, nullableInt(req.IdentifiedByUserID), req.IdentifiedByName,
		req.GitIssueURL, req.EmailSubject, req.Remarks,
		req.CreatedBy, req.CreatedBy)
	if err != nil {
		return nil, fmt.Errorf("risk.Create insert: %w", err)
	}

	id, _ := res.LastInsertId()
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("risk.Create commit: %w", err)
	}
	return r.GetRiskByID(ctx, int(id))
}

func (r *riskRepo) UpdateRisk(ctx context.Context, id int, req domain.UpdateRiskRequest) (*domain.Risk, error) {
	sets := []string{}
	args := []any{}

	if req.RiskTitle != nil {
		sets = append(sets, "risk_title = ?")
		args = append(args, *req.RiskTitle)
	}
	if req.RiskDescription != nil {
		sets = append(sets, "risk_description = ?")
		args = append(args, *req.RiskDescription)
	}
	if req.WorkflowStatus != nil {
		sets = append(sets, "workflow_status = ?")
		args = append(args, *req.WorkflowStatus)
	}
	if req.TreatmentStrategy != nil {
		sets = append(sets, "treatment_strategy = ?")
		args = append(args, *req.TreatmentStrategy)
	}
	if req.GrossScoreID != nil {
		sets = append(sets, "gross_score_id = ?")
		args = append(args, *req.GrossScoreID)
	}
	if req.ImplementationDate != nil {
		sets = append(sets, "implementation_date = ?")
		args = append(args, *req.ImplementationDate)
	}
	if req.ReassessmentDate != nil {
		sets = append(sets, "reassessment_date = ?")
		args = append(args, *req.ReassessmentDate)
	}
	if req.Progress != nil {
		sets = append(sets, "progress = ?")
		args = append(args, *req.Progress)
	}
	if req.RejectionComment != nil {
		sets = append(sets, "rejection_comment = ?")
		args = append(args, *req.RejectionComment)
	}
	if req.RejectionStage != nil {
		sets = append(sets, "rejection_stage = ?")
		args = append(args, *req.RejectionStage)
	}
	if req.ComplianceApprovalBy != nil {
		sets = append(sets, "compliance_approval_by = ?")
		args = append(args, *req.ComplianceApprovalBy)
	}
	if req.ComplianceApprovalDate != nil {
		sets = append(sets, "compliance_approval_date = ?")
		args = append(args, *req.ComplianceApprovalDate)
	}
	if req.AssignmentTeamID != nil {
		sets = append(sets, "assignment_team_id = ?")
		args = append(args, *req.AssignmentTeamID)
	}
	if req.OwnerID != nil {
		sets = append(sets, "owner_id = ?")
		args = append(args, *req.OwnerID)
	}
	if req.ActionPlanID != nil {
		sets = append(sets, "action_plan_id = ?")
		args = append(args, *req.ActionPlanID)
	}
	if req.GitIssueURL != nil {
		sets = append(sets, "git_issue_url = ?")
		args = append(args, *req.GitIssueURL)
	}
	if req.Remarks != nil {
		sets = append(sets, "remarks = ?")
		args = append(args, *req.Remarks)
	}
	sets = append(sets, "updated_by = ?")
	args = append(args, req.UpdatedBy)

	var (
		query  string
		result sql.Result
		err    error
	)
	if req.ExpectedStatus != "" {
		args = append(args, id, req.ExpectedStatus)
		query = "UPDATE risk SET " + strings.Join(sets, ", ") + " WHERE id = ? AND workflow_status = ?" // #nosec G202
	} else {
		args = append(args, id)
		query = "UPDATE risk SET " + strings.Join(sets, ", ") + " WHERE id = ?" // #nosec G202
	}
	if result, err = r.db.ExecContext(ctx, query, args...); err != nil {
		return nil, fmt.Errorf("risk.Update(%d): %w", id, err)
	}
	if req.ExpectedStatus != "" {
		if n, _ := result.RowsAffected(); n == 0 {
			current, err := r.GetRiskByID(ctx, id)
			if err != nil {
				return nil, err // propagates NotFoundError if record was deleted
			}
			if current.WorkflowStatus == req.ExpectedStatus && (req.WorkflowStatus == nil || *req.WorkflowStatus == req.ExpectedStatus) {
				return current, nil // MySQL no-op: status not being changed, or already at the target value
			}
			return nil, &apierror.ConflictError{Msg: "risk was modified concurrently, please retry"}
		}
	}
	return r.GetRiskByID(ctx, id)
}

func scanRisk(s scanner) (*domain.Risk, error) {
	var r domain.Risk
	var desc, treatment, grossLevel, implDate, reassDate sql.NullString
	var grossScoreID sql.NullInt64
	err := s.Scan(
		&r.ID, &r.RiskCode, &r.RiskYear, &r.RiskQuarter, &r.RiskTitle, &desc,
		&r.SourceRegisterID, &r.SourceRegisterName,
		&r.AssignmentTeamID, &r.AssignmentTeamName,
		&r.AssignerID, &r.AssignerName,
		&r.OwnerID, &r.OwnerName,
		&r.WorkflowStatus, &treatment,
		&grossScoreID, &grossLevel,
		&implDate, &reassDate,
		&r.CreatedOn, &r.UpdatedOn,
	)
	if err != nil {
		return nil, err
	}
	nullStr := func(ns sql.NullString) *string {
		if ns.Valid {
			return &ns.String
		}
		return nil
	}
	r.RiskDescription = nullStr(desc)
	r.TreatmentStrategy = nullStr(treatment)
	r.GrossRiskLevel = nullStr(grossLevel)
	r.ImplementationDate = nullStr(implDate)
	r.ReassessmentDate = nullStr(reassDate)
	if grossScoreID.Valid {
		v := int(grossScoreID.Int64)
		r.GrossScoreID = &v
	}
	return &r, nil
}
