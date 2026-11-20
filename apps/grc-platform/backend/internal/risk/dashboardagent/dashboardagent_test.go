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
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func countSpec() Spec {
	return Spec{Dataset: "risks", Measures: []string{"risk_count"}}
}

func TestValidateAcceptsStarterViews(t *testing.T) {
	specs := map[string]Spec{
		"heat map": {Dataset: "risks", Measures: []string{"risk_count"},
			Dimensions: []string{"residual_likelihood", "residual_impact"}},
		"overdue remediation by action owner": {Dataset: "risks", Measures: []string{"risk_count"},
			Dimensions: []string{"action_owner"},
			Filters: []Filter{
				{Field: "workflow_status", Op: "in", Values: []string{"APPROVED"}},
				{Field: "is_overdue", Op: "is", Values: []string{"true"}},
			},
			Sort: &Sort{By: "risk_count", Desc: true}},
		"compliance coverage": {Dataset: "risks", Measures: []string{"risk_count"},
			Dimensions: []string{"compliance_reference"},
			Filters:    []Filter{{Field: "workflow_status", Op: "in", Values: []string{"APPROVED", "ESCALATED"}}}},
		"AI acceptance rate": {Dataset: "ai_suggestions", Measures: []string{"suggestion_count", "acceptance_rate"},
			Dimensions: []string{"feature"}},
		"average score by team": {Dataset: "risks", Measures: []string{"avg_residual_rating"},
			Dimensions: []string{"assignment_team"}},
	}
	for name, spec := range specs {
		if _, err := Validate(spec, nil); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestValidateCleansAndDoesNotMutate(t *testing.T) {
	in := Spec{
		Dataset:  "risks",
		Measures: []string{"risk_count"},
		Filters:  []Filter{{Field: "source_register", Op: "in", Values: []string{"  Asgardeo  "}}},
	}
	out, err := Validate(in, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Limit != DefaultLimit {
		t.Errorf("limit = %d, want %d", out.Limit, DefaultLimit)
	}
	if out.Filters[0].Values[0] != "Asgardeo" {
		t.Errorf("value not trimmed: %q", out.Filters[0].Values[0])
	}
	if in.Filters[0].Values[0] != "  Asgardeo  " || in.Limit != 0 {
		t.Error("input was modified")
	}
}

func TestValidateRejectsBadSpecs(t *testing.T) {
	long := strings.Repeat("a", MaxValueLen+1)
	cases := map[string]Spec{
		"unknown dataset":          {Dataset: "audits", Measures: []string{"risk_count"}},
		"no measures":              {Dataset: "risks"},
		"too many measures":        {Dataset: "risks", Measures: []string{"risk_count", "avg_gross_rating", "avg_residual_rating", "risk_count"}},
		"unknown measure":          {Dataset: "risks", Measures: []string{"risk_title"}},
		"unknown dimension":        {Dataset: "risks", Measures: []string{"risk_count"}, Dimensions: []string{"owner_id"}},
		"table.column":             {Dataset: "risks", Measures: []string{"risk_count"}, Dimensions: []string{"r.owner_id"}},
		"date as dimension":        {Dataset: "risks", Measures: []string{"risk_count"}, Dimensions: []string{"implementation_date"}},
		"flag as dimension":        {Dataset: "risks", Measures: []string{"risk_count"}, Dimensions: []string{"is_overdue"}},
		"too many dimensions":      {Dataset: "risks", Measures: []string{"risk_count"}, Dimensions: []string{"source_register", "assignment_team", "risk_year"}},
		"duplicate dimension":      {Dataset: "risks", Measures: []string{"risk_count"}, Dimensions: []string{"risk_year", "risk_year"}},
		"other dataset field":      {Dataset: "ai_suggestions", Measures: []string{"suggestion_count"}, Dimensions: []string{"workflow_status"}},
		"other dataset measure":    {Dataset: "ai_suggestions", Measures: []string{"risk_count"}},
		"limit too big":            {Dataset: "risks", Measures: []string{"risk_count"}, Limit: MaxLimit + 1},
		"negative limit":           {Dataset: "risks", Measures: []string{"risk_count"}, Limit: -1},
		"sort not selected":        {Dataset: "risks", Measures: []string{"risk_count"}, Sort: &Sort{By: "risk_year"}},
		"bad enum":                 {Dataset: "risks", Measures: []string{"risk_count"}, Filters: []Filter{{Field: "workflow_status", Op: "in", Values: []string{"DONE"}}}},
		"op not for text":          {Dataset: "risks", Measures: []string{"risk_count"}, Filters: []Filter{{Field: "source_register", Op: "gte", Values: []string{"a"}}}},
		"int out of range":         {Dataset: "risks", Measures: []string{"risk_count"}, Filters: []Filter{{Field: "gross_likelihood", Op: "in", Values: []string{"4"}}}},
		"int not a number":         {Dataset: "risks", Measures: []string{"risk_count"}, Filters: []Filter{{Field: "risk_year", Op: "gte", Values: []string{"2026 OR 1=1"}}}},
		"bad date":                 {Dataset: "risks", Measures: []string{"risk_count"}, Filters: []Filter{{Field: "implementation_date", Op: "lte", Values: []string{"tomorrow"}}}},
		"flag value":               {Dataset: "risks", Measures: []string{"risk_count"}, Filters: []Filter{{Field: "is_overdue", Op: "is", Values: []string{"maybe"}}}},
		"is on non-flag":           {Dataset: "risks", Measures: []string{"risk_count"}, Filters: []Filter{{Field: "risk_year", Op: "is", Values: []string{"true"}}}},
		"filter no values":         {Dataset: "risks", Measures: []string{"risk_count"}, Filters: []Filter{{Field: "source_register", Op: "in"}}},
		"value too long":           {Dataset: "risks", Measures: []string{"risk_count"}, Filters: []Filter{{Field: "source_register", Op: "in", Values: []string{long}}}},
		"too many filters":         {Dataset: "risks", Measures: []string{"risk_count"}, Filters: make([]Filter, MaxFilters+1)},
		"average on repeated rows": {Dataset: "risks", Measures: []string{"avg_residual_rating"}, Dimensions: []string{"category"}},
	}
	for name, spec := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Validate(spec, nil)
			if _, ok := err.(*ValidationError); !ok {
				t.Fatalf("expected a ValidationError, got %v", err)
			}
		})
	}
}

// A field the caller may not use must be reported exactly like one that does
// not exist, so a spec cannot be used to probe the catalog.
func TestValidateHidesFieldsWithoutPrivilege(t *testing.T) {
	saved := datasets
	defer func() { datasets = saved }()
	datasets = append(append([]DatasetDef{}, saved...), DatasetDef{
		ID: "restricted",
		Fields: []FieldDef{
			{ID: "secret_field", Kind: KindText, Group: true, RequiredPrivilege: "RISK_SECRET"},
			{ID: "open_field", Kind: KindText, Group: true},
		},
		Measures: []MeasureDef{{ID: "n", Kind: KindInt, DistinctSafe: true}},
	})

	spec := func(dim string) Spec {
		return Spec{Dataset: "restricted", Measures: []string{"n"}, Dimensions: []string{dim}}
	}

	if _, err := Validate(spec("secret_field"), func(string) bool { return true }); err != nil {
		t.Fatalf("holder of the privilege: %v", err)
	}
	_, hidden := Validate(spec("secret_field"), func(string) bool { return false })
	_, unknown := Validate(spec("no_such_field"), func(string) bool { return false })
	if hidden == nil || unknown == nil {
		t.Fatal("both should be rejected")
	}
	if strings.ReplaceAll(hidden.Error(), "secret_field", "X") != strings.ReplaceAll(unknown.Error(), "no_such_field", "X") {
		t.Errorf("a hidden field is distinguishable from an unknown one:\n %q\n %q", hidden, unknown)
	}
	if _, err := Validate(spec("secret_field"), nil); err == nil {
		t.Fatal("a nil canUse must hide privileged fields")
	}
	if _, err := Validate(spec("open_field"), nil); err != nil {
		t.Fatalf("open field: %v", err)
	}
}

func TestVisibilityNone(t *testing.T) {
	one := 1
	cases := []struct {
		vis  Visibility
		want bool
	}{
		{Visibility{}, true},
		{Visibility{TeamIDs: []int{}}, true},
		{Visibility{All: true}, false},
		{Visibility{TeamIDs: []int{2}}, false},
		{Visibility{ActionOwnerUserID: &one}, false},
	}
	for _, c := range cases {
		if got := c.vis.None(); got != c.want {
			t.Errorf("%+v: None() = %v, want %v", c.vis, got, c.want)
		}
	}
}

func TestLimiter(t *testing.T) {
	now := time.Unix(1000, 0)
	l := NewLimiter(2, time.Minute)
	l.now = func() time.Time { return now }

	if !l.Allow("u1") || !l.Allow("u1") {
		t.Fatal("first two requests should pass")
	}
	if l.Allow("u1") {
		t.Fatal("third request inside the window should be refused")
	}
	if !l.Allow("u2") {
		t.Fatal("another user is not affected")
	}
	now = now.Add(61 * time.Second)
	if !l.Allow("u1") {
		t.Fatal("the window should have passed")
	}
}

// The ids here and the ids in the Compliance Entity's catalog must be the same
// set: the entity maps each one to SQL and rejects any it does not know, so a
// mismatch is either a field that can never run or SQL nobody can reach.
func TestCatalogMatchesEntity(t *testing.T) {
	// The risk register's catalog and the Audit Hub's.
	var src string
	for _, name := range []string{"widget_catalog.go", "widget_catalog_audit.go"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "..", "entity", "compliance-entity",
			"internal", "repository", name))
		if err != nil {
			t.Skipf("entity catalog not found (%v)", err)
		}
		src += string(raw) + "\n"
	}

	mine := map[string]bool{}
	for _, d := range Datasets() {
		if !strings.Contains(src, `"`+d.ID+`": {`) {
			t.Errorf("dataset %q is not in the entity catalog", d.ID)
		}
		for _, f := range d.Fields {
			mine[f.ID] = true
			if !strings.Contains(src, `"`+f.ID+`":`) {
				t.Errorf("field %q is not in the entity catalog", f.ID)
			}
		}
		for _, c := range d.Columns {
			mine[c.ID] = true
			if !strings.Contains(src, `"`+c.ID+`":`) {
				t.Errorf("column %q is not in the entity catalog", c.ID)
			}
		}
		for _, m := range d.Measures {
			mine[m.ID] = true
			if !strings.Contains(src, `"`+m.ID+`":`) {
				t.Errorf("measure %q is not in the entity catalog", m.ID)
			}
		}
	}

	// And the other way: nothing in the entity that the backend cannot name.
	re := regexp.MustCompile(`(?m)^\s*"([a-z_]+)":\s+\{(?:expr|\n\s*expr):`)
	found := re.FindAllStringSubmatch(src, -1)
	if len(found) == 0 {
		t.Fatal("found no entity fields; the catalog layout changed and this check needs updating")
	}
	for _, m := range found {
		if !mine[m[1]] {
			t.Errorf("entity catalog has %q, which the backend catalog does not", m[1])
		}
	}
}

func TestStartersAreValid(t *testing.T) {
	starters := Starters()
	if len(starters) != 4 {
		t.Fatalf("expected 4 starters, got %d", len(starters))
	}
	seen := map[string]bool{}
	for _, st := range starters {
		if seen[st.ID] {
			t.Errorf("duplicate starter id %q", st.ID)
		}
		seen[st.ID] = true
		if _, err := ValidateWidget(st.Widget, nil); err != nil {
			t.Errorf("starter %q does not pass validation: %v", st.ID, err)
		}
	}
}

func TestOpenStatusesLeaveOutClosedAndCancelled(t *testing.T) {
	got := strings.Join(openStatuses(), ",")
	if strings.Contains(got, "CLOSED") || strings.Contains(got, "CANCELLED") || !strings.Contains(got, "APPROVED") {
		t.Fatalf("open statuses = %s", got)
	}
}

func TestValidateWidgetVisuals(t *testing.T) {
	one := Spec{Dataset: "risks", Measures: []string{"risk_count"}}
	byTeam := Spec{Dataset: "risks", Measures: []string{"risk_count"}, Dimensions: []string{"assignment_team"}}
	grid := Spec{Dataset: "risks", Measures: []string{"risk_count"}, Dimensions: []string{"residual_likelihood", "residual_impact"}}
	cases := []struct {
		name   string
		visual string
		spec   Spec
		ok     bool
	}{
		{"kpi on one number", VisualKPI, one, true},
		{"kpi with a dimension", VisualKPI, byTeam, false},
		{"bar on one dimension", VisualBar, byTeam, true},
		{"bar with no dimension", VisualBar, one, false},
		{"bar on two dimensions", VisualBar, grid, true},
		{"heatmap on two dimensions", VisualHeatmap, grid, true},
		{"heatmap on one dimension", VisualHeatmap, byTeam, false},
		{"table on anything", VisualTable, one, true},
		{"unknown visual", "pie3d", byTeam, false},
		{"empty visual", "", byTeam, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ValidateWidget(Widget{Title: "t", Visual: c.visual, Spec: c.spec}, nil)
			if (err == nil) != c.ok {
				t.Fatalf("err = %v, want ok = %v", err, c.ok)
			}
		})
	}
}

func TestValidateWidgetTitle(t *testing.T) {
	spec := Spec{Dataset: "risks", Measures: []string{"risk_count"}}
	w, err := ValidateWidget(Widget{Title: "   ", Visual: VisualKPI, Spec: spec}, nil)
	if err != nil || w.Title != "Untitled widget" {
		t.Fatalf("%+v, %v", w, err)
	}
	w, err = ValidateWidget(Widget{Title: strings.Repeat("é", 200), Visual: VisualKPI, Spec: spec}, nil)
	if err != nil || len([]rune(w.Title)) != maxTitleLen {
		t.Fatalf("title length %d, %v", len([]rune(w.Title)), err)
	}
}

func TestCatalogForLeavesOutPrivilegedEntries(t *testing.T) {
	saved := datasets
	defer func() { datasets = saved }()
	datasets = []DatasetDef{{
		ID: "d",
		Fields: []FieldDef{
			{ID: "open", Kind: KindText, Group: true},
			{ID: "secret", Kind: KindText, Group: true, RequiredPrivilege: "P"},
		},
		Measures: []MeasureDef{{ID: "m", DistinctSafe: true}, {ID: "sm", DistinctSafe: true, RequiredPrivilege: "P"}},
	}}
	for name, canUse := range map[string]func(string) bool{
		"nil":    nil,
		"denied": func(string) bool { return false },
	} {
		got := CatalogFor(canUse)[0]
		if len(got.Fields) != 1 || got.Fields[0].ID != "open" || len(got.Measures) != 1 || got.Measures[0].ID != "m" {
			t.Errorf("%s: %+v", name, got)
		}
	}
	got := CatalogFor(func(string) bool { return true })[0]
	if len(got.Fields) != 2 || len(got.Measures) != 2 {
		t.Errorf("held privilege: %+v", got)
	}
}

func TestLabelColumns(t *testing.T) {
	res := &Result{Columns: []Column{
		{ID: "assignment_team", Role: "dimension"},
		{ID: "risk_count", Role: "measure"},
		{ID: "not_in_catalog", Role: "measure"},
	}}
	LabelColumns(Spec{Dataset: "risks"}, res)
	want := []string{"Assignment Team", "Number of risks", "not_in_catalog"}
	for i, c := range res.Columns {
		if c.Label != want[i] {
			t.Errorf("column %d label = %q, want %q", i, c.Label, want[i])
		}
	}
	// An unknown dataset still leaves every column readable.
	res = &Result{Columns: []Column{{ID: "x"}}}
	LabelColumns(Spec{Dataset: "nope"}, res)
	if res.Columns[0].Label != "x" {
		t.Errorf("label = %q", res.Columns[0].Label)
	}
}

func listSpec(columns ...string) Spec { return Spec{Dataset: "risk_list", Columns: columns} }

func TestValidateList(t *testing.T) {
	spec := listSpec("risk_code", "risk_title", "residual_level")
	spec.Filters = []Filter{{Field: "workflow_status", Op: "in", Values: []string{"APPROVED"}}}
	spec.Sort = &Sort{By: "risk_code", Desc: true}
	out, err := Validate(spec, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Limit != MaxListRows || len(out.Columns) != 3 {
		t.Fatalf("%+v", out)
	}

	bad := map[string]Spec{
		"no columns":           {Dataset: "risk_list"},
		"description column":   listSpec("risk_description"),
		"owner column":         listSpec("owner_id"),
		"duplicate column":     listSpec("risk_code", "risk_code"),
		"too many columns":     listSpec("risk_code", "risk_title", "source_register", "assignment_team", "workflow_status", "treatment_strategy", "gross_level", "residual_level", "implementation_date"),
		"measures on a list":   {Dataset: "risk_list", Columns: []string{"risk_code"}, Measures: []string{"risk_count"}},
		"dimensions on a list": {Dataset: "risk_list", Columns: []string{"risk_code"}, Dimensions: []string{"risk_year"}},
		"over the row cap":     {Dataset: "risk_list", Columns: []string{"risk_code"}, Limit: MaxListRows + 1},
		"sort on unselected":   {Dataset: "risk_list", Columns: []string{"risk_code"}, Sort: &Sort{By: "risk_title"}},
		"action owner filter":  {Dataset: "risk_list", Columns: []string{"risk_code"}, Filters: []Filter{{Field: "action_owner", Op: "in", Values: []string{"x"}}}},
		"columns on a count":   {Dataset: "risks", Measures: []string{"risk_count"}, Columns: []string{"risk_code"}},
		"bad filter value":     {Dataset: "risk_list", Columns: []string{"risk_code"}, Filters: []Filter{{Field: "workflow_status", Op: "in", Values: []string{"DONE"}}}},
	}
	for name, s := range bad {
		t.Run(name, func(t *testing.T) {
			if _, err := Validate(s, nil); err == nil {
				t.Fatal("expected the spec to be rejected")
			}
		})
	}
}

func TestListIsShownAsATable(t *testing.T) {
	spec := listSpec("risk_code")
	if _, err := ValidateWidget(Widget{Title: "Risks", Visual: VisualTable, Spec: spec}, nil); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{VisualBar, VisualKPI, VisualHeatmap} {
		if _, err := ValidateWidget(Widget{Title: "Risks", Visual: v, Spec: spec}, nil); err == nil {
			t.Errorf("a list must not be drawn as %q", v)
		}
	}
}

func TestLabelColumnsForAList(t *testing.T) {
	res := &Result{Columns: []Column{{ID: "risk_code", Role: "column"}, {ID: "risk_title", Role: "column"}, {ID: "source_register", Role: "column"}}}
	LabelColumns(listSpec("risk_code"), res)
	want := []string{"Risk code", "Title", "Source Register"}
	for i, c := range res.Columns {
		if c.Label != want[i] {
			t.Errorf("column %d label = %q, want %q", i, c.Label, want[i])
		}
	}
}

func col(id, role, kind, label string) Column {
	return Column{ID: id, Role: role, Kind: kind, Label: label}
}

func TestSummarize(t *testing.T) {
	cases := []struct {
		name string
		spec Spec
		res  *Result
		want string
	}{
		{
			"one number",
			Spec{Dataset: "risks"},
			&Result{Columns: []Column{col("risk_count", "measure", KindInt, "Number of risks")}, Rows: [][]any{{float64(12)}}},
			"There are 12 risks.",
		},
		{
			"one risk, singular",
			Spec{Dataset: "risks"},
			&Result{Columns: []Column{col("risk_count", "measure", KindInt, "Number of risks")}, Rows: [][]any{{float64(1)}}},
			"There is 1 risk.",
		},
		{
			"several measures",
			Spec{Dataset: "ai_suggestions"},
			&Result{Columns: []Column{col("suggestion_count", "measure", KindInt, "n"), col("acceptance_rate", "measure", KindNumber, "Acceptance rate")}, Rows: [][]any{{float64(40), 0.425}}},
			"There are 40 AI suggestions. Users accepted 42.5% of AI suggestions (self-reported by the web app, not independently verified).",
		},
		{
			"breakdown with a total and a largest group",
			Spec{Dataset: "risks"},
			&Result{Columns: []Column{col("assignment_team", "dimension", KindText, "Assignment Team"), col("risk_count", "measure", KindInt, "Number of risks")},
				Rows: [][]any{{"Choreo", float64(7)}, {"Asgardeo", float64(5)}}},
			"There are 12 risks in total, grouped by Assignment Team. The largest group is Choreo with 7.",
		},
		{
			// A risk with two categories is in both groups, so adding them up
			// would overstate the number of risks: no total is given.
			"no total when a risk can be in two groups",
			Spec{Dataset: "risks"},
			&Result{Columns: []Column{col("category", "dimension", KindText, "Risk Category"), col("risk_count", "measure", KindInt, "Number of risks")},
				Rows: [][]any{{"Secrets", float64(7)}, {"Access", float64(5)}}},
			"The largest group by Risk Category is Secrets with 7.",
		},
		{
			"highest average",
			Spec{Dataset: "risks"},
			&Result{Columns: []Column{col("assignment_team", "dimension", KindText, "Assignment Team"), col("avg_residual_rating", "measure", KindNumber, "Average residual score")},
				Rows: [][]any{{"A", 2.0}, {"B", 2.456}}},
			"The highest average residual score is in B at 2.46.",
		},
		{
			"two dimensions give a total",
			Spec{Dataset: "risks"},
			&Result{Columns: []Column{col("residual_likelihood", "dimension", KindInt, "L"), col("residual_impact", "dimension", KindInt, "I"), col("risk_count", "measure", KindInt, "n")},
				Rows: [][]any{{float64(1), float64(1), float64(4)}, {float64(3), float64(3), float64(2)}}},
			"There are 6 risks in total across this breakdown.",
		},
		{
			"truncated breakdown gives no total",
			Spec{Dataset: "risks"},
			&Result{Truncated: true, Columns: []Column{col("assignment_team", "dimension", KindText, "Assignment Team"), col("risk_count", "measure", KindInt, "n")}, Rows: [][]any{{"A", float64(2)}}},
			"The largest group shown is A with 2. Only the first row is shown.",
		},
		{
			"a list",
			Spec{Dataset: "risk_list"},
			&Result{Columns: []Column{col("risk_code", "column", KindText, "Risk code")}, Rows: [][]any{{"A"}, {"B"}}},
			"Showing 2 risks.",
		},
		{
			"a list that was cut short",
			Spec{Dataset: "risk_list"},
			&Result{Truncated: true, Columns: []Column{col("risk_code", "column", KindText, "Risk code")}, Rows: [][]any{{"A"}, {"B"}}},
			"Showing the first 2 risks; there are more. Add a filter to narrow it down.",
		},
		{"no rows", Spec{Dataset: "risks"}, &Result{Columns: []Column{col("risk_count", "measure", KindInt, "n")}}, ""},
		{"unknown dataset", Spec{Dataset: "nope"}, &Result{Rows: [][]any{{1}}}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Summarize(c.spec, c.res); got != c.want {
				t.Fatalf("\n got: %q\nwant: %q", got, c.want)
			}
		})
	}
	if Summarize(Spec{Dataset: "risks"}, nil) != "" {
		t.Error("a nil result has no summary")
	}
}

// A cleaned spec always serializes its lists as [], never null, so the web app
// can treat them as arrays.
func TestValidateReturnsEmptyListsNotNull(t *testing.T) {
	for _, spec := range []Spec{
		{Dataset: "risks", Measures: []string{"risk_count"}},
		listSpec("risk_code"),
	} {
		out, err := Validate(spec, nil)
		if err != nil {
			t.Fatal(err)
		}
		if out.Measures == nil || out.Dimensions == nil || out.Filters == nil {
			t.Errorf("%s: nil list in %+v", spec.Dataset, out)
		}
	}
}

func TestTidyCorrectsHarmlessSlips(t *testing.T) {
	// A list asked for more rows than the cap, twice-named columns, and a sort
	// on a column it does not show.
	got := Tidy(Spec{
		Dataset: "risk_list", Columns: []string{"risk_code", "risk_code", "risk_title"},
		Limit: 100, Sort: &Sort{By: "risk_year"},
	})
	if got.Limit != MaxListRows || len(got.Columns) != 2 || got.Sort != nil {
		t.Fatalf("%+v", got)
	}
	// A sort on a shown column survives.
	got = Tidy(Spec{Dataset: "risk_list", Columns: []string{"risk_code"}, Sort: &Sort{By: "risk_code", Desc: true}})
	if got.Sort == nil || !got.Sort.Desc {
		t.Fatalf("a valid sort was dropped: %+v", got)
	}
	// Counts: the cap is the larger one, and repeated fields collapse.
	got = Tidy(Spec{Dataset: "risks", Measures: []string{"risk_count", "risk_count"}, Dimensions: []string{"risk_year", "risk_year"}, Limit: 9999})
	if got.Limit != MaxLimit || len(got.Measures) != 1 || len(got.Dimensions) != 1 {
		t.Fatalf("%+v", got)
	}
	// Too many columns are trimmed to the maximum.
	cols := []string{"risk_code", "risk_title", "source_register", "assignment_team", "workflow_status", "treatment_strategy", "gross_level", "residual_level", "implementation_date", "risk_year"}
	if got := Tidy(Spec{Dataset: "risk_list", Columns: cols}); len(got.Columns) != MaxListColumns {
		t.Fatalf("%d columns", len(got.Columns))
	}
}

// Tidy never lets through something Validate would refuse for a protective
// reason: unknown ids and bad values are still rejected afterwards.
func TestTidyDoesNotRelaxValidation(t *testing.T) {
	for name, spec := range map[string]Spec{
		"unknown column": {Dataset: "risk_list", Columns: []string{"risk_description"}},
		"unknown field":  {Dataset: "risks", Measures: []string{"risk_count"}, Dimensions: []string{"owner_id"}},
		"bad filter":     {Dataset: "risks", Measures: []string{"risk_count"}, Filters: []Filter{{Field: "risk_year", Op: "gte", Values: []string{"1 OR 1=1"}}}},
		"other dataset":  {Dataset: "audits", Measures: []string{"risk_count"}},
	} {
		if _, err := Validate(Tidy(spec), nil); err == nil {
			t.Errorf("%s: Tidy made an invalid spec valid", name)
		}
	}
}

func TestFitVisual(t *testing.T) {
	one := Spec{Dataset: "risks", Measures: []string{"risk_count"}}
	byTeam := Spec{Dataset: "risks", Measures: []string{"risk_count"}, Dimensions: []string{"assignment_team"}}
	grid := Spec{Dataset: "risks", Measures: []string{"risk_count"}, Dimensions: []string{"residual_likelihood", "residual_impact"}}
	several := Spec{Dataset: "risks", Measures: []string{"risk_count", "avg_gross_rating"}}
	cases := []struct {
		name   string
		visual string
		spec   Spec
		want   string
	}{
		{"a suitable visual is kept", VisualBar, byTeam, VisualBar},
		{"a list is always a table", VisualBar, listSpec("risk_code"), VisualTable},
		{"an unknown visual", "list", one, VisualKPI},
		{"a bar with no dimension", VisualBar, one, VisualKPI},
		{"a heat map on one dimension", VisualHeatmap, byTeam, VisualBar},
		{"a tile on a breakdown", VisualKPI, byTeam, VisualBar},
		{"a bar on two dimensions", "pie", grid, VisualHeatmap},
		{"nothing fits", "pie", several, VisualTable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := FitVisual(c.visual, c.spec)
			if got != c.want {
				t.Fatalf("FitVisual(%q) = %q, want %q", c.visual, got, c.want)
			}
			// Whatever it picks must be accepted.
			spec := c.spec
			if spec.Limit == 0 {
				spec.Limit = 10
			}
			if _, err := ValidateWidget(Widget{Title: "t", Visual: got, Spec: spec}, nil); err != nil {
				t.Fatalf("the chosen visual is not valid: %v", err)
			}
		})
	}
}

type fakePeople map[string][]Person

func (f fakePeople) SearchPeople(q string) []Person { return f[strings.ToLower(q)] }

func personSpec(values ...string) Spec {
	return Spec{Dataset: "risk_list", Columns: []string{"risk_code"},
		Filters: []Filter{{Field: "involved_person", Op: "in", Values: values}}}
}

func TestValidatePersonFilters(t *testing.T) {
	if _, err := Validate(personSpec("tharushi"), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := Validate(Spec{Dataset: "risks", Measures: []string{"risk_count"},
		Filters: []Filter{{Field: "owner_person", Op: "in", Values: []string{"me"}}}}, nil); err != nil {
		t.Fatal(err)
	}
	for name, s := range map[string]Spec{
		"too many people": personSpec("a", "b", "c", "d", "e", "f"),
		"wrong op":        {Dataset: "risk_list", Columns: []string{"risk_code"}, Filters: []Filter{{Field: "involved_person", Op: "gte", Values: []string{"a"}}}},
		"as a dimension":  {Dataset: "risks", Measures: []string{"risk_count"}, Dimensions: []string{"involved_person"}},
		"empty name":      personSpec("  "),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Validate(s, nil); err == nil {
				t.Fatal("expected the spec to be rejected")
			}
		})
	}
}

func TestResolvePeople(t *testing.T) {
	dir := fakePeople{
		"tharushi":   {{UUID: "uuid-tharushi", Name: "Tharushi Pushpakumara", Email: "tharuship@wso2.com"}},
		"ann":        {{UUID: "uuid-ann-1", Name: "Ann Perera"}, {UUID: "uuid-ann-2", Name: "Anna Silva"}},
		"ann perera": {{UUID: "uuid-ann-1", Name: "Ann Perera"}},
		"sam":        {{UUID: "uuid-sam-1", Name: "Sam One"}, {UUID: "uuid-sam-2", Name: "Sam Two"}, {UUID: "uuid-sam-3", Name: "Sam Three"}, {UUID: "uuid-sam-4", Name: "Sam Four"}, {UUID: "uuid-sam-5", Name: "Sam Five"}, {UUID: "uuid-sam-6", Name: "Sam Six"}},
	}

	got, problem := ResolvePeople(personSpec("Tharushi"), "uuid-caller", dir)
	if problem != "" || got.Filters[0].Values[0] != "uuid-tharushi" {
		t.Fatalf("a single match: %+v, %q", got, problem)
	}

	got, problem = ResolvePeople(personSpec("me"), "uuid-caller", dir)
	if problem != "" || got.Filters[0].Values[0] != "uuid-caller" {
		t.Fatalf("me: %+v, %q", got, problem)
	}
	if _, problem = ResolvePeople(personSpec("my"), "", dir); problem == "" {
		t.Error("'my' with no known caller must not be guessed")
	}

	if _, problem = ResolvePeople(personSpec("nobody"), "u", dir); !strings.Contains(problem, "couldn't find anyone called “nobody”") {
		t.Errorf("no match: %q", problem)
	}
	// Two people match: ask, and name them; never pick one.
	_, problem = ResolvePeople(personSpec("ann"), "u", dir)
	if !strings.Contains(problem, "Ann Perera") || !strings.Contains(problem, "Anna Silva") || !strings.Contains(problem, "Which one") {
		t.Errorf("ambiguous: %q", problem)
	}
	// A full name settles it even when the short one is ambiguous.
	got, problem = ResolvePeople(personSpec("Ann Perera"), "u", dir)
	if problem != "" || got.Filters[0].Values[0] != "uuid-ann-1" {
		t.Errorf("full name: %+v, %q", got, problem)
	}
	// Too many matches: do not list them all.
	if _, problem = ResolvePeople(personSpec("sam"), "u", dir); !strings.Contains(problem, "Several people match") || strings.Contains(problem, "Sam One") {
		t.Errorf("many: %q", problem)
	}
	if _, problem = ResolvePeople(personSpec("tharushi"), "u", nil); problem == "" {
		t.Error("no directory must be reported, not guessed around")
	}
}

func TestResolvePeopleLeavesTheOriginalAndOtherFiltersAlone(t *testing.T) {
	dir := fakePeople{"tharushi": {{UUID: "uuid-tharushi", Name: "Tharushi"}}}
	in := Spec{Dataset: "risk_list", Columns: []string{"risk_code"}, Filters: []Filter{
		{Field: "workflow_status", Op: "in", Values: []string{"APPROVED"}},
		{Field: "involved_person", Op: "in", Values: []string{"tharushi", "me", "tharushi"}},
	}}
	out, problem := ResolvePeople(in, "uuid-me", dir)
	if problem != "" {
		t.Fatal(problem)
	}
	if in.Filters[1].Values[0] != "tharushi" {
		t.Error("the input spec was modified")
	}
	if out.Filters[0].Values[0] != "APPROVED" {
		t.Error("another filter was changed")
	}
	if got := out.Filters[1].Values; len(got) != 2 || got[0] != "uuid-tharushi" || got[1] != "uuid-me" {
		t.Errorf("resolved people = %v (duplicates must collapse)", got)
	}
	// A spec with no person filter is untouched.
	plain := Spec{Dataset: "risks", Measures: []string{"risk_count"}}
	if got, p := ResolvePeople(plain, "", nil); p != "" || got.Dataset != "risks" {
		t.Errorf("%+v, %q", got, p)
	}
}

func TestVisibilityKind(t *testing.T) {
	one := 1
	for want, v := range map[string]Visibility{
		"all":        {All: true},
		"team":       {TeamIDs: []int{2}},
		"owned":      {ActionOwnerUserID: &one},
		"team+owned": {TeamIDs: []int{2}, ActionOwnerUserID: &one},
		"none":       {},
	} {
		if got := v.Kind(); got != want {
			t.Errorf("%+v: Kind() = %q, want %q", v, got, want)
		}
	}
}

// The risk id exists so the web app can link a row. Only the platform adds it:
// the model is never told about it, and a request that names it is refused.
func TestTheLinkColumnIsInternal(t *testing.T) {
	if _, err := Validate(listSpec("risk_code", "risk_id"), nil); err == nil {
		t.Error("a request must not be able to ask for the internal risk id")
	}
	for _, ds := range CatalogFor(nil) {
		for _, c := range ds.Columns {
			if c.ID == "risk_id" {
				t.Errorf("the model is offered the internal column in %s", ds.ID)
			}
		}
	}
	found := false
	for _, ds := range Datasets() {
		if c, ok := ds.column("risk_id"); ok && c.Internal {
			found = true
		}
	}
	if !found {
		t.Error("risk_id should be defined as an internal column")
	}
}

func TestWithLinkColumn(t *testing.T) {
	got := WithLinkColumn(listSpec("risk_code", "risk_title"))
	if len(got.Columns) != 3 || got.Columns[2] != "risk_id" {
		t.Fatalf("%v", got.Columns)
	}
	// the input is not modified
	in := listSpec("risk_code")
	_ = WithLinkColumn(in)
	if len(in.Columns) != 1 {
		t.Fatal("the input spec was modified")
	}
	for name, spec := range map[string]Spec{
		"no risk code to link": listSpec("risk_title"),
		"already there":        {Dataset: "risk_list", Columns: []string{"risk_code", "risk_id"}},
		"a count":              {Dataset: "risks", Measures: []string{"risk_count"}},
		"unknown dataset":      {Dataset: "nope", Columns: []string{"risk_code"}},
	} {
		if out := WithLinkColumn(spec); len(out.Columns) != len(spec.Columns) {
			t.Errorf("%s: columns changed to %v", name, out.Columns)
		}
	}
}

// A hub's agent can only reach its own datasets, and each hub's catalog holds
// only its own.
func TestModulesAreSeparate(t *testing.T) {
	audit := Spec{Dataset: "controls", Measures: []string{"control_count"}}
	if _, err := Validate(audit, nil); err == nil {
		t.Fatal("the risk agent reached an audit dataset")
	}
	if _, err := ValidateIn(ModuleAudit, audit, nil); err != nil {
		t.Fatalf("the audit agent cannot reach its own dataset: %v", err)
	}
	if _, err := ValidateIn(ModuleAudit, Spec{Dataset: "risks", Measures: []string{"risk_count"}}, nil); err == nil {
		t.Fatal("the audit agent reached the risk register")
	}
	if _, err := ValidateIn(ModuleAudit, Spec{Dataset: "ai_suggestions", Measures: []string{"suggestion_count"}}, nil); err == nil {
		t.Fatal("the audit agent reached risk AI suggestions")
	}
	for _, ds := range CatalogFor(nil) {
		if ds.Module != ModuleRisk {
			t.Errorf("the risk catalog offers %q", ds.ID)
		}
	}
	got := map[string]bool{}
	for _, ds := range CatalogForModule(ModuleAudit, nil) {
		got[ds.ID] = true
	}
	for _, want := range []string{"controls", "control_list", "evidence_rounds", "audits", "audit_list"} {
		if !got[want] {
			t.Errorf("the audit catalog lacks %q", want)
		}
	}
	if len(got) != 5 {
		t.Errorf("the audit catalog has %d datasets, want 5", len(got))
	}
}

func TestAuditScopeNone(t *testing.T) {
	for scope, none := range map[string]bool{"": true, "none": true, "everything": true, "all": false, "team": false, "owned": false, "assigned": false} {
		if got := (AuditScope{Scope: scope}).None(); got != none {
			t.Errorf("%q: None() = %v, want %v", scope, got, none)
		}
	}
}

func TestAuditSpecsValidateAndSummarise(t *testing.T) {
	for _, spec := range []Spec{
		{Dataset: "controls", Measures: []string{"control_count"}, Dimensions: []string{"team"}},
		{Dataset: "controls", Measures: []string{"overdue_count"}, Dimensions: []string{"owner"}},
		{Dataset: "evidence_rounds", Measures: []string{"evidence_count"}, Dimensions: []string{"evidence_status"}},
		{Dataset: "audits", Measures: []string{"audit_count"}, Dimensions: []string{"framework"}},
		{Dataset: "audit_list", Columns: []string{"framework"}, Filters: []Filter{{Field: "audit_status", Op: "in", Values: []string{"ACTIVE"}}}},
		{Dataset: "control_list", Columns: []string{"control_number", "control_status"}, Filters: []Filter{{Field: "is_overdue", Op: "is", Values: []string{"true"}}}},
	} {
		if _, err := ValidateIn(ModuleAudit, spec, nil); err != nil {
			t.Errorf("%s: %v", spec.Dataset, err)
		}
	}
	// The sentences are written from the numbers.
	res := &Result{
		Columns: []Column{{ID: "team", Role: "dimension", Label: "Team"}, {ID: "control_count", Role: "measure", Kind: KindInt, Label: "Number of controls"}},
		Rows:    [][]any{{"Security", int64(7)}, {"Infra", int64(3)}},
	}
	got := Summarize(Spec{Dataset: "controls", Measures: []string{"control_count"}, Dimensions: []string{"team"}}, res)
	if !strings.Contains(got, "There are 10 controls") || !strings.Contains(got, "Security") {
		t.Errorf("%q", got)
	}
	list := &Result{Columns: []Column{{ID: "control_number", Role: "column"}}, Rows: [][]any{{"A.1"}}}
	if got := Summarize(Spec{Dataset: "control_list", Columns: []string{"control_number"}}, list); got != "Showing 1 control." {
		t.Errorf("%q", got)
	}
}

func TestWithControlLinkColumns(t *testing.T) {
	spec := Spec{Dataset: "control_list", Columns: []string{"control_number", "audit"}}
	got := WithControlLinkColumns(spec)
	if len(got.Columns) != 4 || got.Columns[2] != "audit_id" || got.Columns[3] != "control_id" {
		t.Fatalf("%v", got.Columns)
	}
	if len(spec.Columns) != 2 {
		t.Fatal("the input was modified")
	}
	// Without a control number there is nothing to link; already added is not repeated.
	for _, s := range []Spec{
		{Dataset: "control_list", Columns: []string{"audit"}},
		{Dataset: "control_list", Columns: []string{"control_number", "audit_id"}},
		{Dataset: "controls", Measures: []string{"control_count"}},
		{Dataset: "audit_list", Columns: []string{"audit"}},
	} {
		if out := WithControlLinkColumns(s); len(out.Columns) != len(s.Columns) {
			t.Errorf("%+v changed to %v", s, out.Columns)
		}
	}
	// The ids are the platform's alone.
	for _, col := range []string{"audit_id", "control_id"} {
		if _, err := ValidateIn(ModuleAudit, Spec{Dataset: "control_list", Columns: []string{"control_number", col}}, nil); err == nil {
			t.Errorf("%s can be requested", col)
		}
	}
	for _, spec := range []Spec{
		{Dataset: "controls", Measures: []string{"overdue_rate"}, Dimensions: []string{"team"}},
		{Dataset: "controls", Measures: []string{"control_count"}, Filters: []Filter{{Field: "has_owner", Op: "is", Values: []string{"false"}}}},
		{Dataset: "controls", Measures: []string{"control_count"}, Filters: []Filter{{Field: "updated_date", Op: "lte", Values: []string{"2026-10-13"}}}},
		{Dataset: "evidence_rounds", Measures: []string{"rejection_rate"}, Dimensions: []string{"auditor"}},
		{Dataset: "control_list", Columns: []string{"control_number", "age_days"}, Sort: &Sort{By: "age_days", Desc: true}},
	} {
		if _, err := ValidateIn(ModuleAudit, spec, nil); err != nil {
			t.Errorf("%+v: %v", spec, err)
		}
	}
	for _, spec := range []Spec{
		{Dataset: "risks", Measures: []string{"avg_age_days"}, Dimensions: []string{"assignment_team"}},
		{Dataset: "risks", Measures: []string{"risk_count"}, Filters: []Filter{{Field: "has_action_owner", Op: "is", Values: []string{"false"}}}},
		{Dataset: "risk_list", Columns: []string{"risk_code", "age_days", "created_date"}, Sort: &Sort{By: "age_days", Desc: true}, Filters: []Filter{{Field: "updated_date", Op: "lte", Values: []string{"2026-10-13"}}}},
	} {
		if _, err := Validate(spec, nil); err != nil {
			t.Errorf("%+v: %v", spec, err)
		}
	}
}

func TestRateSentences(t *testing.T) {
	col := Column{ID: "overdue_rate", Role: "measure", Kind: KindNumber, Label: "Share overdue"}
	if got := measureSentence(col, 0.125); got != "12.5% of controls are overdue." {
		t.Errorf("%q", got)
	}
	col.ID = "rejection_rate"
	if got := measureSentence(col, 0.5); got != "50% of evidence submissions were rejected." {
		t.Errorf("%q", got)
	}
}

// Acceptance figures come from the "used" and decision values the web app
// sends back, so every place they are shown says they are self-reported.
func TestAcceptanceStatisticsAreMarkedSelfReported(t *testing.T) {
	var found bool
	for _, w := range Starters() {
		if w.ID != "ai-acceptance" {
			continue
		}
		found = true
		if !strings.Contains(w.Description, "Self-reported") {
			t.Errorf("ai-acceptance description is not marked self-reported: %q", w.Description)
		}
	}
	if !found {
		t.Fatal("ai-acceptance view not found")
	}
	for _, ds := range Datasets() {
		if ds.ID == "ai_suggestions" && !strings.Contains(ds.Label, "reported by the web app") {
			t.Errorf("ai_suggestions label is not marked self-reported: %q", ds.Label)
		}
	}
}
