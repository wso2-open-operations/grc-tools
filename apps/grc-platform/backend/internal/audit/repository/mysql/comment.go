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

type commentRepository struct{ db *sql.DB }

// NewCommentRepository creates a MySQL-backed repository.CommentRepository.
func NewCommentRepository(db *sql.DB) repository.CommentRepository {
	return &commentRepository{db: db}
}

func (r *commentRepository) Create(ctx context.Context, evidenceID int, content string, isInternal bool, parentCommentID *int, createdBy string) (*model.AuditComment, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO audit_comment
		 (evidence_id, parent_comment_id, content, is_internal, created_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, NOW(), NOW())`,
		evidenceID, parentCommentID, content, isInternal, createdBy,
	)
	if err != nil {
		return nil, fmt.Errorf("comment.Create: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("comment.Create insert id: %w", err)
	}
	return r.getByID(ctx, int(id))
}

func (r *commentRepository) getByID(ctx context.Context, id int) (*model.AuditComment, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, evidence_id, parent_comment_id, content, is_internal, created_by, created_at
		 FROM audit_comment WHERE id = ?`, id)
	return scanComment(row)
}

func (r *commentRepository) ListByEvidence(ctx context.Context, evidenceID int) ([]*model.AuditComment, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, evidence_id, parent_comment_id, content, is_internal, created_by, created_at
		 FROM audit_comment WHERE evidence_id = ? ORDER BY created_at ASC`,
		evidenceID,
	)
	if err != nil {
		return nil, fmt.Errorf("comment.ListByEvidence: %w", err)
	}
	defer rows.Close()

	comments := []*model.AuditComment{}
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			return nil, fmt.Errorf("comment.ListByEvidence scan: %w", err)
		}
		comments = append(comments, c)
	}
	return comments, rows.Err()
}

func scanComment(s scanner) (*model.AuditComment, error) {
	c := &model.AuditComment{}
	var parentID sql.NullInt64
	var createdBy sql.NullString
	if err := s.Scan(&c.ID, &c.EvidenceID, &parentID, &c.Content, &c.IsInternal, &createdBy, &c.CreatedAt); err != nil {
		return nil, err
	}
	c.ParentCommentID = nullIntPtr(parentID)
	if createdBy.Valid {
		c.CreatedBy = createdBy.String
	}
	return c, nil
}
