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
	"strings"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/repository"
)

type trailService struct{ repo repository.TrailRepository }

// NewTrailService constructs a TrailService.
func NewTrailService(repo repository.TrailRepository) TrailService {
	return &trailService{repo: repo}
}

var validTrailActions = map[string]bool{
	"CREATED": true, "UPLOADED": true, "RESUBMITTED": true,
	"APPROVED": true, "REJECTED": true, "COMMENTED": true,
	"ESCALATED": true, "AI_VALIDATED": true, "EXPORTED": true,
}

func (s *trailService) CreateTrail(ctx context.Context, auditID int, req domain.CreateAuditTrailRequest) (domain.AuditTrail, error) {
	if auditID <= 0 {
		return domain.AuditTrail{}, &apierror.ValidationError{Msg: "auditId must be a positive integer"}
	}
	if !validTrailActions[strings.ToUpper(req.Action)] {
		return domain.AuditTrail{}, &apierror.ValidationError{Msg: "invalid action: " + req.Action}
	}
	e, err := s.repo.CreateTrail(ctx, auditID, req)
	if err != nil {
		return domain.AuditTrail{}, err
	}
	return *e, nil
}

func (s *trailService) ListTrail(ctx context.Context, auditID int, limit, offset int) (domain.ListAuditTrailResponse, error) {
	if auditID <= 0 {
		return domain.ListAuditTrailResponse{}, &apierror.ValidationError{Msg: "auditId must be a positive integer"}
	}
	p := domain.Pagination{Limit: limit, Offset: offset}
	normalizePagination(&p)
	entries, total, err := s.repo.ListTrail(ctx, auditID, p.Limit, p.Offset)
	if err != nil {
		return domain.ListAuditTrailResponse{}, err
	}
	if entries == nil {
		entries = []domain.AuditTrail{}
	}
	return domain.ListAuditTrailResponse{Trail: entries, Total: total, Limit: p.Limit, Offset: p.Offset}, nil
}
