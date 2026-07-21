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

// TestComplianceReferenceParity asserts the entity-backed
// ComplianceReferenceRepository.List returns exactly what the MySQL one does.
func TestComplianceReferenceParity(t *testing.T) {
	skipUnlessParity(t)

	mysqlRepo := riskmysql.NewComplianceReferenceRepository(parityDB(t))
	entityRepo := NewComplianceReferenceRepository(parityClient(t))

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
		t.Fatal("MySQL returned 0 references — the comparison would prove nothing")
	}
	if len(got) != len(want) {
		t.Fatalf("count: mysql %d, entity %d", len(want), len(got))
	}
	// Both sides ORDER BY name and the handler passes the slice straight
	// through, so a reordering would be user-visible. DeepEqual, not ==:
	// Description is a *string.
	for i := range want {
		if !reflect.DeepEqual(want[i], got[i]) {
			t.Errorf("index %d differs:\n  mysql  %s\n  entity %s",
				i, fmtRef(want[i]), fmtRef(got[i]))
		}
	}
	if !t.Failed() {
		t.Logf("%d compliance references identical", len(want))
	}
}

// fmtRef renders a reference with its pointer field dereferenced.
func fmtRef(r *model.ComplianceReference) string {
	return fmt.Sprintf("{ID:%d Name:%q Description:%q}", r.ID, r.Name, derefString(r.Description))
}
