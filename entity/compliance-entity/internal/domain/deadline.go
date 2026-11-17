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
	Phase     string `json:"phase,omitempty"` // EVIDENCE | POPULATION | OE_EVIDENCE
	HandledBy string `json:"handledBy"`       // OWNER | COMPLIANCE | AUDITOR
	Date      string `json:"date"`            // YYYY-MM-DD
}

// PhasePlan is the original plan for one phase of a control: the date each
// group was first expected to hand it on. A phase appears once it has started.
type PhasePlan struct {
	Phase                 string  `json:"phase"` // EVIDENCE | POPULATION | OE_EVIDENCE
	PlannedOwnerDate      *string `json:"plannedOwnerDate"`
	PlannedComplianceDate *string `json:"plannedComplianceDate"`
	PlannedAuditorDate    *string `json:"plannedAuditorDate"`
}

// DeadlineDaysRow is one allowed-days setting for an audit.
type DeadlineDaysRow struct {
	Phase       string `json:"phase"`       // EVIDENCE (Design) | POPULATION | OE_EVIDENCE; empty means EVIDENCE
	CycleType   string `json:"cycleType"`   // INITIAL | REVISION
	HandledBy   string `json:"handledBy"`   // OWNER | COMPLIANCE | AUDITOR
	AllowedDays int    `json:"allowedDays"` // 1..365
	IsDefault   bool   `json:"isDefault"`   // true when no saved value overrides the default
}

// DeadlineDaysResponse is returned by GET and PUT /audits/{auditId}/deadline-days.
// It always lists the six cycle/handler combinations of every phase.
type DeadlineDaysResponse struct {
	AuditID int               `json:"auditId"`
	Phase   string            `json:"phase"` // EVIDENCE; kept for older clients, rows carry their own phase
	Days    []DeadlineDaysRow `json:"days"`
	// Recalculated is set when the request asked to apply the change to open
	// stages (or to preview it); DryRun says nothing was saved.
	Recalculated *RecalcSummary `json:"recalculated,omitempty"`
	DryRun       bool           `json:"dryRun,omitempty"`
}

// RecalcSummary is what applying changed days to the open stages did.
type RecalcSummary struct {
	// StagesUpdated is the number of open stages (one per control) whose
	// deadline moved to its start day plus the new days.
	StagesUpdated int `json:"stagesUpdated"`
	// NewlyOverdue is how many of those were within their deadline before and
	// are past the new one.
	NewlyOverdue int `json:"newlyOverdue"`
}

// UpdateDeadlineDaysRequest is the payload for PUT /audits/{auditId}/deadline-days.
// Only the rows listed are changed.
type UpdateDeadlineDaysRequest struct {
	Days      []DeadlineDaysRow `json:"days"`
	UpdatedBy string            `json:"updatedBy"`
	// ApplyToOpenStages also moves the deadline of every open stage whose phase,
	// cycle and group match a value that changed. Off, a change only affects
	// hand-offs that happen afterwards.
	ApplyToOpenStages bool `json:"applyToOpenStages,omitempty"`
	// DryRun works out what ApplyToOpenStages would do and saves nothing.
	DryRun bool `json:"dryRun,omitempty"`
}
