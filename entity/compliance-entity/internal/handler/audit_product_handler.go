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
	"strconv"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/service"
)

// AuditProductHandler handles /audit/products routes.
type AuditProductHandler struct{ svc service.AuditProductService }

// NewAuditProductHandler constructs an AuditProductHandler.
func NewAuditProductHandler(svc service.AuditProductService) *AuditProductHandler {
	return &AuditProductHandler{svc: svc}
}

// SearchAuditProducts handles POST /audit/products/search.
func (h *AuditProductHandler) SearchAuditProducts(w http.ResponseWriter, r *http.Request) {
	var req domain.SearchAuditProductsRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	resp, err := h.svc.SearchAuditProducts(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// GetAuditProductByID handles GET /audit/products/{id}.
func (h *AuditProductHandler) GetAuditProductByID(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "id must be a positive integer"})
		return
	}
	p, err := h.svc.GetAuditProductByID(r.Context(), id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(p)
}

// CreateAuditProduct handles POST /audit/products.
func (h *AuditProductHandler) CreateAuditProduct(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateAuditProductRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	p, err := h.svc.CreateAuditProduct(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(p)
}

// UpdateAuditProduct handles PATCH /audit/products/{id}.
func (h *AuditProductHandler) UpdateAuditProduct(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "id must be a positive integer"})
		return
	}
	var req domain.UpdateAuditProductRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	p, err := h.svc.UpdateAuditProduct(r.Context(), id, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(p)
}
