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

package dashboardagent

import (
	"strings"
	"unicode/utf8"
)

// Visuals a widget can be drawn as.
const (
	VisualKPI     = "kpi"
	VisualBar     = "bar"
	VisualHeatmap = "heatmap"
	VisualTable   = "table"
)

const maxTitleLen = 80

// Widget is a spec plus how to draw it.
type Widget struct {
	Title  string `json:"title"`
	Visual string `json:"visual"`
	Spec   Spec   `json:"spec"`
}

// ValidateWidget checks the spec against the catalog, the visual against the
// shape of the spec, and cleans the title. The input is not modified.
func ValidateWidget(w Widget, canUse func(priv string) bool) (Widget, error) {
	return ValidateWidgetIn(ModuleRisk, w, canUse)
}

// ValidateWidgetIn is ValidateWidget for one hub's agent.
func ValidateWidgetIn(module string, w Widget, canUse func(priv string) bool) (Widget, error) {
	spec, err := ValidateIn(module, w.Spec, canUse)
	if err != nil {
		return Widget{}, err
	}
	if ds, ok := lookupDataset(spec.Dataset); ok && ds.Rows {
		if w.Visual != VisualTable {
			return Widget{}, invalid("a list is shown as a table")
		}
	} else if err := validateVisual(w.Visual, spec); err != nil {
		return Widget{}, err
	}
	title := strings.TrimSpace(w.Title)
	if title == "" {
		title = "Untitled widget"
	}
	if utf8.RuneCountInString(title) > maxTitleLen {
		title = string([]rune(title)[:maxTitleLen])
	}
	return Widget{Title: title, Visual: w.Visual, Spec: spec}, nil
}

// validateVisual checks that a visual can show a spec's result.
func validateVisual(visual string, spec Spec) error {
	dims, measures := len(spec.Dimensions), len(spec.Measures)
	switch visual {
	case VisualKPI:
		if dims != 0 || measures != 1 {
			return invalid("a number tile needs one measure and no dimension")
		}
	case VisualBar:
		if !((dims == 1 && measures >= 1) || (dims == 2 && measures == 1)) {
			return invalid("a bar chart needs one dimension, or two dimensions with one measure")
		}
	case VisualHeatmap:
		if dims != 2 || measures != 1 {
			return invalid("a heat map needs exactly two dimensions and one measure")
		}
	case VisualTable:
	default:
		return invalid("unknown visual")
	}
	return nil
}

// CatalogFor returns the part of the catalog the caller may use: fields and
// measures that need a privilege they lack are left out, so the model is never
// told they exist.
func CatalogFor(canUse func(priv string) bool) []DatasetDef {
	return CatalogForModule(ModuleRisk, canUse)
}

// CatalogForModule is CatalogFor for one hub's agent: it holds only that hub's
// datasets.
func CatalogForModule(module string, canUse func(priv string) bool) []DatasetDef {
	hidden := func(priv string) bool { return priv != "" && (canUse == nil || !canUse(priv)) }
	out := make([]DatasetDef, 0, len(datasets))
	for _, ds := range datasets {
		if ds.Module != module {
			continue
		}
		cp := DatasetDef{ID: ds.ID, Label: ds.Label, Rows: ds.Rows, MaxRows: ds.MaxRows}
		for _, c := range ds.Columns {
			if !c.Internal {
				cp.Columns = append(cp.Columns, c)
			}
		}
		for _, f := range ds.Fields {
			if !hidden(f.RequiredPrivilege) {
				cp.Fields = append(cp.Fields, f)
			}
		}
		for _, m := range ds.Measures {
			if !hidden(m.RequiredPrivilege) {
				cp.Measures = append(cp.Measures, m)
			}
		}
		out = append(out, cp)
	}
	return out
}

// Starter is a ready-made widget the user can open with one click.
type Starter struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Widget      Widget `json:"widget"`
}

// openStatuses is every workflow status except CLOSED and CANCELLED.
func openStatuses() []string {
	out := make([]string, 0, len(statuses))
	for _, s := range statuses {
		if s != "CLOSED" && s != "CANCELLED" {
			out = append(out, s)
		}
	}
	return out
}

// Starters returns the ready-made views. Each one is checked against the
// catalog by a test, so a catalog change cannot leave a broken starter behind.
func Starters() []Starter {
	open := Filter{Field: "workflow_status", Op: "in", Values: openStatuses()}
	return []Starter{
		{
			ID:          "risk-heat-map",
			Title:       "Risk heat map",
			Description: "Open risks by residual Likelihood and Impact.",
			Widget: Widget{Title: "Risk heat map (residual)", Visual: VisualHeatmap, Spec: Spec{
				Dataset:    "risks",
				Measures:   []string{"risk_count"},
				Dimensions: []string{"residual_likelihood", "residual_impact"},
				Filters:    []Filter{open},
			}},
		},
		{
			ID:          "overdue-remediation",
			Title:       "Overdue remediation",
			Description: "Approved risks past their Implementation Date, by Action Owner.",
			Widget: Widget{Title: "Overdue remediation by Action Owner", Visual: VisualBar, Spec: Spec{
				Dataset:    "risks",
				Measures:   []string{"risk_count"},
				Dimensions: []string{"action_owner"},
				Filters: []Filter{
					{Field: "workflow_status", Op: "in", Values: []string{"APPROVED"}},
					{Field: "is_overdue", Op: "is", Values: []string{"true"}},
				},
				Sort: &Sort{By: "risk_count", Desc: true},
			}},
		},
		{
			ID:          "compliance-coverage",
			Title:       "Compliance coverage",
			Description: "Open risks per compliance reference.",
			Widget: Widget{Title: "Open risks per compliance reference", Visual: VisualBar, Spec: Spec{
				Dataset:    "risks",
				Measures:   []string{"risk_count"},
				Dimensions: []string{"compliance_reference"},
				Filters:    []Filter{open},
				Sort:       &Sort{By: "risk_count", Desc: true},
			}},
		},
		{
			ID:          "ai-acceptance",
			Title:       "AI acceptance rate",
			Description: "How often users accept the AI category, likelihood and action-plan suggestions.",
			Widget: Widget{Title: "AI suggestion acceptance", Visual: VisualTable, Spec: Spec{
				Dataset:    "ai_suggestions",
				Measures:   []string{"suggestion_count", "acceptance_rate"},
				Dimensions: []string{"feature"},
			}},
		},
	}
}

// LabelColumns fills in each column's label from the catalog, so the web app
// shows the catalog's names and keeps no copy of them. A column the catalog
// does not know keeps its id as the label.
func LabelColumns(spec Spec, res *Result) {
	ds, ok := lookupDataset(spec.Dataset)
	for i := range res.Columns {
		c := &res.Columns[i]
		c.Label = c.ID
		if !ok {
			continue
		}
		if f, found := ds.field(c.ID); found && f.Label != "" && c.Role != "column" {
			c.Label = f.Label
		} else if m, found := ds.measure(c.ID); found && m.Label != "" {
			c.Label = m.Label
		} else if col, found := ds.column(c.ID); found && col.Label != "" {
			c.Label = col.Label
		}
	}
}

// WithLinkColumn adds the hidden risk id to a list that shows risk codes, so the
// web app can turn each code into a link. Only the platform does this, after the
// spec has been validated; it is not something a user or the model can request.
// Any other spec is returned unchanged.
func WithLinkColumn(spec Spec) Spec {
	ds, ok := lookupDataset(spec.Dataset)
	if !ok || !ds.Rows {
		return spec
	}
	hasCode, hasID := false, false
	for _, c := range spec.Columns {
		hasCode = hasCode || c == "risk_code"
		hasID = hasID || c == "risk_id"
	}
	if !hasCode || hasID {
		return spec
	}
	out := spec
	out.Columns = append(append([]string{}, spec.Columns...), "risk_id")
	return out
}

// WithControlLinkColumns adds the hidden audit and control ids to a list that
// shows control numbers, so the web app can turn each number into a link. Only
// the platform does this, after the spec has been validated; it is not something
// a user or the model can request. Any other spec is returned unchanged.
func WithControlLinkColumns(spec Spec) Spec {
	if spec.Dataset != "control_list" {
		return spec
	}
	hasNumber, hasIDs := false, false
	for _, c := range spec.Columns {
		hasNumber = hasNumber || c == "control_number"
		hasIDs = hasIDs || c == "audit_id" || c == "control_id"
	}
	if !hasNumber || hasIDs {
		return spec
	}
	out := spec
	out.Columns = append(append([]string{}, spec.Columns...), "audit_id", "control_id")
	return out
}
