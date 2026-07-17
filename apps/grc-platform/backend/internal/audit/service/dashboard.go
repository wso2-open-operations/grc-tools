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

package service

import (
	"context"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
)

// DashboardService fetches role-scoped dashboard data.
type DashboardService interface {
	Get(ctx context.Context, f model.DashboardFilter) (*model.DashboardData, error)
	GetWorkQueuePage(ctx context.Context, f model.DashboardFilter, tab model.WorkQueueTab, page, limit int) (*model.WorkQueuePage, error)
}

type dashboardService struct {
	repo repository.DashboardRepository
}

// NewDashboardService creates a DashboardService backed by repo.
func NewDashboardService(repo repository.DashboardRepository) DashboardService {
	return &dashboardService{repo: repo}
}

func (s *dashboardService) Get(ctx context.Context, f model.DashboardFilter) (*model.DashboardData, error) {
	return s.repo.Get(ctx, f)
}

func (s *dashboardService) GetWorkQueuePage(ctx context.Context, f model.DashboardFilter, tab model.WorkQueueTab, page, limit int) (*model.WorkQueuePage, error) {
	return s.repo.GetWorkQueuePage(ctx, f, tab, page, limit)
}
