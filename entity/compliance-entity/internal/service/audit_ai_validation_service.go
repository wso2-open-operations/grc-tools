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

// PASS/FAIL/UNCERTAIN are verdicts; PENDING/ERROR are lifecycle rows written
// by the in-process validation trigger (append-only — a terminal row is
// appended later rather than updating the PENDING one). SKIPPED is written
// when the submitter opted out via the checkbox — no LLM call is made.
var validAIResults = map[string]bool{"PASS": true, "FAIL": true, "UNCERTAIN": true, "PENDING": true, "ERROR": true, "SKIPPED": true}

func (s *aiValidationService) validateRequest(req *domain.CreateAuditAIValidationLogRequest) error {
	if req.ControlID <= 0 {
		return &apierror.ValidationError{Msg: "controlId must be a positive integer"}
	}
	req.Result = strings.ToUpper(req.Result)
	if !validAIResults[req.Result] {
		return &apierror.ValidationError{Msg: "invalid result: " + req.Result + " (must be PASS, FAIL, UNCERTAIN, PENDING, ERROR, or SKIPPED)"}
	}
	if req.CreatedBy == "" {
		return &apierror.ValidationError{Msg: "createdBy is required"}
	}
	return nil
}

func (s *aiValidationService) CreateValidation(ctx context.Context, evidenceID int, req domain.CreateAuditAIValidationLogRequest) (domain.AuditAIValidationLog, error) {
	if evidenceID <= 0 {
		return domain.AuditAIValidationLog{}, &apierror.ValidationError{Msg: "evidenceId must be a positive integer"}
	}
	if err := s.validateRequest(&req); err != nil {
		return domain.AuditAIValidationLog{}, err
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

func (s *aiValidationService) CreateValidationForPopulation(ctx context.Context, populationID int, req domain.CreateAuditAIValidationLogRequest) (domain.AuditAIValidationLog, error) {
	if populationID <= 0 {
		return domain.AuditAIValidationLog{}, &apierror.ValidationError{Msg: "populationId must be a positive integer"}
	}
	if err := s.validateRequest(&req); err != nil {
		return domain.AuditAIValidationLog{}, err
	}
	l, err := s.repo.CreateValidationForPopulation(ctx, populationID, req)
	if err != nil {
		return domain.AuditAIValidationLog{}, err
	}
	return *l, nil
}

func (s *aiValidationService) ListValidationsByPopulation(ctx context.Context, populationID int) (domain.ListAuditAIValidationLogsResponse, error) {
	if populationID <= 0 {
		return domain.ListAuditAIValidationLogsResponse{}, &apierror.ValidationError{Msg: "populationId must be a positive integer"}
	}
	logs, err := s.repo.ListValidationsByPopulation(ctx, populationID)
	if err != nil {
		return domain.ListAuditAIValidationLogsResponse{}, err
	}
	if logs == nil {
		logs = []domain.AuditAIValidationLog{}
	}
	return domain.ListAuditAIValidationLogsResponse{Validations: logs}, nil
}
