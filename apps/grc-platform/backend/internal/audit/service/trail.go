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

package service

import (
	"context"
	"encoding/json"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
)

// TrailService defines business operations for the audit trail (append-only log).
type TrailService interface {
	// TODO: define List method once the audit trail model type is added to audit/model/
	List(ctx context.Context, auditID int) (any, error)
	// RecordEvidenceAction appends an attribution entry for an evidence/population
	// action, tagging the channel it came through (web-app vs evidence-app) and the
	// token issuer so portal actions stay distinguishable (design §I). evidenceID may
	// be 0 (population submit) — it is then omitted.
	RecordEvidenceAction(ctx context.Context, auditID, controlID, evidenceID int, action, actor, via, issuer string) error
}

type trailService struct {
	repo repository.TrailRepository
}

func NewTrailService(repo repository.TrailRepository) TrailService {
	return &trailService{repo: repo}
}

func (s *trailService) List(ctx context.Context, auditID int) (any, error) {
	// TODO: delegate to repo; trail is append-only, never update or delete
	return nil, nil
}

func (s *trailService) RecordEvidenceAction(ctx context.Context, auditID, controlID, evidenceID int, action, actor, via, issuer string) error {
	details, err := json.Marshal(map[string]string{"via": via, "issuer": issuer})
	if err != nil {
		return err
	}
	var evidencePtr *int
	if evidenceID > 0 {
		evidencePtr = &evidenceID
	}
	ctrl := controlID
	return s.repo.Create(ctx, auditID, &ctrl, evidencePtr, action, string(details), actor)
}
