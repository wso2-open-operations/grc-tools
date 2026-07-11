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

type riskService struct{ repo repository.RiskRepository }

// NewRiskService constructs a RiskService.
func NewRiskService(repo repository.RiskRepository) RiskService {
	return &riskService{repo: repo}
}

// validRiskStatuses mirrors the risk.workflow_status ENUM in risk_schema.sql
// exactly (12 statuses). Keep in sync with the schema.
var validRiskStatuses = map[string]bool{
	"DRAFT":                             true,
	"PENDING_RISK_OWNER_APPROVAL":       true,
	"PENDING_MANAGEMENT_APPROVAL":       true,
	"PENDING_COMPLIANCE_REVIEW":         true,
	"IN_REMEDIATION":                    true,
	"PENDING_OWNER_COMPLETION_APPROVAL": true,
	"PENDING_COMPLIANCE_CLOSURE":        true,
	"PENDING_AMENDMENT":                 true,
	"PENDING_REVISION":                  true,
	"ESCALATED":                         true,
	"CLOSED":                            true,
	"CANCELLED":                         true,
}

var validRiskQuarters = map[string]bool{"Q1": true, "Q2": true, "Q3": true, "Q4": true}

// allowedRiskTransitions defines the legal next statuses for each risk workflow_status.
// Happy path: DRAFT → PENDING_RISK_OWNER_APPROVAL → PENDING_MANAGEMENT_APPROVAL →
// PENDING_COMPLIANCE_REVIEW → IN_REMEDIATION → PENDING_OWNER_COMPLETION_APPROVAL →
// PENDING_COMPLIANCE_CLOSURE → CLOSED. Plus amendment, revision, escalation, and
// cancellation paths. CLOSED and CANCELLED are terminal.
var allowedRiskTransitions = map[string][]string{
	"DRAFT":                             {"PENDING_RISK_OWNER_APPROVAL", "CANCELLED"},
	"PENDING_RISK_OWNER_APPROVAL":       {"PENDING_MANAGEMENT_APPROVAL", "PENDING_AMENDMENT", "CANCELLED"},
	"PENDING_MANAGEMENT_APPROVAL":       {"PENDING_COMPLIANCE_REVIEW", "PENDING_AMENDMENT", "ESCALATED", "CANCELLED"},
	"PENDING_COMPLIANCE_REVIEW":         {"IN_REMEDIATION", "PENDING_REVISION", "ESCALATED", "CANCELLED"},
	"IN_REMEDIATION":                    {"PENDING_OWNER_COMPLETION_APPROVAL", "ESCALATED"},
	"PENDING_OWNER_COMPLETION_APPROVAL": {"PENDING_COMPLIANCE_CLOSURE", "IN_REMEDIATION"},
	"PENDING_COMPLIANCE_CLOSURE":        {"CLOSED", "IN_REMEDIATION"},
	"PENDING_AMENDMENT":                 {"PENDING_RISK_OWNER_APPROVAL", "CANCELLED"},
	"PENDING_REVISION":                  {"PENDING_COMPLIANCE_REVIEW", "CANCELLED"},
	"ESCALATED":                         {"PENDING_MANAGEMENT_APPROVAL", "CANCELLED"},
	"CLOSED":                            {},
	"CANCELLED":                         {},
}

// isValidRiskTransition reports whether moving from → to is a legal workflow step.
// A no-op (from == to) and an empty current status are always allowed.
func isValidRiskTransition(from, to string) bool {
	from = strings.ToUpper(from)
	to = strings.ToUpper(to)
	if from == to || from == "" {
		return true
	}
	for _, next := range allowedRiskTransitions[from] {
		if next == to {
			return true
		}
	}
	return false
}

// validTreatmentStrategies / validIdentifiedByTypes mirror the risk.treatment_strategy
// and risk.identified_by_type ENUMs in risk_schema.sql. Both columns are nullable,
// so these are only enforced when a value is provided.
var validTreatmentStrategies = map[string]bool{"MITIGATE": true, "ACCEPT": true, "TRANSFER": true, "VOID": true}
var validIdentifiedByTypes = map[string]bool{"EMPLOYEE": true, "EXTERNAL_PERSON": true, "TOOL": true}

func (s *riskService) SearchRisks(ctx context.Context, req domain.SearchRisksRequest) (domain.SearchRisksResponse, error) {
	for i, sk := range req.WorkflowStatusKeys {
		if !validRiskStatuses[strings.ToUpper(sk)] {
			return domain.SearchRisksResponse{}, &apierror.ValidationError{Msg: "invalid workflowStatusKey: " + sk}
		}
		req.WorkflowStatusKeys[i] = strings.ToUpper(sk)
	}
	for i, qk := range req.RiskQuarterKeys {
		if !validRiskQuarters[strings.ToUpper(qk)] {
			return domain.SearchRisksResponse{}, &apierror.ValidationError{Msg: "invalid riskQuarterKey: " + qk + " (must be Q1, Q2, Q3, or Q4)"}
		}
		req.RiskQuarterKeys[i] = strings.ToUpper(qk)
	}
	normalizePagination(&req.Pagination)
	risks, total, err := s.repo.SearchRisks(ctx, req)
	if err != nil {
		return domain.SearchRisksResponse{}, err
	}
	if risks == nil {
		risks = []domain.Risk{}
	}
	return domain.SearchRisksResponse{Risks: risks, Total: total, Limit: req.Pagination.Limit, Offset: req.Pagination.Offset}, nil
}

func (s *riskService) GetRiskByID(ctx context.Context, id int) (domain.Risk, error) {
	if id <= 0 {
		return domain.Risk{}, &apierror.ValidationError{Msg: "risk id must be a positive integer"}
	}
	r, err := s.repo.GetRiskByID(ctx, id)
	if err != nil {
		return domain.Risk{}, err
	}
	return *r, nil
}

func (s *riskService) CreateRisk(ctx context.Context, req domain.CreateRiskRequest) (domain.Risk, error) {
	if req.RiskTitle == "" {
		return domain.Risk{}, &apierror.ValidationError{Msg: "riskTitle is required"}
	}
	if req.SourceRegisterID <= 0 {
		return domain.Risk{}, &apierror.ValidationError{Msg: "sourceRegisterId is required"}
	}
	if req.AssignmentTeamID <= 0 {
		return domain.Risk{}, &apierror.ValidationError{Msg: "assignmentTeamId is required"}
	}
	if req.AssignerID <= 0 {
		return domain.Risk{}, &apierror.ValidationError{Msg: "assignerId is required"}
	}
	if req.OwnerID <= 0 {
		return domain.Risk{}, &apierror.ValidationError{Msg: "ownerId is required"}
	}
	if req.RiskYear <= 0 {
		return domain.Risk{}, &apierror.ValidationError{Msg: "riskYear is required"}
	}
	req.RiskQuarter = strings.ToUpper(req.RiskQuarter)
	if !validRiskQuarters[req.RiskQuarter] {
		return domain.Risk{}, &apierror.ValidationError{Msg: "riskQuarter must be Q1, Q2, Q3, or Q4"}
	}
	if req.TreatmentStrategy != nil && !validTreatmentStrategies[strings.ToUpper(*req.TreatmentStrategy)] {
		return domain.Risk{}, &apierror.ValidationError{Msg: "treatmentStrategy must be MITIGATE, ACCEPT, TRANSFER, or VOID"}
	}
	if req.TreatmentStrategy != nil {
		up := strings.ToUpper(*req.TreatmentStrategy)
		req.TreatmentStrategy = &up
	}
	if req.IdentifiedByType != nil && !validIdentifiedByTypes[strings.ToUpper(*req.IdentifiedByType)] {
		return domain.Risk{}, &apierror.ValidationError{Msg: "identifiedByType must be EMPLOYEE, EXTERNAL_PERSON, or TOOL"}
	}
	if req.IdentifiedByType != nil {
		up := strings.ToUpper(*req.IdentifiedByType)
		req.IdentifiedByType = &up
	}
	if req.CreatedBy == "" {
		return domain.Risk{}, &apierror.ValidationError{Msg: "createdBy is required"}
	}
	r, err := s.repo.CreateRisk(ctx, req)
	if err != nil {
		return domain.Risk{}, err
	}
	return *r, nil
}

func (s *riskService) UpdateRisk(ctx context.Context, id int, req domain.UpdateRiskRequest) (domain.Risk, error) {
	if id <= 0 {
		return domain.Risk{}, &apierror.ValidationError{Msg: "risk id must be a positive integer"}
	}
	if req.UpdatedBy == "" {
		return domain.Risk{}, &apierror.ValidationError{Msg: "updatedBy is required"}
	}
	if req.WorkflowStatus != nil && !validRiskStatuses[strings.ToUpper(*req.WorkflowStatus)] {
		return domain.Risk{}, &apierror.ValidationError{Msg: "invalid workflowStatus: " + *req.WorkflowStatus}
	}
	if req.WorkflowStatus != nil {
		up := strings.ToUpper(*req.WorkflowStatus)
		req.WorkflowStatus = &up
	}
	if req.WorkflowStatus != nil {
		current, err := s.repo.GetRiskByID(ctx, id)
		if err != nil {
			return domain.Risk{}, err
		}
		if !isValidRiskTransition(current.WorkflowStatus, *req.WorkflowStatus) {
			return domain.Risk{}, &apierror.ValidationError{
				Msg: "invalid workflow transition: " + current.WorkflowStatus + " → " + *req.WorkflowStatus,
			}
		}
		req.ExpectedStatus = current.WorkflowStatus
	}
	if req.TreatmentStrategy != nil && !validTreatmentStrategies[strings.ToUpper(*req.TreatmentStrategy)] {
		return domain.Risk{}, &apierror.ValidationError{Msg: "treatmentStrategy must be MITIGATE, ACCEPT, TRANSFER, or VOID"}
	}
	if req.TreatmentStrategy != nil {
		up := strings.ToUpper(*req.TreatmentStrategy)
		req.TreatmentStrategy = &up
	}
	r, err := s.repo.UpdateRisk(ctx, id, req)
	if err != nil {
		return domain.Risk{}, err
	}
	return *r, nil
}
