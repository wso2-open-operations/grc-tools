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

// ControlRepository defines persistence operations for the audit_control table.
type ControlRepository interface {
	SearchControls(ctx context.Context, auditID int, req domain.SearchControlsRequest) ([]domain.AuditControl, int, error)
	SearchControlsGlobal(ctx context.Context, req domain.SearchControlsRequest) ([]domain.AuditControl, int, error)
	GetControlByID(ctx context.Context, auditID, controlID int) (*domain.AuditControl, error)
	CreateControl(ctx context.Context, auditID int, req domain.CreateControlRequest) (*domain.AuditControl, error)
	BulkCreateControls(ctx context.Context, auditID int, reqs []domain.CreateControlRequest) ([]domain.AuditControl, error)
	UpdateControl(ctx context.Context, auditID, controlID int, req domain.UpdateControlRequest) (*domain.AuditControl, error)
	DeleteControl(ctx context.Context, auditID, controlID int) error
}

type controlRepo struct{ db *sql.DB }

// NewControlRepository constructs a ControlRepository.
func NewControlRepository(db *sql.DB) ControlRepository { return &controlRepo{db: db} }

const controlSelectCols = `
  c.id, c.audit_id,
  c.control_number, c.description, c.evidence_requirement,
  c.requirement_type, c.control_type, c.scope,
  c.owner_id,   u_owner.display_name AS owner_name,
  c.team_id,    t.name               AS team_name,
  c.auditor_id, u_aud.display_name   AS auditor_name,
  DATE_FORMAT(c.due_date, '%Y-%m-%d') AS due_date,
  c.status, c.is_manually_added,
  (c.due_date IS NOT NULL AND c.due_date < CURDATE() AND c.status != 'COMPLETE') AS is_overdue,
  c.created_at, c.updated_at`

const controlFromClause = `
FROM audit_control c
LEFT JOIN ` + "`user`" + ` u_owner ON u_owner.id = c.owner_id
LEFT JOIN audit_team t            ON t.id          = c.team_id
LEFT JOIN ` + "`user`" + ` u_aud   ON u_aud.id     = c.auditor_id`

func (r *controlRepo) SearchControls(ctx context.Context, auditID int, req domain.SearchControlsRequest) ([]domain.AuditControl, int, error) {
	args := []any{auditID}
	where := "WHERE c.audit_id = ?"

	if req.SearchQuery != "" {
		where += " AND (c.control_number LIKE ? OR c.description LIKE ?)"
		p := "%" + req.SearchQuery + "%"
		args = append(args, p, p)
	}
	if len(req.StatusKeys) > 0 {
		ph := strings.Repeat("?,", len(req.StatusKeys))
		ph = ph[:len(ph)-1]
		where += " AND c.status IN (" + ph + ")"
		for _, s := range req.StatusKeys {
			args = append(args, s)
		}
	}
	if len(req.RequirementTypes) > 0 {
		ph := strings.Repeat("?,", len(req.RequirementTypes))
		ph = ph[:len(ph)-1]
		where += " AND c.requirement_type IN (" + ph + ")"
		for _, rt := range req.RequirementTypes {
			args = append(args, rt)
		}
	}
	if len(req.TeamIDs) > 0 {
		ph := strings.Repeat("?,", len(req.TeamIDs))
		ph = ph[:len(ph)-1]
		where += " AND c.team_id IN (" + ph + ")"
		for _, id := range req.TeamIDs {
			args = append(args, id)
		}
	}
	if len(req.AuditorIDs) > 0 {
		ph := strings.Repeat("?,", len(req.AuditorIDs))
		ph = ph[:len(ph)-1]
		where += " AND c.auditor_id IN (" + ph + ")"
		for _, id := range req.AuditorIDs {
			args = append(args, id)
		}
	}
	if len(req.OwnerIDs) > 0 {
		ph := strings.Repeat("?,", len(req.OwnerIDs))
		ph = ph[:len(ph)-1]
		where += " AND c.owner_id IN (" + ph + ")"
		for _, id := range req.OwnerIDs {
			args = append(args, id)
		}
	}

	var total int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) "+controlFromClause+" "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("control.Search count: %w", err)
	}

	dataArgs := append(append([]any{}, args...), req.Pagination.Limit, req.Pagination.Offset)
	rows, err := r.db.QueryContext(ctx,
		"SELECT"+controlSelectCols+controlFromClause+" "+where+
			" ORDER BY c.control_number LIMIT ? OFFSET ?",
		dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("control.Search query: %w", err)
	}
	defer rows.Close()

	var controls []domain.AuditControl
	for rows.Next() {
		c, err := scanControl(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("control.Search scan: %w", err)
		}
		controls = append(controls, *c)
	}
	return controls, total, rows.Err()
}

func (r *controlRepo) SearchControlsGlobal(ctx context.Context, req domain.SearchControlsRequest) ([]domain.AuditControl, int, error) {
	args := []any{}
	where := "WHERE 1=1"

	if req.SearchQuery != "" {
		where += " AND (c.control_number LIKE ? OR c.description LIKE ?)"
		p := "%" + req.SearchQuery + "%"
		args = append(args, p, p)
	}
	if len(req.StatusKeys) > 0 {
		ph := strings.Repeat("?,", len(req.StatusKeys))
		ph = ph[:len(ph)-1]
		where += " AND c.status IN (" + ph + ")"
		for _, s := range req.StatusKeys {
			args = append(args, s)
		}
	}
	if len(req.RequirementTypes) > 0 {
		ph := strings.Repeat("?,", len(req.RequirementTypes))
		ph = ph[:len(ph)-1]
		where += " AND c.requirement_type IN (" + ph + ")"
		for _, rt := range req.RequirementTypes {
			args = append(args, rt)
		}
	}
	if len(req.TeamIDs) > 0 {
		ph := strings.Repeat("?,", len(req.TeamIDs))
		ph = ph[:len(ph)-1]
		where += " AND c.team_id IN (" + ph + ")"
		for _, id := range req.TeamIDs {
			args = append(args, id)
		}
	}
	if len(req.AuditorIDs) > 0 {
		ph := strings.Repeat("?,", len(req.AuditorIDs))
		ph = ph[:len(ph)-1]
		where += " AND c.auditor_id IN (" + ph + ")"
		for _, id := range req.AuditorIDs {
			args = append(args, id)
		}
	}
	if len(req.OwnerIDs) > 0 {
		ph := strings.Repeat("?,", len(req.OwnerIDs))
		ph = ph[:len(ph)-1]
		where += " AND c.owner_id IN (" + ph + ")"
		for _, id := range req.OwnerIDs {
			args = append(args, id)
		}
	}

	var total int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) "+controlFromClause+" "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("control.SearchGlobal count: %w", err)
	}

	dataArgs := append(append([]any{}, args...), req.Pagination.Limit, req.Pagination.Offset)
	rows, err := r.db.QueryContext(ctx,
		"SELECT"+controlSelectCols+controlFromClause+" "+where+
			" ORDER BY c.control_number LIMIT ? OFFSET ?",
		dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("control.SearchGlobal query: %w", err)
	}
	defer rows.Close()

	var controls []domain.AuditControl
	for rows.Next() {
		c, err := scanControl(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("control.SearchGlobal scan: %w", err)
		}
		controls = append(controls, *c)
	}
	return controls, total, rows.Err()
}

func (r *controlRepo) GetControlByID(ctx context.Context, auditID, controlID int) (*domain.AuditControl, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT"+controlSelectCols+controlFromClause+" WHERE c.audit_id = ? AND c.id = ?",
		auditID, controlID)
	c, err := scanControl(row)
	if err == sql.ErrNoRows {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("control %d not found in audit %d", controlID, auditID)}
	}
	if err != nil {
		return nil, fmt.Errorf("control.GetByID(%d,%d): %w", auditID, controlID, err)
	}
	return c, nil
}

func (r *controlRepo) CreateControl(ctx context.Context, auditID int, req domain.CreateControlRequest) (*domain.AuditControl, error) {
	initialStatus := "EVIDENCE_PENDING"
	if req.RequirementType == "OE" {
		initialStatus = "POPULATION_PENDING"
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO audit_control
		 (audit_id, control_number, description, evidence_requirement, requirement_type, control_type, scope,
		  owner_id, team_id, auditor_id, due_date, status, is_manually_added, created_by, updated_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
		auditID,
		req.ControlNumber, req.Description, nullableString(req.EvidenceRequirement),
		req.RequirementType, req.ControlType, req.Scope,
		nullableInt(req.OwnerID), nullableInt(req.TeamID), nullableInt(req.AuditorID),
		req.DueDate,
		initialStatus,
		req.CreatedBy, req.CreatedBy)
	if err != nil {
		return nil, fmt.Errorf("control.Create: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.GetControlByID(ctx, auditID, int(id))
}

func (r *controlRepo) BulkCreateControls(ctx context.Context, auditID int, reqs []domain.CreateControlRequest) ([]domain.AuditControl, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("control.BulkCreate begin: %w", err)
	}
	defer tx.Rollback()

	var ids []int
	for _, req := range reqs {
		initialStatus := "EVIDENCE_PENDING"
		if req.RequirementType == "OE" {
			initialStatus = "POPULATION_PENDING"
		}
		res, err := tx.ExecContext(ctx,
			`INSERT INTO audit_control
			 (audit_id, control_number, description, evidence_requirement, requirement_type, control_type, scope,
			  owner_id, team_id, auditor_id, due_date, status, is_manually_added, created_by, updated_by)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
			auditID,
			req.ControlNumber, req.Description, nullableString(req.EvidenceRequirement),
			req.RequirementType, req.ControlType, req.Scope,
			nullableInt(req.OwnerID), nullableInt(req.TeamID), nullableInt(req.AuditorID),
			req.DueDate, initialStatus,
			req.CreatedBy, req.CreatedBy)
		if err != nil {
			return nil, fmt.Errorf("control.BulkCreate insert %q: %w", req.ControlNumber, err)
		}
		id, _ := res.LastInsertId()
		ids = append(ids, int(id))
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("control.BulkCreate commit: %w", err)
	}

	var controls []domain.AuditControl
	for _, id := range ids {
		c, err := r.GetControlByID(ctx, auditID, id)
		if err != nil {
			return nil, fmt.Errorf("control.BulkCreate fetch %d: %w", id, err)
		}
		controls = append(controls, *c)
	}
	return controls, nil
}

func (r *controlRepo) DeleteControl(ctx context.Context, auditID, controlID int) error {
	result, err := r.db.ExecContext(ctx,
		"DELETE FROM audit_control WHERE audit_id = ? AND id = ?", auditID, controlID)
	if err != nil {
		return fmt.Errorf("control.Delete(%d,%d): %w", auditID, controlID, err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return &apierror.NotFoundError{Msg: fmt.Sprintf("control %d not found in audit %d", controlID, auditID)}
	}
	return nil
}

func (r *controlRepo) UpdateControl(ctx context.Context, auditID, controlID int, req domain.UpdateControlRequest) (*domain.AuditControl, error) {
	sets := []string{}
	args := []any{}

	if req.OwnerID != nil {
		sets = append(sets, "owner_id = ?")
		args = append(args, *req.OwnerID)
	}
	if req.TeamID != nil {
		sets = append(sets, "team_id = ?")
		args = append(args, *req.TeamID)
	}
	if req.AuditorID != nil {
		sets = append(sets, "auditor_id = ?")
		args = append(args, *req.AuditorID)
	}
	if req.DueDate != nil {
		sets = append(sets, "due_date = ?")
		args = append(args, *req.DueDate)
	}
	if req.Status != nil {
		sets = append(sets, "status = ?")
		args = append(args, *req.Status)
	}
	if req.Comments != nil {
		sets = append(sets, "comments = ?")
		args = append(args, *req.Comments)
	}
	if req.SampleReference != nil {
		sets = append(sets, "sample_reference = ?")
		args = append(args, *req.SampleReference)
	}
	if req.SampleFileURL != nil {
		sets = append(sets, "sample_file_url = ?")
		args = append(args, *req.SampleFileURL)
	}
	if req.SampleFileName != nil {
		sets = append(sets, "sample_file_name = ?")
		args = append(args, *req.SampleFileName)
	}
	sets = append(sets, "updated_by = ?")
	args = append(args, req.UpdatedBy)
	args = append(args, auditID, controlID)

	if _, err := r.db.ExecContext(ctx,
		"UPDATE audit_control SET "+strings.Join(sets, ", ")+" WHERE audit_id = ? AND id = ?", args...); err != nil { // #nosec G202
		return nil, fmt.Errorf("control.Update(%d,%d): %w", auditID, controlID, err)
	}
	return r.GetControlByID(ctx, auditID, controlID)
}

func scanControl(s scanner) (*domain.AuditControl, error) {
	var c domain.AuditControl
	var evidenceReq, ownerName, teamName, auditorName, dueDate sql.NullString
	var ownerID, teamID, auditorID sql.NullInt64
	err := s.Scan(
		&c.ID, &c.AuditID,
		&c.ControlNumber, &c.Description, &evidenceReq,
		&c.RequirementType, &c.ControlType, &c.Scope,
		&ownerID, &ownerName,
		&teamID, &teamName,
		&auditorID, &auditorName,
		&dueDate,
		&c.Status, &c.IsManuallyAdded, &c.IsOverdue,
		&c.CreatedOn, &c.UpdatedOn,
	)
	if err != nil {
		return nil, err
	}
	nullStrPtr := func(ns sql.NullString) *string {
		if ns.Valid {
			return &ns.String
		}
		return nil
	}
	nullIntPtr := func(ni sql.NullInt64) *int {
		if ni.Valid {
			v := int(ni.Int64)
			return &v
		}
		return nil
	}
	c.EvidenceRequirement = nullStrPtr(evidenceReq)
	c.OwnerID = nullIntPtr(ownerID)
	c.OwnerName = nullStrPtr(ownerName)
	c.TeamID = nullIntPtr(teamID)
	c.TeamName = nullStrPtr(teamName)
	c.AuditorID = nullIntPtr(auditorID)
	c.AuditorName = nullStrPtr(auditorName)
	c.DueDate = nullStrPtr(dueDate)
	return &c, nil
}
