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

// Package handler contains small, cross-module endpoints that answer with a
// single piece of static configuration rather than any module's own data.
package handler

import (
	"net/http"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/routeguard"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// Deps holds dependencies for the meta handlers.
type Deps struct {
	// UserGuideURL is GRC_USER_GUIDE_URL (config.Config.UserGuideURL). May be
	// "", meaning not configured — the caller (One WSO2's Security and
	// Compliance rail) hides its "User Guide" row rather than treating an
	// empty string as an error.
	UserGuideURL string
}

type userGuideURLResponse struct {
	URL string `json:"url"`
}

// RegisterRoutes mounts the meta routes onto mux.
func RegisterRoutes(mux routeguard.Router, deps Deps) {
	mux.HandleFunc("GET /api/v1/meta/user-guide-url", func(w http.ResponseWriter, r *http.Request) {
		url := deps.UserGuideURL
		if !holdsAnyPrivilege(r) {
			url = ""
		}
		response.WriteJSONValue(w, http.StatusOK, userGuideURLResponse{URL: url})
	})
}

// holdsAnyPrivilege reports whether the caller has access to at least one
// GRC Platform privilege. A caller with none has nothing in the platform, so
// the guide is withheld (empty URL, which the rail already treats as "hide
// the row"). Local dev allow-all mode counts as access.
func holdsAnyPrivilege(r *http.Request) bool {
	if auth.AllowAll(r.Context()) {
		return true
	}
	for _, held := range privilege.FromContext(r.Context()) {
		if held {
			return true
		}
	}
	return false
}
