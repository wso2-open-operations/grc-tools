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
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Limits on a spec. The Compliance Entity enforces the same ones.
const (
	MaxMeasures   = 3
	MaxDimensions = 2
	MaxFilters    = 8
	MaxValues     = 20
	MaxValueLen   = 100
	DefaultLimit  = 100
	MaxLimit      = 500
)

// ValidationError is a spec that cannot be run. Its message is built from
// catalog ids and the caller's own input, so it is safe to show to them.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

func invalid(format string, a ...any) error {
	return &ValidationError{Msg: fmt.Sprintf(format, a...)}
}

// Validate checks spec against the catalog and returns a cleaned copy: values
// trimmed, the default limit filled in. The input is not modified.
//
// canUse reports whether the caller holds a privilege; it decides which catalog
// entries exist for them. An entry the caller may not use is reported exactly
// like one that does not exist, so a spec cannot be used to find out what the
// catalog holds beyond their access.
func Validate(spec Spec, canUse func(priv string) bool) (Spec, error) {
	return ValidateIn(ModuleRisk, spec, canUse)
}

// ValidateIn is Validate for one hub's agent: a dataset that belongs to another
// hub is reported as unknown, exactly like one that does not exist.
func ValidateIn(module string, spec Spec, canUse func(priv string) bool) (Spec, error) {
	ds, ok := lookupDataset(spec.Dataset)
	if !ok || ds.Module != module {
		return Spec{}, invalid("unknown dataset")
	}
	hidden := func(priv string) bool { return priv != "" && (canUse == nil || !canUse(priv)) }
	if ds.Rows {
		return validateList(spec, ds, hidden)
	}
	if len(spec.Columns) > 0 {
		return Spec{}, invalid("columns can only be used with a list")
	}

	if n := len(spec.Measures); n < 1 || n > MaxMeasures {
		return Spec{}, invalid("choose between 1 and %d measures", MaxMeasures)
	}
	if len(spec.Dimensions) > MaxDimensions {
		return Spec{}, invalid("at most %d dimensions are allowed", MaxDimensions)
	}
	if len(spec.Filters) > MaxFilters {
		return Spec{}, invalid("at most %d filters are allowed", MaxFilters)
	}
	out := newSpec(spec)
	switch {
	case out.Limit < 0 || out.Limit > MaxLimit:
		return Spec{}, invalid("limit must be between 1 and %d", MaxLimit)
	case out.Limit == 0:
		out.Limit = DefaultLimit
	}

	selected := map[string]bool{}
	repeats := false

	for _, id := range spec.Dimensions {
		f, ok := ds.field(id)
		if !ok || !f.Group || hidden(f.RequiredPrivilege) {
			return Spec{}, invalid("%q cannot be used as a dimension", id)
		}
		if selected[id] {
			return Spec{}, invalid("%q is selected twice", id)
		}
		selected[id] = true
		repeats = repeats || f.Repeats
		out.Dimensions = append(out.Dimensions, id)
	}

	unsafeMeasure := ""
	for _, id := range spec.Measures {
		m, ok := ds.measure(id)
		if !ok || hidden(m.RequiredPrivilege) {
			return Spec{}, invalid("unknown measure %q", id)
		}
		if selected[id] {
			return Spec{}, invalid("%q is selected twice", id)
		}
		selected[id] = true
		if !m.DistinctSafe && unsafeMeasure == "" {
			unsafeMeasure = id
		}
		out.Measures = append(out.Measures, id)
	}

	for _, flt := range spec.Filters {
		f, ok := ds.field(flt.Field)
		if !ok || hidden(f.RequiredPrivilege) {
			return Spec{}, invalid("%q cannot be used as a filter", flt.Field)
		}
		repeats = repeats || f.Repeats
		clean, err := validateFilter(flt, f)
		if err != nil {
			return Spec{}, err
		}
		out.Filters = append(out.Filters, clean)
	}

	if repeats && unsafeMeasure != "" {
		return Spec{}, invalid("%q cannot be combined with a field that repeats a risk's row", unsafeMeasure)
	}

	if spec.Sort != nil {
		if !selected[spec.Sort.By] {
			return Spec{}, invalid("sort must use a selected dimension or measure")
		}
		s := *spec.Sort
		out.Sort = &s
	}
	return out, nil
}

// newSpec starts a cleaned spec with empty lists rather than nil ones, so it
// always serializes as [] and never as null.
func newSpec(in Spec) Spec {
	return Spec{
		Dataset:    in.Dataset,
		Limit:      in.Limit,
		Measures:   []string{},
		Dimensions: []string{},
		Filters:    []Filter{},
	}
}

func validateFilter(flt Filter, f FieldDef) (Filter, error) {
	if flt.Op == "is" {
		if f.Kind != KindFlag {
			return Filter{}, invalid("%q does not support this filter", flt.Field)
		}
		if len(flt.Values) != 1 || (flt.Values[0] != "true" && flt.Values[0] != "false") {
			return Filter{}, invalid("%q needs the value true or false", flt.Field)
		}
		return Filter{Field: flt.Field, Op: "is", Values: []string{flt.Values[0]}}, nil
	}

	var ops []string
	switch f.Kind {
	case KindText:
		ops = []string{"in"}
	case KindInt:
		ops = []string{"in", "gte", "lte"}
	case KindDate:
		ops = []string{"gte", "lte"}
	case KindPerson:
		ops = []string{"in"}
	}
	allowed := false
	for _, o := range ops {
		allowed = allowed || o == flt.Op
	}
	if !allowed {
		return Filter{}, invalid("%q does not support the %q filter", flt.Field, flt.Op)
	}
	if len(flt.Values) < 1 || len(flt.Values) > MaxValues {
		return Filter{}, invalid("%q needs between 1 and %d values", flt.Field, MaxValues)
	}
	if f.Kind == KindPerson && len(flt.Values) > MaxPeople {
		return Filter{}, invalid("%q takes at most %d people", flt.Field, MaxPeople)
	}
	if flt.Op != "in" && len(flt.Values) != 1 {
		return Filter{}, invalid("%q needs exactly one value", flt.Field)
	}

	clean := Filter{Field: flt.Field, Op: flt.Op}
	for _, raw := range flt.Values {
		v := strings.TrimSpace(raw)
		if v == "" || len(v) > MaxValueLen || strings.ContainsRune(v, 0) {
			return Filter{}, invalid("%q has an invalid value", flt.Field)
		}
		switch f.Kind {
		case KindInt:
			n, err := strconv.Atoi(v)
			if err != nil || (f.Max > 0 && (n < f.Min || n > f.Max)) {
				return Filter{}, invalid("%q has an invalid value", flt.Field)
			}
		case KindDate:
			if _, err := time.Parse("2006-01-02", v); err != nil {
				return Filter{}, invalid("%q needs a date as YYYY-MM-DD", flt.Field)
			}
		default:
			if f.Allowed != nil {
				found := false
				for _, a := range f.Allowed {
					found = found || a == v
				}
				if !found {
					return Filter{}, invalid("%q has an invalid value", flt.Field)
				}
			}
		}
		clean.Values = append(clean.Values, v)
	}
	return clean, nil
}

// validateList checks a spec for a list dataset: one row per risk, the chosen
// columns, an optional filter and sort, at most ds.MaxRows rows.
func validateList(spec Spec, ds DatasetDef, hidden func(priv string) bool) (Spec, error) {
	if len(spec.Measures) > 0 || len(spec.Dimensions) > 0 {
		return Spec{}, invalid("a list shows columns, not measures or dimensions")
	}
	if n := len(spec.Columns); n < 1 || n > MaxListColumns {
		return Spec{}, invalid("choose between 1 and %d columns", MaxListColumns)
	}
	if len(spec.Filters) > MaxFilters {
		return Spec{}, invalid("at most %d filters are allowed", MaxFilters)
	}
	out := newSpec(spec)
	switch {
	case out.Limit < 0 || out.Limit > ds.MaxRows:
		return Spec{}, invalid("a list shows at most %d rows", ds.MaxRows)
	case out.Limit == 0:
		out.Limit = ds.MaxRows
	}

	selected := map[string]bool{}
	for _, id := range spec.Columns {
		if c, ok := ds.column(id); !ok || c.Internal {
			return Spec{}, invalid("unknown column %q", id)
		}
		if selected[id] {
			return Spec{}, invalid("%q is selected twice", id)
		}
		selected[id] = true
		out.Columns = append(out.Columns, id)
	}

	for _, flt := range spec.Filters {
		f, ok := ds.field(flt.Field)
		if !ok || hidden(f.RequiredPrivilege) {
			return Spec{}, invalid("%q cannot be used as a filter", flt.Field)
		}
		clean, err := validateFilter(flt, f)
		if err != nil {
			return Spec{}, err
		}
		out.Filters = append(out.Filters, clean)
	}

	if spec.Sort != nil {
		if !selected[spec.Sort.By] {
			return Spec{}, invalid("sort must use a selected column")
		}
		s := *spec.Sort
		out.Sort = &s
	}
	return out, nil
}
