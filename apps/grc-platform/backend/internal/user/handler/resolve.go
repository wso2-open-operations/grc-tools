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
	"strings"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	userentity "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/user"
)

type resolveUserRequest struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

// handleResolveUser links an HR entity employee (name + email, from
// GET /api/v1/employees/search) to an internal user.id by email, creating
// the user row on the fly if one doesn't exist yet. Used wherever a form
// needs to assign any employee — not just an existing grc-platform
// user — to an FK field (e.g. a risk's Action Owner).
func handleResolveUser(repo userentity.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req resolveUserRequest
		if err := response.DecodeJSON(w, r, &req); err != nil {
			return
		}

		req.Email = strings.TrimSpace(req.Email)
		req.DisplayName = strings.TrimSpace(req.DisplayName)
		if req.Email == "" || req.DisplayName == "" {
			response.WriteError(w, http.StatusBadRequest, "email and display_name are required")
			return
		}

		u, err := repo.Upsert(r.Context(), req.Email, req.DisplayName)
		if err != nil {
			response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
			return
		}
		response.WriteJSONValue(w, http.StatusOK, u)
	}
}
