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
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/deadline"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/repository"
)

type fakeStageRepo struct {
	stages  map[int][]deadline.Stage
	days    deadline.Days
	listErr error
	put     []domain.DeadlineDaysRow
	listed  int
}

func (f *fakeStageRepo) ListStagesByControls(_ context.Context, ids []int) (map[int][]deadline.Stage, error) {
	f.listed++
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.stages, nil
}
func (f *fakeStageRepo) GetDeadlineDays(context.Context, int) (deadline.Days, error) {
	if f.days == nil {
		return deadline.Days{}, nil
	}
	return f.days, nil
}
func (f *fakeStageRepo) PutDeadlineDays(_ context.Context, _ int, rows []domain.DeadlineDaysRow) error {
	f.put = rows
	for _, r := range rows {
		if f.days == nil {
			f.days = deadline.Days{}
		}
		f.days.Set(r.CycleType, r.HandledBy, r.AllowedDays)
	}
	return nil
}

type fakeAuditRepo struct {
	repository.AuditRepository
	status string
	err    error
}

func (f fakeAuditRepo) GetAuditByID(context.Context, int) (*domain.Audit, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &domain.Audit{Status: f.status}, nil
}

func dayAt(m time.Month, d, h int) time.Time {
	return time.Date(2026, m, d, h, 0, 0, 0, deadline.Location)
}

func TestEnrichDeadlinesFillsDesignControls(t *testing.T) {
	planned := func(m time.Month, d int) *time.Time {
		v := time.Date(2026, m, d, 0, 0, 0, 0, deadline.Location)
		return &v
	}
	end1, end2 := dayAt(time.November, 10, 14), dayAt(time.November, 23, 21)
	repo := &fakeStageRepo{stages: map[int][]deadline.Stage{
		1: {
			{ID: 1, ControlID: 1, HandledBy: deadline.Owner, CycleType: deadline.Initial, StartedAt: dayAt(time.November, 1, 9),
				DueDate: *planned(time.November, 7), PlannedComplianceDate: planned(time.November, 17), PlannedAuditorDate: planned(time.November, 27), EndedAt: &end1},
			{ID: 2, ControlID: 1, HandledBy: deadline.Compliance, CycleType: deadline.Initial, StartedAt: end1, DueDate: *planned(time.November, 20), EndedAt: &end2},
			{ID: 3, ControlID: 1, HandledBy: deadline.Auditor, CycleType: deadline.Initial, StartedAt: end2, DueDate: *planned(time.December, 3)},
		},
	}}
	svc := &controlService{stageRepo: repo, now: func() time.Time { return dayAt(time.November, 28, 9) }}
	controls := []domain.AuditControl{
		{ID: 1, RequirementType: "DESIGN"},
		{ID: 2, RequirementType: "DESIGN"}, // no stages: predates the feature
		{ID: 3, RequirementType: "OE"},     // never tracked
	}
	_ = svc.enrichDeadlines(context.Background(), controls)

	c := controls[0]
	if !c.DeadlineTracked || c.CurrentHandler == nil || *c.CurrentHandler != "AUDITOR" {
		t.Fatalf("control 1 not tracked correctly: %+v", c)
	}
	if *c.CurrentDueDate != "2026-12-03" || *c.PlannedOwnerDate != "2026-11-07" ||
		*c.PlannedComplianceDate != "2026-11-17" || *c.PlannedAuditorDate != "2026-11-27" {
		t.Fatalf("dates wrong: %+v", c)
	}
	if c.Delayed == nil || !*c.Delayed {
		t.Fatal("original EY date (27 Nov) passed, should be Delayed")
	}
	if len(c.DelayLog) != 6 || c.DelayLog[0].HandledBy != "OWNER" || c.DelayLog[0].Date != "2026-11-08" || c.DelayLog[5].Date != "2026-11-23" {
		t.Fatalf("delay log wrong: %+v", c.DelayLog)
	}
	if controls[1].DeadlineTracked || controls[1].Delayed != nil {
		t.Fatal("a control with no stages is left untouched")
	}
	if controls[2].DeadlineTracked {
		t.Fatal("OE controls are never tracked")
	}
}

func TestEnrichDeadlinesOnlyQueriesDesignControls(t *testing.T) {
	repo := &fakeStageRepo{}
	svc := &controlService{stageRepo: repo, now: time.Now}
	_ = svc.enrichDeadlines(context.Background(), []domain.AuditControl{{ID: 1, RequirementType: "OE"}})
	if repo.listed != 0 {
		t.Fatal("no stage query is needed when there are no DESIGN controls")
	}
}

func TestEnrichDeadlinesOffDoesNothing(t *testing.T) {
	svc := &controlService{now: time.Now}
	controls := []domain.AuditControl{{ID: 1, RequirementType: "DESIGN"}}
	_ = svc.enrichDeadlines(context.Background(), controls)
	if controls[0].DeadlineTracked {
		t.Fatal("with no stage repository the fields stay empty")
	}
}

func TestEnrichDeadlinesLoadFailureIsReturned(t *testing.T) {
	svc := &controlService{stageRepo: &fakeStageRepo{listErr: errors.New("boom")}, now: time.Now}
	controls := []domain.AuditControl{{ID: 1, RequirementType: "DESIGN"}}
	if err := svc.enrichDeadlines(context.Background(), controls); err == nil {
		t.Fatal("a failed stage load must be returned, not shown as an untracked control")
	}
	if controls[0].DeadlineTracked {
		t.Fatal("fields stay empty when stages cannot be loaded")
	}
}

func TestEnrichAfterWriteLoadFailureDoesNotFailTheWrite(t *testing.T) {
	svc := &controlService{stageRepo: &fakeStageRepo{listErr: errors.New("boom")}, now: time.Now}
	c := &domain.AuditControl{ID: 1, RequirementType: "DESIGN"}
	svc.enrichAfterWrite(context.Background(), c)
	if c.DeadlineTracked {
		t.Fatal("fields stay empty when stages cannot be loaded")
	}
}

func TestNormalizeCurrentHandlers(t *testing.T) {
	req := domain.SearchControlsRequest{CurrentHandlers: []string{"owner", " Auditor "}}
	if err := normalizeCurrentHandlers(&req); err != nil {
		t.Fatal(err)
	}
	if req.CurrentHandlers[0] != "OWNER" || req.CurrentHandlers[1] != "AUDITOR" {
		t.Fatalf("got %v", req.CurrentHandlers)
	}
	bad := domain.SearchControlsRequest{CurrentHandlers: []string{"EY"}}
	var ve *apierror.ValidationError
	if err := normalizeCurrentHandlers(&bad); !errors.As(err, &ve) {
		t.Fatalf("expected a validation error, got %v", err)
	}
}

func TestGetDeadlineDaysListsDefaults(t *testing.T) {
	svc := NewDeadlineDaysService(&fakeStageRepo{}, fakeAuditRepo{status: "ACTIVE"})
	resp, err := svc.GetDeadlineDays(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Days) != 6 || resp.Phase != "EVIDENCE" {
		t.Fatalf("got %+v", resp)
	}
	for _, d := range resp.Days {
		if !d.IsDefault {
			t.Errorf("%s/%s should be a default", d.CycleType, d.HandledBy)
		}
	}
	if resp.Days[0].CycleType != "INITIAL" || resp.Days[0].HandledBy != "OWNER" || resp.Days[0].AllowedDays != 30 {
		t.Fatalf("first row = %+v", resp.Days[0])
	}
}

func TestUpdateDeadlineDaysSavesAndMarksCustom(t *testing.T) {
	repo := &fakeStageRepo{}
	svc := NewDeadlineDaysService(repo, fakeAuditRepo{status: "ACTIVE"})
	resp, err := svc.UpdateDeadlineDays(context.Background(), 7, domain.UpdateDeadlineDaysRequest{
		UpdatedBy: "admin@wso2.com",
		Days:      []domain.DeadlineDaysRow{{CycleType: "initial", HandledBy: "compliance", AllowedDays: 12}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.put) != 1 || repo.put[0].CycleType != "INITIAL" || repo.put[0].HandledBy != "COMPLIANCE" {
		t.Fatalf("saved %+v", repo.put)
	}
	for _, d := range resp.Days {
		isEdited := d.CycleType == "INITIAL" && d.HandledBy == "COMPLIANCE"
		if isEdited && (d.IsDefault || d.AllowedDays != 12) {
			t.Errorf("edited row wrong: %+v", d)
		}
		if !isEdited && !d.IsDefault {
			t.Errorf("untouched row should stay default: %+v", d)
		}
	}
}

func TestUpdateDeadlineDaysValidation(t *testing.T) {
	svc := NewDeadlineDaysService(&fakeStageRepo{}, fakeAuditRepo{status: "ACTIVE"})
	row := func(c, h string, n int) domain.DeadlineDaysRow {
		return domain.DeadlineDaysRow{CycleType: c, HandledBy: h, AllowedDays: n}
	}
	cases := map[string]domain.UpdateDeadlineDaysRequest{
		"missing updatedBy": {Days: []domain.DeadlineDaysRow{row("INITIAL", "OWNER", 5)}},
		"empty days":        {UpdatedBy: "a"},
		"bad cycle":         {UpdatedBy: "a", Days: []domain.DeadlineDaysRow{row("FIRST", "OWNER", 5)}},
		"bad handler":       {UpdatedBy: "a", Days: []domain.DeadlineDaysRow{row("INITIAL", "EY", 5)}},
		"zero days":         {UpdatedBy: "a", Days: []domain.DeadlineDaysRow{row("INITIAL", "OWNER", 0)}},
		"too many days":     {UpdatedBy: "a", Days: []domain.DeadlineDaysRow{row("INITIAL", "OWNER", 366)}},
		"duplicate":         {UpdatedBy: "a", Days: []domain.DeadlineDaysRow{row("INITIAL", "OWNER", 5), row("INITIAL", "OWNER", 6)}},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			var ve *apierror.ValidationError
			if _, err := svc.UpdateDeadlineDays(context.Background(), 7, req); !errors.As(err, &ve) {
				t.Fatalf("expected a validation error, got %v", err)
			}
		})
	}
}

func TestUpdateDeadlineDaysRejectsRemovedAudit(t *testing.T) {
	svc := NewDeadlineDaysService(&fakeStageRepo{}, fakeAuditRepo{status: "REMOVED"})
	_, err := svc.UpdateDeadlineDays(context.Background(), 7, domain.UpdateDeadlineDaysRequest{
		UpdatedBy: "a", Days: []domain.DeadlineDaysRow{{CycleType: "INITIAL", HandledBy: "OWNER", AllowedDays: 5}},
	})
	var ce *apierror.ConflictError
	if !errors.As(err, &ce) || !strings.Contains(ce.Msg, "removed") {
		t.Fatalf("expected a conflict, got %v", err)
	}
}

type fakeCreateRepo struct {
	repository.ControlRepository
	created *domain.CreateControlRequest
}

func (f *fakeCreateRepo) CreateControl(_ context.Context, _ int, req domain.CreateControlRequest) (*domain.AuditControl, error) {
	f.created = &req
	return &domain.AuditControl{ID: 1, RequirementType: req.RequirementType}, nil
}

func newCreateReq(requirementType string) domain.CreateControlRequest {
	ev := "evidence"
	return domain.CreateControlRequest{
		ControlNumber: "CA-1", Description: "d", EvidenceRequirement: &ev, RequirementType: requirementType,
		ControlType: "CONFIG", Scope: "COMMON", CreatedBy: "admin",
	}
}

// 9 Nov 23:30 UTC is already 10 Nov in Sri Lanka.
var createNow = func() time.Time { return time.Date(2026, time.November, 9, 23, 30, 0, 0, time.UTC) }

func TestCreateDesignControlWithoutDueDateUsesOwnerInitialDays(t *testing.T) {
	cases := map[string]struct {
		days deadline.Days
		want string
	}{
		"audit has no saved days: default 30": {nil, "2026-12-10"},
		"audit saved 20 days for the owner":   {deadline.Days{"INITIAL": {"OWNER": 20}}, "2026-11-30"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &fakeCreateRepo{}
			svc := &controlService{repo: repo, stageRepo: &fakeStageRepo{days: tc.days}, now: createNow}
			if _, err := svc.CreateControl(context.Background(), 7, newCreateReq("DESIGN")); err != nil {
				t.Fatal(err)
			}
			if repo.created.DueDate == nil || *repo.created.DueDate != tc.want {
				t.Fatalf("due date = %v, want %s", repo.created.DueDate, tc.want)
			}
		})
	}
}

func TestCreateDesignControlKeepsTheDueDateItWasGiven(t *testing.T) {
	repo := &fakeCreateRepo{}
	svc := &controlService{repo: repo, stageRepo: &fakeStageRepo{}, now: createNow}
	req := newCreateReq("DESIGN")
	given := "2026-12-31"
	req.DueDate = &given
	if _, err := svc.CreateControl(context.Background(), 7, req); err != nil {
		t.Fatal(err)
	}
	if *repo.created.DueDate != given {
		t.Fatalf("due date = %s, want %s", *repo.created.DueDate, given)
	}
}

func TestCreateOEControlStillNeedsADueDate(t *testing.T) {
	svc := &controlService{repo: &fakeCreateRepo{}, stageRepo: &fakeStageRepo{}, now: createNow}
	var ve *apierror.ValidationError
	if _, err := svc.CreateControl(context.Background(), 7, newCreateReq("OE")); !errors.As(err, &ve) {
		t.Fatalf("expected a validation error, got %v", err)
	}
}
