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

// Package model defines the domain types for the Audit Hub module.
package model

// AuditAssignment represents an auditor assignment derived from audit_control.auditor_id.
// There is no separate assignment table; assignments are managed per-control.
type AuditAssignment struct {
	AuditorID   int    `json:"auditorId"`
	AuditorName string `json:"auditorName"`
	ControlID   int    `json:"controlId"`
}

// CreateAssignmentRequest is currently unused (assignment management is per-control).
type CreateAssignmentRequest struct{}
