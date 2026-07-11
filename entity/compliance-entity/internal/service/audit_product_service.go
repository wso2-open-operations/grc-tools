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

type auditProductService struct {
	repo repository.AuditProductRepository
}

// NewAuditProductService constructs an AuditProductService.
func NewAuditProductService(repo repository.AuditProductRepository) AuditProductService {
	return &auditProductService{repo: repo}
}

var validAuditProductStatuses = map[string]bool{"ACTIVE": true, "INACTIVE": true}

func (s *auditProductService) SearchAuditProducts(ctx context.Context, req domain.SearchAuditProductsRequest) (domain.SearchAuditProductsResponse, error) {
	if req.StatusKey != "" && !validAuditProductStatuses[strings.ToUpper(req.StatusKey)] {
		return domain.SearchAuditProductsResponse{}, &apierror.ValidationError{Msg: "invalid statusKey: must be ACTIVE or INACTIVE"}
	}
	req.StatusKey = strings.ToUpper(req.StatusKey)
	normalizePagination(&req.Pagination)
	products, total, err := s.repo.SearchAuditProducts(ctx, req)
	if err != nil {
		return domain.SearchAuditProductsResponse{}, err
	}
	if products == nil {
		products = []domain.AuditProduct{}
	}
	return domain.SearchAuditProductsResponse{Products: products, Total: total, Limit: req.Pagination.Limit, Offset: req.Pagination.Offset}, nil
}

func (s *auditProductService) GetAuditProductByID(ctx context.Context, id int) (domain.AuditProduct, error) {
	if id <= 0 {
		return domain.AuditProduct{}, &apierror.ValidationError{Msg: "product id must be a positive integer"}
	}
	p, err := s.repo.GetAuditProductByID(ctx, id)
	if err != nil {
		return domain.AuditProduct{}, err
	}
	return *p, nil
}

func (s *auditProductService) CreateAuditProduct(ctx context.Context, req domain.CreateAuditProductRequest) (domain.AuditProduct, error) {
	if req.Name == "" {
		return domain.AuditProduct{}, &apierror.ValidationError{Msg: "name is required"}
	}
	if req.Status != "" && !validAuditProductStatuses[strings.ToUpper(req.Status)] {
		return domain.AuditProduct{}, &apierror.ValidationError{Msg: "invalid status: must be ACTIVE or INACTIVE"}
	}
	req.Status = strings.ToUpper(req.Status)
	if req.CreatedBy == "" {
		return domain.AuditProduct{}, &apierror.ValidationError{Msg: "createdBy is required"}
	}
	p, err := s.repo.CreateAuditProduct(ctx, req)
	if err != nil {
		return domain.AuditProduct{}, err
	}
	return *p, nil
}

func (s *auditProductService) UpdateAuditProduct(ctx context.Context, id int, req domain.UpdateAuditProductRequest) (domain.AuditProduct, error) {
	if id <= 0 {
		return domain.AuditProduct{}, &apierror.ValidationError{Msg: "product id must be a positive integer"}
	}
	if req.Status != nil && !validAuditProductStatuses[strings.ToUpper(*req.Status)] {
		return domain.AuditProduct{}, &apierror.ValidationError{Msg: "invalid status: must be ACTIVE or INACTIVE"}
	}
	if req.Status != nil {
		up := strings.ToUpper(*req.Status)
		req.Status = &up
	}
	if req.UpdatedBy == "" {
		return domain.AuditProduct{}, &apierror.ValidationError{Msg: "updatedBy is required"}
	}
	p, err := s.repo.UpdateAuditProduct(ctx, id, req)
	if err != nil {
		return domain.AuditProduct{}, err
	}
	return *p, nil
}
