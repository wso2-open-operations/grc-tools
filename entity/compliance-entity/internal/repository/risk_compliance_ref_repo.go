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

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

// RiskComplianceRefRepository manages the risk_compliance_reference junction table
// (many-to-many between risk and risk_security_compliance_reference).
type RiskComplianceRefRepository interface {
	AddRiskComplianceRef(ctx context.Context, riskID int, req domain.AddRiskComplianceRefRequest) (*domain.RiskComplianceRefLink, error)
	DeleteRiskComplianceRef(ctx context.Context, riskID, referenceID int) error
	ListRiskComplianceRefs(ctx context.Context, riskID int) ([]domain.RiskComplianceRefLink, error)
}

type riskComplianceRefRepo struct{ db *sql.DB }

// NewRiskComplianceRefRepository constructs a RiskComplianceRefRepository.
func NewRiskComplianceRefRepository(db *sql.DB) RiskComplianceRefRepository {
	return &riskComplianceRefRepo{db: db}
}

func (r *riskComplianceRefRepo) AddRiskComplianceRef(ctx context.Context, riskID int, req domain.AddRiskComplianceRefRequest) (*domain.RiskComplianceRefLink, error) {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO risk_compliance_reference (risk_id, reference_id) VALUES (?, ?)`,
		riskID, req.ReferenceID)
	if err != nil {
		if isDuplicateKey(err) {
			return r.getLink(ctx, riskID, req.ReferenceID)
		}
		if isFKViolation(err) {
			return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("compliance reference %d not found", req.ReferenceID)}
		}
		return nil, fmt.Errorf("risk_compliance_reference.Add: %w", err)
	}
	return r.getLink(ctx, riskID, req.ReferenceID)
}

func (r *riskComplianceRefRepo) getLink(ctx context.Context, riskID, referenceID int) (*domain.RiskComplianceRefLink, error) {
	var link domain.RiskComplianceRefLink
	var desc sql.NullString
	err := r.db.QueryRowContext(ctx,
		`SELECT rcr.risk_id, rcr.reference_id, ref.name, ref.description, rcr.created_at
		 FROM risk_compliance_reference rcr
		 JOIN risk_security_compliance_reference ref ON ref.id = rcr.reference_id
		 WHERE rcr.risk_id = ? AND rcr.reference_id = ?`,
		riskID, referenceID,
	).Scan(&link.RiskID, &link.ReferenceID, &link.Name, &desc, &link.CreatedOn)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("compliance reference %d not linked to risk %d", referenceID, riskID)}
	}
	if err != nil {
		return nil, fmt.Errorf("risk_compliance_reference.get: %w", err)
	}
	if desc.Valid {
		link.Description = &desc.String
	}
	return &link, nil
}

func (r *riskComplianceRefRepo) DeleteRiskComplianceRef(ctx context.Context, riskID, referenceID int) error {
	result, err := r.db.ExecContext(ctx,
		"DELETE FROM risk_compliance_reference WHERE risk_id = ? AND reference_id = ?",
		riskID, referenceID)
	if err != nil {
		return fmt.Errorf("risk_compliance_reference.Delete: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return &apierror.NotFoundError{Msg: fmt.Sprintf("compliance reference %d not linked to risk %d", referenceID, riskID)}
	}
	return nil
}

func (r *riskComplianceRefRepo) ListRiskComplianceRefs(ctx context.Context, riskID int) ([]domain.RiskComplianceRefLink, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT rcr.risk_id, rcr.reference_id, ref.name, ref.description, rcr.created_at
		 FROM risk_compliance_reference rcr
		 JOIN risk_security_compliance_reference ref ON ref.id = rcr.reference_id
		 WHERE rcr.risk_id = ?
		 ORDER BY ref.name ASC`,
		riskID)
	if err != nil {
		return nil, fmt.Errorf("risk_compliance_reference.List: %w", err)
	}
	defer rows.Close()

	var links []domain.RiskComplianceRefLink
	for rows.Next() {
		var link domain.RiskComplianceRefLink
		var desc sql.NullString
		if err := rows.Scan(&link.RiskID, &link.ReferenceID, &link.Name, &desc, &link.CreatedOn); err != nil {
			return nil, fmt.Errorf("risk_compliance_reference.List scan: %w", err)
		}
		if desc.Valid {
			link.Description = &desc.String
		}
		links = append(links, link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("risk_compliance_reference.List rows: %w", err)
	}
	return links, nil
}
