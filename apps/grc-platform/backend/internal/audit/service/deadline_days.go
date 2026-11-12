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
	"log/slog"
	"net/http"
	"strings"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
)

// maxAllowedDays is the largest value an admin may set. The entity enforces the
// same limit; checking here gives a clear message before the call.
const maxAllowedDays = 365

// DeadlineDaysService defines business operations for an audit's allowed days.
type DeadlineDaysService interface {
	Get(ctx context.Context, auditID int) (*model.DeadlineDays, error)
	// Update changes the listed rows. Only hand-offs that happen afterwards use
	// the new values; deadlines already given never move.
	Update(ctx context.Context, auditID int, req model.UpdateDeadlineDaysRequest, updatedBy string) (*model.DeadlineDays, error)
}

type deadlineDaysService struct {
	repo  repository.DeadlineDaysRepository
	trail TrailService
}

// NewDeadlineDaysService constructs a DeadlineDaysService. trail may be nil
// (recording is then skipped), same contract as controlService.recordTrail.
func NewDeadlineDaysService(repo repository.DeadlineDaysRepository, trail TrailService) DeadlineDaysService {
	return &deadlineDaysService{repo: repo, trail: trail}
}

func (s *deadlineDaysService) Get(ctx context.Context, auditID int) (*model.DeadlineDays, error) {
	return s.repo.Get(ctx, auditID)
}

func (s *deadlineDaysService) Update(ctx context.Context, auditID int, req model.UpdateDeadlineDaysRequest, updatedBy string) (*model.DeadlineDays, error) {
	if len(req.Days) == 0 {
		return nil, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "days must not be empty"}
	}
	seen := map[string]bool{}
	rows := make([]model.DeadlineDaysRow, 0, len(req.Days))
	for i, row := range req.Days {
		row.CycleType = strings.ToUpper(row.CycleType)
		row.HandledBy = strings.ToUpper(row.HandledBy)
		if row.CycleType != "INITIAL" && row.CycleType != "REVISION" {
			return nil, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: fmt.Sprintf("days[%d]: cycleType must be INITIAL or REVISION", i)}
		}
		if row.HandledBy != "OWNER" && row.HandledBy != "COMPLIANCE" && row.HandledBy != "AUDITOR" {
			return nil, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: fmt.Sprintf("days[%d]: handledBy must be OWNER, COMPLIANCE or AUDITOR", i)}
		}
		if row.AllowedDays < 1 || row.AllowedDays > maxAllowedDays {
			return nil, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: fmt.Sprintf("days[%d]: allowedDays must be a whole number from 1 to %d", i, maxAllowedDays)}
		}
		key := row.CycleType + "|" + row.HandledBy
		if seen[key] {
			return nil, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: fmt.Sprintf("days[%d]: %s/%s is listed more than once", i, row.CycleType, row.HandledBy)}
		}
		seen[key] = true
		rows = append(rows, row)
	}
	before, err := s.repo.Get(ctx, auditID)
	if err != nil {
		return nil, err
	}
	after, err := s.repo.Put(ctx, auditID, rows, updatedBy)
	if err != nil {
		return nil, err
	}
	s.recordTrail(ctx, auditID, updatedBy, before, rows)
	return after, nil
}

// recordTrail appends a best-effort audit-level entry listing each changed value
// as from/to. A failure here must never fail the edit it describes.
func (s *deadlineDaysService) recordTrail(ctx context.Context, auditID int, actor string, before *model.DeadlineDays, rows []model.DeadlineDaysRow) {
	if s.trail == nil {
		return
	}
	old := map[string]int{}
	for _, d := range before.Days {
		old[d.CycleType+"|"+d.HandledBy] = d.AllowedDays
	}
	changes := []map[string]any{}
	for _, r := range rows {
		from := old[r.CycleType+"|"+r.HandledBy]
		if from == r.AllowedDays {
			continue
		}
		changes = append(changes, map[string]any{
			"cycleType": r.CycleType, "handledBy": r.HandledBy, "from": from, "to": r.AllowedDays,
		})
	}
	if len(changes) == 0 {
		return
	}
	if err := s.trail.RecordAuditAction(ctx, auditID, "UPDATED", actor, map[string]any{"deadlineDays": changes}); err != nil {
		slog.WarnContext(ctx, "record deadline days trail failed", "auditId", auditID, "err", err)
	}
}
