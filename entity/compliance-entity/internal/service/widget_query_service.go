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

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/repository"
)

// WidgetQueryService answers a widget spec with an aggregate over the risk
// register or the Audit Hub.
type WidgetQueryService interface {
	Run(ctx context.Context, req domain.WidgetQueryRequest) (*domain.WidgetQueryResult, error)
	RunAudit(ctx context.Context, req domain.AuditWidgetQueryRequest) (*domain.WidgetQueryResult, error)
}

type widgetQueryService struct {
	repo repository.WidgetQueryRepository
}

// NewWidgetQueryService constructs a WidgetQueryService.
func NewWidgetQueryService(repo repository.WidgetQueryRepository) WidgetQueryService {
	return &widgetQueryService{repo: repo}
}

// Run compiles the spec, which checks it, and only then touches the database.
func (s *widgetQueryService) Run(ctx context.Context, req domain.WidgetQueryRequest) (*domain.WidgetQueryResult, error) {
	q, err := repository.CompileWidgetQuery(req)
	if err != nil {
		return nil, err
	}
	return s.repo.Run(ctx, q)
}

// RunAudit is Run for the Audit Hub.
func (s *widgetQueryService) RunAudit(ctx context.Context, req domain.AuditWidgetQueryRequest) (*domain.WidgetQueryResult, error) {
	q, err := repository.CompileAuditWidgetQuery(req)
	if err != nil {
		return nil, err
	}
	return s.repo.Run(ctx, q)
}
