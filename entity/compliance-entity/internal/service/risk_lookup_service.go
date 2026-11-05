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
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/repository"
)

type riskLookupService struct {
	repo repository.RiskLookupRepository
	kind repository.LookupKind
}

// NewRiskLookupService constructs a RiskLookupService for one lookup kind.
func NewRiskLookupService(repo repository.RiskLookupRepository, kind repository.LookupKind) RiskLookupService {
	return &riskLookupService{repo: repo, kind: kind}
}

// customerCodePattern mirrors chk_risk_customer_code in risk_schema.sql. The
// code is embedded in risk codes, so it may not contain the "-" separator.
var customerCodePattern = regexp.MustCompile(`^[A-Z0-9]{1,12}$`)

// maxLookupNameLen matches the VARCHAR(255) name column of every lookup table.
// Checked here so an over-long name is a clear 400 rather than a database
// error that surfaces as a 500.
const maxLookupNameLen = 255

func checkLookupNameLength(name string) error {
	if utf8.RuneCountInString(name) > maxLookupNameLen {
		return &apierror.ValidationError{Msg: fmt.Sprintf("name must be at most %d characters", maxLookupNameLen)}
	}
	return nil
}

var validRiskLookupStatuses = map[string]bool{"ACTIVE": true, "INACTIVE": true}

func (s *riskLookupService) ListRiskLookups(ctx context.Context, statusKey string) (domain.ListRiskLookupsResponse, error) {
	statusKey = strings.ToUpper(statusKey)
	if statusKey != "" && !validRiskLookupStatuses[statusKey] {
		return domain.ListRiskLookupsResponse{}, &apierror.ValidationError{Msg: "invalid status: must be ACTIVE or INACTIVE"}
	}
	values, err := s.repo.ListRiskLookups(ctx, statusKey)
	if err != nil {
		return domain.ListRiskLookupsResponse{}, err
	}
	if values == nil {
		values = []domain.RiskLookup{}
	}
	return domain.ListRiskLookupsResponse{Values: values}, nil
}

func (s *riskLookupService) CreateRiskLookup(ctx context.Context, req domain.CreateRiskLookupRequest) (domain.RiskLookup, error) {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return domain.RiskLookup{}, &apierror.ValidationError{Msg: "name is required"}
	}
	if err := checkLookupNameLength(req.Name); err != nil {
		return domain.RiskLookup{}, err
	}
	if req.CreatedBy == "" {
		return domain.RiskLookup{}, &apierror.ValidationError{Msg: "createdBy is required"}
	}
	if s.kind.HasCode() {
		if req.Code == nil {
			return domain.RiskLookup{}, &apierror.ValidationError{Msg: "code is required"}
		}
		if err := validateCustomerCode(*req.Code); err != nil {
			return domain.RiskLookup{}, err
		}
	} else if req.Code != nil {
		return domain.RiskLookup{}, &apierror.ValidationError{Msg: "a " + s.kind.Noun() + " has no code"}
	}
	if req.ParentID != nil {
		if !s.kind.HasParent() {
			return domain.RiskLookup{}, &apierror.ValidationError{Msg: "a " + s.kind.Noun() + " has no parent"}
		}
		if *req.ParentID <= 0 {
			return domain.RiskLookup{}, &apierror.ValidationError{Msg: "parentId must be a positive integer"}
		}
	}
	v, err := s.repo.CreateRiskLookup(ctx, req)
	if err != nil {
		return domain.RiskLookup{}, err
	}
	return *v, nil
}

func (s *riskLookupService) UpdateRiskLookup(ctx context.Context, id int, req domain.UpdateRiskLookupRequest) (domain.RiskLookup, error) {
	if id <= 0 {
		return domain.RiskLookup{}, &apierror.ValidationError{Msg: s.kind.Noun() + " id must be a positive integer"}
	}
	if req.UpdatedBy == "" {
		return domain.RiskLookup{}, &apierror.ValidationError{Msg: "updatedBy is required"}
	}
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if trimmed == "" {
			return domain.RiskLookup{}, &apierror.ValidationError{Msg: "name cannot be empty"}
		}
		if err := checkLookupNameLength(trimmed); err != nil {
			return domain.RiskLookup{}, err
		}
		req.Name = &trimmed
	}
	if req.Code != nil {
		if !s.kind.HasCode() {
			return domain.RiskLookup{}, &apierror.ValidationError{Msg: "a " + s.kind.Noun() + " has no code"}
		}
		if err := validateCustomerCode(*req.Code); err != nil {
			return domain.RiskLookup{}, err
		}
	}
	if req.Status != nil {
		upper := strings.ToUpper(*req.Status)
		if !validRiskLookupStatuses[upper] {
			return domain.RiskLookup{}, &apierror.ValidationError{Msg: "invalid status: must be ACTIVE or INACTIVE"}
		}
		req.Status = &upper
	}
	v, err := s.repo.UpdateRiskLookup(ctx, id, req)
	if err != nil {
		return domain.RiskLookup{}, err
	}
	return *v, nil
}

func (s *riskLookupService) DeleteRiskLookup(ctx context.Context, id int) error {
	if id <= 0 {
		return &apierror.ValidationError{Msg: s.kind.Noun() + " id must be a positive integer"}
	}
	return s.repo.DeleteRiskLookup(ctx, id)
}

// validateCustomerCode checks code exactly as given: it is not upper-cased
// for the caller, because the admin should see the code that will appear in
// risk codes, not a silently altered one.
func validateCustomerCode(code string) error {
	if !customerCodePattern.MatchString(code) {
		return &apierror.ValidationError{Msg: "code must be 1-12 uppercase letters (A-Z) or digits (0-9)"}
	}
	return nil
}
