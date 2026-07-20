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

package middleware

import (
	"net/http"
	"strings"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/config"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
)

// evidenceAppPathPrefix is the only route group evidence-app-scoped tokens may reach.
const evidenceAppPathPrefix = "/api/v1/evidence-app/"

// IssuerScope confines evidence-app-scoped tokens (IdP-2, the Evidence Portal) to
// the /api/v1/evidence-app/* route group. The rest of the GRC API is unreachable
// from the portal even if privileges were somehow misconfigured. Must run after
// Auth (it reads the UserInfo the auth middleware placed in the context).
//
// Full-scope tokens (IdP-1) and local-dev tokens (empty scope) pass through.
func IssuerScope(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info := UserInfoFromContext(r.Context())
		if info != nil && info.Scope == config.ScopeEvidenceApp &&
			!strings.HasPrefix(r.URL.Path, evidenceAppPathPrefix) {
			response.WriteError(w, http.StatusForbidden, response.ErrMsgForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
