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

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/repository"
)

type riskComplianceRefService struct {
	repo repository.RiskComplianceRefRepository
}

// NewRiskComplianceRefService constructs a RiskComplianceRefService.
func NewRiskComplianceRefService(repo repository.RiskComplianceRefRepository) RiskComplianceRefService {
	return &riskComplianceRefService{repo: repo}
}

func (s *riskComplianceRefService) AddRiskComplianceRef(ctx context.Context, riskID int, req domain.AddRiskComplianceRefRequest) (domain.RiskComplianceRefLink, error) {
	if riskID <= 0 {
		return domain.RiskComplianceRefLink{}, &apierror.ValidationError{Msg: "riskId must be a positive integer"}
	}
	if req.ReferenceID <= 0 {
		return domain.RiskComplianceRefLink{}, &apierror.ValidationError{Msg: "referenceId must be a positive integer"}
	}
	link, err := s.repo.AddRiskComplianceRef(ctx, riskID, req)
	if err != nil {
		return domain.RiskComplianceRefLink{}, err
	}
	return *link, nil
}

func (s *riskComplianceRefService) DeleteRiskComplianceRef(ctx context.Context, riskID, referenceID int) error {
	if riskID <= 0 {
		return &apierror.ValidationError{Msg: "riskId must be a positive integer"}
	}
	if referenceID <= 0 {
		return &apierror.ValidationError{Msg: "referenceId must be a positive integer"}
	}
	return s.repo.DeleteRiskComplianceRef(ctx, riskID, referenceID)
}

func (s *riskComplianceRefService) ListRiskComplianceRefs(ctx context.Context, riskID int) (domain.ListRiskComplianceRefsResponse, error) {
	if riskID <= 0 {
		return domain.ListRiskComplianceRefsResponse{}, &apierror.ValidationError{Msg: "riskId must be a positive integer"}
	}
	links, err := s.repo.ListRiskComplianceRefs(ctx, riskID)
	if err != nil {
		return domain.ListRiskComplianceRefsResponse{}, err
	}
	if links == nil {
		links = []domain.RiskComplianceRefLink{}
	}
	return domain.ListRiskComplianceRefsResponse{References: links}, nil
}
