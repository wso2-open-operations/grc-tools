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
	"time"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/cache"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

type cachedAuditFrameworkService struct {
	inner AuditFrameworkService
	byID  *cache.Cache[int, domain.AuditFramework]
}

// NewCachedAuditFrameworkService wraps inner with a 5-minute in-memory cache on
// GetAuditFrameworkByID. The cache entry is invalidated when UpdateAuditFramework
// is called for that ID.
func NewCachedAuditFrameworkService(inner AuditFrameworkService) AuditFrameworkService {
	return &cachedAuditFrameworkService{
		inner: inner,
		byID:  cache.New[int, domain.AuditFramework](5 * time.Minute),
	}
}

func (s *cachedAuditFrameworkService) SearchAuditFrameworks(ctx context.Context, req domain.SearchAuditFrameworksRequest) (domain.SearchAuditFrameworksResponse, error) {
	return s.inner.SearchAuditFrameworks(ctx, req)
}

func (s *cachedAuditFrameworkService) GetAuditFrameworkByID(ctx context.Context, id int) (domain.AuditFramework, error) {
	if v, ok := s.byID.Get(id); ok {
		return v, nil
	}
	f, err := s.inner.GetAuditFrameworkByID(ctx, id)
	if err != nil {
		return domain.AuditFramework{}, err
	}
	s.byID.Set(id, f)
	return f, nil
}

func (s *cachedAuditFrameworkService) CreateAuditFramework(ctx context.Context, req domain.CreateAuditFrameworkRequest) (domain.AuditFramework, error) {
	return s.inner.CreateAuditFramework(ctx, req)
}

func (s *cachedAuditFrameworkService) UpdateAuditFramework(ctx context.Context, id int, req domain.UpdateAuditFrameworkRequest) (domain.AuditFramework, error) {
	f, err := s.inner.UpdateAuditFramework(ctx, id, req)
	if err != nil {
		return domain.AuditFramework{}, err
	}
	s.byID.Delete(id)
	return f, nil
}
