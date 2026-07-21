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

// TestRiskScoreParity asserts the entity-backed RiskScoreRepository.List
// returns exactly what the MySQL one does.
//
// Order is the point of this test. The entity originally ordered by
// risk_rating, which ties — (1,2) and (2,1) both rate 2 — leaving the sequence
// up to MySQL. It now orders by likelihood then impact like the MySQL query.
// If this fails on ordering after an entity change, fix the entity's ORDER BY
// rather than sorting in the repository, and remember the entity caches this
// response for 30 minutes so it needs a restart to pick the change up.
func TestRiskScoreParity(t *testing.T) {
	skipUnlessParity(t)

	mysqlRepo := riskmysql.NewRiskScoreRepository(parityDB(t))
	entityRepo := NewRiskScoreRepository(parityClient(t))

	ctx := context.Background()

	want, err := mysqlRepo.List(ctx)
	if err != nil {
		t.Fatalf("mysql List: %v", err)
	}
	got, err := entityRepo.List(ctx)
	if err != nil {
		t.Fatalf("entity List: %v", err)
	}

	if len(want) == 0 {
		t.Fatal("MySQL returned 0 scores — the comparison would prove nothing")
	}
	if len(got) != len(want) {
		t.Fatalf("count: mysql %d, entity %d", len(want), len(got))
	}
	for i := range want {
		if !reflect.DeepEqual(want[i], got[i]) {
			t.Errorf("index %d differs:\n  mysql  %+v\n  entity %+v", i, *want[i], *got[i])
		}
	}
	t.Logf("%d scores identical, in the same order", len(want))
}
