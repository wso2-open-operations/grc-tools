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

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/repository"
)

// stubEscalationRiskSvc and spyEscalationRepo embed their interfaces so only
// the methods EscalateRisk touches need writing; any other call panics on the
// nil embedded value, which is the right failure for a test that shouldn't
// reach it.
type stubEscalationRiskSvc struct {
	RiskService
	risk domain.Risk
}

func (s stubEscalationRiskSvc) GetRiskByID(context.Context, int) (domain.Risk, error) {
	return s.risk, nil
}

type spyEscalationRepo struct {
	repository.RiskEscalationRepository
	got domain.CreateRiskEscalationRequest
}

func (s *spyEscalationRepo) Escalate(_ context.Context, _ int, req domain.CreateRiskEscalationRequest) (*domain.RiskEscalation, error) {
	s.got = req
	return &domain.RiskEscalation{ID: 1, Status: "OPEN"}, nil
}

// The flag on the manual request must reach the repo, which is the only place
// that resolves an answered escalation; dropping it here would silently bring
// the 409 back.
func TestEscalateRisk_PassesReescalateToRepo(t *testing.T) {
	past := "2000-01-01"
	for _, reescalate := range []bool{true, false} {
		repo := &spyEscalationRepo{}
		svc := NewRiskEscalationService(repo, stubEscalationRiskSvc{
			risk: domain.Risk{WorkflowStatus: "APPROVED", ImplementationDate: &past},
		})

		_, err := svc.EscalateRisk(context.Background(), 63, domain.EscalateRiskRequest{
			CreatedBy: "by-1", Reescalate: reescalate,
		})
		if err != nil {
			t.Fatalf("reescalate=%v: EscalateRisk: %v", reescalate, err)
		}
		if repo.got.Reescalate != reescalate {
			t.Errorf("repo got Reescalate=%v, want %v", repo.got.Reescalate, reescalate)
		}
	}
}
