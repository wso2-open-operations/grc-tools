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

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
)

type evidenceRepository struct{ db *sql.DB }

// NewEvidenceRepository creates a MySQL-backed repository.EvidenceRepository.
func NewEvidenceRepository(db *sql.DB) repository.EvidenceRepository {
	return &evidenceRepository{db: db}
}

func (r *evidenceRepository) Create(ctx context.Context, controlID int, folderPath, createdBy string) (int, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO audit_evidence (control_id, status, folder_path, created_by, created_at, updated_at)
		 VALUES (?, 'SUBMITTED', ?, ?, NOW(), NOW())`,
		controlID, folderPath, createdBy,
	)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	return int(id), err
}

func (r *evidenceRepository) AddFile(ctx context.Context, evidenceID int, fileName, filePath string, fileType *string, fileSize *int64, createdBy string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO audit_evidence_file
		 (evidence_id, file_name, file_path, file_type, file_size, created_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, NOW(), NOW())`,
		evidenceID, fileName, filePath, fileType, fileSize, createdBy,
	)
	return err
}

func (r *evidenceRepository) ListByControl(ctx context.Context, controlID int) ([]*model.AuditEvidence, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, control_id, status, COALESCE(folder_path, ''), created_by, created_at
		 FROM audit_evidence
		 WHERE control_id = ?
		 ORDER BY created_at DESC`,
		controlID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var evidence []*model.AuditEvidence
	for rows.Next() {
		e := &model.AuditEvidence{}
		var createdBy sql.NullString
		if err := rows.Scan(&e.ID, &e.ControlID, &e.Status, &e.FolderPath, &createdBy, &e.CreatedAt); err != nil {
			return nil, err
		}
		if createdBy.Valid {
			e.CreatedBy = createdBy.String
		}
		e.Files = []*model.AuditEvidenceFile{}
		evidence = append(evidence, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Load files for each evidence record.
	for _, e := range evidence {
		files, err := r.listFiles(ctx, e.ID)
		if err != nil {
			return nil, err
		}
		e.Files = files
	}
	return evidence, nil
}

func (r *evidenceRepository) listFiles(ctx context.Context, evidenceID int) ([]*model.AuditEvidenceFile, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, evidence_id, file_name, file_path, file_type, file_size, created_by, created_at
		 FROM audit_evidence_file
		 WHERE evidence_id = ?
		 ORDER BY created_at ASC`,
		evidenceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []*model.AuditEvidenceFile
	for rows.Next() {
		f := &model.AuditEvidenceFile{}
		var fileType sql.NullString
		var fileSize sql.NullInt64
		var createdBy sql.NullString
		if err := rows.Scan(&f.ID, &f.EvidenceID, &f.FileName, &f.FilePath, &fileType, &fileSize, &createdBy, &f.CreatedAt); err != nil {
			return nil, err
		}
		f.FileType = nullStringPtr(fileType)
		if fileSize.Valid {
			v := fileSize.Int64
			f.FileSize = &v
		}
		if createdBy.Valid {
			f.CreatedBy = createdBy.String
		}
		files = append(files, f)
	}
	return files, rows.Err()
}
