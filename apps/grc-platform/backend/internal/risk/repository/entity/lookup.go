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

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
)

// lookupRepository serves one register-template lookup through the entity's
// /risk/{platforms|customers|products|deployment-types} routes. The four
// share one shape, so one implementation is configured with its path.
type lookupRepository struct {
	c    *entityclient.Client
	path string // e.g. "/risk/customers"
}

// NewLookupRepository creates a Compliance Entity-backed repository.LookupRepository
// for the lookup served at path.
func NewLookupRepository(c *entityclient.Client, path string) repository.LookupRepository {
	return &lookupRepository{c: c, path: path}
}

type entLookup struct {
	ID       int     `json:"id"`
	Name     string  `json:"name"`
	Code     *string `json:"code"`
	ParentID *int    `json:"parentId"`
	Status   string  `json:"status"`
	InUse    bool    `json:"inUse"`
}

func (e entLookup) toModel() *model.Lookup {
	return &model.Lookup{ID: e.ID, Name: e.Name, Code: e.Code, ParentID: e.ParentID, Status: e.Status, InUse: e.InUse}
}

func (r *lookupRepository) List(ctx context.Context, status string) ([]*model.Lookup, error) {
	path := r.path
	if status != "" {
		path += "?status=" + url.QueryEscape(status)
	}
	var resp struct {
		Values []entLookup `json:"values"`
	}
	if err := r.c.Get(ctx, path, &resp); err != nil {
		return nil, fmt.Errorf("list %s: %w", r.path, err)
	}
	out := make([]*model.Lookup, 0, len(resp.Values))
	for _, v := range resp.Values {
		out = append(out, v.toModel())
	}
	return out, nil
}

func (r *lookupRepository) Create(ctx context.Context, req model.CreateLookupRequest, createdBy string) (*model.Lookup, error) {
	body := map[string]any{"name": req.Name, "createdBy": createdBy}
	if req.Code != nil {
		body["code"] = *req.Code
	}
	if req.ParentID != nil {
		body["parentId"] = *req.ParentID
	}
	var v entLookup
	if err := r.c.Post(ctx, r.path, body, &v); err != nil {
		return nil, fmt.Errorf("create %s: %w", r.path, err)
	}
	return v.toModel(), nil
}

func (r *lookupRepository) Update(ctx context.Context, id int, req model.UpdateLookupRequest, updatedBy string) (*model.Lookup, error) {
	body := map[string]any{"updatedBy": updatedBy}
	if req.Name != nil {
		body["name"] = *req.Name
	}
	if req.Code != nil {
		body["code"] = *req.Code
	}
	if req.Status != nil {
		body["status"] = *req.Status
	}
	var v entLookup
	if err := r.c.Patch(ctx, fmt.Sprintf("%s/%d", r.path, id), body, &v); err != nil {
		return nil, fmt.Errorf("update %s/%d: %w", r.path, id, err)
	}
	return v.toModel(), nil
}

func (r *lookupRepository) Delete(ctx context.Context, id int) error {
	if err := r.c.Delete(ctx, fmt.Sprintf("%s/%d", r.path, id)); err != nil {
		return fmt.Errorf("delete %s/%d: %w", r.path, id, err)
	}
	return nil
}
