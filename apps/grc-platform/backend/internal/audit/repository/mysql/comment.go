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
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
)

type commentRepository struct{ db *sql.DB }

// NewCommentRepository creates a MySQL-backed repository.CommentRepository.
func NewCommentRepository(db *sql.DB) repository.CommentRepository {
	return &commentRepository{db: db}
}

func (r *commentRepository) Create(ctx context.Context, evidenceID int, content string, isInternal bool, parentCommentID *int, createdBy string) (*model.AuditComment, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO audit_comment
		  (evidence_id, parent_comment_id, content, is_internal, created_by)
		VALUES (?, ?, ?, ?, ?)`,
		evidenceID, intPtrVal(parentCommentID), content, isInternal, createdBy,
	)
	if err != nil {
		return nil, fmt.Errorf("comment.Create: %w", err)
	}
	id64, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("comment.Create lastInsertId: %w", err)
	}
	return &model.AuditComment{
		ID:              int(id64),
		EvidenceID:      evidenceID,
		ParentCommentID: parentCommentID,
		Content:         content,
		IsInternal:      isInternal,
		CreatedBy:       createdBy,
		CreatedAt:       time.Now(),
	}, nil
}

func (r *commentRepository) ListByEvidence(ctx context.Context, evidenceID int) ([]*model.AuditComment, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, evidence_id, parent_comment_id, content, is_internal, created_by, created_at
		FROM audit_comment
		WHERE evidence_id = ?
		ORDER BY created_at ASC`, evidenceID)
	if err != nil {
		return nil, fmt.Errorf("comment.ListByEvidence: %w", err)
	}
	defer rows.Close()

	var list []*model.AuditComment
	for rows.Next() {
		var c model.AuditComment
		var parentID sql.NullInt64
		if err := rows.Scan(&c.ID, &c.EvidenceID, &parentID, &c.Content, &c.IsInternal, &c.CreatedBy, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("comment.ListByEvidence scan: %w", err)
		}
		if parentID.Valid {
			v := int(parentID.Int64)
			c.ParentCommentID = &v
		}
		list = append(list, &c)
	}
	if list == nil {
		list = []*model.AuditComment{}
	}
	return list, rows.Err()
}
