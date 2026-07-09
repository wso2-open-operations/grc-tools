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
	"net/url"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
)

type controlRepo struct{ c *entityclient.Client }

// NewControlRepository returns an entity-backed ControlRepository.
func NewControlRepository(c *entityclient.Client) repository.ControlRepository {
	return &controlRepo{c: c}
}

func (r *controlRepo) List(ctx context.Context, auditID int) ([]*model.AuditControl, error) {
	var all []*model.AuditControl
	path := fmt.Sprintf("/audits/%d/controls/search", auditID)
	for offset := 0; ; offset += pageLimit {
		var resp struct {
			Controls []*model.AuditControl `json:"controls"`
		}
		if err := r.c.Post(ctx, path, pageBody(offset), &resp); err != nil {
			return nil, err
		}
		all = append(all, resp.Controls...)
		if len(resp.Controls) < pageLimit {
			return all, nil
		}
	}
}

func (r *controlRepo) GetByID(ctx context.Context, auditID, controlID int) (*model.AuditControl, error) {
	var c model.AuditControl
	if err := r.c.Get(ctx, fmt.Sprintf("/audits/%d/controls/%d", auditID, controlID), &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *controlRepo) Create(ctx context.Context, auditID int, req model.AddControlRequest, createdBy string) (*model.AuditControl, error) {
	var c model.AuditControl
	body := withField(req, map[string]any{"createdBy": createdBy})
	if err := r.c.Post(ctx, fmt.Sprintf("/audits/%d/controls", auditID), body, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *controlRepo) BulkCreate(ctx context.Context, auditID int, reqs []model.AddControlRequest, createdBy string) ([]*model.AuditControl, error) {
	controls := make([]map[string]any, 0, len(reqs))
	for _, req := range reqs {
		controls = append(controls, withField(req, map[string]any{"createdBy": createdBy}))
	}
	var resp struct {
		Controls []*model.AuditControl `json:"controls"`
	}
	if err := r.c.Post(ctx, fmt.Sprintf("/audits/%d/controls/bulk", auditID), map[string]any{"controls": controls}, &resp); err != nil {
		return nil, err
	}
	return resp.Controls, nil
}

func (r *controlRepo) Update(ctx context.Context, auditID, controlID int, req model.UpdateControlRequest, updatedBy string) error {
	body := withField(req, map[string]any{"updatedBy": updatedBy})
	return r.c.Patch(ctx, fmt.Sprintf("/audits/%d/controls/%d", auditID, controlID), body, nil)
}

func (r *controlRepo) UpdateStatus(ctx context.Context, auditID, controlID int, status string, comment *string, updatedBy string) error {
	body := map[string]any{"status": status, "updatedBy": updatedBy}
	return r.c.Patch(ctx, fmt.Sprintf("/audits/%d/controls/%d", auditID, controlID), body, nil)
}

func (r *controlRepo) Delete(ctx context.Context, auditID, controlID int) error {
	return r.c.Delete(ctx, fmt.Sprintf("/audits/%d/controls/%d", auditID, controlID))
}

func (r *controlRepo) ListAssignedForEvidence(ctx context.Context, userEmail string) ([]*model.AssignedControlForEvidence, error) {
	var resp struct {
		Controls []*model.AssignedControlForEvidence `json:"controls"`
	}
	path := "/controls/assigned-for-evidence?email=" + url.QueryEscape(userEmail)
	if err := r.c.Get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return resp.Controls, nil
}
