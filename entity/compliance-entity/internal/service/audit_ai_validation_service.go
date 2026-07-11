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

type aiValidationService struct {
	repo repository.AIValidationRepository
}

// NewAIValidationService constructs an AIValidationService.
func NewAIValidationService(repo repository.AIValidationRepository) AIValidationService {
	return &aiValidationService{repo: repo}
}

var validAIResults = map[string]bool{"PASS": true, "FAIL": true, "UNCERTAIN": true}

func (s *aiValidationService) CreateValidation(ctx context.Context, evidenceID int, req domain.CreateAuditAIValidationLogRequest) (domain.AuditAIValidationLog, error) {
	if evidenceID <= 0 {
		return domain.AuditAIValidationLog{}, &apierror.ValidationError{Msg: "evidenceId must be a positive integer"}
	}
	if req.ControlID <= 0 {
		return domain.AuditAIValidationLog{}, &apierror.ValidationError{Msg: "controlId must be a positive integer"}
	}
	req.Result = strings.ToUpper(req.Result)
	if !validAIResults[req.Result] {
		return domain.AuditAIValidationLog{}, &apierror.ValidationError{Msg: "invalid result: " + req.Result + " (must be PASS, FAIL, or UNCERTAIN)"}
	}
	if req.CreatedBy == "" {
		return domain.AuditAIValidationLog{}, &apierror.ValidationError{Msg: "createdBy is required"}
	}
	if req.ConfidenceScore != nil && (*req.ConfidenceScore < 0 || *req.ConfidenceScore > 1) {
		return domain.AuditAIValidationLog{}, &apierror.ValidationError{Msg: "confidenceScore must be between 0 and 1"}
	}
	l, err := s.repo.CreateValidation(ctx, evidenceID, req)
	if err != nil {
		return domain.AuditAIValidationLog{}, err
	}
	return *l, nil
}

func (s *aiValidationService) ListValidationsByEvidence(ctx context.Context, evidenceID int) (domain.ListAuditAIValidationLogsResponse, error) {
	if evidenceID <= 0 {
		return domain.ListAuditAIValidationLogsResponse{}, &apierror.ValidationError{Msg: "evidenceId must be a positive integer"}
	}
	logs, err := s.repo.ListValidationsByEvidence(ctx, evidenceID)
	if err != nil {
		return domain.ListAuditAIValidationLogsResponse{}, err
	}
	if logs == nil {
		logs = []domain.AuditAIValidationLog{}
	}
	return domain.ListAuditAIValidationLogsResponse{Validations: logs}, nil
}
