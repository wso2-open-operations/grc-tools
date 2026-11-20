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
	"testing"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/repository"
)

type spyWidgetRepo struct {
	calls int
	last  *repository.CompiledWidgetQuery
}

func (s *spyWidgetRepo) Run(_ context.Context, q *repository.CompiledWidgetQuery) (*domain.WidgetQueryResult, error) {
	s.calls++
	s.last = q
	return &domain.WidgetQueryResult{Columns: q.Columns, Rows: [][]any{}}, nil
}

func TestWidgetQueryServiceNeverQueriesAnInvalidSpec(t *testing.T) {
	repo := &spyWidgetRepo{}
	svc := NewWidgetQueryService(repo)

	_, err := svc.Run(context.Background(), domain.WidgetQueryRequest{
		Spec:       domain.WidgetSpec{Dataset: "users", Measures: []string{"risk_count"}},
		Visibility: domain.WidgetVisibility{All: true},
	})
	if _, ok := err.(*apierror.ValidationError); !ok {
		t.Fatalf("expected a validation error, got %v", err)
	}
	if repo.calls != 0 {
		t.Fatal("the database must not be reached for an invalid spec")
	}
}

func TestWidgetQueryServiceRunsAValidSpec(t *testing.T) {
	repo := &spyWidgetRepo{}
	svc := NewWidgetQueryService(repo)

	res, err := svc.Run(context.Background(), domain.WidgetQueryRequest{
		Spec:       domain.WidgetSpec{Dataset: "risks", Measures: []string{"risk_count"}},
		Visibility: domain.WidgetVisibility{},
	})
	if err != nil || res == nil {
		t.Fatalf("res = %v, err = %v", res, err)
	}
	if repo.calls != 1 || repo.last.Args == nil {
		t.Fatalf("calls = %d", repo.calls)
	}
}
