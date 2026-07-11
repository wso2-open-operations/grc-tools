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

// AuditStats are audit-level counts shown on the dashboard.
type AuditStats struct {
	TotalAudits     int `json:"totalAudits"`
	ActiveAudits    int `json:"activeAudits"`
	CompletedAudits int `json:"completedAudits"`
	ArchivedAudits  int `json:"archivedAudits"`
}

// DashboardStats are the top-level summary numbers on the dashboard.
type DashboardStats struct {
	TotalControls            int     `json:"totalControls"`
	CompletedControls        int     `json:"completedControls"`
	OverdueControls          int     `json:"overdueControls"`
	EvidenceRequiredControls int     `json:"evidenceRequiredControls"`
	CompletionPercent        float64 `json:"completionPercent"`
}

// StatusCount is one slice of the "Controls by Status" donut chart.
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

// ActionItem is a single entry in the "My Action Items" list.
type ActionItem struct {
	ControlID     int    `json:"controlId"`
	AuditID       int    `json:"auditId"`
	AuditName     string `json:"auditName"`
	ControlNumber string `json:"controlNumber"`
	Description   string `json:"description"`
	Status        string `json:"status"`
	DueDate       string `json:"dueDate"`
}

// OverdueControl is a single entry in the "Overdue Controls" list.
type OverdueControl struct {
	ControlID     int    `json:"controlId"`
	AuditID       int    `json:"auditId"`
	AuditName     string `json:"auditName"`
	ControlNumber string `json:"controlNumber"`
	Description   string `json:"description"`
	Status        string `json:"status"`
	DueDate       string `json:"dueDate"`
}

// DashboardData is the full payload returned by GET /api/v1/audit/dashboard.
type DashboardData struct {
	AuditStats         AuditStats       `json:"auditStats"`
	Stats              DashboardStats   `json:"stats"`
	StatusDistribution []StatusCount    `json:"statusDistribution"`
	TeamCompletion     []TeamCompletion `json:"teamCompletion"`
	ActionItems        []ActionItem     `json:"actionItems"`
	OverdueControls    []OverdueControl `json:"overdueControls"`
}

// DashboardFilter carries the query scope derived from the user's JWT roles.
type DashboardFilter struct {
	// Roles is the set of role strings from the JWT groups claim.
	Roles []string
	// UserEmail is the authenticated user's email (used to look up team/auditor ID).
	UserEmail string
}

// Role constants — mirror the Asgardeo group names exactly.
const (
	RoleComplianceAdmin = "compliance_admin"
	RoleComplianceTeam  = "compliance_team"
	RoleInternalTeam    = "internal_team"
	RoleExternalAuditor = "external_auditor"
	RoleManagement      = "management"
)

// asgardeoGroupRoles maps Asgardeo group names (JWT groups claim) to the
// canonical dashboard role tokens the Compliance Entity understands. The
// entity scopes unknown roles to zero rows, so translation must happen here
// (the entity itself is owned by another team and cannot be changed).
var asgardeoGroupRoles = map[string]string{
	"grc-platform-compliance-audit-admin": RoleComplianceAdmin,
	"grc-platform-compliance-audit-team":  RoleComplianceTeam,
	"grc-platform-internal-team":          RoleInternalTeam,
	"grc-platform-external-auditor":       RoleExternalAuditor,
	"grc-platform-management":             RoleManagement,
	// Testing catch-all group — full visibility, mirroring its allow-all privileges.
	"wso2-everyone": RoleComplianceAdmin,
}

// NormalizedRoles returns Roles translated to canonical role tokens.
// Values that are already canonical (or unknown) pass through unchanged.
func (f DashboardFilter) NormalizedRoles() []string {
	out := make([]string, 0, len(f.Roles))
	for _, g := range f.Roles {
		if r, ok := asgardeoGroupRoles[g]; ok {
			out = append(out, r)
			continue
		}
		out = append(out, g)
	}
	return out
}

// PrimaryRole returns the highest-priority audit role from the filter's role list.
func (f DashboardFilter) PrimaryRole() string {
	priority := []string{
		RoleComplianceAdmin,
		RoleComplianceTeam,
		RoleExternalAuditor,
		RoleInternalTeam,
		RoleManagement,
	}
	for _, r := range priority {
		for _, g := range f.Roles {
			if g == r {
				return r
			}
		}
	}
	return ""
}
