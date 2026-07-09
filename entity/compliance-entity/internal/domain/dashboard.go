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

// AuditDashboardRequest is the body of POST /audit/dashboard. Roles come from the
// caller's JWT groups claim; UserEmail is the authenticated user (for team/auditor scope).
type AuditDashboardRequest struct {
	Roles     []string `json:"roles"`
	UserEmail string   `json:"userEmail"`
}

// Audit role constants — mirror the Asgardeo group names exactly.
const (
	RoleComplianceAdmin = "compliance_admin"
	RoleComplianceTeam  = "compliance_team"
	RoleInternalTeam    = "internal_team"
	RoleExternalAuditor = "external_auditor"
	RoleManagement      = "management"
)

// PrimaryRole returns the highest-priority audit role from the request's role list.
func (req AuditDashboardRequest) PrimaryRole() string {
	priority := []string{RoleComplianceAdmin, RoleComplianceTeam, RoleExternalAuditor, RoleInternalTeam, RoleManagement}
	for _, r := range priority {
		for _, g := range req.Roles {
			if g == r {
				return r
			}
		}
	}
	return ""
}

// AuditStats are the audit-count summary tiles.
type AuditStats struct {
	TotalAudits     int `json:"totalAudits"`
	ActiveAudits    int `json:"activeAudits"`
	CompletedAudits int `json:"completedAudits"`
	ArchivedAudits  int `json:"archivedAudits"`
}

// DashboardStats are the top-level control summary numbers.
type DashboardStats struct {
	TotalControls            int     `json:"totalControls"`
	CompletedControls        int     `json:"completedControls"`
	OverdueControls          int     `json:"overdueControls"`
	EvidenceRequiredControls int     `json:"evidenceRequiredControls"`
	CompletionPercent        float64 `json:"completionPercent"`
}

// StatusCount is one slice of the "Controls by Status" chart.
type StatusCount struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}

// TeamCompletion is one bar in the "Completed by Team" chart.
type TeamCompletion struct {
	Team      string `json:"team"`
	Completed int    `json:"completed"`
	Total     int    `json:"total"`
}

// ActionItem is a single entry in "My Action Items".
type ActionItem struct {
	ControlID     int    `json:"controlId"`
	AuditID       int    `json:"auditId"`
	AuditName     string `json:"auditName"`
	ControlNumber string `json:"controlNumber"`
	Description   string `json:"description"`
	Status        string `json:"status"`
	DueDate       string `json:"dueDate"`
}

// OverdueControl is a single overdue control entry.
type OverdueControl struct {
	ControlID     int    `json:"controlId"`
	AuditID       int    `json:"auditId"`
	AuditName     string `json:"auditName"`
	ControlNumber string `json:"controlNumber"`
	Description   string `json:"description"`
	Status        string `json:"status"`
	DueDate       string `json:"dueDate"`
}

// DashboardData is the full audit dashboard payload.
type DashboardData struct {
	AuditStats         AuditStats       `json:"auditStats"`
	Stats              DashboardStats   `json:"stats"`
	StatusDistribution []StatusCount    `json:"statusDistribution"`
	TeamCompletion     []TeamCompletion `json:"teamCompletion"`
	ActionItems        []ActionItem     `json:"actionItems"`
	OverdueControls    []OverdueControl `json:"overdueControls"`
}
