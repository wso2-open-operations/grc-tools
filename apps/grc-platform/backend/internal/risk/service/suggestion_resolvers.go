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

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/repository"
)

// resolveCategoryName looks up a category's display name by id, for building
// an AI Gateway prompt — shared by LikelihoodSuggestionService and
// ActionPlanSuggestionService (CategorySuggestionService doesn't need it; it
// sends the full live category list instead). Returns "" (not an error) for
// id == 0 or an id that no longer resolves, so a stale/missing category never
// blocks a suggestion request — the gateway prompt just omits that line.
func resolveCategoryName(ctx context.Context, categoryRepo repository.RiskCategoryRepository, id int) (string, error) {
	if id == 0 {
		return "", nil
	}
	cats, err := categoryRepo.List(ctx)
	if err != nil {
		return "", err
	}
	for _, c := range cats {
		if c.ID == id {
			return c.Name, nil
		}
	}
	return "", nil
}

// resolveComplianceReferenceNames looks up display names for a set of
// compliance reference ids, for building an AI Gateway prompt — shared by
// CategorySuggestionService, LikelihoodSuggestionService, and
// ActionPlanSuggestionService.
func resolveComplianceReferenceNames(ctx context.Context, complianceRepo repository.ComplianceReferenceRepository, ids []int) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	refs, err := complianceRepo.List(ctx)
	if err != nil {
		return nil, err
	}
	want := make(map[int]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	names := make([]string, 0, len(ids))
	for _, r := range refs {
		if want[r.ID] {
			names = append(names, r.Name)
		}
	}
	return names, nil
}
