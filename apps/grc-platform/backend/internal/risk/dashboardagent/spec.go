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

// Package dashboardagent holds the parts of the Risk Hub dashboard agent that
// decide what a widget may ask for and who may see the answer: the spec a
// widget is described by, the catalog of datasets, fields and measures it may
// use, and the validator that checks a spec against that catalog.
//
// The package never builds SQL and never sees data. A spec names catalog
// entries; the Compliance Entity maps them to SQL and applies the visibility
// the handler works out from the caller's grants.
package dashboardagent

// Filter narrows a widget. Op is one of in, gte, lte, is; which ops a field
// accepts follows from its kind (see Validate).
type Filter struct {
	Field  string   `json:"field"`
	Op     string   `json:"op"`
	Values []string `json:"values"`
}

// Sort orders the result by one selected dimension or measure.
type Sort struct {
	By   string `json:"by"`
	Desc bool   `json:"desc"`
}

// Spec describes what a widget shows. Only catalog ids and values appear in
// it: there is nowhere in a Spec to put a table, a column, a join, a condition
// or the caller's visibility.
type Spec struct {
	Dataset    string   `json:"dataset"`
	Measures   []string `json:"measures"`
	Dimensions []string `json:"dimensions"`
	// Columns picks what a list dataset shows, one row per risk. It is used
	// instead of Measures and Dimensions, never with them.
	Columns []string `json:"columns,omitempty"`
	Filters []Filter `json:"filters"`
	Sort    *Sort    `json:"sort,omitempty"`
	Limit   int      `json:"limit"`
}

// Visibility is which risks the caller may see, worked out on the server from
// their grants. The zero value sees nothing.
type Visibility struct {
	All               bool  `json:"all"`
	TeamIDs           []int `json:"teamIds"`
	ActionOwnerUserID *int  `json:"actionOwnerUserId,omitempty"`
}

// Kind names the sort of access, for the audit log: all, team, owned (named on a
// risk), both of the last two, or none.
func (v Visibility) Kind() string {
	switch {
	case v.All:
		return "all"
	case len(v.TeamIDs) > 0 && v.ActionOwnerUserID != nil:
		return "team+owned"
	case len(v.TeamIDs) > 0:
		return "team"
	case v.ActionOwnerUserID != nil:
		return "owned"
	}
	return "none"
}

// None reports whether the caller may see no risk at all. Such a caller gets no
// dashboard agent.
func (v Visibility) None() bool {
	return !v.All && len(v.TeamIDs) == 0 && v.ActionOwnerUserID == nil
}

// AuditScope is which Audit Hub controls the caller may see, worked out on the
// server from their grants. Scope is all, team, owned, assigned or none; the zero
// value sees nothing.
type AuditScope struct {
	Scope   string `json:"scope"`
	UserID  int    `json:"userId"`
	TeamIDs []int  `json:"scopeTeamIds"`
}

// Kind names the sort of access, for the audit log.
func (a AuditScope) Kind() string {
	switch a.Scope {
	case "all", "team", "owned", "assigned":
		return a.Scope
	}
	return "none"
}

// None reports whether the caller may see no audit data at all. Such a caller
// gets no audit agent.
func (a AuditScope) None() bool { return a.Kind() == "none" }

// Column describes one result column. Role is "dimension" or "measure" for an
// aggregate and "column" for a list. Label is the catalog's name for it, filled
// in by LabelColumns.
type Column struct {
	ID    string `json:"id"`
	Role  string `json:"role"`
	Kind  string `json:"kind"`
	Label string `json:"label"`
}

// Result is the aggregate a widget draws. Rows follow Columns.
type Result struct {
	Columns   []Column `json:"columns"`
	Rows      [][]any  `json:"rows"`
	Truncated bool     `json:"truncated"`
	// Summary is a short sentence written by Summarize from the numbers. It is
	// built in this process; no model ever sees the data it describes.
	Summary string `json:"summary,omitempty"`
}
