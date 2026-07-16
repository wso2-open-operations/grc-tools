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

const riskScoreCacheKey = "all"

type cachedRiskScoreService struct {
	inner RiskScoreService
	cache *cache.Cache[string, domain.ListRiskScoresResponse]
}

// NewCachedRiskScoreService wraps inner with a 30-minute in-memory cache.
// Risk scores are reference data that never changes in normal operation.
func NewCachedRiskScoreService(inner RiskScoreService) RiskScoreService {
	return &cachedRiskScoreService{
		inner: inner,
		cache: cache.New[string, domain.ListRiskScoresResponse](30 * time.Minute),
	}
}

func (s *cachedRiskScoreService) ListRiskScores(ctx context.Context) (domain.ListRiskScoresResponse, error) {
	if v, ok := s.cache.Get(riskScoreCacheKey); ok {
		return v, nil
	}
	resp, err := s.inner.ListRiskScores(ctx)
	if err != nil {
		return domain.ListRiskScoresResponse{}, err
	}
	s.cache.Set(riskScoreCacheKey, resp)
	return resp, nil
}
