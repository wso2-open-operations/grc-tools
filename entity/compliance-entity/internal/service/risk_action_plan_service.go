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

type riskActionPlanService struct {
	repo repository.RiskActionPlanRepository
}

// NewRiskActionPlanService constructs a RiskActionPlanService.
func NewRiskActionPlanService(repo repository.RiskActionPlanRepository) RiskActionPlanService {
	return &riskActionPlanService{repo: repo}
}

// validPlanTypes / validActionPlanStatuses mirror the risk_action_plan.plan_type
// and risk_action_plan.status ENUMs in risk_schema.sql.
var validPlanTypes = map[string]bool{"STANDARD": true, "MANAGEMENT": true}
var validActionPlanStatuses = map[string]bool{"PENDING": true, "IN_PROGRESS": true, "COMPLETED": true}

func (s *riskActionPlanService) CreateRiskActionPlan(ctx context.Context, riskID int, req domain.CreateRiskActionPlanRequest) (domain.RiskActionPlan, error) {
	if riskID <= 0 {
		return domain.RiskActionPlan{}, &apierror.ValidationError{Msg: "riskId must be a positive integer"}
	}
	if !validPlanTypes[strings.ToUpper(req.PlanType)] {
		return domain.RiskActionPlan{}, &apierror.ValidationError{Msg: "planType must be STANDARD or MANAGEMENT"}
	}
	if req.CreatedBy == "" {
		return domain.RiskActionPlan{}, &apierror.ValidationError{Msg: "createdBy is required"}
	}
	p, err := s.repo.CreateRiskActionPlan(ctx, riskID, req)
	if err != nil {
		return domain.RiskActionPlan{}, err
	}
	return *p, nil
}

func (s *riskActionPlanService) GetRiskActionPlanByID(ctx context.Context, planID int) (domain.RiskActionPlan, error) {
	if planID <= 0 {
		return domain.RiskActionPlan{}, &apierror.ValidationError{Msg: "planId must be a positive integer"}
	}
	p, err := s.repo.GetRiskActionPlanByID(ctx, planID)
	if err != nil {
		return domain.RiskActionPlan{}, err
	}
	return *p, nil
}

func (s *riskActionPlanService) UpdateRiskActionPlan(ctx context.Context, planID int, req domain.UpdateRiskActionPlanRequest) (domain.RiskActionPlan, error) {
	if planID <= 0 {
		return domain.RiskActionPlan{}, &apierror.ValidationError{Msg: "planId must be a positive integer"}
	}
	if req.UpdatedBy == "" {
		return domain.RiskActionPlan{}, &apierror.ValidationError{Msg: "updatedBy is required"}
	}
	if req.Status != nil && !validActionPlanStatuses[strings.ToUpper(*req.Status)] {
		return domain.RiskActionPlan{}, &apierror.ValidationError{Msg: "status must be PENDING, IN_PROGRESS, or COMPLETED"}
	}
	p, err := s.repo.UpdateRiskActionPlan(ctx, planID, req)
	if err != nil {
		return domain.RiskActionPlan{}, err
	}
	return *p, nil
}

func (s *riskActionPlanService) ListRiskActionPlans(ctx context.Context, riskID int) (domain.ListRiskActionPlansResponse, error) {
	if riskID <= 0 {
		return domain.ListRiskActionPlansResponse{}, &apierror.ValidationError{Msg: "riskId must be a positive integer"}
	}
	resp, err := s.repo.ListRiskActionPlans(ctx, riskID)
	if err != nil {
		return domain.ListRiskActionPlansResponse{}, err
	}
	if resp.Plans == nil {
		resp.Plans = []domain.RiskActionPlan{}
	}
	return *resp, nil
}
