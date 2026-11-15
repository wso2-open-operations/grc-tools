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

// A model asked for a widget sometimes slips on things that do not matter: a
// limit above the cap, a sort on a column it did not select, a visual that does
// not fit, the same field twice. Refusing the whole widget for those would make
// the assistant fail on ordinary questions, so Tidy and FitVisual correct them.
//
// Neither relaxes anything that protects data. Dataset, field, measure and
// column ids, filter ops and values, and the visibility the query runs with are
// all still checked by Validate exactly as before. These two are used only on
// what the model returns, never on a spec a client sends to be run.

// Tidy corrects harmless slips in a spec the model produced.
func Tidy(spec Spec) Spec {
	out := spec
	out.Measures = dedupe(spec.Measures)
	out.Dimensions = dedupe(spec.Dimensions)
	out.Columns = dedupe(spec.Columns)

	ds, known := lookupDataset(spec.Dataset)
	maxLimit := MaxLimit
	if known && ds.Rows {
		maxLimit = ds.MaxRows
		if len(out.Columns) > MaxListColumns {
			out.Columns = out.Columns[:MaxListColumns]
		}
	}
	if out.Limit > maxLimit {
		out.Limit = maxLimit
	}
	if out.Limit < 0 {
		out.Limit = 0
	}

	// A sort that names something not shown is dropped; the default order applies.
	if out.Sort != nil {
		shown := map[string]bool{}
		for _, id := range out.Measures {
			shown[id] = true
		}
		for _, id := range out.Dimensions {
			shown[id] = true
		}
		for _, id := range out.Columns {
			shown[id] = true
		}
		if !shown[out.Sort.By] {
			out.Sort = nil
		}
	}
	return out
}

func dedupe(in []string) []string {
	if len(in) < 2 {
		return in
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// FitVisual keeps the model's visual when it suits the spec, and otherwise picks
// the one that does: a list is always a table; a single number is a tile; one
// dimension is a bar chart; two dimensions with one measure is a heat map;
// anything else is a table.
func FitVisual(visual string, spec Spec) string {
	if ds, ok := lookupDataset(spec.Dataset); ok && ds.Rows {
		return VisualTable
	}
	if validateVisual(visual, spec) == nil {
		return visual
	}
	dims, measures := len(spec.Dimensions), len(spec.Measures)
	switch {
	case dims == 0 && measures == 1:
		return VisualKPI
	case dims == 1:
		return VisualBar
	case dims == 2 && measures == 1:
		return VisualHeatmap
	}
	return VisualTable
}
