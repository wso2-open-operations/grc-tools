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

// Package service implements the business logic for the Risk Hub module.
// Handlers call service methods; services call repository methods.
// Business rules (status transition guards, validations, changelog writes)
// live here — not in handlers, not in repositories.
package service

import (
	"context"
	"fmt"
	"net/http"

	"github.com/wso2-open-operations/grc-platform/backend/internal/apierror"
	"github.com/wso2-open-operations/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-platform/backend/internal/risk/repository"
)

// RiskService defines the business operations for risk lifecycle management.
type RiskService interface {
	List(ctx context.Context, filter model.ListRisksFilter) ([]*model.RiskListItem, error)
	GetByID(ctx context.Context, id int) (*model.RiskDetail, error)
	Create(ctx context.Context, req model.CreateRiskRequest, createdBy string) (*model.CreateRiskResponse, error)
	NextSequenceID(ctx context.Context, sourceRegisterID int) (int, error)
	Update(ctx context.Context, id int, req model.UpdateRiskRequest, updatedBy string) error

	// Workflow transitions — each validates the current status before advancing.
	OwnerApprove(ctx context.Context, id int, byUserEmail string) error
	ManagementApprove(ctx context.Context, id int, byUserEmail string) error
	Approve(ctx context.Context, id int, byUserEmail string) error
	Reject(ctx context.Context, id int, req model.RejectRiskRequest, byUserEmail string) error
	Complete(ctx context.Context, id int, byUserEmail string) error
	Resubmit(ctx context.Context, id int, byUserEmail string) error
	Close(ctx context.Context, id int, byUserEmail string) error
	Cancel(ctx context.Context, id int, byUserEmail string) error
}

type riskService struct {
	repo repository.RiskRepository
}

// NewRiskService creates a RiskService backed by the given repository.
func NewRiskService(repo repository.RiskRepository) RiskService {
	return &riskService{repo: repo}
}

func (s *riskService) List(ctx context.Context, filter model.ListRisksFilter) ([]*model.RiskListItem, error) {
	return s.repo.List(ctx, filter)
}

func (s *riskService) GetByID(ctx context.Context, id int) (*model.RiskDetail, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *riskService) Create(ctx context.Context, req model.CreateRiskRequest, createdBy string) (*model.CreateRiskResponse, error) {
	return s.repo.Create(ctx, req, createdBy)
}

func (s *riskService) NextSequenceID(ctx context.Context, sourceRegisterID int) (int, error) {
	return s.repo.NextSequenceID(ctx, sourceRegisterID)
}

// Update saves risk field changes. If a restricted field changed while the risk is
// IN_REMEDIATION, it marks risk_type = UPDATED and moves to PENDING_AMENDMENT so
// it re-enters the full approval chain.
func (s *riskService) Update(ctx context.Context, id int, req model.UpdateRiskRequest, updatedBy string) error {
	status, err := s.repo.GetWorkflowStatus(ctx, id)
	if err != nil {
		return err
	}

	restrictedChanged, err := s.repo.Update(ctx, id, req, updatedBy)
	if err != nil {
		return err
	}

	if restrictedChanged && status == "IN_REMEDIATION" {
		if err = s.repo.SetRiskType(ctx, id, "UPDATED", updatedBy); err != nil {
			return err
		}
		return s.repo.UpdateStatus(ctx, id, "PENDING_AMENDMENT", updatedBy)
	}
	return nil
}

// OwnerApprove handles three situations:
//   - PENDING_RISK_OWNER_APPROVAL or PENDING_AMENDMENT (initial/amendment approval):
//     Accept+HIGH → PENDING_MANAGEMENT_APPROVAL; otherwise → PENDING_COMPLIANCE_REVIEW
//   - PENDING_OWNER_COMPLETION_APPROVAL (post-remediation): → PENDING_COMPLIANCE_CLOSURE
func (s *riskService) OwnerApprove(ctx context.Context, id int, byUserEmail string) error {
	detail, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	switch detail.WorkflowStatus {
	case "PENDING_RISK_OWNER_APPROVAL", "PENDING_AMENDMENT":
		if detail.OwnerFirstApprovedAt == nil {
			if err = s.repo.SetOwnerFirstApprovedAt(ctx, id, byUserEmail); err != nil {
				return err
			}
		}
		next := "PENDING_COMPLIANCE_REVIEW"
		if stringVal(detail.TreatmentStrategy) == "ACCEPT" && detail.GrossScore != nil && detail.GrossScore.RiskLevel == "HIGH" {
			next = "PENDING_MANAGEMENT_APPROVAL"
		}
		return s.repo.UpdateStatus(ctx, id, next, byUserEmail)

	case "PENDING_OWNER_COMPLETION_APPROVAL":
		return s.repo.UpdateStatus(ctx, id, "PENDING_COMPLIANCE_CLOSURE", byUserEmail)

	default:
		return &apierror.Error{StatusCode: http.StatusConflict, Body: fmt.Sprintf("cannot be owner-approved from status: %s", detail.WorkflowStatus)}
	}
}

// ManagementApprove moves PENDING_MANAGEMENT_APPROVAL → PENDING_COMPLIANCE_REVIEW.
func (s *riskService) ManagementApprove(ctx context.Context, id int, byUserEmail string) error {
	status, err := s.repo.GetWorkflowStatus(ctx, id)
	if err != nil {
		return err
	}
	if status != "PENDING_MANAGEMENT_APPROVAL" {
		return &apierror.Error{StatusCode: http.StatusConflict, Body: fmt.Sprintf("cannot be management-approved from status: %s", status)}
	}
	return s.repo.UpdateStatus(ctx, id, "PENDING_COMPLIANCE_REVIEW", byUserEmail)
}

// Approve is the compliance approval step: PENDING_COMPLIANCE_REVIEW → IN_REMEDIATION.
func (s *riskService) Approve(ctx context.Context, id int, byUserEmail string) error {
	status, err := s.repo.GetWorkflowStatus(ctx, id)
	if err != nil {
		return err
	}
	if status != "PENDING_COMPLIANCE_REVIEW" {
		return &apierror.Error{StatusCode: http.StatusConflict, Body: fmt.Sprintf("cannot be approved from status: %s", status)}
	}
	return s.repo.UpdateStatus(ctx, id, "IN_REMEDIATION", byUserEmail)
}

// Reject routes all rejections back to PENDING_REVISION (or IN_REMEDIATION for
// post-remediation owner rejection) and records where the rejection occurred.
func (s *riskService) Reject(ctx context.Context, id int, req model.RejectRiskRequest, byUserEmail string) error {
	if req.RejectionComment == "" {
		return &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "rejection_comment is required"}
	}
	status, err := s.repo.GetWorkflowStatus(ctx, id)
	if err != nil {
		return err
	}

	var stage, nextStatus string
	switch status {
	case "PENDING_RISK_OWNER_APPROVAL", "PENDING_AMENDMENT":
		stage = "OWNER"
		nextStatus = "PENDING_REVISION"
	case "PENDING_MANAGEMENT_APPROVAL":
		stage = "MANAGEMENT"
		nextStatus = "PENDING_REVISION"
	case "PENDING_COMPLIANCE_REVIEW":
		stage = "COMPLIANCE"
		nextStatus = "PENDING_REVISION"
	case "PENDING_OWNER_COMPLETION_APPROVAL":
		stage = "COMPLETION_OWNER"
		nextStatus = "PENDING_REVISION"
	default:
		return &apierror.Error{StatusCode: http.StatusConflict, Body: fmt.Sprintf("cannot be rejected from status: %s", status)}
	}

	if err = s.repo.SetRejection(ctx, id, req.RejectionComment, stage, byUserEmail); err != nil {
		return err
	}
	return s.repo.UpdateStatus(ctx, id, nextStatus, byUserEmail)
}

// Complete moves IN_REMEDIATION → PENDING_OWNER_COMPLETION_APPROVAL.
func (s *riskService) Complete(ctx context.Context, id int, byUserEmail string) error {
	status, err := s.repo.GetWorkflowStatus(ctx, id)
	if err != nil {
		return err
	}
	if status != "IN_REMEDIATION" {
		return &apierror.Error{StatusCode: http.StatusConflict, Body: fmt.Sprintf("cannot be completed from status: %s", status)}
	}
	return s.repo.UpdateStatus(ctx, id, "PENDING_OWNER_COMPLETION_APPROVAL", byUserEmail)
}

// Resubmit clears rejection info and moves PENDING_REVISION back to the appropriate approval stage:
// - COMPLETION_OWNER rejection → PENDING_OWNER_COMPLETION_APPROVAL
// - all other rejections → PENDING_RISK_OWNER_APPROVAL
func (s *riskService) Resubmit(ctx context.Context, id int, byUserEmail string) error {
	detail, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if detail.WorkflowStatus != "PENDING_REVISION" {
		return &apierror.Error{StatusCode: http.StatusConflict, Body: fmt.Sprintf("cannot be resubmitted from status: %s", detail.WorkflowStatus)}
	}
	next := "PENDING_RISK_OWNER_APPROVAL"
	if stringVal(detail.RejectionStage) == "COMPLETION_OWNER" {
		next = "PENDING_OWNER_COMPLETION_APPROVAL"
	}
	if err = s.repo.ClearRejection(ctx, id, byUserEmail); err != nil {
		return err
	}
	return s.repo.UpdateStatus(ctx, id, next, byUserEmail)
}

// Close moves PENDING_COMPLIANCE_CLOSURE → CLOSED.
func (s *riskService) Close(ctx context.Context, id int, byUserEmail string) error {
	status, err := s.repo.GetWorkflowStatus(ctx, id)
	if err != nil {
		return err
	}
	if status != "PENDING_COMPLIANCE_CLOSURE" {
		return &apierror.Error{StatusCode: http.StatusConflict, Body: fmt.Sprintf("cannot be closed from status: %s", status)}
	}
	return s.repo.UpdateStatus(ctx, id, "CLOSED", byUserEmail)
}

// Cancel soft-deletes a risk by setting it to CANCELLED. Only valid from PENDING_RISK_OWNER_APPROVAL.
func (s *riskService) Cancel(ctx context.Context, id int, byUserEmail string) error {
	status, err := s.repo.GetWorkflowStatus(ctx, id)
	if err != nil {
		return err
	}
	if status != "PENDING_RISK_OWNER_APPROVAL" {
		return &apierror.Error{StatusCode: http.StatusConflict, Body: fmt.Sprintf("cannot be cancelled from status: %s", status)}
	}
	return s.repo.Cancel(ctx, id, byUserEmail)
}

func stringVal(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
