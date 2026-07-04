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

package service

import (
	"context"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/repository"
)

type commentService struct{ repo repository.CommentRepository }

// NewCommentService constructs a CommentService.
func NewCommentService(repo repository.CommentRepository) CommentService {
	return &commentService{repo: repo}
}

func (s *commentService) CreateComment(ctx context.Context, evidenceID int, req domain.CreateAuditCommentRequest) (domain.AuditComment, error) {
	if evidenceID <= 0 {
		return domain.AuditComment{}, &apierror.ValidationError{Msg: "evidenceId must be a positive integer"}
	}
	if req.Content == "" {
		return domain.AuditComment{}, &apierror.ValidationError{Msg: "content is required"}
	}
	if req.CreatedBy == "" {
		return domain.AuditComment{}, &apierror.ValidationError{Msg: "createdBy is required"}
	}
	c, err := s.repo.CreateComment(ctx, evidenceID, req)
	if err != nil {
		return domain.AuditComment{}, err
	}
	return *c, nil
}

func (s *commentService) ListCommentsByEvidence(ctx context.Context, evidenceID int) (domain.ListAuditCommentsResponse, error) {
	if evidenceID <= 0 {
		return domain.ListAuditCommentsResponse{}, &apierror.ValidationError{Msg: "evidenceId must be a positive integer"}
	}
	comments, err := s.repo.ListCommentsByEvidence(ctx, evidenceID)
	if err != nil {
		return domain.ListAuditCommentsResponse{}, err
	}
	if comments == nil {
		comments = []domain.AuditComment{}
	}
	return domain.ListAuditCommentsResponse{Comments: comments}, nil
}

func (s *commentService) DeleteComment(ctx context.Context, commentID int) error {
	if commentID <= 0 {
		return &apierror.ValidationError{Msg: "commentId must be a positive integer"}
	}
	return s.repo.DeleteComment(ctx, commentID)
}
