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
package model

// Lookup is one admin-managed register-template dropdown value: a platform,
// customer, product or deployment type (RISK_MODULE_DESIGN.md §14).
type Lookup struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	// Code is set for customers only: the A-Z/0-9 code embedded in Managed
	// Services risk codes.
	Code *string `json:"code,omitempty"`
	// ParentID is set on a sub-platform: the platform it sits under. Platforms
	// only.
	ParentID *int   `json:"parent_id,omitempty"`
	Status   string `json:"status"` // ACTIVE | INACTIVE
	// InUse reports whether any risk references the value. One in use can be
	// deactivated but not deleted, and a customer's code is then frozen.
	InUse bool `json:"in_use"`
}

// CreateLookupRequest is the payload for POST /api/v1/risks/{platforms|customers|products|deployment-types}.
type CreateLookupRequest struct {
	Name string  `json:"name"`
	Code *string `json:"code,omitempty"` // required for customers, rejected for the rest
	// ParentID makes the new value a sub-platform of that top-level platform.
	// Platforms only; it cannot be changed afterwards.
	ParentID *int `json:"parent_id,omitempty"`
}

// UpdateLookupRequest is the payload for PUT /api/v1/risks/{platforms|customers|products|deployment-types}/{id}.
// A nil field is left unchanged.
type UpdateLookupRequest struct {
	Name   *string `json:"name,omitempty"`
	Code   *string `json:"code,omitempty"`   // customers only; refused once the customer is in use
	Status *string `json:"status,omitempty"` // ACTIVE | INACTIVE
}
