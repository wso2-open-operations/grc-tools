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
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/file"
)

const maxRiskEvidenceBytes = 25 << 20 // 25 MiB

// EvidenceService defines business operations for risk evidence files.
type EvidenceService interface {
	List(ctx context.Context, riskID int) ([]*model.RiskEvidence, error)
	Upload(ctx context.Context, riskID int, fileName, contentType string, content io.Reader, createdBy string) (*model.RiskEvidence, error)
	Delete(ctx context.Context, riskID, evidenceID int, byUserID string) error
}

type evidenceService struct {
	repo    repository.RiskEvidenceRepository
	storage *file.Service
}

func NewEvidenceService(repo repository.RiskEvidenceRepository, storage *file.Service) EvidenceService {
	return &evidenceService{repo: repo, storage: storage}
}

func (s *evidenceService) List(ctx context.Context, riskID int) ([]*model.RiskEvidence, error) {
	return s.repo.List(ctx, riskID)
}

func (s *evidenceService) Upload(ctx context.Context, riskID int, fileName, contentType string, content io.Reader, createdBy string) (*model.RiskEvidence, error) {
	data, err := io.ReadAll(io.LimitReader(content, maxRiskEvidenceBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxRiskEvidenceBytes {
		return nil, &apierror.Error{StatusCode: http.StatusRequestEntityTooLarge, Body: "file exceeds 25 MB limit"}
	}
	// Store under a per-risk evidence folder; the Compliance Entity writes to Azure
	// (the backend never talks to Azure directly). The stored file_path is the
	// relative blob name, downloaded later by proxy through the entity.
	blobName := fmt.Sprintf("risks/%d/evidence/%d/%s", riskID, time.Now().UnixNano(), fileName)
	if err := s.storage.UploadBlob(ctx, blobName, contentType, data); err != nil {
		return nil, err
	}
	// evidence_type defaults to ACTION_PLAN_ATTACHMENT for uploaded attachments.
	ev, err := s.repo.Create(ctx, riskID, fileName, blobName, "", "ACTION_PLAN_ATTACHMENT", createdBy)
	if err != nil {
		// Best-effort blob cleanup so the orphaned file doesn't linger in Azure.
		_ = s.storage.Delete(ctx, blobName)
		return nil, err
	}
	return ev, nil
}

func (s *evidenceService) Delete(ctx context.Context, riskID, evidenceID int, byUserID string) error {
	return s.repo.Delete(ctx, riskID, evidenceID, byUserID)
}
