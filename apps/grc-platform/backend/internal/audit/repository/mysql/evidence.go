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

type evidenceRepository struct{ db *sql.DB }

// NewEvidenceRepository creates a MySQL-backed repository.EvidenceRepository.
func NewEvidenceRepository(db *sql.DB) repository.EvidenceRepository {
	return &evidenceRepository{db: db}
}

func (r *evidenceRepository) Create(ctx context.Context, auditID, controlID int, folderPath, createdBy string) (int, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO audit_evidence (control_id, folder_path, created_by, updated_by)
		VALUES (?, ?, ?, ?)`,
		controlID, folderPath, createdBy, createdBy,
	)
	if err != nil {
		return 0, fmt.Errorf("evidence.Create: %w", err)
	}
	id64, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("evidence.Create lastInsertId: %w", err)
	}
	return int(id64), nil
}

func (r *evidenceRepository) AddFile(ctx context.Context, evidenceID int, fileName, filePath string, fileType *string, fileSize *int64, createdBy string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO audit_evidence_file
		  (evidence_id, file_name, file_path, file_type, file_size, created_by, updated_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		evidenceID, fileName, filePath, fileType, fileSize, createdBy, createdBy,
	)
	if err != nil {
		return fmt.Errorf("evidence.AddFile: %w", err)
	}
	return nil
}

func (r *evidenceRepository) ListByControl(ctx context.Context, auditID, controlID int) ([]*model.AuditEvidence, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT e.id, e.control_id, e.status, COALESCE(e.folder_path,''), e.created_by, e.created_at
		FROM audit_evidence e
		JOIN audit_control c ON c.id = e.control_id
		WHERE e.control_id = ? AND c.audit_id = ?
		ORDER BY e.created_at DESC`, controlID, auditID)
	if err != nil {
		return nil, fmt.Errorf("evidence.ListByControl: %w", err)
	}
	defer rows.Close()

	var list []*model.AuditEvidence
	for rows.Next() {
		var ev model.AuditEvidence
		if err := rows.Scan(&ev.ID, &ev.ControlID, &ev.Status, &ev.FolderPath, &ev.CreatedBy, &ev.CreatedAt); err != nil {
			return nil, fmt.Errorf("evidence.ListByControl scan: %w", err)
		}
		files, err := r.listFilesByEvidence(ctx, ev.ID)
		if err != nil {
			return nil, err
		}
		ev.Files = files
		list = append(list, &ev)
	}
	if list == nil {
		list = []*model.AuditEvidence{}
	}
	return list, rows.Err()
}

func (r *evidenceRepository) listFilesByEvidence(ctx context.Context, evidenceID int) ([]*model.AuditEvidenceFile, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, evidence_id, file_name, file_path, file_type, file_size, created_by, created_at
		FROM audit_evidence_file
		WHERE evidence_id = ?
		ORDER BY created_at ASC`, evidenceID)
	if err != nil {
		return nil, fmt.Errorf("evidence.listFiles(%d): %w", evidenceID, err)
	}
	defer rows.Close()

	var files []*model.AuditEvidenceFile
	for rows.Next() {
		var f model.AuditEvidenceFile
		var fileType sql.NullString
		var fileSize sql.NullInt64
		if err := rows.Scan(&f.ID, &f.EvidenceID, &f.FileName, &f.FilePath, &fileType, &fileSize, &f.CreatedBy, &f.CreatedAt); err != nil {
			return nil, fmt.Errorf("evidence.listFiles scan: %w", err)
		}
		f.FileType = nullStringPtr(fileType)
		f.FileSize = nullInt64Ptr(fileSize)
		files = append(files, &f)
	}
	if files == nil {
		files = []*model.AuditEvidenceFile{}
	}
	return files, rows.Err()
}

func (r *evidenceRepository) GetFileByID(ctx context.Context, fileID int) (*model.AuditEvidenceFile, error) {
	var f model.AuditEvidenceFile
	var fileType sql.NullString
	var fileSize sql.NullInt64
	err := r.db.QueryRowContext(ctx, `
		SELECT id, evidence_id, file_name, file_path, file_type, file_size, created_by, created_at
		FROM audit_evidence_file WHERE id = ?`, fileID,
	).Scan(&f.ID, &f.EvidenceID, &f.FileName, &f.FilePath, &fileType, &fileSize, &f.CreatedBy, &f.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("evidence.GetFileByID(%d): %w", fileID, err)
	}
	f.FileType = nullStringPtr(fileType)
	f.FileSize = nullInt64Ptr(fileSize)
	return &f, nil
}
