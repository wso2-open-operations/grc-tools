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

// LookupKind identifies one of the four register-template lookup tables
// (RISK_MODULE_DESIGN.md §14). The tables share one shape, so one repository
// serves all four; a kind carries everything that differs between them.
type LookupKind struct {
	// table is the lookup table. Never caller-supplied: only the four kinds
	// below exist, so concatenating it into SQL is safe.
	table string
	// noun names a value in error messages, e.g. "platform".
	noun string
	// hasCode is true for risk_customer only.
	hasCode bool
	// hasParent is true for risk_platform only: a row with a parent_id is a
	// sub-platform of that top-level platform.
	hasParent bool
	// inUse is a boolean SQL expression over the lookup row aliased `l`. It
	// is true once any risk references the value. For customers it also
	// counts risk_customer_sequence: a sequence row means a risk code with
	// this customer's code was issued, even if that risk has since been
	// hard-deleted (the migration rollback does that), so the code must stay
	// frozen.
	inUse string
}

// The four lookup kinds. Exported so the service and routes name a kind
// rather than a table.
var (
	LookupPlatform = LookupKind{
		table: "risk_platform", noun: "platform", hasParent: true,
		inUse: "(EXISTS(SELECT 1 FROM risk_platform_reference x WHERE x.platform_id = l.id)" +
			" OR EXISTS(SELECT 1 FROM risk_team_platform x WHERE x.platform_id = l.id)" +
			" OR EXISTS(SELECT 1 FROM risk_platform x WHERE x.parent_id = l.id))",
	}
	LookupCustomer = LookupKind{
		table: "risk_customer", noun: "customer", hasCode: true,
		inUse: "(EXISTS(SELECT 1 FROM risk_managed_service_detail x WHERE x.customer_id = l.id)" +
			" OR EXISTS(SELECT 1 FROM risk_customer_sequence x WHERE x.customer_id = l.id))",
	}
	LookupProduct = LookupKind{
		table: "risk_product", noun: "product",
		inUse: "EXISTS(SELECT 1 FROM risk_product_reference x WHERE x.product_id = l.id)",
	}
	LookupDeploymentType = LookupKind{
		table: "risk_deployment_type", noun: "deployment type",
		inUse: "EXISTS(SELECT 1 FROM risk_managed_service_detail x WHERE x.deployment_type_id = l.id)",
	}
)

// Noun names a value of this kind in messages, e.g. "deployment type".
func (k LookupKind) Noun() string { return k.noun }

// HasCode reports whether values of this kind carry a code (customers only).
func (k LookupKind) HasCode() bool { return k.hasCode }

// HasParent reports whether values of this kind can sit under a parent
// (platforms only).
func (k LookupKind) HasParent() bool { return k.hasParent }

// RiskLookupRepository defines persistence operations for one lookup table.
type RiskLookupRepository interface {
	// ListRiskLookups returns every value ordered by name; statusKey
	// ("ACTIVE" | "INACTIVE" | "" for all) narrows it.
	ListRiskLookups(ctx context.Context, statusKey string) ([]domain.RiskLookup, error)
	GetRiskLookupByID(ctx context.Context, id int) (*domain.RiskLookup, error)
	CreateRiskLookup(ctx context.Context, req domain.CreateRiskLookupRequest) (*domain.RiskLookup, error)
	UpdateRiskLookup(ctx context.Context, id int, req domain.UpdateRiskLookupRequest) (*domain.RiskLookup, error)
	DeleteRiskLookup(ctx context.Context, id int) error
}

type riskLookupRepo struct {
	db   *sql.DB
	kind LookupKind
}

// NewRiskLookupRepository constructs a RiskLookupRepository over kind's table.
func NewRiskLookupRepository(db *sql.DB, kind LookupKind) RiskLookupRepository {
	return &riskLookupRepo{db: db, kind: kind}
}

// selectCols is the column list scanRiskLookup reads. code and parent_id are
// selected as NULL for the tables that lack them so one scan serves every kind.
func (r *riskLookupRepo) selectCols() string {
	code, parent := "NULL", "NULL"
	if r.kind.hasCode {
		code = "l.code"
	}
	if r.kind.hasParent {
		parent = "l.parent_id"
	}
	return "l.id, l.name, " + code + ", " + parent + ", l.status, " + r.kind.inUse
}

func (r *riskLookupRepo) ListRiskLookups(ctx context.Context, statusKey string) ([]domain.RiskLookup, error) {
	q := "SELECT " + r.selectCols() + " FROM " + r.kind.table + " l"
	args := []any{}
	if statusKey != "" {
		q += " WHERE l.status = ?"
		args = append(args, statusKey)
	}
	rows, err := r.db.QueryContext(ctx, q+" ORDER BY l.name", args...) // #nosec G202 -- table and columns are fixed per kind
	if err != nil {
		return nil, fmt.Errorf("%s.List: %w", r.kind.table, err)
	}
	defer rows.Close()

	var values []domain.RiskLookup
	for rows.Next() {
		v, err := scanRiskLookup(rows)
		if err != nil {
			return nil, fmt.Errorf("%s.List scan: %w", r.kind.table, err)
		}
		values = append(values, *v)
	}
	return values, rows.Err()
}

func (r *riskLookupRepo) GetRiskLookupByID(ctx context.Context, id int) (*domain.RiskLookup, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT "+r.selectCols()+" FROM "+r.kind.table+" l WHERE l.id = ?", id) // #nosec G202
	v, err := scanRiskLookup(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("%s %d not found", r.kind.noun, id)}
	}
	if err != nil {
		return nil, fmt.Errorf("%s.GetByID(%d): %w", r.kind.table, id, err)
	}
	return v, nil
}

// CreateRiskLookup inserts a new ACTIVE value. The unique keys on name (and
// code, for customers) are case-insensitive through the column collation;
// a clash maps to a 409.
func (r *riskLookupRepo) CreateRiskLookup(ctx context.Context, req domain.CreateRiskLookupRequest) (*domain.RiskLookup, error) {
	if req.ParentID != nil {
		return r.createSubValue(ctx, req)
	}
	var res sql.Result
	var err error
	if r.kind.hasCode {
		res, err = r.db.ExecContext(ctx,
			"INSERT INTO "+r.kind.table+" (name, code, created_by, updated_by) VALUES (?, ?, ?, ?)", // #nosec G202
			req.Name, *req.Code, req.CreatedBy, req.CreatedBy)
	} else {
		res, err = r.db.ExecContext(ctx,
			"INSERT INTO "+r.kind.table+" (name, created_by, updated_by) VALUES (?, ?, ?)", // #nosec G202
			req.Name, req.CreatedBy, req.CreatedBy)
	}
	if err != nil {
		if isDuplicateKey(err) {
			return nil, r.duplicateError()
		}
		return nil, fmt.Errorf("%s.Create: %w", r.kind.table, err)
	}
	id, _ := res.LastInsertId()
	return r.GetRiskLookupByID(ctx, int(id))
}

// UpdateRiskLookup renames, recodes or (de)activates a value. A customer's
// code is frozen once the customer is in use, because it is already part of
// issued risk codes that are never regenerated. The row is locked first so a
// risk created concurrently cannot slip in between the in-use check and the
// code change: inserting a risk_managed_service_detail or
// risk_customer_sequence row takes a shared lock on this customer row through
// its FK, so it waits for this transaction.
func (r *riskLookupRepo) UpdateRiskLookup(ctx context.Context, id int, req domain.UpdateRiskLookupRequest) (*domain.RiskLookup, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("%s.Update(%d) begin: %w", r.kind.table, id, err)
	}
	defer tx.Rollback() //nolint:errcheck

	var inUse bool
	if err := tx.QueryRowContext(ctx,
		"SELECT "+r.kind.inUse+" FROM "+r.kind.table+" l WHERE l.id = ? FOR UPDATE", id).Scan(&inUse); err != nil { // #nosec G202
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("%s %d not found", r.kind.noun, id)}
		}
		return nil, fmt.Errorf("%s.Update(%d) lock: %w", r.kind.table, id, err)
	}

	sets := []string{}
	args := []any{}
	if req.Name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *req.Name)
	}
	if req.Code != nil {
		var current string
		if err := tx.QueryRowContext(ctx,
			"SELECT code FROM "+r.kind.table+" WHERE id = ?", id).Scan(&current); err != nil { // #nosec G202
			return nil, fmt.Errorf("%s.Update(%d) read code: %w", r.kind.table, id, err)
		}
		// Re-sending the current code (a form that posts every field) is
		// not a change, so it is allowed even while the customer is in use.
		if *req.Code != current {
			if inUse {
				return nil, &apierror.ConflictError{
					Msg: "this customer's code is already part of issued risk codes and cannot be changed"}
			}
			sets = append(sets, "code = ?")
			args = append(args, *req.Code)
		}
	}
	if req.Status != nil {
		sets = append(sets, "status = ?")
		args = append(args, *req.Status)
	}
	sets = append(sets, "updated_by = ?")
	args = append(args, req.UpdatedBy, id)

	if _, err := tx.ExecContext(ctx,
		"UPDATE "+r.kind.table+" SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...); err != nil { // #nosec G202
		if isDuplicateKey(err) {
			return nil, r.duplicateError()
		}
		return nil, fmt.Errorf("%s.Update(%d): %w", r.kind.table, id, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("%s.Update(%d) commit: %w", r.kind.table, id, err)
	}
	return r.GetRiskLookupByID(ctx, id)
}

// DeleteRiskLookup removes a value no risk has ever referenced, e.g. a typo
// caught straight away. A value in use is refused with a 409 telling the
// admin to deactivate it instead. Every FK to these tables is RESTRICT, so
// the database refuses such a DELETE anyway; the explicit check gives a
// clear message, and the FK error mapping below covers a race the check
// cannot see.
func (r *riskLookupRepo) DeleteRiskLookup(ctx context.Context, id int) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%s.Delete(%d) begin: %w", r.kind.table, id, err)
	}
	defer tx.Rollback() //nolint:errcheck

	var inUse bool
	if err := tx.QueryRowContext(ctx,
		"SELECT "+r.kind.inUse+" FROM "+r.kind.table+" l WHERE l.id = ? FOR UPDATE", id).Scan(&inUse); err != nil { // #nosec G202
		if errors.Is(err, sql.ErrNoRows) {
			return &apierror.NotFoundError{Msg: fmt.Sprintf("%s %d not found", r.kind.noun, id)}
		}
		return fmt.Errorf("%s.Delete(%d) lock: %w", r.kind.table, id, err)
	}
	if inUse {
		return r.inUseError()
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM "+r.kind.table+" WHERE id = ?", id); err != nil { // #nosec G202
		if isFKViolation(err) {
			return r.inUseError()
		}
		return fmt.Errorf("%s.Delete(%d): %w", r.kind.table, id, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%s.Delete(%d) commit: %w", r.kind.table, id, err)
	}
	return nil
}

// createSubValue inserts a sub-platform under req.ParentID. The parent is read
// FOR SHARE so an admin cannot deactivate it, or make it a child, between the
// check and the insert. Only one level exists: the parent must itself be
// top-level, and must be ACTIVE like any value offered for new use.
func (r *riskLookupRepo) createSubValue(ctx context.Context, req domain.CreateRiskLookupRequest) (*domain.RiskLookup, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("%s.Create begin: %w", r.kind.table, err)
	}
	defer tx.Rollback() //nolint:errcheck

	var parentOfParent sql.NullInt64
	var status string
	if err := tx.QueryRowContext(ctx,
		"SELECT parent_id, status FROM "+r.kind.table+" WHERE id = ? FOR SHARE", *req.ParentID, // #nosec G202 -- table is fixed per kind
	).Scan(&parentOfParent, &status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &apierror.ValidationError{Msg: fmt.Sprintf("parent %s %d not found", r.kind.noun, *req.ParentID)}
		}
		return nil, fmt.Errorf("%s.Create read parent: %w", r.kind.table, err)
	}
	if parentOfParent.Valid {
		return nil, &apierror.ValidationError{Msg: "a sub-" + r.kind.noun + " cannot have its own sub-" + r.kind.noun + "s"}
	}
	if status != "ACTIVE" {
		return nil, &apierror.ValidationError{Msg: "the parent " + r.kind.noun + " is not active"}
	}
	res, err := tx.ExecContext(ctx,
		"INSERT INTO "+r.kind.table+" (name, parent_id, created_by, updated_by) VALUES (?, ?, ?, ?)", // #nosec G202
		req.Name, *req.ParentID, req.CreatedBy, req.CreatedBy)
	if err != nil {
		if isDuplicateKey(err) {
			return nil, r.duplicateError()
		}
		return nil, fmt.Errorf("%s.Create: %w", r.kind.table, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("%s.Create commit: %w", r.kind.table, err)
	}
	id, _ := res.LastInsertId()
	return r.GetRiskLookupByID(ctx, int(id))
}

func (r *riskLookupRepo) duplicateError() error {
	if r.kind.hasCode {
		return &apierror.ConflictError{Msg: fmt.Sprintf("a %s with that name or code already exists", r.kind.noun)}
	}
	return &apierror.ConflictError{Msg: fmt.Sprintf("a %s with that name already exists", r.kind.noun)}
}

func (r *riskLookupRepo) inUseError() error {
	return &apierror.ConflictError{
		Msg: fmt.Sprintf("this %s is used by existing risks, registers or sub-%ss and cannot be deleted; deactivate it instead", r.kind.noun, r.kind.noun)}
}

func scanRiskLookup(s scanner) (*domain.RiskLookup, error) {
	var v domain.RiskLookup
	var code sql.NullString
	var parentID sql.NullInt64
	if err := s.Scan(&v.ID, &v.Name, &code, &parentID, &v.Status, &v.InUse); err != nil {
		return nil, err
	}
	if parentID.Valid {
		id := int(parentID.Int64)
		v.ParentID = &id
	}
	if code.Valid {
		v.Code = &code.String
	}
	return &v, nil
}
