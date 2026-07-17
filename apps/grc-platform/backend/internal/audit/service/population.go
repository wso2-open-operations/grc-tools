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
	"net/http"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/file"
)

// PopulationService defines the OE-control population submission flow used by the
// Evidence Portal. File uploads reuse EvidenceService.UploadFile (phase-agnostic
// blob write); this service records the uploaded blobs and advances the round.
type PopulationService interface {
	// SubmitPopulation records every blob at folderPath as a POPULATION file on the
	// population round and advances it to SUBMITTED. The caller (handler) advances
	// the control to POPULATION_INTERNAL_REVIEW afterwards.
	SubmitPopulation(ctx context.Context, controlID, populationID int, folderPath, submittedBy string) (*model.PopulationSubmitResult, error)
}

type populationService struct {
	repo    repository.PopulationRepository
	storage *file.Service
}

// NewPopulationService constructs a PopulationService.
func NewPopulationService(repo repository.PopulationRepository, storage *file.Service) PopulationService {
	return &populationService{repo: repo, storage: storage}
}

func (s *populationService) SubmitPopulation(ctx context.Context, controlID, populationID int, folderPath, submittedBy string) (*model.PopulationSubmitResult, error) {
	blobs, err := s.storage.ListBlobs(ctx, folderPath)
	if err != nil {
		return nil, err
	}
	if len(blobs) == 0 {
		return nil, &apierror.Error{
			StatusCode: http.StatusUnprocessableEntity,
			Body:       "no files found at the specified folderPath — upload files first",
		}
	}

	for _, blob := range blobs {
		ct := blob.ContentType
		sz := blob.Size
		if err := s.repo.AddFile(ctx, populationID, "POPULATION", blob.FileName(), blob.Name, &ct, &sz, submittedBy); err != nil {
			return nil, err
		}
	}

	if err := s.repo.UpdateStatus(ctx, populationID, "SUBMITTED", submittedBy); err != nil {
		return nil, err
	}

	return &model.PopulationSubmitResult{
		PopulationID: populationID,
		ControlID:    controlID,
		Status:       "SUBMITTED",
		FolderPath:   folderPath,
		FileCount:    len(blobs),
	}, nil
}
