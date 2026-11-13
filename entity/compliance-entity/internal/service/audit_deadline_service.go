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
	"strings"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/deadline"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/repository"
)

// DeadlineDaysService reads and edits an audit's allowed days.
type DeadlineDaysService interface {
	GetDeadlineDays(ctx context.Context, auditID int) (domain.DeadlineDaysResponse, error)
	UpdateDeadlineDays(ctx context.Context, auditID int, req domain.UpdateDeadlineDaysRequest) (domain.DeadlineDaysResponse, error)
}

type deadlineDaysService struct {
	repo      repository.StageRepository
	auditRepo repository.AuditRepository
}

// NewDeadlineDaysService constructs a DeadlineDaysService.
func NewDeadlineDaysService(repo repository.StageRepository, auditRepo repository.AuditRepository) DeadlineDaysService {
	return &deadlineDaysService{repo: repo, auditRepo: auditRepo}
}

var (
	deadlineCycles   = []string{deadline.Initial, deadline.Revision}
	deadlineHandlers = []string{deadline.Owner, deadline.Compliance, deadline.Auditor}
)

func (s *deadlineDaysService) GetDeadlineDays(ctx context.Context, auditID int) (domain.DeadlineDaysResponse, error) {
	if auditID <= 0 {
		return domain.DeadlineDaysResponse{}, &apierror.ValidationError{Msg: "auditId must be a positive integer"}
	}
	if _, err := s.auditRepo.GetAuditByID(ctx, auditID); err != nil {
		return domain.DeadlineDaysResponse{}, err
	}
	return s.build(ctx, auditID)
}

func (s *deadlineDaysService) UpdateDeadlineDays(ctx context.Context, auditID int, req domain.UpdateDeadlineDaysRequest) (domain.DeadlineDaysResponse, error) {
	if auditID <= 0 {
		return domain.DeadlineDaysResponse{}, &apierror.ValidationError{Msg: "auditId must be a positive integer"}
	}
	if req.UpdatedBy == "" {
		return domain.DeadlineDaysResponse{}, &apierror.ValidationError{Msg: "updatedBy is required"}
	}
	if len(req.Days) == 0 {
		return domain.DeadlineDaysResponse{}, &apierror.ValidationError{Msg: "days must not be empty"}
	}
	seen := map[string]bool{}
	rows := make([]domain.DeadlineDaysRow, 0, len(req.Days))
	for i, row := range req.Days {
		row.CycleType = strings.ToUpper(row.CycleType)
		row.HandledBy = strings.ToUpper(row.HandledBy)
		if !deadline.ValidCycle(row.CycleType) {
			return domain.DeadlineDaysResponse{}, &apierror.ValidationError{Msg: fmt.Sprintf("days[%d]: cycleType must be INITIAL or REVISION", i)}
		}
		if !deadline.ValidHandler(row.HandledBy) {
			return domain.DeadlineDaysResponse{}, &apierror.ValidationError{Msg: fmt.Sprintf("days[%d]: handledBy must be OWNER, COMPLIANCE or AUDITOR", i)}
		}
		if row.AllowedDays < 1 || row.AllowedDays > deadline.MaxAllowedDays {
			return domain.DeadlineDaysResponse{}, &apierror.ValidationError{Msg: fmt.Sprintf("days[%d]: allowedDays must be a whole number from 1 to %d", i, deadline.MaxAllowedDays)}
		}
		key := row.CycleType + "|" + row.HandledBy
		if seen[key] {
			return domain.DeadlineDaysResponse{}, &apierror.ValidationError{Msg: fmt.Sprintf("days[%d]: %s/%s is listed more than once", i, row.CycleType, row.HandledBy)}
		}
		seen[key] = true
		rows = append(rows, row)
	}
	audit, err := s.auditRepo.GetAuditByID(ctx, auditID)
	if err != nil {
		return domain.DeadlineDaysResponse{}, err
	}
	if audit.Status == "REMOVED" {
		return domain.DeadlineDaysResponse{}, &apierror.ConflictError{Msg: "cannot edit deadline days: audit is removed"}
	}
	if err := s.repo.PutDeadlineDays(ctx, auditID, rows); err != nil {
		return domain.DeadlineDaysResponse{}, err
	}
	return s.build(ctx, auditID)
}

// build lists all six cycle/handler values, marking the ones still on the default.
func (s *deadlineDaysService) build(ctx context.Context, auditID int) (domain.DeadlineDaysResponse, error) {
	saved, err := s.repo.GetDeadlineDays(ctx, auditID)
	if err != nil {
		return domain.DeadlineDaysResponse{}, err
	}
	defaults := deadline.DefaultDays()
	resp := domain.DeadlineDaysResponse{AuditID: auditID, Phase: deadline.PhaseEvidence}
	for _, cycle := range deadlineCycles {
		for _, handler := range deadlineHandlers {
			v, ok := saved[cycle][handler]
			if !ok || v <= 0 {
				v, ok = defaults[cycle][handler], false
			}
			resp.Days = append(resp.Days, domain.DeadlineDaysRow{
				CycleType: cycle, HandledBy: handler, AllowedDays: v, IsDefault: !ok,
			})
		}
	}
	return resp, nil
}
