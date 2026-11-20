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

package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

func TestWidgetQueryRepoRun(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Limit 2 means the statement fetches 3 rows; the third proves truncation.
	q, err := CompileWidgetQuery(domain.WidgetQueryRequest{
		Spec: domain.WidgetSpec{
			Dataset: "risks", Dimensions: []string{"source_register"}, Measures: []string{"risk_count", "avg_gross_rating"}, Limit: 2,
		},
		Visibility: domain.WidgetVisibility{All: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT /\*\+ MAX_EXECUTION_TIME\(5000\) \*/`).
		WillReturnRows(sqlmock.NewRows([]string{"source_register", "risk_count", "avg_gross_rating"}).
			AddRow("Asgardeo", 4, "2.5").
			AddRow(nil, 2, nil).
			AddRow("Choreo", 1, "1"))
	mock.ExpectRollback() // read-only: nothing is ever committed

	res, err := NewWidgetQueryRepository(db).Run(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Truncated || len(res.Rows) != 2 {
		t.Fatalf("truncated = %v, rows = %d", res.Truncated, len(res.Rows))
	}
	if res.Rows[0][0] != "Asgardeo" || res.Rows[0][1] != int64(4) || res.Rows[0][2] != 2.5 {
		t.Fatalf("row 0 = %#v", res.Rows[0])
	}
	if res.Rows[1][0] != nil || res.Rows[1][2] != nil {
		t.Fatalf("NULLs should come back as nil: %#v", res.Rows[1])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
