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

// Team represents a risk team, mapping to the `risk_team` table.
type Team struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Code        *string `json:"code"`
	Description *string `json:"description"`
	TeamType    string  `json:"team_type"`
	// RegisterTemplate is STANDARD, AGGREGATED or MANAGED_SERVICES. On a
	// register it decides which fields its risks carry (Add Risk reads it to
	// show the right form); an assignment-only team ignores it. See
	// RISK_MODULE_DESIGN.md §14.
	RegisterTemplate string `json:"register_template"`
	// HasRisks is true once any risk uses the team as its source register. The
	// template can then no longer change, so the Admin Console disables that
	// choice.
	HasRisks bool `json:"has_risks"`
	// PlatformIDs are the platforms and sub-platforms an AGGREGATED register
	// offers on Add Risk. Empty means it offers every active platform.
	PlatformIDs []int  `json:"platform_ids"`
	Status      string `json:"status"`
}

// Register templates, as stored in risk_team.register_template.
const (
	TemplateStandard        = "STANDARD"
	TemplateAggregated      = "AGGREGATED"
	TemplateManagedServices = "MANAGED_SERVICES"
)

// ListTeamsFilter controls which teams are returned by GET /api/v1/risks/teams.
// Type uses semantic values: "SOURCE_REGISTER" returns teams where team_type
// IN ('SOURCE_REGISTER','BOTH'); "ASSIGNMENT" returns IN ('ASSIGNMENT','BOTH').
// Empty Type returns all ACTIVE teams.
type ListTeamsFilter struct {
	Type string
	// IncludeInactive returns every status, not just ACTIVE. Every existing
	// caller of this endpoint is a picker (Add Risk's register dropdown, the
	// grant editor's scope picker, ...) that must never offer an inactive
	// team, so this defaults to false and only the Admin Console's Risk Teams
	// management table — which needs to show and let someone reactivate an
	// inactive team, not just hide it — sets it true.
	IncludeInactive bool
}

// CreateTeamRequest is the payload for POST /api/v1/risks/teams.
type CreateTeamRequest struct {
	Name        string  `json:"name"`
	Code        *string `json:"code"`
	Description string  `json:"description"`
	TeamType    string  `json:"team_type"`
	// RegisterTemplate defaults to STANDARD when empty.
	RegisterTemplate string `json:"register_template,omitempty"`
	// PlatformIDs narrows the platforms an AGGREGATED register offers; omitted
	// or empty offers every active platform.
	PlatformIDs []int `json:"platform_ids,omitempty"`
}

// UpdateTeamRequest is the payload for PUT /api/v1/risks/teams/{id}.
type UpdateTeamRequest struct {
	Name        string  `json:"name"`
	Code        *string `json:"code"`
	Description string  `json:"description"`
	TeamType    string  `json:"team_type"`
	Status      string  `json:"status"`
	// RegisterTemplate empty leaves it unchanged. Changing it once the register
	// has risks is refused by the entity with a 409.
	RegisterTemplate string `json:"register_template,omitempty"`
	// PlatformIDs replaces the register's platform list when present; an empty
	// list clears it (every active platform is offered again), and an absent
	// one leaves it unchanged.
	PlatformIDs []int `json:"platform_ids,omitempty"`
}
