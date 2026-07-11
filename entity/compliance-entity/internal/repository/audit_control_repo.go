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

// ControlRepository defines persistence operations for the audit_control table.
type ControlRepository interface {
	SearchControls(ctx context.Context, auditID int, req domain.SearchControlsRequest) ([]domain.AuditControl, int, error)
	SearchControlsGlobal(ctx context.Context, req domain.SearchControlsRequest) ([]domain.AuditControl, int, error)
	GetControlByID(ctx context.Context, auditID, controlID int) (*domain.AuditControl, error)
	CreateControl(ctx context.Context, auditID int, req domain.CreateControlRequest) (*domain.AuditControl, error)
	BulkCreateControls(ctx context.Context, auditID int, reqs []domain.CreateControlRequest) ([]domain.AuditControl, error)
	UpdateControl(ctx context.Context, auditID, controlID int, req domain.UpdateControlRequest) (*domain.AuditControl, error)
	DeleteControl(ctx context.Context, auditID, controlID int) error
	ListAssignedForEvidence(ctx context.Context, userEmail string) ([]domain.AssignedControlForEvidence, error)
}

type controlRepo struct{ db *sql.DB }

// NewControlRepository constructs a ControlRepository.
func NewControlRepository(db *sql.DB) ControlRepository { return &controlRepo{db: db} }

// ListAssignedForEvidence returns the active-audit controls whose team the user
// belongs to and whose status requires evidence submission.
func (r *controlRepo) ListAssignedForEvidence(ctx context.Context, userEmail string) ([]domain.AssignedControlForEvidence, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT a.id, a.name, c.id, c.control_number, c.description, c.status
		FROM audit_control c
		JOIN audit      a ON a.id = c.audit_id
		JOIN audit_team t ON t.id = c.team_id
		JOIN `+"`user`"+` u ON u.audit_team_id = t.id
		WHERE u.email = ?
		  AND a.status = 'ACTIVE'
		  AND c.status IN ('EVIDENCE_PENDING','EVIDENCE_NEED_CLARIFICATION','SUBMITTED_SAMPLE')
		ORDER BY a.id, c.control_number`, userEmail)
	if err != nil {
		return nil, fmt.Errorf("control.ListAssignedForEvidence: %w", err)
	}
	defer rows.Close()

	out := []domain.AssignedControlForEvidence{}
	for rows.Next() {
		var ac domain.AssignedControlForEvidence
		if err := rows.Scan(&ac.AuditID, &ac.AuditName, &ac.ControlID, &ac.ControlNumber, &ac.Description, &ac.Status); err != nil {
			return nil, fmt.Errorf("control.ListAssignedForEvidence scan: %w", err)
		}
		ac.BaseFolderPath = fmt.Sprintf("audits/%d/controls/%d/evidence/", ac.AuditID, ac.ControlID)
		out = append(out, ac)
	}
	return out, rows.Err()
}

const controlSelectCols = `
  c.id, c.audit_id,
  c.framework_control_id,
  COALESCE(fc.control_number,       c.control_number)       AS control_number,
  COALESCE(fc.description,          c.description)          AS description,
  COALESCE(fc.evidence_requirement, c.evidence_requirement) AS evidence_requirement,
  COALESCE(fc.requirement_type,     c.requirement_type)     AS requirement_type,
  COALESCE(fc.control_type,         c.control_type)         AS control_type,
  COALESCE(fc.scope,                c.scope)                AS scope,
  fc.version                                                AS template_version,
  c.owner_id,   u_owner.display_name AS owner_name,
  c.team_id,    t.name               AS team_name,
  c.auditor_id, u_aud.display_name   AS auditor_name,
  DATE_FORMAT(c.due_date, '%Y-%m-%d') AS due_date,
  c.status, c.control_source,
  (c.due_date IS NOT NULL AND c.due_date < CURDATE() AND c.status != 'COMPLETE') AS is_overdue,
  c.created_at, c.updated_at`

const controlFromClause = `
FROM audit_control c
LEFT JOIN audit_framework_control fc ON fc.id = c.framework_control_id
LEFT JOIN ` + "`user`" + ` u_owner ON u_owner.id = c.owner_id
LEFT JOIN audit_team t            ON t.id          = c.team_id
LEFT JOIN ` + "`user`" + ` u_aud   ON u_aud.id     = c.auditor_id`

func (r *controlRepo) SearchControls(ctx context.Context, auditID int, req domain.SearchControlsRequest) ([]domain.AuditControl, int, error) {
	where, args := buildControlFilters("WHERE c.audit_id = ?", []any{auditID}, req)
	return r.runControlSearch(ctx, where, args, req, "control.Search")
}

func (r *controlRepo) SearchControlsGlobal(ctx context.Context, req domain.SearchControlsRequest) ([]domain.AuditControl, int, error) {
	where, args := buildControlFilters("WHERE 1=1", []any{}, req)
	return r.runControlSearch(ctx, where, args, req, "control.SearchGlobal")
}

// buildControlFilters appends the optional filter clauses from req onto seedWhere/seedArgs
// and returns the combined WHERE clause and argument list.
func buildControlFilters(seedWhere string, seedArgs []any, req domain.SearchControlsRequest) (string, []any) {
	where := seedWhere
	args := seedArgs

	if req.SearchQuery != "" {
		where += " AND (c.control_number LIKE ? OR c.description LIKE ?)"
		p := "%" + likeEscape(req.SearchQuery) + "%"
		args = append(args, p, p)
	}
	if len(req.StatusKeys) > 0 {
		ph := strings.Repeat("?,", len(req.StatusKeys))
		where += " AND c.status IN (" + ph[:len(ph)-1] + ")"
		for _, s := range req.StatusKeys {
			args = append(args, s)
		}
	}
	if len(req.RequirementTypes) > 0 {
		ph := strings.Repeat("?,", len(req.RequirementTypes))
		where += " AND c.requirement_type IN (" + ph[:len(ph)-1] + ")"
		for _, rt := range req.RequirementTypes {
			args = append(args, rt)
		}
	}
	if len(req.TeamIDs) > 0 {
		ph := strings.Repeat("?,", len(req.TeamIDs))
		where += " AND c.team_id IN (" + ph[:len(ph)-1] + ")"
		for _, id := range req.TeamIDs {
			args = append(args, id)
		}
	}
	if len(req.AuditorIDs) > 0 {
		ph := strings.Repeat("?,", len(req.AuditorIDs))
		where += " AND c.auditor_id IN (" + ph[:len(ph)-1] + ")"
		for _, id := range req.AuditorIDs {
			args = append(args, id)
		}
	}
	if len(req.OwnerIDs) > 0 {
		ph := strings.Repeat("?,", len(req.OwnerIDs))
		where += " AND c.owner_id IN (" + ph[:len(ph)-1] + ")"
		for _, id := range req.OwnerIDs {
			args = append(args, id)
		}
	}
	return where, args
}

// runControlSearch executes the count + paginated data query and scans the results.
func (r *controlRepo) runControlSearch(ctx context.Context, where string, args []any, req domain.SearchControlsRequest, errPrefix string) ([]domain.AuditControl, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) "+controlFromClause+" "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("%s count: %w", errPrefix, err)
	}

	dataArgs := append(append([]any{}, args...), req.Pagination.Limit, req.Pagination.Offset)
	rows, err := r.db.QueryContext(ctx,
		"SELECT"+controlSelectCols+controlFromClause+" "+where+
			" ORDER BY c.control_number LIMIT ? OFFSET ?",
		dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("%s query: %w", errPrefix, err)
	}
	defer rows.Close()

	var controls []domain.AuditControl
	for rows.Next() {
		c, err := scanControl(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("%s scan: %w", errPrefix, err)
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
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("control %d not found in audit %d", controlID, auditID)}
	}
	if err != nil {
		return nil, fmt.Errorf("control.GetByID(%d,%d): %w", auditID, controlID, err)
	}
	return c, nil
}

func (r *controlRepo) CreateControl(ctx context.Context, auditID int, req domain.CreateControlRequest) (*domain.AuditControl, error) {
	controlSource := req.ControlSource
	if controlSource == "" {
		controlSource = "MANUAL"
	}
	initialStatus := "EVIDENCE_PENDING"
	if req.RequirementType == "OE" {
		initialStatus = "POPULATION_PENDING"
	}
	defCols := controlDefinitionCols(req)

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("control.Create begin: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`INSERT INTO audit_control
		 (audit_id, framework_control_id,
		  control_number, description, evidence_requirement, requirement_type, control_type, scope,
		  owner_id, team_id, auditor_id, due_date, status, control_source, created_by, updated_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		auditID,
		nullableInt(req.FrameworkControlID),
		defCols.controlNumber, defCols.description, defCols.evidenceReq,
		defCols.requirementType, defCols.controlType, defCols.scope,
		nullableInt(req.OwnerID), nullableInt(req.TeamID), nullableInt(req.AuditorID),
		req.DueDate,
		initialStatus,
		controlSource,
		req.CreatedBy, req.CreatedBy)
	if err != nil {
		return nil, fmt.Errorf("control.Create: %w", err)
	}
	id, _ := res.LastInsertId()
	if req.Population != nil && strings.EqualFold(req.RequirementType, "OE") {
		p := req.Population
		desc := nullableString(&p.Description)
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO audit_population
			 (control_id, owner_id, team_id, reference_number, description, due_date, status, created_by, updated_by)
			 VALUES (?, ?, ?, ?, ?, ?, 'PENDING', ?, ?)`,
			id, nullableInt(p.OwnerID), nullableInt(p.TeamID),
			p.ReferenceNumber, desc, p.DueDate,
			req.CreatedBy, req.CreatedBy); err != nil {
			return nil, fmt.Errorf("control.Create population: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("control.Create commit: %w", err)
	}
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
		controlSource := req.ControlSource
		if controlSource == "" {
			controlSource = "MANUAL"
		}
		initialStatus := "EVIDENCE_PENDING"
		if req.RequirementType == "OE" {
			initialStatus = "POPULATION_PENDING"
		}
		defCols := controlDefinitionCols(req)
		res, err := tx.ExecContext(ctx,
			`INSERT INTO audit_control
			 (audit_id, framework_control_id,
			  control_number, description, evidence_requirement, requirement_type, control_type, scope,
			  owner_id, team_id, auditor_id, due_date, status, control_source, created_by, updated_by)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			auditID,
			nullableInt(req.FrameworkControlID),
			defCols.controlNumber, defCols.description, defCols.evidenceReq,
			defCols.requirementType, defCols.controlType, defCols.scope,
			nullableInt(req.OwnerID), nullableInt(req.TeamID), nullableInt(req.AuditorID),
			req.DueDate, initialStatus, controlSource,
			req.CreatedBy, req.CreatedBy)
		if err != nil {
			return nil, fmt.Errorf("control.BulkCreate insert %q: %w", req.ControlNumber, err)
		}
		id, _ := res.LastInsertId()
		ids = append(ids, int(id))
		if req.Population != nil && strings.EqualFold(req.RequirementType, "OE") {
			p := req.Population
			desc := nullableString(&p.Description)
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO audit_population
				 (control_id, owner_id, team_id, reference_number, description, due_date, status, created_by, updated_by)
				 VALUES (?, ?, ?, ?, ?, ?, 'PENDING', ?, ?)`,
				id, nullableInt(p.OwnerID), nullableInt(p.TeamID),
				p.ReferenceNumber, desc, p.DueDate,
				req.CreatedBy, req.CreatedBy); err != nil {
				return nil, fmt.Errorf("control.BulkCreate population %q: %w", req.ControlNumber, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("control.BulkCreate commit: %w", err)
	}

	ph := strings.Repeat("?,", len(ids))
	inArgs := []any{auditID}
	for _, id := range ids {
		inArgs = append(inArgs, id)
	}
	rows, err := r.db.QueryContext(ctx,
		"SELECT"+controlSelectCols+controlFromClause+
			" WHERE c.audit_id = ? AND c.id IN ("+ph[:len(ph)-1]+")"+
			" ORDER BY c.control_number",
		inArgs...)
	if err != nil {
		return nil, fmt.Errorf("control.BulkCreate fetch: %w", err)
	}
	defer rows.Close()

	var controls []domain.AuditControl
	for rows.Next() {
		c, err := scanControl(rows)
		if err != nil {
			return nil, fmt.Errorf("control.BulkCreate fetch scan: %w", err)
		}
		controls = append(controls, *c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("control.BulkCreate fetch rows: %w", err)
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
	sets = append(sets, "updated_by = ?")
	args = append(args, req.UpdatedBy)

	var query string
	if req.ExpectedStatus != "" {
		args = append(args, auditID, controlID, req.ExpectedStatus)
		query = "UPDATE audit_control SET " + strings.Join(sets, ", ") + " WHERE audit_id = ? AND id = ? AND status = ?" // #nosec G202
	} else {
		args = append(args, auditID, controlID)
		query = "UPDATE audit_control SET " + strings.Join(sets, ", ") + " WHERE audit_id = ? AND id = ?" // #nosec G202
	}

	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("control.Update(%d,%d): %w", auditID, controlID, err)
	}
	if req.ExpectedStatus != "" {
		if n, _ := result.RowsAffected(); n == 0 {
			current, err := r.GetControlByID(ctx, auditID, controlID)
			if err != nil {
				return nil, err // propagates NotFoundError if record was deleted
			}
			if current.Status == req.ExpectedStatus && (req.Status == nil || *req.Status == req.ExpectedStatus) {
				return current, nil // MySQL no-op: status not being changed, or being set to its current value
			}
			return nil, &apierror.ConflictError{Msg: "control was modified concurrently, please retry"}
		}
	}
	return r.GetControlByID(ctx, auditID, controlID)
}

func scanControl(s scanner) (*domain.AuditControl, error) {
	var c domain.AuditControl
	var frameworkControlID, templateVersion, ownerID, teamID, auditorID sql.NullInt64
	var evidenceReq, ownerName, teamName, auditorName, dueDate sql.NullString
	err := s.Scan(
		&c.ID, &c.AuditID,
		&frameworkControlID,
		&c.ControlNumber, &c.Description, &evidenceReq,
		&c.RequirementType, &c.ControlType, &c.Scope,
		&templateVersion,
		&ownerID, &ownerName,
		&teamID, &teamName,
		&auditorID, &auditorName,
		&dueDate,
		&c.Status, &c.ControlSource, &c.IsOverdue,
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
	c.FrameworkControlID = nullIntPtr(frameworkControlID)
	c.TemplateVersion = nullIntPtr(templateVersion)
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

// controlDefCols holds the nullable definition values to store in audit_control.
// When framework_control_id is set these are NULL (resolved via COALESCE on read).
type controlDefCols struct {
	controlNumber, description, requirementType, controlType, scope sql.NullString
	evidenceReq                                                     sql.NullString
}

// controlDefinitionCols returns the definition column values for an INSERT.
// When FrameworkControlID is set in the request all definition columns become NULL
// because the COALESCE query reads them from the template table instead.
func controlDefinitionCols(req domain.CreateControlRequest) controlDefCols {
	if req.FrameworkControlID != nil {
		return controlDefCols{} // all NullString{Valid:false} → NULL
	}
	d := controlDefCols{
		controlNumber:   sql.NullString{String: req.ControlNumber, Valid: req.ControlNumber != ""},
		description:     sql.NullString{String: req.Description, Valid: req.Description != ""},
		requirementType: sql.NullString{String: req.RequirementType, Valid: req.RequirementType != ""},
		controlType:     sql.NullString{String: req.ControlType, Valid: req.ControlType != ""},
		scope:           sql.NullString{String: req.Scope, Valid: req.Scope != ""},
	}
	if req.EvidenceRequirement != nil {
		d.evidenceReq = sql.NullString{String: *req.EvidenceRequirement, Valid: true}
	}
	return d
}
