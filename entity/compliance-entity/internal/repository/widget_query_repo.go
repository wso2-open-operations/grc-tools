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
	"database/sql"
	"fmt"
	"time"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

// WidgetQueryRepository runs an already-compiled widget query.
type WidgetQueryRepository interface {
	Run(ctx context.Context, q *CompiledWidgetQuery) (*domain.WidgetQueryResult, error)
}

type widgetQueryRepo struct{ db *sql.DB }

// NewWidgetQueryRepository constructs a WidgetQueryRepository.
func NewWidgetQueryRepository(db *sql.DB) WidgetQueryRepository {
	return &widgetQueryRepo{db: db}
}

// Run executes q in a read-only transaction with a hard deadline. The deadline
// sits a little above MAX_EXECUTION_TIME so MySQL normally stops the statement
// first and the context only catches a stuck connection.
func (r *widgetQueryRepo) Run(ctx context.Context, q *CompiledWidgetQuery) (*domain.WidgetQueryResult, error) {
	ctx, cancel := context.WithTimeout(ctx, (widgetMaxExecutionMS+1000)*time.Millisecond)
	defer cancel()

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("widget query: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, q.SQL, q.Args...)
	if err != nil {
		return nil, fmt.Errorf("widget query: %w", err)
	}
	defer rows.Close()

	out := &domain.WidgetQueryResult{Columns: q.Columns, Rows: [][]any{}}
	for rows.Next() {
		targets := make([]any, len(q.Columns))
		for i, c := range q.Columns {
			switch c.Kind {
			case widgetKindInt:
				targets[i] = new(sql.NullInt64)
			case widgetKindNumber:
				targets[i] = new(sql.NullFloat64)
			default:
				targets[i] = new(sql.NullString)
			}
		}
		if err := rows.Scan(targets...); err != nil {
			return nil, fmt.Errorf("widget query: scan: %w", err)
		}
		row := make([]any, len(targets))
		for i, t := range targets {
			row[i] = widgetCell(t)
		}
		out.Rows = append(out.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("widget query: rows: %w", err)
	}

	if len(out.Rows) > q.Limit {
		out.Rows = out.Rows[:q.Limit]
		out.Truncated = true
	}
	return out, nil
}

// widgetCell turns a scanned nullable into a plain JSON value.
func widgetCell(target any) any {
	switch v := target.(type) {
	case *sql.NullInt64:
		if v.Valid {
			return v.Int64
		}
	case *sql.NullFloat64:
		if v.Valid {
			return v.Float64
		}
	case *sql.NullString:
		if v.Valid {
			return v.String
		}
	}
	return nil
}
