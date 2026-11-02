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

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/repository"
)

type spyEscalationRepo struct {
	repository.EscalationRepository
	reescalate []bool
}

func (s *spyEscalationRepo) Escalate(_ context.Context, _ int, _ string, _, _ *string, reescalate bool) (*model.Escalation, error) {
	s.reescalate = append(s.reescalate, reescalate)
	return &model.Escalation{ID: 1}, nil
}

// Escalate is what the daily job calls and must never ask the entity to resolve
// an answered escalation, or it would re-escalate and re-email every commented
// but still-overdue risk on each sweep. Reescalate is the manual click's path.
func TestEscalate_OnlyReescalateSetsTheFlag(t *testing.T) {
	repo := &spyEscalationRepo{}
	// hr is nil, so resolveLeads returns early and no other dependency is touched.
	svc := &escalationService{repo: repo}

	if _, err := svc.Escalate(context.Background(), 63, "job"); err != nil {
		t.Fatalf("Escalate: %v", err)
	}
	if _, err := svc.Reescalate(context.Background(), 63, "user"); err != nil {
		t.Fatalf("Reescalate: %v", err)
	}

	if len(repo.reescalate) != 2 || repo.reescalate[0] || !repo.reescalate[1] {
		t.Errorf("reescalate flags = %v, want [false true]", repo.reescalate)
	}
}
