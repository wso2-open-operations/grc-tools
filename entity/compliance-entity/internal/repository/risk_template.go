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

// Register templates (RISK_MODULE_DESIGN.md §14), as stored in
// risk_team.register_template.
const (
	TemplateStandard        = "STANDARD"
	TemplateAggregated      = "AGGREGATED"
	TemplateManagedServices = "MANAGED_SERVICES"
)

// checkTemplateFields reports whether req carries exactly the template fields
// its source register's template allows and requires. It only looks at what
// was sent; whether the referenced values exist and are ACTIVE is checked
// against the database separately.
func checkTemplateFields(template string, req domain.CreateRiskRequest) error {
	var bad []string
	reject := func(present bool, field string) {
		if present {
			bad = append(bad, field)
		}
	}
	require := func(present bool, msg string) error {
		if !present {
			return &apierror.ValidationError{Msg: msg}
		}
		return nil
	}

	hasPlatforms := len(req.PlatformIDs) > 0
	hasCustomer := req.CustomerID != nil
	hasDeployment := req.DeploymentTypeID != nil
	hasProducts := len(req.ProductIDs) > 0
	hasEnvironments := len(req.Environments) > 0

	switch template {
	case TemplateStandard:
		reject(hasPlatforms, "platformIds")
		reject(hasCustomer, "customerId")
		reject(hasDeployment, "deploymentTypeId")
		reject(hasProducts, "productIds")
		reject(hasEnvironments, "environments")
	case TemplateAggregated:
		if err := require(hasPlatforms, "at least one platform is required for this register"); err != nil {
			return err
		}
		reject(hasCustomer, "customerId")
		reject(hasDeployment, "deploymentTypeId")
		reject(hasProducts, "productIds")
		reject(hasEnvironments, "environments")
	case TemplateManagedServices:
		for _, check := range []struct {
			present bool
			msg     string
		}{
			{hasCustomer, "customerId is required for this register"},
			{hasDeployment, "deploymentTypeId is required for this register"},
			{hasProducts, "at least one product is required for this register"},
			{hasEnvironments, "at least one environment is required for this register"},
		} {
			if err := require(check.present, check.msg); err != nil {
				return err
			}
		}
		reject(hasPlatforms, "platformIds")
		reject(len(req.ComplianceReferenceIDs) > 0, "complianceReferenceIds")
	default:
		// The column is an ENUM, so this means the code is behind the schema.
		return fmt.Errorf("unknown register template %q", template)
	}
	if len(bad) > 0 {
		return &apierror.ValidationError{
			Msg: fmt.Sprintf("%s not allowed for a register on the %s template", strings.Join(bad, ", "), template)}
	}
	return nil
}

// checkAssignmentTeam verifies, inside tx, that the assignment team exists.
// Any team may be the assignment team on any register: the register template
// only decides which fields a register's risks carry. The row is read FOR SHARE
// so an admin cannot delete it between this check and the risk that references
// it.
func checkAssignmentTeam(ctx context.Context, tx *sql.Tx, teamID int) error {
	var one int
	if err := tx.QueryRowContext(ctx,
		"SELECT 1 FROM risk_team WHERE id = ? FOR SHARE", teamID).Scan(&one); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &apierror.ValidationError{Msg: fmt.Sprintf("assignment team %d not found", teamID)}
		}
		return fmt.Errorf("risk.Create assignment team: %w", err)
	}
	return nil
}

// requireActive verifies, inside tx, that every id names an ACTIVE row of
// kind's table. The rows are read FOR SHARE so an admin cannot deactivate or
// delete one between this check and the insert that references it.
func requireActive(ctx context.Context, tx *sql.Tx, kind LookupKind, ids []int) error {
	if len(ids) == 0 {
		return nil
	}
	ph := strings.Repeat("?,", len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	var active int
	if err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM "+kind.table+" WHERE status = 'ACTIVE' AND id IN ("+ph[:len(ph)-1]+") FOR SHARE", // #nosec G202 -- table is fixed per kind
		args...).Scan(&active); err != nil {
		return fmt.Errorf("risk.Create check %s: %w", kind.table, err)
	}
	// The service has already rejected duplicate ids, so a short count means
	// at least one id is missing or INACTIVE.
	if active != len(ids) {
		return &apierror.ValidationError{Msg: fmt.Sprintf("a selected %s does not exist or is no longer active", kind.noun)}
	}
	return nil
}

// checkTemplateValues verifies every referenced lookup value is ACTIVE.
func checkTemplateValues(ctx context.Context, tx *sql.Tx, req domain.CreateRiskRequest) error {
	single := func(id *int) []int {
		if id == nil {
			return nil
		}
		return []int{*id}
	}
	for _, c := range []struct {
		kind LookupKind
		ids  []int
	}{
		{LookupPlatform, req.PlatformIDs},
		{LookupProduct, req.ProductIDs},
		{LookupCustomer, single(req.CustomerID)},
		{LookupDeploymentType, single(req.DeploymentTypeID)},
	} {
		if err := requireActive(ctx, tx, c.kind, c.ids); err != nil {
			return err
		}
	}
	return checkRegisterPlatforms(ctx, tx, req.SourceRegisterID, req.PlatformIDs, req.PlatformIDs)
}

// checkRegisterPlatforms applies a register's platform list to a risk's
// platforms (want), inside tx:
//   - a platform in added must be on the register's list, when it has one (a
//     register with no rows offers every platform, as every register did before
//     the list existed);
//   - a sub-platform needs its platform on the risk too;
//   - a newly added platform whose sub-platforms the register lists needs at
//     least one of them on the risk.
//
// added is the subset of want the caller is writing now. Platforms the risk
// already had are not re-checked for membership, so a register narrowed later
// never blocks an edit of a risk it already carries.
func checkRegisterPlatforms(ctx context.Context, tx *sql.Tx, registerID int, want, added []int) error {
	if len(want) == 0 {
		return nil
	}
	ph := strings.Repeat("?,", len(want))
	parentOf, err := platformParents(ctx, tx,
		"SELECT id, parent_id FROM risk_platform WHERE id IN ("+ph[:len(ph)-1]+")", toAny(want)...) // #nosec G202 -- placeholders only
	if err != nil {
		return err
	}
	onRisk := map[int]bool{}
	for _, id := range want {
		onRisk[id] = true
	}
	for _, id := range want {
		if parent := parentOf[id]; parent != 0 && !onRisk[parent] {
			return &apierror.ValidationError{
				Msg: fmt.Sprintf("sub-platform %d needs its platform %d selected too", id, parent)}
		}
	}

	offered, err := platformParents(ctx, tx, `
		SELECT p.id, p.parent_id FROM risk_team_platform t
		JOIN risk_platform p ON p.id = t.platform_id
		WHERE t.risk_team_id = ?`, registerID)
	if err != nil {
		return err
	}
	if len(offered) == 0 {
		return nil
	}
	// Only an active sub-platform counts towards the requirement below: an
	// inactive one drops out of every picker, so demanding it would leave the
	// risk impossible to raise. The register stays restricted regardless.
	active, err := platformParents(ctx, tx, `
		SELECT p.id, p.parent_id FROM risk_team_platform t
		JOIN risk_platform p ON p.id = t.platform_id
		WHERE t.risk_team_id = ? AND p.status = 'ACTIVE'`, registerID)
	if err != nil {
		return err
	}
	for _, id := range added {
		if _, ok := offered[id]; !ok {
			return &apierror.ValidationError{Msg: fmt.Sprintf("platform %d is not offered by this register", id)}
		}
	}
	for _, id := range added {
		if parentOf[id] != 0 {
			continue
		}
		hasOfferedSub, hasChosenSub := false, false
		for sub, parent := range active {
			if parent != id {
				continue
			}
			hasOfferedSub = true
			if onRisk[sub] {
				hasChosenSub = true
			}
		}
		if hasOfferedSub && !hasChosenSub {
			return &apierror.ValidationError{
				Msg: fmt.Sprintf("select at least one sub-platform of platform %d", id)}
		}
	}
	return nil
}

// platformParents runs query, which selects (id, parent_id), and returns each
// row's parent, 0 for a top-level platform (ids start at 1).
func platformParents(ctx context.Context, tx *sql.Tx, query string, args ...any) (map[int]int, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("risk platforms: %w", err)
	}
	defer rows.Close()
	out := map[int]int{}
	for rows.Next() {
		var id int
		var parent sql.NullInt64
		if err := rows.Scan(&id, &parent); err != nil {
			return nil, fmt.Errorf("risk platforms scan: %w", err)
		}
		out[id] = int(parent.Int64) // 0 when NULL
	}
	return out, rows.Err()
}

// reserveCustomerSequence takes the next per-(register, customer) sequence
// number for a Managed Services risk, with the same INSERT IGNORE + SELECT
// FOR UPDATE + UPDATE pattern CreateRisk uses for risk_register_sequence.
func reserveCustomerSequence(ctx context.Context, tx *sql.Tx, registerID, customerID int) (int, error) {
	if _, err := tx.ExecContext(ctx,
		"INSERT IGNORE INTO risk_customer_sequence (risk_team_id, customer_id, last_sequence_number) VALUES (?, ?, 0)",
		registerID, customerID); err != nil {
		return 0, fmt.Errorf("risk.Create ensure customer sequence: %w", err)
	}
	var last int
	if err := tx.QueryRowContext(ctx,
		"SELECT last_sequence_number FROM risk_customer_sequence WHERE risk_team_id = ? AND customer_id = ? FOR UPDATE",
		registerID, customerID).Scan(&last); err != nil {
		return 0, fmt.Errorf("risk.Create lock customer sequence: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE risk_customer_sequence SET last_sequence_number = ? WHERE risk_team_id = ? AND customer_id = ?",
		last+1, registerID, customerID); err != nil {
		return 0, fmt.Errorf("risk.Create bump customer sequence: %w", err)
	}
	return last + 1, nil
}

// writeTemplateData inserts riskID's detail and junction rows. checkTemplateFields
// has already made sure only the current template's fields are set, so this
// writes whatever is present.
func writeTemplateData(ctx context.Context, tx *sql.Tx, riskID int, req domain.CreateRiskRequest) error {
	if req.CustomerID != nil {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO risk_managed_service_detail (risk_id, customer_id, deployment_type_id, created_by, updated_by)
			 VALUES (?, ?, ?, ?, ?)`,
			riskID, *req.CustomerID, *req.DeploymentTypeID, req.CreatedBy, req.CreatedBy); err != nil {
			return fmt.Errorf("risk.Create managed service detail: %w", err)
		}
	}
	for _, id := range req.PlatformIDs {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO risk_platform_reference (risk_id, platform_id) VALUES (?, ?)", riskID, id); err != nil {
			return fmt.Errorf("risk.Create platform %d: %w", id, err)
		}
	}
	for _, id := range req.ProductIDs {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO risk_product_reference (risk_id, product_id) VALUES (?, ?)", riskID, id); err != nil {
			return fmt.Errorf("risk.Create product %d: %w", id, err)
		}
	}
	for _, env := range req.Environments {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO risk_environment_reference (risk_id, environment) VALUES (?, ?)", riskID, env); err != nil {
			return fmt.Errorf("risk.Create environment %s: %w", env, err)
		}
	}
	return nil
}

// detailTemplateValues fills d's register-template values. Each read is
// keyed on the risk, so a template that lacks a field simply finds no rows.
func (r *riskRepo) detailTemplateValues(ctx context.Context, id int, d *domain.RiskDetail) error {
	var custID, deployID sql.NullInt64
	var custName, custCode, custStatus, deployName, deployStatus sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT c.id, c.name, c.code, c.status, dt.id, dt.name, dt.status
		FROM risk_managed_service_detail m
		JOIN risk_customer c         ON c.id  = m.customer_id
		JOIN risk_deployment_type dt ON dt.id = m.deployment_type_id
		WHERE m.risk_id = ?`, id).
		Scan(&custID, &custName, &custCode, &custStatus, &deployID, &deployName, &deployStatus)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("risk.GetDetail managed service detail: %w", err)
	}
	if err == nil {
		code := custCode.String
		d.Customer = &domain.RiskLookupRef{ID: int(custID.Int64), Name: custName.String, Code: &code, Status: custStatus.String}
		d.DeploymentType = &domain.RiskLookupRef{ID: int(deployID.Int64), Name: deployName.String, Status: deployStatus.String}
	}

	if d.Products, err = r.detailLookupList(ctx, `
		SELECT p.id, p.name, p.status, NULL FROM risk_product_reference x
		JOIN risk_product p ON p.id = x.product_id
		WHERE x.risk_id = ? ORDER BY p.name`, id); err != nil {
		return fmt.Errorf("risk.GetDetail products: %w", err)
	}
	if d.Platforms, err = r.detailLookupList(ctx, `
		SELECT p.id, p.name, p.status, p.parent_id FROM risk_platform_reference x
		JOIN risk_platform p ON p.id = x.platform_id
		WHERE x.risk_id = ? ORDER BY p.name`, id); err != nil {
		return fmt.Errorf("risk.GetDetail platforms: %w", err)
	}
	return nil
}

// detailLookupList runs query (selecting id, name, status for one risk) and
// returns the rows as refs; empty, not nil, when there are none.
func (r *riskRepo) detailLookupList(ctx context.Context, query string, id int) ([]domain.RiskLookupRef, error) {
	rows, err := r.db.QueryContext(ctx, query, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	refs := []domain.RiskLookupRef{}
	for rows.Next() {
		var ref domain.RiskLookupRef
		var parentID sql.NullInt64
		if err := rows.Scan(&ref.ID, &ref.Name, &ref.Status, &parentID); err != nil {
			return nil, err
		}
		if parentID.Valid {
			id := int(parentID.Int64)
			ref.ParentID = &id
		}
		refs = append(refs, ref)
	}
	return refs, rows.Err()
}

// touchesTemplate reports whether an update needs the risk's template: it
// sets a template field, adds compliance references, or moves the risk to
// another assignment team.
func touchesTemplate(req domain.UpdateRiskRequest) bool {
	return req.PlatformIDs != nil || req.DeploymentTypeID != nil || req.ProductIDs != nil ||
		req.Environments != nil || len(req.ComplianceReferenceIDs) > 0 || req.AssignmentTeamID != nil
}

// checkTemplateUpdate applies the register-template rules to an update,
// inside its transaction:
//   - a template field may only be set on a risk whose template has it, and a
//     multi-valued one may not be emptied;
//   - a Managed Services risk takes no compliance references;
//   - a new assignment team must exist (an unchanged one is not re-checked,
//     so an edit form that re-posts every field still works on older data);
//   - newly added lookup values must be ACTIVE, while values the risk
//     already has may stay even if since deactivated.
//
// The risk row is locked FOR UPDATE, because UpdateRisk writes it later in the
// same transaction (a FOR SHARE here would deadlock two concurrent edits when
// both upgrade to exclusive). Its register is read FOR SHARE, so the
// register's template cannot change underneath this edit.
func checkTemplateUpdate(ctx context.Context, tx *sql.Tx, riskID int, req domain.UpdateRiskRequest) error {
	if !touchesTemplate(req) {
		return nil
	}
	var template string
	var currentTeam, registerID int
	if err := tx.QueryRowContext(ctx, `
		SELECT t.register_template, r.assignment_team_id, r.source_register_id
		FROM risk r JOIN risk_team t ON t.id = r.source_register_id
		WHERE r.id = ? FOR UPDATE OF r FOR SHARE OF t`, riskID).Scan(&template, &currentTeam, &registerID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &apierror.NotFoundError{Msg: fmt.Sprintf("risk %d not found", riskID)}
		}
		return fmt.Errorf("risk.Update read template: %w", err)
	}

	for _, f := range []struct {
		set      bool
		empty    bool
		field    string
		template string
	}{
		{req.PlatformIDs != nil, len(req.PlatformIDs) == 0, "platformIds", TemplateAggregated},
		{req.DeploymentTypeID != nil, false, "deploymentTypeId", TemplateManagedServices},
		{req.ProductIDs != nil, len(req.ProductIDs) == 0, "productIds", TemplateManagedServices},
		{req.Environments != nil, len(req.Environments) == 0, "environments", TemplateManagedServices},
	} {
		if !f.set {
			continue
		}
		if template != f.template {
			return &apierror.ValidationError{
				Msg: fmt.Sprintf("%s not allowed for a register on the %s template", f.field, template)}
		}
		if f.empty {
			return &apierror.ValidationError{Msg: f.field + " cannot be emptied; at least one is required"}
		}
	}
	if len(req.ComplianceReferenceIDs) > 0 && template == TemplateManagedServices {
		return &apierror.ValidationError{
			Msg: fmt.Sprintf("complianceReferenceIds not allowed for a register on the %s template", template)}
	}
	if req.AssignmentTeamID != nil && *req.AssignmentTeamID != currentTeam {
		if err := checkAssignmentTeam(ctx, tx, *req.AssignmentTeamID); err != nil {
			return err
		}
	}

	added, err := addedIDs(ctx, tx, "SELECT platform_id FROM risk_platform_reference WHERE risk_id = ?", riskID, req.PlatformIDs)
	if err != nil {
		return err
	}
	if err := requireActive(ctx, tx, LookupPlatform, added); err != nil {
		return err
	}
	if err := checkRegisterPlatforms(ctx, tx, registerID, req.PlatformIDs, added); err != nil {
		return err
	}
	if added, err = addedIDs(ctx, tx, "SELECT product_id FROM risk_product_reference WHERE risk_id = ?", riskID, req.ProductIDs); err != nil {
		return err
	}
	if err := requireActive(ctx, tx, LookupProduct, added); err != nil {
		return err
	}
	if req.DeploymentTypeID != nil {
		// writeTemplateUpdate changes the deployment type with a plain UPDATE,
		// which would silently change nothing if the risk had no detail row.
		// Every Managed Services risk has one (it is written in the same
		// transaction as the risk), so a missing row is a data problem to
		// surface, not to skip.
		var hasDetail bool
		if err := tx.QueryRowContext(ctx,
			"SELECT EXISTS(SELECT 1 FROM risk_managed_service_detail WHERE risk_id = ?)", riskID).Scan(&hasDetail); err != nil {
			return fmt.Errorf("risk.Update check managed service detail: %w", err)
		}
		if !hasDetail {
			return fmt.Errorf("risk %d is on a Managed Services register but has no managed service detail row", riskID)
		}
		if added, err = addedIDs(ctx, tx, "SELECT deployment_type_id FROM risk_managed_service_detail WHERE risk_id = ?",
			riskID, []int{*req.DeploymentTypeID}); err != nil {
			return err
		}
		if err := requireActive(ctx, tx, LookupDeploymentType, added); err != nil {
			return err
		}
	}
	return nil
}

// addedIDs returns the ids in want that the risk does not already have, per
// currentQuery (one id column, keyed on the risk).
func addedIDs(ctx context.Context, tx *sql.Tx, currentQuery string, riskID int, want []int) ([]int, error) {
	if len(want) == 0 {
		return nil, nil
	}
	rows, err := tx.QueryContext(ctx, currentQuery, riskID)
	if err != nil {
		return nil, fmt.Errorf("risk.Update current values: %w", err)
	}
	defer rows.Close()
	have := map[int]bool{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("risk.Update current values scan: %w", err)
		}
		have[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("risk.Update current values: %w", err)
	}
	var added []int
	for _, id := range want {
		if !have[id] {
			added = append(added, id)
		}
	}
	return added, nil
}

// writeTemplateUpdate rewrites the template fields the update sets, each as a
// whole set. checkTemplateUpdate has already confirmed they belong to the
// risk's template.
func writeTemplateUpdate(ctx context.Context, tx *sql.Tx, riskID int, req domain.UpdateRiskRequest) error {
	if req.DeploymentTypeID != nil {
		if _, err := tx.ExecContext(ctx,
			"UPDATE risk_managed_service_detail SET deployment_type_id = ?, updated_by = ? WHERE risk_id = ?",
			*req.DeploymentTypeID, req.UpdatedBy, riskID); err != nil {
			return fmt.Errorf("risk.Update deployment type: %w", err)
		}
	}
	for _, j := range []struct {
		set    bool
		table  string
		column string
		values []any
	}{
		{req.PlatformIDs != nil, "risk_platform_reference", "platform_id", toAny(req.PlatformIDs)},
		{req.ProductIDs != nil, "risk_product_reference", "product_id", toAny(req.ProductIDs)},
		{req.Environments != nil, "risk_environment_reference", "environment", toAny(req.Environments)},
	} {
		if !j.set {
			continue
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+j.table+" WHERE risk_id = ?", riskID); err != nil { // #nosec G202 -- fixed table
			return fmt.Errorf("risk.Update clear %s: %w", j.table, err)
		}
		for _, v := range j.values {
			if _, err := tx.ExecContext(ctx,
				"INSERT INTO "+j.table+" (risk_id, "+j.column+") VALUES (?, ?)", riskID, v); err != nil { // #nosec G202
				return fmt.Errorf("risk.Update %s: %w", j.table, err)
			}
		}
	}
	return nil
}

func toAny[T any](vs []T) []any {
	out := make([]any, len(vs))
	for i, v := range vs {
		out[i] = v
	}
	return out
}
