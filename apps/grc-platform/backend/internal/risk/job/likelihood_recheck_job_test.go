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

package job

import (
	"testing"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
)

func utc(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestQuarterBounds(t *testing.T) {
	cases := []struct {
		name               string
		in                 time.Time
		wantStart, wantEnd time.Time
	}{
		{"Jan is Q1", utc(2026, time.January, 15), utc(2026, time.January, 1), utc(2026, time.April, 1)},
		{"Mar is Q1", utc(2026, time.March, 31), utc(2026, time.January, 1), utc(2026, time.April, 1)},
		{"Apr is Q2", utc(2026, time.April, 1), utc(2026, time.April, 1), utc(2026, time.July, 1)},
		{"Sep is Q3", utc(2026, time.September, 30), utc(2026, time.July, 1), utc(2026, time.October, 1)},
		{"Dec is Q4, rolls into next year", utc(2026, time.December, 25), utc(2026, time.October, 1), utc(2027, time.January, 1)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			start, end := quarterBounds(c.in)
			if !start.Equal(c.wantStart) || !end.Equal(c.wantEnd) {
				t.Errorf("quarterBounds(%v) = (%v, %v), want (%v, %v)", c.in, start, end, c.wantStart, c.wantEnd)
			}
		})
	}
}

func TestInRecheckWindow(t *testing.T) {
	quarterEnd := utc(2026, time.April, 1) // end of Q1 2026 (exclusive)

	cases := []struct {
		name string
		now  time.Time
		want bool
	}{
		{"well before window", utc(2026, time.February, 1), false},
		{"exactly at window start (14 days before end)", utc(2026, time.March, 18), true},
		{"one day before window start", utc(2026, time.March, 17), false},
		{"mid-window", utc(2026, time.March, 25), true},
		{"last day of quarter", utc(2026, time.March, 31), true},
		{"exactly at quarter end (next quarter, excluded)", utc(2026, time.April, 1), false},
		{"well after", utc(2026, time.May, 1), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := inRecheckWindow(c.now, quarterEnd); got != c.want {
				t.Errorf("inRecheckWindow(%v, %v) = %v, want %v", c.now, quarterEnd, got, c.want)
			}
		})
	}
}

func TestBuildSuggestRequest(t *testing.T) {
	t.Run("missing category is not ok", func(t *testing.T) {
		detail := &model.RiskDetail{RiskTitle: "t", RiskDescription: "d"}
		_, ok := buildSuggestRequest(detail)
		if ok {
			t.Error("expected ok=false when no category is set")
		}
	})

	t.Run("full detail maps correctly", func(t *testing.T) {
		impact := "serious impact"
		detail := &model.RiskDetail{
			RiskTitle:            "Outdated library",
			RiskDescription:      "CVE-2024-1234 in libfoo",
			ImpactDescription:    &impact,
			SourceRegisterID:     7,
			RiskCategories:       []model.RiskCategory{{ID: 3, Name: "EOL / Unpatched"}},
			ComplianceReferences: []model.ComplianceReference{{ID: 1}, {ID: 2}},
		}
		req, ok := buildSuggestRequest(detail)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if req.Title != "Outdated library" || req.ImpactDescription != "serious impact" {
			t.Errorf("got %+v", req)
		}
		if req.CategoryID != 3 || req.SourceRegisterID != 7 {
			t.Errorf("got %+v", req)
		}
		if len(req.ComplianceReferenceIDs) != 2 {
			t.Errorf("got %+v", req.ComplianceReferenceIDs)
		}
	})
}
