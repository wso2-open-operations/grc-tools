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

package entity

import (
	"context"
	"reflect"
	"testing"

	riskmysql "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/repository/mysql"
)

// TestAssessmentParity compares ListByRiskID across both implementations for
// every risk that actually has assessments.
//
// The entity originally returned a bare scoreId with no risk_score join, so the
// residual likelihood/impact/rating/level/colour came back zeroed — this test
// is what holds that fix in place. It also pins the reassessment_date
// rendering: MySQL emits RFC3339 ("2026-08-31T00:00:00Z") because parseTime is
// on, while the entity returns "2026-08-31", and the repository converts.
//
// Create is deliberately not covered: it writes, and the risk IDs it would
// dirty are the same ones this test reads. Verify Create through the UI or a
// throwaway risk instead.
func TestAssessmentParity(t *testing.T) {
	skipUnlessParity(t)

	db := parityDB(t)
	mysqlRepo := riskmysql.NewAssessmentRepository(db)
	entityRepo := NewAssessmentRepository(parityClient(t))

	ctx := context.Background()

	rows, err := db.QueryContext(ctx, "SELECT DISTINCT risk_id FROM risk_assessment ORDER BY risk_id")
	if err != nil {
		t.Fatalf("find risks with assessments: %v", err)
	}
	defer rows.Close()

	var riskIDs []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan risk id: %v", err)
		}
		riskIDs = append(riskIDs, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate risk ids: %v", err)
	}
	if len(riskIDs) == 0 {
		t.Fatal("no risk has any assessment — the comparison would prove nothing")
	}

	total := 0
	for _, riskID := range riskIDs {
		want, err := mysqlRepo.ListByRiskID(ctx, riskID)
		if err != nil {
			t.Fatalf("risk %d: mysql ListByRiskID: %v", riskID, err)
		}
		got, err := entityRepo.ListByRiskID(ctx, riskID)
		if err != nil {
			t.Fatalf("risk %d: entity ListByRiskID: %v", riskID, err)
		}

		if len(got) != len(want) {
			t.Errorf("risk %d count: mysql %d, entity %d", riskID, len(want), len(got))
			continue
		}
		for i := range want {
			if !reflect.DeepEqual(want[i], got[i]) {
				t.Errorf("risk %d index %d differs:\n  mysql  %+v\n  entity %+v",
					riskID, i, want[i], got[i])
			}
		}
		total += len(want)
	}

	if !t.Failed() {
		t.Logf("%d assessments across %d risks identical", total, len(riskIDs))
	}
}
