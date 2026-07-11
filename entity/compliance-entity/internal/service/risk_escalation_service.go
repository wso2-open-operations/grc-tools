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

type riskEscalationService struct {
	repo repository.RiskEscalationRepository
}

// NewRiskEscalationService constructs a RiskEscalationService.
func NewRiskEscalationService(repo repository.RiskEscalationRepository) RiskEscalationService {
	return &riskEscalationService{repo: repo}
}

var validEscalationStatuses = map[string]bool{"OPEN": true, "RESOLVED": true}

func (s *riskEscalationService) CreateRiskEscalation(ctx context.Context, riskID int, req domain.CreateRiskEscalationRequest) (domain.RiskEscalation, error) {
	if riskID <= 0 {
		return domain.RiskEscalation{}, &apierror.ValidationError{Msg: "riskId must be a positive integer"}
	}
	if req.EscalatedTo <= 0 {
		return domain.RiskEscalation{}, &apierror.ValidationError{Msg: "escalatedTo must be a positive integer"}
	}
	if req.NewTreatmentStrategy != nil {
		up := strings.ToUpper(*req.NewTreatmentStrategy)
		if !validTreatmentStrategies[up] {
			return domain.RiskEscalation{}, &apierror.ValidationError{Msg: "newTreatmentStrategy must be MITIGATE, ACCEPT, TRANSFER, or VOID"}
		}
		req.NewTreatmentStrategy = &up
	}
	if req.CreatedBy == "" {
		return domain.RiskEscalation{}, &apierror.ValidationError{Msg: "createdBy is required"}
	}
	e, err := s.repo.CreateRiskEscalation(ctx, riskID, req)
	if err != nil {
		return domain.RiskEscalation{}, err
	}
	return *e, nil
}

func (s *riskEscalationService) GetRiskEscalationByID(ctx context.Context, riskID, escalationID int) (domain.RiskEscalation, error) {
	if riskID <= 0 {
		return domain.RiskEscalation{}, &apierror.ValidationError{Msg: "riskId must be a positive integer"}
	}
	if escalationID <= 0 {
		return domain.RiskEscalation{}, &apierror.ValidationError{Msg: "escalationId must be a positive integer"}
	}
	e, err := s.repo.GetRiskEscalationByID(ctx, riskID, escalationID)
	if err != nil {
		return domain.RiskEscalation{}, err
	}
	return *e, nil
}

func (s *riskEscalationService) UpdateRiskEscalation(ctx context.Context, riskID, escalationID int, req domain.UpdateRiskEscalationRequest) (domain.RiskEscalation, error) {
	if riskID <= 0 {
		return domain.RiskEscalation{}, &apierror.ValidationError{Msg: "riskId must be a positive integer"}
	}
	if escalationID <= 0 {
		return domain.RiskEscalation{}, &apierror.ValidationError{Msg: "escalationId must be a positive integer"}
	}
	if req.UpdatedBy == "" {
		return domain.RiskEscalation{}, &apierror.ValidationError{Msg: "updatedBy is required"}
	}
	if req.Status != nil {
		up := strings.ToUpper(*req.Status)
		if !validEscalationStatuses[up] {
			return domain.RiskEscalation{}, &apierror.ValidationError{Msg: "invalid status: " + *req.Status}
		}
		req.Status = &up
	}
	if req.NewTreatmentStrategy != nil {
		up := strings.ToUpper(*req.NewTreatmentStrategy)
		if !validTreatmentStrategies[up] {
			return domain.RiskEscalation{}, &apierror.ValidationError{Msg: "newTreatmentStrategy must be MITIGATE, ACCEPT, TRANSFER, or VOID"}
		}
		req.NewTreatmentStrategy = &up
	}
	e, err := s.repo.UpdateRiskEscalation(ctx, riskID, escalationID, req)
	if err != nil {
		return domain.RiskEscalation{}, err
	}
	return *e, nil
}

func (s *riskEscalationService) ListRiskEscalations(ctx context.Context, riskID int) (domain.ListRiskEscalationsResponse, error) {
	if riskID <= 0 {
		return domain.ListRiskEscalationsResponse{}, &apierror.ValidationError{Msg: "riskId must be a positive integer"}
	}
	escalations, err := s.repo.ListRiskEscalations(ctx, riskID)
	if err != nil {
		return domain.ListRiskEscalationsResponse{}, err
	}
	if escalations == nil {
		escalations = []domain.RiskEscalation{}
	}
	return domain.ListRiskEscalationsResponse{Escalations: escalations}, nil
}
