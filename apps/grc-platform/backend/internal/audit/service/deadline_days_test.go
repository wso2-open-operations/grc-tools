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
	"net/http"
	"testing"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
)

type fakeDeadlineDaysRepo struct {
	before *model.DeadlineDays
	put    []model.DeadlineDaysRow
	putBy  string
	getErr error
}

func (f *fakeDeadlineDaysRepo) Get(context.Context, int) (*model.DeadlineDays, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.before, nil
}
func (f *fakeDeadlineDaysRepo) Put(_ context.Context, auditID int, rows []model.DeadlineDaysRow, by string) (*model.DeadlineDays, error) {
	f.put, f.putBy = rows, by
	return &model.DeadlineDays{AuditID: auditID, Phase: "EVIDENCE", Days: rows}, nil
}

type recordingTrail struct {
	TrailService
	action  string
	actor   string
	details map[string]any
	calls   int
}

func (r *recordingTrail) RecordAuditAction(_ context.Context, _ int, action, actor string, details map[string]any) error {
	r.calls++
	r.action, r.actor, r.details = action, actor, details
	return nil
}

func defaultsBefore() *model.DeadlineDays {
	return &model.DeadlineDays{AuditID: 7, Phase: "EVIDENCE", Days: []model.DeadlineDaysRow{
		{CycleType: "INITIAL", HandledBy: "OWNER", AllowedDays: 30, IsDefault: true},
		{CycleType: "INITIAL", HandledBy: "COMPLIANCE", AllowedDays: 10, IsDefault: true},
		{CycleType: "INITIAL", HandledBy: "AUDITOR", AllowedDays: 10, IsDefault: true},
	}}
}

func TestDeadlineDaysUpdateValidation(t *testing.T) {
	row := func(c, h string, n int) model.DeadlineDaysRow {
		return model.DeadlineDaysRow{CycleType: c, HandledBy: h, AllowedDays: n}
	}
	cases := map[string][]model.DeadlineDaysRow{
		"empty":        nil,
		"bad cycle":    {row("FIRST", "OWNER", 5)},
		"bad handler":  {row("INITIAL", "EY", 5)},
		"zero":         {row("INITIAL", "OWNER", 0)},
		"negative":     {row("INITIAL", "OWNER", -3)},
		"over the cap": {row("INITIAL", "OWNER", 366)},
		"listed twice": {row("INITIAL", "OWNER", 5), row("initial", "owner", 6)},
	}
	for name, rows := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &fakeDeadlineDaysRepo{before: defaultsBefore()}
			svc := NewDeadlineDaysService(repo, nil)
			_, err := svc.Update(context.Background(), 7, model.UpdateDeadlineDaysRequest{Days: rows}, "admin")
			var ae *apierror.Error
			if !errors.As(err, &ae) || ae.StatusCode != http.StatusUnprocessableEntity {
				t.Fatalf("expected a 422, got %v", err)
			}
			if repo.put != nil {
				t.Fatal("nothing may be saved when validation fails")
			}
		})
	}
}

func TestDeadlineDaysUpdateSavesAndRecordsOnlyChanges(t *testing.T) {
	repo := &fakeDeadlineDaysRepo{before: defaultsBefore()}
	trail := &recordingTrail{}
	svc := NewDeadlineDaysService(repo, trail)
	_, err := svc.Update(context.Background(), 7, model.UpdateDeadlineDaysRequest{Days: []model.DeadlineDaysRow{
		{CycleType: "initial", HandledBy: "compliance", AllowedDays: 12}, // changed
		{CycleType: "INITIAL", HandledBy: "AUDITOR", AllowedDays: 10},    // same as before
	}}, "admin@wso2.com")
	if err != nil {
		t.Fatal(err)
	}
	if repo.putBy != "admin@wso2.com" || len(repo.put) != 2 || repo.put[0].CycleType != "INITIAL" || repo.put[0].HandledBy != "COMPLIANCE" {
		t.Fatalf("saved %+v by %q", repo.put, repo.putBy)
	}
	if trail.calls != 1 || trail.action != "UPDATED" || trail.actor != "admin@wso2.com" {
		t.Fatalf("trail = %+v", trail)
	}
	changes, _ := trail.details["deadlineDays"].([]map[string]any)
	if len(changes) != 1 || changes[0]["from"] != 10 || changes[0]["to"] != 12 || changes[0]["handledBy"] != "COMPLIANCE" {
		t.Fatalf("only the changed value belongs in the trail, got %+v", changes)
	}
}

func TestDeadlineDaysUpdateWithNoRealChangeWritesNoTrail(t *testing.T) {
	repo := &fakeDeadlineDaysRepo{before: defaultsBefore()}
	trail := &recordingTrail{}
	svc := NewDeadlineDaysService(repo, trail)
	if _, err := svc.Update(context.Background(), 7, model.UpdateDeadlineDaysRequest{Days: []model.DeadlineDaysRow{
		{CycleType: "INITIAL", HandledBy: "OWNER", AllowedDays: 30},
	}}, "admin"); err != nil {
		t.Fatal(err)
	}
	if trail.calls != 0 {
		t.Fatal("saving a value that did not change must not write a trail entry")
	}
}

func TestDeadlineDaysUpdateStopsWhenAuditCannotBeRead(t *testing.T) {
	repo := &fakeDeadlineDaysRepo{getErr: errors.New("boom")}
	svc := NewDeadlineDaysService(repo, nil)
	_, err := svc.Update(context.Background(), 7, model.UpdateDeadlineDaysRequest{Days: []model.DeadlineDaysRow{
		{CycleType: "INITIAL", HandledBy: "OWNER", AllowedDays: 5},
	}}, "admin")
	if err == nil || repo.put != nil {
		t.Fatal("an unreadable audit must not be written to")
	}
}
