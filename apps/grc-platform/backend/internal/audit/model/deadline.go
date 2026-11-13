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

package model

// DelayLogEntry is one late day for one group.
type DelayLogEntry struct {
	HandledBy string `json:"handledBy"` // OWNER | COMPLIANCE | AUDITOR
	Date      string `json:"date"`      // YYYY-MM-DD
}

// DeadlineDaysRow is one allowed-days setting: how many calendar days a group
// may hold a control before passing it on, in a given cycle.
type DeadlineDaysRow struct {
	CycleType   string `json:"cycleType"`   // INITIAL | REVISION
	HandledBy   string `json:"handledBy"`   // OWNER | COMPLIANCE | AUDITOR
	AllowedDays int    `json:"allowedDays"` // 1..365
	IsDefault   bool   `json:"isDefault"`   // true while no saved value overrides the default
}

// DeadlineDays is the response of GET/PUT /api/v1/audits/{id}/settings/deadline-days.
// It always lists all six cycle/handler combinations.
type DeadlineDays struct {
	AuditID int               `json:"auditId"`
	Phase   string            `json:"phase"` // EVIDENCE
	Days    []DeadlineDaysRow `json:"days"`
}

// UpdateDeadlineDaysRequest is the payload for PUT /api/v1/audits/{id}/settings/deadline-days.
// Only the rows listed are changed.
type UpdateDeadlineDaysRequest struct {
	Days []DeadlineDaysRow `json:"days"`
}
