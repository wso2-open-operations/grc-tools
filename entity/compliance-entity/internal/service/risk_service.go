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

// validTreatmentStrategies / validIdentifiedByTypes mirror the risk.treatment_strategy
// and risk.identified_by_type ENUMs in risk_schema.sql. Both columns are nullable,
// so these are only enforced when a value is provided.
var validTreatmentStrategies = map[string]bool{"MITIGATE": true, "ACCEPT": true, "TRANSFER": true, "VOID": true}
var validIdentifiedByTypes = map[string]bool{"EMPLOYEE": true, "EXTERNAL_PERSON": true, "TOOL": true}

func (s *riskService) SearchRisks(ctx context.Context, req domain.SearchRisksRequest) (domain.SearchRisksResponse, error) {
	for _, sk := range req.WorkflowStatusKeys {
		if !validRiskStatuses[strings.ToUpper(sk)] {
			return domain.SearchRisksResponse{}, &apierror.ValidationError{Msg: "invalid workflowStatusKey: " + sk}
		}
	}
	for _, qk := range req.RiskQuarterKeys {
		if !validRiskQuarters[strings.ToUpper(qk)] {
			return domain.SearchRisksResponse{}, &apierror.ValidationError{Msg: "invalid riskQuarterKey: " + qk + " (must be Q1, Q2, Q3, or Q4)"}
		}
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
	if !validRiskQuarters[strings.ToUpper(req.RiskQuarter)] {
		return domain.Risk{}, &apierror.ValidationError{Msg: "riskQuarter must be Q1, Q2, Q3, or Q4"}
	}
	if req.TreatmentStrategy != nil && !validTreatmentStrategies[strings.ToUpper(*req.TreatmentStrategy)] {
		return domain.Risk{}, &apierror.ValidationError{Msg: "treatmentStrategy must be MITIGATE, ACCEPT, TRANSFER, or VOID"}
	}
	if req.IdentifiedByType != nil && !validIdentifiedByTypes[strings.ToUpper(*req.IdentifiedByType)] {
		return domain.Risk{}, &apierror.ValidationError{Msg: "identifiedByType must be EMPLOYEE, EXTERNAL_PERSON, or TOOL"}
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
	// TODO(risk-workflow): enforce the risk workflow_status TRANSITION rules here,
	// the same way audit_control_service.go / audit_evidence_service.go do it.
	// Currently only enum-membership is checked above, so a caller can jump to any
	// valid status (e.g. DRAFT -> CLOSED) and skip owner/management/compliance
	// approval stages. To implement:
	//   1. Add an `allowedRiskTransitions map[string][]string` mapping each of the
	//      12 workflow_status values to its legal next states (register → owner
	//      approval → management approval → compliance review → remediation →
	//      completion approval → compliance closure → closed; plus amendment/
	//      revision/escalated/cancelled paths).
	//   2. Fetch the current risk (s.repo.GetRiskByID) and reject the update if
	//      isValidRiskTransition(current.WorkflowStatus, *req.WorkflowStatus) is false.
	// See isValidControlTransition for the pattern to copy.
	if req.TreatmentStrategy != nil && !validTreatmentStrategies[strings.ToUpper(*req.TreatmentStrategy)] {
		return domain.Risk{}, &apierror.ValidationError{Msg: "treatmentStrategy must be MITIGATE, ACCEPT, TRANSFER, or VOID"}
	}
	r, err := s.repo.UpdateRisk(ctx, id, req)
	if err != nil {
		return domain.Risk{}, err
	}
	return *r, nil
}
