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
	"strings"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
)

type controlRepository struct{ db *sql.DB }

// NewControlRepository creates a MySQL-backed repository.ControlRepository.
func NewControlRepository(db *sql.DB) repository.ControlRepository {
	return &controlRepository{db: db}
}

// controlSelectCols is the SELECT list used in both List and GetByID.
const controlSelectCols = `
  c.id, c.audit_id,
  c.owner_id,   u_owner.display_name AS owner_name,
  c.team_id,    t.name               AS team_name,
  c.auditor_id, u_aud.display_name   AS auditor_name,
  c.control_number, c.description, c.evidence_requirement,
  c.requirement_type, c.control_type, c.scope,
  DATE_FORMAT(c.due_date, '%Y-%m-%d') AS due_date,
  c.status,
  c.sample_reference, c.sample_file_url, c.sample_file_name,
  c.comments, c.is_manually_added,
  (c.due_date IS NOT NULL AND c.due_date < CURDATE() AND c.status != 'COMPLETE') AS is_overdue,
  c.created_at, c.updated_at`

const controlFromClause = `
FROM audit_control c
LEFT JOIN ` + "`user`" + ` u_owner ON u_owner.id = c.owner_id
LEFT JOIN audit_team t          ON t.id    = c.team_id
LEFT JOIN ` + "`user`" + ` u_aud   ON u_aud.id   = c.auditor_id`

func (r *controlRepository) List(ctx context.Context, auditID int) ([]*model.AuditControl, error) {
	q := "SELECT" + controlSelectCols + controlFromClause + " WHERE c.audit_id = ? ORDER BY c.control_number"
	rows, err := r.db.QueryContext(ctx, q, auditID)
	if err != nil {
		return nil, fmt.Errorf("control.List: %w", err)
	}
	defer rows.Close()

	var controls []*model.AuditControl
	for rows.Next() {
		c, err := scanControl(rows)
		if err != nil {
			return nil, fmt.Errorf("control.List scan: %w", err)
		}
		controls = append(controls, c)
	}
	return controls, rows.Err()
}

func (r *controlRepository) GetByID(ctx context.Context, auditID, controlID int) (*model.AuditControl, error) {
	q := "SELECT" + controlSelectCols + controlFromClause + " WHERE c.audit_id = ? AND c.id = ?"
	row := r.db.QueryRowContext(ctx, q, auditID, controlID)
	c, err := scanControl(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("control.GetByID(%d,%d): %w", auditID, controlID, err)
	}
	return c, nil
}

// createInTx inserts one control (and its population row for OE controls) using
// the provided transaction. It returns the new row's auto-increment id.
// The caller is responsible for committing or rolling back the transaction.
func (r *controlRepository) createInTx(ctx context.Context, tx *sql.Tx, auditID int, req model.AddControlRequest, createdBy string) (int64, error) {
	initialStatus := "EVIDENCE_PENDING"
	if req.RequirementType == "OE" {
		initialStatus = "POPULATION_PENDING"
	}

	res, err := tx.ExecContext(ctx, `
		INSERT INTO audit_control
		  (audit_id, control_number, description, evidence_requirement,
		   requirement_type, control_type, scope,
		   owner_id, team_id, auditor_id, due_date,
		   status, is_manually_added, created_by, updated_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		auditID,
		req.ControlNumber, req.Description, stringPtrVal(req.EvidenceRequirement),
		req.RequirementType, req.ControlType, req.Scope,
		intPtrVal(req.OwnerID), intPtrVal(req.TeamID), intPtrVal(req.AuditorID),
		stringPtrVal(req.DueDate),
		initialStatus, req.IsManuallyAdded,
		createdBy, createdBy,
	)
	if err != nil {
		return 0, fmt.Errorf("control.Create: %w", err)
	}
	id64, err := res.LastInsertId()
	if err != nil || id64 == 0 {
		return 0, fmt.Errorf("control.Create get insert id: %w", err)
	}

	if req.RequirementType == "OE" {
		var desc string
		var refNum *int
		var dueDate, comments *string
		if req.Population != nil {
			desc = req.Population.Description
			refNum = req.Population.ReferenceNumber
			dueDate = req.Population.DueDate
			comments = req.Population.Comments
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO audit_population
			  (audit_id, control_id, description, reference_number, due_date, comments, created_by, updated_by)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			auditID, id64,
			desc,
			intPtrVal(refNum),
			stringPtrVal(dueDate),
			stringPtrVal(comments),
			createdBy, createdBy,
		)
		if err != nil {
			return 0, fmt.Errorf("control.Create population: %w", err)
		}
	}

	return id64, nil
}

func (r *controlRepository) Create(ctx context.Context, auditID int, req model.AddControlRequest, createdBy string) (*model.AuditControl, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("control.Create begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // error irrelevant after Commit or on rollback path

	id64, err := r.createInTx(ctx, tx, auditID, req, createdBy)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("control.Create commit: %w", err)
	}

	return r.GetByID(ctx, auditID, int(id64))
}

func (r *controlRepository) BulkCreate(ctx context.Context, auditID int, reqs []model.AddControlRequest, createdBy string) ([]*model.AuditControl, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("control.BulkCreate begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // error irrelevant after Commit or on rollback path

	ids := make([]int64, 0, len(reqs))
	for _, req := range reqs {
		id64, err := r.createInTx(ctx, tx, auditID, req, createdBy)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id64)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("control.BulkCreate commit: %w", err)
	}

	controls := make([]*model.AuditControl, 0, len(ids))
	for _, id64 := range ids {
		c, err := r.GetByID(ctx, auditID, int(id64))
		if err != nil {
			return nil, err
		}
		controls = append(controls, c)
	}
	return controls, nil
}

func (r *controlRepository) Update(ctx context.Context, auditID, controlID int, req model.UpdateControlRequest, updatedBy string) error {
	setParts := []string{"updated_by = ?", "updated_at = NOW()"}
	args := []any{updatedBy}

	// Each optional field is prepended so updated_by stays at the end before WHERE args.
	addField := func(col string, val any) {
		setParts = append([]string{col + " = ?"}, setParts...)
		args = append([]any{val}, args...)
	}

	if req.ControlNumber != nil {
		addField("control_number", *req.ControlNumber)
	}
	if req.Description != nil {
		addField("description", *req.Description)
	}
	if req.EvidenceRequirement != nil {
		addField("evidence_requirement", *req.EvidenceRequirement)
	}
	if req.RequirementType != nil {
		addField("requirement_type", *req.RequirementType)
	}
	if req.ControlType != nil {
		addField("control_type", *req.ControlType)
	}
	if req.Scope != nil {
		addField("scope", *req.Scope)
	}
	if req.OwnerID != nil {
		addField("owner_id", *req.OwnerID)
	}
	if req.TeamID != nil {
		addField("team_id", *req.TeamID)
	}
	if req.AuditorID != nil {
		addField("auditor_id", *req.AuditorID)
	}
	if req.DueDate != nil {
		addField("due_date", *req.DueDate)
	}

	args = append(args, auditID, controlID)
	_, err := r.db.ExecContext(ctx,
		"UPDATE audit_control SET "+strings.Join(setParts, ", ")+" WHERE audit_id = ? AND id = ?",
		args...)
	if err != nil {
		return fmt.Errorf("control.Update(%d,%d): %w", auditID, controlID, err)
	}
	return nil
}

func (r *controlRepository) UpdateStatus(ctx context.Context, auditID, controlID int, status string, comment *string, updatedBy string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE audit_control
		SET status = ?, comments = ?, updated_by = ?, updated_at = NOW()
		WHERE audit_id = ? AND id = ?`,
		status, stringPtrVal(comment), updatedBy, auditID, controlID,
	)
	if err != nil {
		return fmt.Errorf("control.UpdateStatus(%d,%d): %w", auditID, controlID, err)
	}
	return nil
}

func (r *controlRepository) Delete(ctx context.Context, auditID, controlID int) error {
	_, err := r.db.ExecContext(ctx,
		"DELETE FROM audit_control WHERE audit_id = ? AND id = ?",
		auditID, controlID,
	)
	if err != nil {
		return fmt.Errorf("control.Delete(%d,%d): %w", auditID, controlID, err)
	}
	return nil
}

func (r *controlRepository) ListAssignedForEvidence(ctx context.Context, userEmail string) ([]*model.AssignedControlForEvidence, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT c.audit_id, a.name, c.id, c.control_number, c.description, c.status,
		       CONCAT('audits/', c.audit_id, '/controls/', c.id, '/evidence/') AS base_folder_path
		FROM audit_control c
		JOIN audit a ON a.id = c.audit_id
		JOIN ` + "`user`" + ` u ON u.email = ?
		WHERE c.team_id = u.audit_team_id
		  AND a.status = 'ACTIVE'
		  AND c.status IN (
		    'EVIDENCE_PENDING','SUBMITTED_SAMPLE','EVIDENCE_NEED_CLARIFICATION',
		    'POPULATION_PENDING','POPULATION_NEED_CLARIFICATION'
		  )
		ORDER BY c.due_date ASC, c.id ASC`, userEmail)
	if err != nil {
		return nil, fmt.Errorf("control.ListAssignedForEvidence: %w", err)
	}
	defer rows.Close()

	var list []*model.AssignedControlForEvidence
	for rows.Next() {
		var ac model.AssignedControlForEvidence
		if err := rows.Scan(&ac.AuditID, &ac.AuditName, &ac.ControlID, &ac.ControlNumber, &ac.Description, &ac.Status, &ac.BaseFolderPath); err != nil {
			return nil, fmt.Errorf("control.ListAssignedForEvidence scan: %w", err)
		}
		list = append(list, &ac)
	}
	if list == nil {
		list = []*model.AssignedControlForEvidence{}
	}
	return list, rows.Err()
}

func scanControl(s scanner) (*model.AuditControl, error) {
	var (
		id              int
		auditID         int
		ownerID         sql.NullInt64
		ownerName       sql.NullString
		teamID          sql.NullInt64
		teamName        sql.NullString
		auditorID       sql.NullInt64
		auditorName     sql.NullString
		controlNumber   string
		description     string
		evidenceReq     sql.NullString
		requirementType string
		controlType     string
		scope           string
		dueDate         sql.NullString
		status          string
		sampleRef       sql.NullString
		sampleFileURL   sql.NullString
		sampleFileName  sql.NullString
		comments        sql.NullString
		isManuallyAdded bool
		isOverdue       bool
		createdAt       time.Time
		updatedAt       time.Time
	)
	err := s.Scan(
		&id, &auditID,
		&ownerID, &ownerName,
		&teamID, &teamName,
		&auditorID, &auditorName,
		&controlNumber, &description, &evidenceReq,
		&requirementType, &controlType, &scope,
		&dueDate, &status,
		&sampleRef, &sampleFileURL, &sampleFileName,
		&comments, &isManuallyAdded, &isOverdue,
		&createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &model.AuditControl{
		ID:                  id,
		AuditID:             auditID,
		OwnerID:             nullIntPtr(ownerID),
		OwnerName:           nullStringPtr(ownerName),
		TeamID:              nullIntPtr(teamID),
		TeamName:            nullStringPtr(teamName),
		AuditorID:           nullIntPtr(auditorID),
		AuditorName:         nullStringPtr(auditorName),
		ControlNumber:       controlNumber,
		Description:         description,
		EvidenceRequirement: nullStringPtr(evidenceReq),
		RequirementType:     requirementType,
		ControlType:         controlType,
		Scope:               scope,
		DueDate:             nullStringPtr(dueDate),
		Status:              status,
		SampleReference:     nullStringPtr(sampleRef),
		SampleFileURL:       nullStringPtr(sampleFileURL),
		SampleFileName:      nullStringPtr(sampleFileName),
		Comments:            nullStringPtr(comments),
		IsManuallyAdded:     isManuallyAdded,
		IsOverdue:           isOverdue,
		CreatedAt:           createdAt,
		UpdatedAt:           updatedAt,
	}, nil
}
