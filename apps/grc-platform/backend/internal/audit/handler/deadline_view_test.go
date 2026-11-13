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

package handler

import (
	"strings"
	"testing"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
)

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }

func TestDelayLogText(t *testing.T) {
	got := delayLogText([]model.DelayLogEntry{
		{HandledBy: "OWNER", Date: "2026-11-08"},
		{HandledBy: "COMPLIANCE", Date: "2026-11-21"},
		{HandledBy: "AUDITOR", Date: "2026-12-04"},
	})
	if got != "Owner - 11/08; Compliance - 11/21; Auditor - 12/04" {
		t.Fatalf("got %q", got)
	}
	if delayLogText(nil) != "" {
		t.Fatal("no entries should give an empty cell")
	}
}

func TestMonthDayKeepsOddInputUntouched(t *testing.T) {
	if monthDay("2026-01-05") != "01/05" || monthDay("junk") != "junk" || monthDay("") != "" {
		t.Fatal("monthDay mishandled its input")
	}
}

func TestRedactDelayLog(t *testing.T) {
	log := []model.DelayLogEntry{{HandledBy: "OWNER", Date: "2026-11-08"}}
	controls := []*model.AuditControl{{DelayLog: log}, {DelayLog: log}}

	redactDelayLog(controls, true)
	if controls[0].DelayLog == nil || controls[1].DelayLog == nil {
		t.Fatal("GRC viewers keep the log")
	}
	redactDelayLog(controls, false)
	if controls[0].DelayLog != nil || controls[1].DelayLog != nil {
		t.Fatal("everyone else must not receive the log")
	}
}

func TestExportRecordForTrackedControl(t *testing.T) {
	c := &model.AuditControl{
		ControlNumber:         "CC-2",
		CurrentHandler:        strPtr("AUDITOR"),
		CurrentDueDate:        strPtr("2026-12-03"),
		PlannedOwnerDate:      strPtr("2026-11-07"),
		PlannedComplianceDate: strPtr("2026-11-17"),
		PlannedAuditorDate:    strPtr("2026-11-27"),
		Delayed:               boolPtr(true),
		DelayLog:              []model.DelayLogEntry{{HandledBy: "OWNER", Date: "2026-11-08"}, {HandledBy: "OWNER", Date: "2026-11-09"}},
	}
	rec := controlExportRecord(c)
	if len(rec) != len(controlExportHeader) {
		t.Fatalf("record has %d cells, header has %d", len(rec), len(controlExportHeader))
	}
	tail := strings.Join(rec[len(rec)-7:], "|")
	if tail != "Auditor|2026-12-03|2026-11-07|2026-11-17|2026-11-27|Yes|Owner - 11/08; Owner - 11/09" {
		t.Fatalf("tail = %q", tail)
	}
}

func TestExportRecordForUntrackedControlIsBlank(t *testing.T) {
	rec := controlExportRecord(&model.AuditControl{ControlNumber: "CC-9"})
	if len(rec) != len(controlExportHeader) {
		t.Fatalf("record has %d cells, header has %d", len(rec), len(controlExportHeader))
	}
	for i, cell := range rec[len(rec)-7:] {
		if cell != "" {
			t.Errorf("deadline column %d should be blank for an untracked control, got %q", i, cell)
		}
	}
}
