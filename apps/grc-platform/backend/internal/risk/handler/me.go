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
	"sort"

	"github.com/wso2-open-operations/grc-platform/backend/internal/response"
	sharedauth "github.com/wso2-open-operations/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-platform/backend/internal/shared/privilege"
)

type privilegesResponse struct {
	Privileges []string `json:"privileges"`
}

// handleGetMyPrivileges serves GET /api/v1/me/privileges.
// Returns the resolved privilege list for the authenticated user (union of all
// their roles' privileges via the role_privilege DB table).
// In production (TokenValidatorEnabled=true) the privilege store is always loaded,
// so privs is never nil here. An empty list means the user has no assigned roles.
func (d *Deps) handleGetMyPrivileges(w http.ResponseWriter, r *http.Request) {
	info := sharedauth.FromContext(r.Context())
	if info == nil {
		response.WriteError(w, http.StatusUnauthorized, response.ErrMsgUnauthorized)
		return
	}

	privs := privilege.FromContext(r.Context())
	names := make([]string, 0, len(privs))
	for p := range privs {
		names = append(names, p)
	}
	sort.Strings(names)

	response.WriteJSONValue(w, http.StatusOK, privilegesResponse{Privileges: names})
}
