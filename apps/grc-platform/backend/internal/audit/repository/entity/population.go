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

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
)

type populationRepo struct{ c *entityclient.Client }

// NewPopulationRepository returns an entity-backed PopulationRepository.
func NewPopulationRepository(c *entityclient.Client) repository.PopulationRepository {
	return &populationRepo{c: c}
}

func (r *populationRepo) AddFile(ctx context.Context, populationID int, fileKind, fileName, filePath string, fileType *string, fileSize *int64, createdBy string) error {
	body := map[string]any{
		"fileKind":  fileKind,
		"fileName":  fileName,
		"filePath":  filePath,
		"fileType":  fileType,
		"fileSize":  fileSize,
		"createdBy": createdBy,
	}
	return r.c.Post(ctx, fmt.Sprintf("/populations/%d/files", populationID), body, nil)
}

func (r *populationRepo) UpdateStatus(ctx context.Context, populationID int, status, updatedBy string) error {
	body := map[string]any{"status": status, "updatedBy": updatedBy}
	return r.c.Patch(ctx, fmt.Sprintf("/populations/%d", populationID), body, nil)
}
