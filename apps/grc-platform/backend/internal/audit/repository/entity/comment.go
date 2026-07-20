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

package entity

import (
	"context"
	"fmt"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
)

type commentRepo struct{ c *entityclient.Client }

// NewCommentRepository returns an entity-backed CommentRepository.
func NewCommentRepository(c *entityclient.Client) repository.CommentRepository {
	return &commentRepo{c: c}
}

func (r *commentRepo) Create(ctx context.Context, evidenceID int, content string, isInternal bool, parentCommentID *int, createdBy string) (*model.AuditComment, error) {
	body := map[string]any{
		"content":         content,
		"isInternal":      isInternal,
		"parentCommentId": parentCommentID,
		"createdBy":       createdBy,
	}
	var cm model.AuditComment
	if err := r.c.Post(ctx, fmt.Sprintf("/evidence/%d/comments", evidenceID), body, &cm); err != nil {
		return nil, err
	}
	return &cm, nil
}

func (r *commentRepo) ListByEvidence(ctx context.Context, evidenceID int) ([]*model.AuditComment, error) {
	var resp struct {
		Comments []*model.AuditComment `json:"comments"`
	}
	if err := r.c.Get(ctx, fmt.Sprintf("/evidence/%d/comments", evidenceID), &resp); err != nil {
		return nil, err
	}
	return resp.Comments, nil
}
