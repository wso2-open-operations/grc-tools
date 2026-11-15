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

package repository

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

const (
	widgetMaxMeasures   = 3
	widgetMaxDimensions = 2
	widgetMaxFilters    = 8
	widgetMaxValues     = 20
	widgetMaxValueLen   = 100
	widgetDefaultLimit  = 100
	widgetMaxLimit      = 500
	widgetMaxTeamIDs    = 500
	// widgetMaxColumns is the 8 a user may pick plus the id the backend adds.
	widgetMaxColumns = 9
	widgetMaxPeople  = 5

	// widgetMaxExecutionMS asks MySQL to stop a statement that runs this long.
	widgetMaxExecutionMS = 5000
)

// CompiledWidgetQuery is one ready-to-run, parameterized SELECT. Limit is the
// row cap that was applied; the statement itself fetches Limit+1 rows so the
// caller can tell whether the result was cut short.
type CompiledWidgetQuery struct {
	SQL     string
	Args    []any
	Columns []domain.WidgetColumn
	Limit   int
}

func widgetErr(format string, a ...any) error {
	return &apierror.ValidationError{Msg: fmt.Sprintf(format, a...)}
}

// CompileWidgetQuery turns a spec and a visibility into one SELECT.
//
// Everything that reaches the SQL text comes from widgetDatasets. Caller input
// is only ever used to pick catalog entries and to supply bound values, and
// every spec is checked here even though the GRC backend has already checked
// it, so the entity is safe on its own.
func CompileWidgetQuery(req domain.WidgetQueryRequest) (*CompiledWidgetQuery, error) {
	return compileWidget("", req.Spec, func() (string, []any, error) { return widgetVisibilityClause(req.Visibility) })
}

// CompileAuditWidgetQuery is CompileWidgetQuery for the Audit Hub. It only
// reaches audit datasets, and the rows it may count are limited by the
// caller's audit scope (see auditWidgetVisibilityClause).
func CompileAuditWidgetQuery(req domain.AuditWidgetQueryRequest) (*CompiledWidgetQuery, error) {
	return compileWidget(widgetModuleAudit, req.Spec, func() (string, []any, error) { return auditWidgetVisibilityClause(req) })
}

// compileWidget is the shared compiler. module says which datasets the caller
// may name ("" for the risk register), so neither module's endpoint can reach
// the other's tables. visibility returns the row-level condition and is always
// applied.
func compileWidget(module string, spec domain.WidgetSpec, visibility func() (string, []any, error)) (*CompiledWidgetQuery, error) {
	ds, ok := widgetDatasets[spec.Dataset]
	if !ok || ds.module != module {
		return nil, widgetErr("unknown dataset")
	}
	if ds.rows {
		return compileWidgetRows(ds, spec, visibility)
	}
	if len(spec.Columns) > 0 {
		return nil, widgetErr("columns can only be used with a list dataset")
	}

	if n := len(spec.Measures); n < 1 || n > widgetMaxMeasures {
		return nil, widgetErr("choose between 1 and %d measures", widgetMaxMeasures)
	}
	if len(spec.Dimensions) > widgetMaxDimensions {
		return nil, widgetErr("at most %d dimensions are allowed", widgetMaxDimensions)
	}
	if len(spec.Filters) > widgetMaxFilters {
		return nil, widgetErr("at most %d filters are allowed", widgetMaxFilters)
	}
	limit := spec.Limit
	switch {
	case limit < 0 || limit > widgetMaxLimit:
		return nil, widgetErr("limit must be between 1 and %d", widgetMaxLimit)
	case limit == 0:
		limit = widgetDefaultLimit
	}

	needed := map[string]bool{}
	var columns []domain.WidgetColumn
	var selects, groupBy []string
	selected := map[string]bool{}

	for _, id := range spec.Dimensions {
		f, ok := ds.fields[id]
		if !ok || !f.group {
			return nil, widgetErr("%q cannot be used as a dimension", id)
		}
		if selected[id] {
			return nil, widgetErr("%q is selected twice", id)
		}
		selected[id] = true
		for _, j := range f.joins {
			needed[j] = true
		}
		selects = append(selects, f.expr+" AS `"+id+"`")
		groupBy = append(groupBy, f.expr)
		columns = append(columns, domain.WidgetColumn{ID: id, Role: "dimension", Kind: f.kind})
	}

	var unsafeMeasure string
	for _, id := range spec.Measures {
		m, ok := ds.measures[id]
		if !ok {
			return nil, widgetErr("unknown measure %q", id)
		}
		if selected[id] {
			return nil, widgetErr("%q is selected twice", id)
		}
		selected[id] = true
		for _, j := range m.joins {
			needed[j] = true
		}
		if !m.distinctSafe && unsafeMeasure == "" {
			unsafeMeasure = id
		}
		selects = append(selects, m.expr+" AS `"+id+"`")
		columns = append(columns, domain.WidgetColumn{ID: id, Role: "measure", Kind: m.kind})
	}

	var where strings.Builder
	var args []any
	where.WriteString(" WHERE 1=1")
	where.WriteString(ds.baseWhere)
	args = append(args, ds.baseArgs...)

	for _, flt := range spec.Filters {
		f, ok := ds.fields[flt.Field]
		if !ok {
			return nil, widgetErr("%q cannot be used as a filter", flt.Field)
		}
		for _, j := range f.joins {
			needed[j] = true
		}
		clause, fargs, err := widgetFilterClause(flt, f)
		if err != nil {
			return nil, err
		}
		where.WriteString(" AND " + clause)
		args = append(args, fargs...)
	}

	visClause, visArgs, err := visibility()
	if err != nil {
		return nil, err
	}
	if ds.wrapVisibility != nil && visClause != "" {
		visClause = ds.wrapVisibility(visClause)
	}
	where.WriteString(visClause)
	args = append(args, visArgs...)

	// Joins, in catalog order. A measure that is only correct per risk (an
	// average) must not run on rows a join has repeated.
	keys := make([]string, 0, len(needed))
	for k := range needed {
		if _, ok := ds.joins[k]; !ok {
			return nil, fmt.Errorf("widget catalog: join %q is not defined for dataset %q", k, spec.Dataset)
		}
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return ds.joins[keys[i]].order < ds.joins[keys[j]].order })
	var joins strings.Builder
	for _, k := range keys {
		j := ds.joins[k]
		if j.multiplies && unsafeMeasure != "" {
			return nil, widgetErr("%q cannot be combined with a field that repeats a risk's row", unsafeMeasure)
		}
		joins.WriteString(j.sql)
	}

	var order string
	switch {
	case spec.Sort != nil:
		if !selected[spec.Sort.By] {
			return nil, widgetErr("sort must use a selected dimension or measure")
		}
		dir := " ASC"
		if spec.Sort.Desc {
			dir = " DESC"
		}
		order = " ORDER BY `" + spec.Sort.By + "`" + dir
	case len(spec.Dimensions) > 0:
		order = " ORDER BY `" + spec.Dimensions[0] + "` ASC"
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "SELECT /*+ MAX_EXECUTION_TIME(%d) */ ", widgetMaxExecutionMS)
	sb.WriteString(strings.Join(selects, ", "))
	sb.WriteString(" " + ds.from)
	sb.WriteString(joins.String())
	sb.WriteString(where.String())
	if len(groupBy) > 0 {
		sb.WriteString(" GROUP BY " + strings.Join(groupBy, ", "))
	}
	sb.WriteString(order)
	sb.WriteString(" LIMIT ?")
	args = append(args, limit+1)

	return &CompiledWidgetQuery{SQL: sb.String(), Args: args, Columns: columns, Limit: limit}, nil
}

// widgetFilterClause builds one condition and its bound values.
func widgetFilterClause(flt domain.WidgetFilter, f widgetField) (string, []any, error) {
	if f.kind == widgetKindPerson {
		return widgetPersonClause(flt, f)
	}
	if flt.Op == widgetOpIs {
		if f.kind != widgetKindFlag {
			return "", nil, widgetErr("%q does not support this filter", flt.Field)
		}
		if len(flt.Values) != 1 || (flt.Values[0] != "true" && flt.Values[0] != "false") {
			return "", nil, widgetErr("%q needs the value true or false", flt.Field)
		}
		if flt.Values[0] == "true" {
			return f.expr, nil, nil
		}
		return "NOT " + f.expr, nil, nil
	}
	if f.kind == widgetKindFlag {
		return "", nil, widgetErr("%q only supports the is filter", flt.Field)
	}

	var allowedOps []string
	switch f.kind {
	case widgetKindText:
		allowedOps = []string{widgetOpIn}
	case widgetKindInt:
		allowedOps = []string{widgetOpIn, widgetOpGte, widgetOpLte}
	case widgetKindDate:
		allowedOps = []string{widgetOpGte, widgetOpLte}
	}
	opOK := false
	for _, o := range allowedOps {
		if o == flt.Op {
			opOK = true
		}
	}
	if !opOK {
		return "", nil, widgetErr("%q does not support the %q filter", flt.Field, flt.Op)
	}

	if len(flt.Values) < 1 || len(flt.Values) > widgetMaxValues {
		return "", nil, widgetErr("%q needs between 1 and %d values", flt.Field, widgetMaxValues)
	}
	if flt.Op != widgetOpIn && len(flt.Values) != 1 {
		return "", nil, widgetErr("%q needs exactly one value", flt.Field)
	}

	args := make([]any, 0, len(flt.Values))
	for _, raw := range flt.Values {
		v, err := widgetValue(flt.Field, f, raw)
		if err != nil {
			return "", nil, err
		}
		args = append(args, v)
	}

	switch flt.Op {
	case widgetOpIn:
		return f.expr + " IN (" + placeholders(len(args)) + ")", args, nil
	case widgetOpGte:
		return f.expr + " >= ?", args, nil
	default:
		return f.expr + " <= ?", args, nil
	}
}

// widgetValue checks one value against its field's kind and returns the value
// to bind.
func widgetValue(field string, f widgetField, raw string) (any, error) {
	v := strings.TrimSpace(raw)
	if v == "" || len(v) > widgetMaxValueLen || strings.ContainsRune(v, 0) {
		return nil, widgetErr("%q has an invalid value", field)
	}
	switch f.kind {
	case widgetKindInt:
		n, err := strconv.Atoi(v)
		if err != nil || (f.max > 0 && (n < f.min || n > f.max)) {
			return nil, widgetErr("%q has an invalid value", field)
		}
		return n, nil
	case widgetKindDate:
		if _, err := time.Parse("2006-01-02", v); err != nil {
			return nil, widgetErr("%q needs a date as YYYY-MM-DD", field)
		}
		return v, nil
	default:
		if f.allowed != nil {
			for _, a := range f.allowed {
				if a == v {
					return v, nil
				}
			}
			return nil, widgetErr("%q has an invalid value", field)
		}
		return v, nil
	}
}

// widgetVisibilityClause is the rule for which risks a query may count. It is
// fail-closed: with nothing granted it matches no row, so a caller whose
// visibility could not be worked out sees an empty result and never an
// unrestricted one. This is deliberately the opposite of scopeFilter, where an
// empty list means "no restriction".
//
// The rule mirrors the risk list the GRC backend serves:
//
//	team scoped   risks raised in, or routed to, any of the caller's teams
//	action owner  risks whose action plan names the caller
//
// The risk table is always aliased "r".
func widgetVisibilityClause(vis domain.WidgetVisibility) (string, []any, error) {
	if vis.All {
		return "", nil, nil
	}
	var parts []string
	var args []any

	if len(vis.TeamIDs) > 0 {
		if len(vis.TeamIDs) > widgetMaxTeamIDs {
			return "", nil, widgetErr("too many teams in visibility")
		}
		ids := make([]any, 0, len(vis.TeamIDs))
		for _, id := range vis.TeamIDs {
			if id <= 0 {
				return "", nil, widgetErr("invalid team in visibility")
			}
			ids = append(ids, id)
		}
		ph := placeholders(len(ids))
		parts = append(parts, "(r.source_register_id IN ("+ph+") OR r.assignment_team_id IN ("+ph+"))")
		args = append(args, ids...)
		args = append(args, ids...)
	}
	if vis.ActionOwnerUserID != nil {
		if *vis.ActionOwnerUserID <= 0 {
			return "", nil, widgetErr("invalid user in visibility")
		}
		parts = append(parts, "EXISTS (SELECT 1 FROM risk_action_plan vap WHERE vap.risk_id = r.id AND vap.action_owner_id = ?)")
		args = append(args, *vis.ActionOwnerUserID)
	}
	if len(parts) == 0 {
		return " AND 1=0", nil, nil
	}
	return " AND (" + strings.Join(parts, " OR ") + ")", args, nil
}

// compileWidgetRows compiles a list dataset: one row per risk, the chosen
// columns, newest first unless sorted. It follows the same rules as an
// aggregate: ids come from the catalog, values are bound, and the visibility
// condition is always added.
func compileWidgetRows(ds widgetDataset, spec domain.WidgetSpec, visibility func() (string, []any, error)) (*CompiledWidgetQuery, error) {
	if len(spec.Measures) > 0 || len(spec.Dimensions) > 0 {
		return nil, widgetErr("a list shows columns, not measures or dimensions")
	}
	if n := len(spec.Columns); n < 1 || n > widgetMaxColumns {
		return nil, widgetErr("choose between 1 and %d columns", widgetMaxColumns)
	}
	if len(spec.Filters) > widgetMaxFilters {
		return nil, widgetErr("at most %d filters are allowed", widgetMaxFilters)
	}
	limit := spec.Limit
	switch {
	case limit < 0 || limit > ds.maxRows:
		return nil, widgetErr("a list shows at most %d rows", ds.maxRows)
	case limit == 0:
		limit = ds.maxRows
	}

	needed := map[string]bool{}
	var columns []domain.WidgetColumn
	var selects []string
	selected := map[string]bool{}
	for _, id := range spec.Columns {
		c, ok := ds.columns[id]
		if !ok {
			return nil, widgetErr("unknown column %q", id)
		}
		if selected[id] {
			return nil, widgetErr("%q is selected twice", id)
		}
		selected[id] = true
		for _, j := range c.joins {
			needed[j] = true
		}
		selects = append(selects, c.expr+" AS `"+id+"`")
		columns = append(columns, domain.WidgetColumn{ID: id, Role: "column", Kind: c.kind})
	}

	var where strings.Builder
	var args []any
	where.WriteString(" WHERE 1=1")
	where.WriteString(ds.baseWhere)
	args = append(args, ds.baseArgs...)
	for _, flt := range spec.Filters {
		f, ok := ds.fields[flt.Field]
		if !ok {
			return nil, widgetErr("%q cannot be used as a filter", flt.Field)
		}
		for _, j := range f.joins {
			needed[j] = true
		}
		clause, fargs, err := widgetFilterClause(flt, f)
		if err != nil {
			return nil, err
		}
		where.WriteString(" AND " + clause)
		args = append(args, fargs...)
	}
	visClause, visArgs, err := visibility()
	if err != nil {
		return nil, err
	}
	if ds.wrapVisibility != nil && visClause != "" {
		visClause = ds.wrapVisibility(visClause)
	}
	where.WriteString(visClause)
	args = append(args, visArgs...)

	keys := make([]string, 0, len(needed))
	for k := range needed {
		if _, ok := ds.joins[k]; !ok {
			return nil, fmt.Errorf("widget catalog: join %q is not defined", k)
		}
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return ds.joins[keys[i]].order < ds.joins[keys[j]].order })
	var joins strings.Builder
	for _, k := range keys {
		joins.WriteString(ds.joins[k].sql)
	}

	// DISTINCT needs the ordering column in the select list, so a sort is only
	// accepted on a selected column; the default is the first one.
	order := spec.Columns[0]
	dir := " ASC"
	if spec.Sort != nil {
		if !selected[spec.Sort.By] {
			return nil, widgetErr("sort must use a selected column")
		}
		order = spec.Sort.By
		if spec.Sort.Desc {
			dir = " DESC"
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "SELECT /*+ MAX_EXECUTION_TIME(%d) */ DISTINCT ", widgetMaxExecutionMS)
	sb.WriteString(strings.Join(selects, ", "))
	sb.WriteString(" " + ds.from)
	sb.WriteString(joins.String())
	sb.WriteString(where.String())
	sb.WriteString(" ORDER BY `" + order + "`" + dir + " LIMIT ?")
	args = append(args, limit+1)

	return &CompiledWidgetQuery{SQL: sb.String(), Args: args, Columns: columns, Limit: limit}, nil
}

// widgetUUID is the shape of an Asgardeo user id. Only identities in this shape
// are accepted for a person filter.
var widgetUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// widgetPersonClause builds a condition on who a risk names. The spec's values
// are identities already resolved by the GRC backend; a name is refused here, so
// nothing a person typed ever reaches the SQL. The users are matched through the
// user table, which holds only the uuid.
func widgetPersonClause(flt domain.WidgetFilter, f widgetField) (string, []any, error) {
	if flt.Op != widgetOpIn {
		return "", nil, widgetErr("%q only supports the in filter", flt.Field)
	}
	if n := len(flt.Values); n < 1 || n > widgetMaxPeople {
		return "", nil, widgetErr("%q needs between 1 and %d people", flt.Field, widgetMaxPeople)
	}
	ids := make([]any, 0, len(flt.Values))
	for _, v := range flt.Values {
		if !widgetUUID.MatchString(v) {
			return "", nil, widgetErr("%q needs people the platform has already looked up", flt.Field)
		}
		ids = append(ids, strings.ToLower(v))
	}

	ph := placeholders(len(ids))
	users := "(SELECT pu.id FROM `user` pu WHERE pu.uuid IN (" + ph + "))"
	actionOwner := "EXISTS (SELECT 1 FROM risk_action_plan pap JOIN `user` pau ON pau.id = pap.action_owner_id " +
		"WHERE pap.risk_id = r.id AND pau.uuid IN (" + ph + "))"

	repeat := func(times int) []any {
		out := make([]any, 0, len(ids)*times)
		for i := 0; i < times; i++ {
			out = append(out, ids...)
		}
		return out
	}
	switch f.person {
	case "owner":
		return "r.owner_id IN " + users, repeat(1), nil
	case "assigner":
		return "r.assigner_id IN " + users, repeat(1), nil
	case "action_owner":
		return actionOwner, repeat(1), nil
	case "control_owner":
		return "c.owner_id IN " + users, repeat(1), nil
	case "control_auditor":
		return "c.auditor_id IN " + users, repeat(1), nil
	case "control_involved":
		return "(c.owner_id IN " + users + " OR c.auditor_id IN " + users + ")", repeat(2), nil
	case "involved":
		return "(r.owner_id IN " + users + " OR r.assigner_id IN " + users +
			" OR r.management_approver_id IN " + users + " OR " + actionOwner + ")", repeat(4), nil
	}
	return "", nil, fmt.Errorf("widget catalog: unknown person kind %q", f.person)
}
