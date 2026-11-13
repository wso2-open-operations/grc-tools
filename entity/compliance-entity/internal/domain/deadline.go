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

package domain

// DelayLogEntry is one late day for one group. The GRC Backend decides who may see it.
type DelayLogEntry struct {
	HandledBy string `json:"handledBy"` // OWNER | COMPLIANCE | AUDITOR
	Date      string `json:"date"`      // YYYY-MM-DD
}

// DeadlineDaysRow is one allowed-days setting for an audit.
type DeadlineDaysRow struct {
	CycleType   string `json:"cycleType"`   // INITIAL | REVISION
	HandledBy   string `json:"handledBy"`   // OWNER | COMPLIANCE | AUDITOR
	AllowedDays int    `json:"allowedDays"` // 1..365
	IsDefault   bool   `json:"isDefault"`   // true when no saved value overrides the default
}

// DeadlineDaysResponse is returned by GET and PUT /audits/{auditId}/deadline-days.
// It always lists all six cycle/handler combinations.
type DeadlineDaysResponse struct {
	AuditID int               `json:"auditId"`
	Phase   string            `json:"phase"` // EVIDENCE
	Days    []DeadlineDaysRow `json:"days"`
}

// UpdateDeadlineDaysRequest is the payload for PUT /audits/{auditId}/deadline-days.
// Only the rows listed are changed.
type UpdateDeadlineDaysRequest struct {
	Days      []DeadlineDaysRow `json:"days"`
	UpdatedBy string            `json:"updatedBy"`
}
