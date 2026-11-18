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
	days    deadline.PhaseDays
	listErr error
	put     []domain.DeadlineDaysRow
	opts    repository.DeadlineDaysOptions
	recalc  domain.RecalcSummary
	listed  int
}

func (f *fakeStageRepo) ListStagesByControls(_ context.Context, ids []int) (map[int][]deadline.Stage, error) {
	f.listed++
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.stages, nil
}
func (f *fakeStageRepo) GetDeadlineDays(context.Context, int) (deadline.PhaseDays, error) {
	if f.days == nil {
		return deadline.PhaseDays{}, nil
	}
	return f.days, nil
}
func (f *fakeStageRepo) PutDeadlineDays(_ context.Context, _ int, rows []domain.DeadlineDaysRow, opts repository.DeadlineDaysOptions) (domain.RecalcSummary, error) {
	f.put, f.opts = rows, opts
	if opts.DryRun {
		return f.recalc, nil // nothing is saved
	}
	for _, r := range rows {
		if f.days == nil {
			f.days = deadline.PhaseDays{}
		}
		ph := r.Phase
		if ph == "" {
			ph = deadline.PhaseEvidence
		}
		f.days.Set(ph, r.CycleType, r.HandledBy, r.AllowedDays)
	}
	return f.recalc, nil
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
		{ID: 3, RequirementType: "OE"},     // no stages yet either
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
	if c.Delayed == nil || *c.Delayed {
		t.Fatal("EY holds the control inside its own deadline (3 Dec), so the flag is reset")
	}
	if len(c.DelayLog) != 6 || c.DelayLog[0].HandledBy != "OWNER" || c.DelayLog[0].Date != "2026-11-08" || c.DelayLog[5].Date != "2026-11-23" {
		t.Fatalf("delay log wrong: %+v", c.DelayLog)
	}
	if controls[1].DeadlineTracked || controls[1].Delayed != nil {
		t.Fatal("a control with no stages is left untouched")
	}
	if controls[2].DeadlineTracked {
		t.Fatal("an OE control with no stages is left untouched")
	}
}

func TestEnrichDeadlinesQueriesOEControlsToo(t *testing.T) {
	repo := &fakeStageRepo{}
	svc := &controlService{stageRepo: repo, now: time.Now}
	_ = svc.enrichDeadlines(context.Background(), []domain.AuditControl{{ID: 1, RequirementType: "OE"}})
	if repo.listed != 1 {
		t.Fatal("OE controls have stages to load")
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
	if len(resp.Days) != 18 || resp.Phase != "EVIDENCE" {
		t.Fatalf("got %+v", resp)
	}
	// Each phase lists its own defaults: Design evidence, OE population, OE evidence.
	want := map[string]int{"EVIDENCE": 30, "POPULATION": 4, "OE_EVIDENCE": 5}
	for _, d := range resp.Days {
		if d.CycleType == "INITIAL" && d.HandledBy == "OWNER" && d.AllowedDays != want[d.Phase] {
			t.Errorf("%s initial owner = %d, want %d", d.Phase, d.AllowedDays, want[d.Phase])
		}
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
		isEdited := d.Phase == "EVIDENCE" && d.CycleType == "INITIAL" && d.HandledBy == "COMPLIANCE"
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
		"bad phase":         {UpdatedBy: "a", Days: []domain.DeadlineDaysRow{{Phase: "SAMPLE", CycleType: "INITIAL", HandledBy: "OWNER", AllowedDays: 5}}},
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

func TestCreateControlRequiresTheUltimateDeadline(t *testing.T) {
	for _, rt := range []string{"DESIGN", "OE"} {
		t.Run(rt, func(t *testing.T) {
			svc := &controlService{repo: &fakeCreateRepo{}, stageRepo: &fakeStageRepo{}, now: createNow}
			var ve *apierror.ValidationError
			if _, err := svc.CreateControl(context.Background(), 7, newCreateReq(rt)); !errors.As(err, &ve) || ve.Msg != "dueDate is required" {
				t.Fatalf("expected the dueDate validation error, got %v", err)
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

func TestApplyUltimate(t *testing.T) {
	now := dayAt(time.December, 5, 9)
	end := func(m time.Month, d int) *time.Time { v := dayAt(m, d, 15); return &v }
	closed := func(e *time.Time) []deadline.Stage { return []deadline.Stage{{EndedAt: e}} }
	due := func(s string) *string { return &s }
	cases := map[string]struct {
		c      domain.AuditControl
		stages []deadline.Stage
		want   *bool
	}{
		"open, before the ultimate date":     {domain.AuditControl{DueDate: due("2026-12-20"), Status: "EVIDENCE_PENDING"}, nil, boolPtr(false)},
		"open, on the ultimate date":         {domain.AuditControl{DueDate: due("2026-12-05"), Status: "EVIDENCE_PENDING"}, nil, boolPtr(false)},
		"open, past the ultimate date":       {domain.AuditControl{DueDate: due("2026-12-01"), Status: "EVIDENCE_PENDING"}, nil, boolPtr(true)},
		"completed on time":                  {domain.AuditControl{DueDate: due("2026-12-01"), Status: "COMPLETE"}, closed(end(time.December, 1)), boolPtr(false)},
		"completed late stays delayed":       {domain.AuditControl{DueDate: due("2026-12-01"), Status: "COMPLETE"}, closed(end(time.December, 3)), boolPtr(true)},
		"completed before tracking: unknown": {domain.AuditControl{DueDate: due("2026-12-01"), Status: "COMPLETE"}, nil, nil},
		"no due date: unknown":               {domain.AuditControl{Status: "EVIDENCE_PENDING"}, nil, nil},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := tc.c
			applyUltimate(&c, tc.stages, now)
			if (c.UltimateDelayed == nil) != (tc.want == nil) || (tc.want != nil && *c.UltimateDelayed != *tc.want) {
				t.Fatalf("ultimateDelayed = %v, want %v", c.UltimateDelayed, tc.want)
			}
		})
	}
}

func boolPtr(b bool) *bool { return &b }

// The same cycle and group in different phases are different settings.
func TestUpdateDeadlineDaysSavesPerPhase(t *testing.T) {
	repo := &fakeStageRepo{}
	svc := NewDeadlineDaysService(repo, fakeAuditRepo{status: "ACTIVE"})
	resp, err := svc.UpdateDeadlineDays(context.Background(), 7, domain.UpdateDeadlineDaysRequest{
		UpdatedBy: "admin@wso2.com",
		Days: []domain.DeadlineDaysRow{
			{Phase: "population", CycleType: "INITIAL", HandledBy: "AUDITOR", AllowedDays: 6},
			{Phase: "OE_EVIDENCE", CycleType: "INITIAL", HandledBy: "AUDITOR", AllowedDays: 3},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, d := range resp.Days {
		if d.CycleType == "INITIAL" && d.HandledBy == "AUDITOR" {
			got[d.Phase] = d.AllowedDays
		}
	}
	if got["POPULATION"] != 6 || got["OE_EVIDENCE"] != 3 || got["EVIDENCE"] != 10 {
		t.Fatalf("got %v", got)
	}
}

func TestUpdateDeadlineDaysAppliesToOpenStagesOnRequest(t *testing.T) {
	repo := &fakeStageRepo{recalc: domain.RecalcSummary{StagesUpdated: 12, NewlyOverdue: 3}}
	svc := NewDeadlineDaysService(repo, fakeAuditRepo{status: "ACTIVE"})
	row := []domain.DeadlineDaysRow{{CycleType: "INITIAL", HandledBy: "COMPLIANCE", AllowedDays: 15}}

	plain, err := svc.UpdateDeadlineDays(context.Background(), 7, domain.UpdateDeadlineDaysRequest{UpdatedBy: "a", Days: row})
	if err != nil {
		t.Fatal(err)
	}
	if repo.opts.ApplyToOpenStages || plain.Recalculated != nil {
		t.Fatal("without the option nothing is recalculated and no summary is returned")
	}

	applied, err := svc.UpdateDeadlineDays(context.Background(), 7, domain.UpdateDeadlineDaysRequest{UpdatedBy: "a", Days: row, ApplyToOpenStages: true})
	if err != nil {
		t.Fatal(err)
	}
	if !repo.opts.ApplyToOpenStages || applied.Recalculated == nil || applied.Recalculated.StagesUpdated != 12 ||
		applied.Recalculated.NewlyOverdue != 3 || applied.DryRun {
		t.Fatalf("got %+v opts=%+v", applied, repo.opts)
	}
}

func TestUpdateDeadlineDaysDryRunSavesNothing(t *testing.T) {
	repo := &fakeStageRepo{recalc: domain.RecalcSummary{StagesUpdated: 4, NewlyOverdue: 1}}
	svc := NewDeadlineDaysService(repo, fakeAuditRepo{status: "ACTIVE"})
	resp, err := svc.UpdateDeadlineDays(context.Background(), 7, domain.UpdateDeadlineDaysRequest{
		UpdatedBy: "a", DryRun: true,
		Days: []domain.DeadlineDaysRow{{CycleType: "INITIAL", HandledBy: "COMPLIANCE", AllowedDays: 3}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.DryRun || resp.Recalculated == nil || resp.Recalculated.StagesUpdated != 4 || resp.Recalculated.NewlyOverdue != 1 {
		t.Fatalf("got %+v", resp)
	}
	for _, d := range resp.Days {
		if d.CycleType == "INITIAL" && d.HandledBy == "COMPLIANCE" && d.Phase == "EVIDENCE" && d.AllowedDays != 10 {
			t.Fatalf("a dry run must leave the saved days alone, got %d", d.AllowedDays)
		}
	}
}
