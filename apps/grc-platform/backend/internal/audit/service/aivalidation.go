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

// AIValidationService reads advisory AI validation results for an evidence
// submission. It is a thin proxy over the Compliance Entity — results are
// hints; this service never mutates evidence or control status.
type AIValidationService interface {
	// ListByEvidence returns the validation rows for an evidence submission,
	// latest first (the UI reads the first element as the current state).
	ListByEvidence(ctx context.Context, evidenceID int) ([]*model.AIValidationLog, error)
}

type aiValidationService struct {
	repo repository.AIValidationLogRepository
}

// NewAIValidationService constructs an AIValidationService.
func NewAIValidationService(repo repository.AIValidationLogRepository) AIValidationService {
	return &aiValidationService{repo: repo}
}

func (s *aiValidationService) ListByEvidence(ctx context.Context, evidenceID int) ([]*model.AIValidationLog, error) {
	return s.repo.ListByEvidence(ctx, evidenceID)
}
