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
	"fmt"
	"reflect"
	"testing"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	riskmysql "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/repository/mysql"
)

// TestTeamParity asserts the entity-backed TeamRepository.List returns exactly
// what the MySQL one does, for every filter the handler can produce.
func TestTeamParity(t *testing.T) {
	skipUnlessParity(t)

	mysqlRepo := riskmysql.NewTeamRepository(parityDB(t))
	entityRepo := NewTeamRepository(parityClient(t))

	// "" is what GET /api/v1/teams sends with no type param; the other two are
	// the only values model.ListTeamsFilter documents.
	for _, filterType := range []string{"", "SOURCE_REGISTER", "ASSIGNMENT"} {
		name := filterType
		if name == "" {
			name = "no-filter"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			filter := model.ListTeamsFilter{Type: filterType}

			want, err := mysqlRepo.List(ctx, filter)
			if err != nil {
				t.Fatalf("mysql List: %v", err)
			}
			got, err := entityRepo.List(ctx, filter)
			if err != nil {
				t.Fatalf("entity List: %v", err)
			}

			if len(want) == 0 {
				t.Fatal("MySQL returned 0 teams — the comparison would prove nothing")
			}
			if len(got) != len(want) {
				t.Fatalf("count: mysql %d, entity %d", len(want), len(got))
			}
			// Order matters: both sides ORDER BY name and the handler passes the
			// slice straight through, so a reordering is a user-visible change.
			// DeepEqual, not ==: model.Team holds *string fields, and == would
			// compare pointer addresses rather than the strings behind them.
			for i := range want {
				if !reflect.DeepEqual(want[i], got[i]) {
					t.Errorf("index %d differs:\n  mysql  %s\n  entity %s",
						i, fmtTeam(want[i]), fmtTeam(got[i]))
				}
			}
			if !t.Failed() {
				t.Logf("%d teams identical", len(want))
			}
		})
	}
}

// fmtTeam renders a team with its pointer fields dereferenced, so a failure
// shows the values that differ rather than their addresses.
func fmtTeam(t *model.Team) string {
	return fmt.Sprintf("{ID:%d Name:%q Code:%s Description:%q TeamType:%s Status:%s}",
		t.ID, t.Name, derefString(t.Code), derefString(t.Description), t.TeamType, t.Status)
}
