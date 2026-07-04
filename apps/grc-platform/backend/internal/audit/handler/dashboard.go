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
	auditservice "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
)

type dashboardHandler struct {
	svc auditservice.DashboardService
}

func (h *dashboardHandler) getDashboard(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())

	f := model.DashboardFilter{}
	if user != nil {
		f.Roles = user.Groups
		f.UserEmail = user.Email
	}

	data, err := h.svc.Get(r.Context(), f)
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "Failed to load dashboard data.")
		return
	}

	response.WriteJSONValue(w, http.StatusOK, data)
}
