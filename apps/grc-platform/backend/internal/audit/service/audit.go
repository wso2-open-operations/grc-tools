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

// Package service implements the business logic for the Audit Hub module.
// Handlers call service methods; services call repository methods.
// Validation rules and workflow guards live here — not in handlers or repositories.
package service

import (
	"context"
	"net/http"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
)

// AuditService defines business operations for audit engagements.
type AuditService interface {
	List(ctx context.Context) ([]*model.Audit, error)
	GetByID(ctx context.Context, id int) (*model.Audit, error)
	Create(ctx context.Context, req model.CreateAuditRequest, createdBy string) (*model.Audit, error)
	Update(ctx context.Context, id int, req model.UpdateAuditRequest, updatedBy string) error
	Delete(ctx context.Context, id int, deletedBy string) error
}

type auditService struct {
	repo          repository.AuditRepository
	frameworkRepo repository.FrameworkRepository
	productRepo   repository.ProductRepository
}

// NewAuditService wires audit, framework, and product repos so Create can validate references.
func NewAuditService(
	repo repository.AuditRepository,
	frameworkRepo repository.FrameworkRepository,
	productRepo repository.ProductRepository,
) AuditService {
	return &auditService{repo: repo, frameworkRepo: frameworkRepo, productRepo: productRepo}
}

func (s *auditService) List(ctx context.Context) ([]*model.Audit, error) {
	return s.repo.List(ctx)
}

func (s *auditService) GetByID(ctx context.Context, id int) (*model.Audit, error) {
	a, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, &apierror.Error{StatusCode: http.StatusNotFound, Body: "audit not found"}
	}
	return a, nil
}

func (s *auditService) Create(ctx context.Context, req model.CreateAuditRequest, createdBy string) (*model.Audit, error) {
	if req.Name == "" {
		return nil, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "name is required"}
	}
	if req.FrameworkID <= 0 || req.ProductID <= 0 {
		return nil, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "frameworkId and productId are required"}
	}
	if req.PeriodStart == "" || req.PeriodEnd == "" {
		return nil, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "periodStart and periodEnd are required"}
	}
	if createdBy == "" {
		return nil, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "authenticated user email is missing from token — check Asgardeo app email scope"}
	}

	// Verify framework and product exist.
	fw, err := s.frameworkRepo.GetByID(ctx, req.FrameworkID)
	if err != nil {
		return nil, err
	}
	if fw == nil {
		return nil, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "framework not found"}
	}
	pr, err := s.productRepo.GetByID(ctx, req.ProductID)
	if err != nil {
		return nil, err
	}
	if pr == nil {
		return nil, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "product not found"}
	}

	return s.repo.Create(ctx, req, createdBy)
}

func (s *auditService) Update(ctx context.Context, id int, req model.UpdateAuditRequest, updatedBy string) error {
	a, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if a == nil {
		return &apierror.Error{StatusCode: http.StatusNotFound, Body: "audit not found"}
	}
	return s.repo.Update(ctx, id, req, updatedBy)
}

func (s *auditService) Delete(ctx context.Context, id int, deletedBy string) error {
	if deletedBy == "" {
		return &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "authenticated user email is missing from token — check Asgardeo app email scope"}
	}
	a, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if a == nil {
		return &apierror.Error{StatusCode: http.StatusNotFound, Body: "audit not found"}
	}
	return s.repo.Delete(ctx, id, deletedBy)
}
