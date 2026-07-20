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

package handler

import (
	"net/http"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

type aiValidationHandler struct {
	svc service.AIValidationService
}

// listValidations handles GET /api/v1/evidence/{evidenceId}/ai-validations.
//
// Advisory review hints. Visible to anyone permitted to see the evidence —
// submitters (for the pre-review feedback loop) and reviewers (for the hint) —
// so it reuses SUBMIT_EVIDENCE OR REVIEW_EVIDENCE rather than a new privilege.
func (h *aiValidationHandler) listValidations(w http.ResponseWriter, r *http.Request) {
	if !auth.RequireAnyPrivilege(r.Context(), w, privilege.SubmitEvidence, privilege.ReviewEvidence) {
		return
	}
	evidenceID, ok := parseIntParam(w, r, "evidenceId")
	if !ok {
		return
	}
	validations, err := h.svc.ListByEvidence(r.Context(), evidenceID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if validations == nil {
		validations = []*model.AIValidationLog{}
	}
	response.WriteJSONValue(w, http.StatusOK, &model.AIValidationListResponse{Validations: validations})
}
