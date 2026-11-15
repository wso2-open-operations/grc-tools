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

package handler

import (
	"encoding/json"
	"net/http"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/service"
)

// WidgetQueryHandler serves dashboard widget queries.
type WidgetQueryHandler struct{ svc service.WidgetQueryService }

// NewWidgetQueryHandler constructs a WidgetQueryHandler.
func NewWidgetQueryHandler(svc service.WidgetQueryService) *WidgetQueryHandler {
	return &WidgetQueryHandler{svc: svc}
}

// Run handles POST /risk/widgets/query. The visibility in the body is derived
// by the GRC backend from the caller's grants, the same trust as every other
// scope this service is handed.
func (h *WidgetQueryHandler) Run(w http.ResponseWriter, r *http.Request) {
	var req domain.WidgetQueryRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	resp, err := h.svc.Run(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// RunAudit handles POST /audit/widgets/query. The scope in the body is derived
// by the GRC backend from the caller's grants, as for every other audit scope
// this service is handed.
func (h *WidgetQueryHandler) RunAudit(w http.ResponseWriter, r *http.Request) {
	var req domain.AuditWidgetQueryRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	resp, err := h.svc.RunAudit(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
