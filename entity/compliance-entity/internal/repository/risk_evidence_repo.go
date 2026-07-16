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

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

// RiskEvidenceRepository defines persistence for risk_evidence.
type RiskEvidenceRepository interface {
	CreateRiskEvidence(ctx context.Context, riskID int, req domain.CreateRiskEvidenceRequest) (*domain.RiskEvidenceFile, error)
	ListRiskEvidence(ctx context.Context, riskID int) (*domain.ListRiskEvidenceResponse, error)
	DeleteRiskEvidence(ctx context.Context, fileID int) error
}

type riskEvidenceRepo struct{ db *sql.DB }

// NewRiskEvidenceRepository constructs a RiskEvidenceRepository.
func NewRiskEvidenceRepository(db *sql.DB) RiskEvidenceRepository {
	return &riskEvidenceRepo{db: db}
}

func (r *riskEvidenceRepo) CreateRiskEvidence(ctx context.Context, riskID int, req domain.CreateRiskEvidenceRequest) (*domain.RiskEvidenceFile, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO risk_evidence (risk_id, file_name, file_path, note, evidence_type, created_by)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		riskID, req.FileName, req.FilePath,
		nullableString(req.Note),
		req.EvidenceType, req.CreatedBy)
	if err != nil {
		if isFKViolation(err) {
			return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("risk %d not found", riskID)}
		}
		return nil, fmt.Errorf("risk_evidence.Create: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.getByID(ctx, int(id))
}

func (r *riskEvidenceRepo) getByID(ctx context.Context, fileID int) (*domain.RiskEvidenceFile, error) {
	var f domain.RiskEvidenceFile
	var note sql.NullString
	err := r.db.QueryRowContext(ctx,
		"SELECT id, risk_id, file_name, file_path, note, evidence_type, created_at FROM risk_evidence WHERE id = ?",
		fileID).Scan(&f.ID, &f.RiskID, &f.FileName, &f.FilePath, &note, &f.EvidenceType, &f.CreatedOn)
	if err != nil {
		return nil, fmt.Errorf("risk_evidence.GetByID(%d): %w", fileID, err)
	}
	if note.Valid {
		f.Note = &note.String
	}
	return &f, nil
}

func (r *riskEvidenceRepo) ListRiskEvidence(ctx context.Context, riskID int) (*domain.ListRiskEvidenceResponse, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, risk_id, file_name, file_path, note, evidence_type, created_at FROM risk_evidence WHERE risk_id = ? ORDER BY created_at DESC",
		riskID)
	if err != nil {
		return nil, fmt.Errorf("risk_evidence.List: %w", err)
	}
	defer rows.Close()

	var evidence []domain.RiskEvidenceFile
	for rows.Next() {
		var f domain.RiskEvidenceFile
		var note sql.NullString
		if err := rows.Scan(&f.ID, &f.RiskID, &f.FileName, &f.FilePath, &note, &f.EvidenceType, &f.CreatedOn); err != nil {
			return nil, fmt.Errorf("risk_evidence.List scan: %w", err)
		}
		if note.Valid {
			f.Note = &note.String
		}
		evidence = append(evidence, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("risk_evidence.List rows: %w", err)
	}
	return &domain.ListRiskEvidenceResponse{Evidence: evidence}, nil
}

func (r *riskEvidenceRepo) DeleteRiskEvidence(ctx context.Context, fileID int) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM risk_evidence WHERE id = ?", fileID)
	if err != nil {
		return fmt.Errorf("risk_evidence.Delete(%d): %w", fileID, err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return &apierror.NotFoundError{Msg: fmt.Sprintf("risk evidence file %d not found", fileID)}
	}
	return nil
}
