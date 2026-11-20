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

package entity

import (
	"context"
	"fmt"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/dashboardagent"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
)

// WidgetQueryRepository runs a validated dashboard widget spec in the
// Compliance Entity, limited to the risks the caller may see.
type WidgetQueryRepository struct{ c *entityclient.Client }

// NewWidgetQueryRepository creates a Compliance Entity-backed widget query.
func NewWidgetQueryRepository(c *entityclient.Client) *WidgetQueryRepository {
	return &WidgetQueryRepository{c: c}
}

// Run posts the spec and the caller's visibility to POST /risk/widgets/query.
// The spec and visibility types use the same keys the entity reads, so they go
// across as they are.
func (r *WidgetQueryRepository) Run(ctx context.Context, spec dashboardagent.Spec, vis dashboardagent.Visibility) (*dashboardagent.Result, error) {
	body := map[string]any{"spec": spec, "visibility": vis}
	var out dashboardagent.Result
	if err := r.c.Post(ctx, "/risk/widgets/query", body, &out); err != nil {
		return nil, fmt.Errorf("risk widget query: %w", err)
	}
	if out.Columns == nil {
		out.Columns = []dashboardagent.Column{}
	}
	if out.Rows == nil {
		out.Rows = [][]any{}
	}
	return &out, nil
}
