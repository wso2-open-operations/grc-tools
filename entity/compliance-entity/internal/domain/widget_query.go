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

// A widget query is a read-only aggregate over the risk register, described by
// a small spec instead of SQL. The GRC backend validates the spec and works out
// what the caller may see; this service turns the pair into one parameterized
// SELECT. Nothing in the spec can name a table, a column, a join or a
// condition: every one of those comes from the catalog in
// repository/widget_catalog.go.

// WidgetFilter narrows a widget query. Op is one of in, gte, lte, is.
type WidgetFilter struct {
	Field  string   `json:"field"`
	Op     string   `json:"op"`
	Values []string `json:"values"`
}

// WidgetSort orders the result by one selected dimension or measure.
type WidgetSort struct {
	By   string `json:"by"`
	Desc bool   `json:"desc"`
}

// WidgetSpec is what a widget shows. Field names are single lower-case words so
// the backend and the entity agree without any key mapping.
type WidgetSpec struct {
	Dataset    string   `json:"dataset"`
	Measures   []string `json:"measures"`
	Dimensions []string `json:"dimensions"`
	// Columns picks what a list dataset shows, one row per risk. It is used
	// instead of Measures and Dimensions, never with them.
	Columns []string       `json:"columns,omitempty"`
	Filters []WidgetFilter `json:"filters"`
	Sort    *WidgetSort    `json:"sort,omitempty"`
	Limit   int            `json:"limit"`
}

// WidgetVisibility is which risks the caller may see, worked out by the GRC
// backend from the caller's grants. It is fail-closed: the zero value sees
// nothing, and an empty team list is never read as "unrestricted" (unlike the
// optional scope filters on the other risk queries).
//
//	All               every risk (a GLOBAL ViewRisks grant)
//	TeamIDs           risks raised in, or routed to, any of these teams
//	ActionOwnerUserID risks whose action plan names this user
type WidgetVisibility struct {
	All               bool  `json:"all"`
	TeamIDs           []int `json:"teamIds"`
	ActionOwnerUserID *int  `json:"actionOwnerUserId,omitempty"`
}

// WidgetQueryRequest is the body of POST /risk/widgets/query.
type WidgetQueryRequest struct {
	Spec       WidgetSpec       `json:"spec"`
	Visibility WidgetVisibility `json:"visibility"`
}

// WidgetColumn describes one result column. Role is "dimension" or "measure"
// for an aggregate, "column" for a list; Kind is text, int, date or number.
type WidgetColumn struct {
	ID   string `json:"id"`
	Role string `json:"role"`
	Kind string `json:"kind"`
}

// WidgetQueryResult is the aggregate. Rows follow Columns, dimensions first.
// Truncated is true when more rows matched than the limit allowed.
type WidgetQueryResult struct {
	Columns   []WidgetColumn `json:"columns"`
	Rows      [][]any        `json:"rows"`
	Truncated bool           `json:"truncated"`
}
