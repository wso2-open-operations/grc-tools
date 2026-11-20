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
	"strings"
	"testing"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

func widgetIntPtr(n int) *int { return &n }

func heatmapSpec() domain.WidgetSpec {
	return domain.WidgetSpec{
		Dataset:    "risks",
		Dimensions: []string{"residual_likelihood", "residual_impact"},
		Measures:   []string{"risk_count"},
	}
}

func TestWidgetVisibilityClause(t *testing.T) {
	cases := []struct {
		name     string
		vis      domain.WidgetVisibility
		wantSQL  string
		wantArgs int
		wantErr  bool
	}{
		{"all sees every risk", domain.WidgetVisibility{All: true}, "", 0, false},
		{"zero value sees nothing", domain.WidgetVisibility{}, " AND 1=0", 0, false},
		{"empty team list is not unrestricted", domain.WidgetVisibility{TeamIDs: []int{}}, " AND 1=0", 0, false},
		{
			"teams match source register or assignment team",
			domain.WidgetVisibility{TeamIDs: []int{3, 5}},
			" AND ((r.source_register_id IN (?,?) OR r.assignment_team_id IN (?,?)))", 4, false,
		},
		{
			"action owner only",
			domain.WidgetVisibility{ActionOwnerUserID: widgetIntPtr(9)},
			" AND (EXISTS (SELECT 1 FROM risk_action_plan vap WHERE vap.risk_id = r.id AND vap.action_owner_id = ?))", 1, false,
		},
		{"non-positive team is refused", domain.WidgetVisibility{TeamIDs: []int{3, 0}}, "", 0, true},
		{"non-positive user is refused", domain.WidgetVisibility{ActionOwnerUserID: widgetIntPtr(0)}, "", 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sql, args, err := widgetVisibilityClause(c.vis)
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, c.wantErr)
			}
			if err == nil && (sql != c.wantSQL || len(args) != c.wantArgs) {
				t.Fatalf("got (%q, %d args), want (%q, %d args)", sql, len(args), c.wantSQL, c.wantArgs)
			}
		})
	}
}

// All is the only way past the visibility condition: with anything else the
// statement must carry it, whatever the spec asks for.
func TestCompileAlwaysAppliesVisibility(t *testing.T) {
	specs := []domain.WidgetSpec{
		heatmapSpec(),
		{Dataset: "risks", Measures: []string{"risk_count"}},
		{Dataset: "risks", Measures: []string{"risk_count"}, Dimensions: []string{"assignment_team"},
			Filters: []domain.WidgetFilter{{Field: "is_overdue", Op: "is", Values: []string{"true"}}}},
		{Dataset: "ai_suggestions", Measures: []string{"acceptance_rate"}, Dimensions: []string{"feature"}},
	}
	for _, spec := range specs {
		for name, vis := range map[string]domain.WidgetVisibility{
			"zero":    {},
			"teams":   {TeamIDs: []int{4}},
			"actionO": {ActionOwnerUserID: widgetIntPtr(2)},
		} {
			q, err := CompileWidgetQuery(domain.WidgetQueryRequest{Spec: spec, Visibility: vis})
			if err != nil {
				t.Fatalf("%s/%s: %v", spec.Dataset, name, err)
			}
			if name == "zero" && !strings.Contains(q.SQL, "1=0") {
				t.Errorf("%s/zero: no deny-all condition in %s", spec.Dataset, q.SQL)
			}
			if name == "teams" && !strings.Contains(q.SQL, "r.source_register_id IN") {
				t.Errorf("%s/teams: team condition missing in %s", spec.Dataset, q.SQL)
			}
			if name == "actionO" && !strings.Contains(q.SQL, "vap.action_owner_id = ?") {
				t.Errorf("%s/actionO: action owner condition missing in %s", spec.Dataset, q.SQL)
			}
		}
		q, err := CompileWidgetQuery(domain.WidgetQueryRequest{Spec: spec, Visibility: domain.WidgetVisibility{All: true}})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(q.SQL, "1=0") || strings.Contains(q.SQL, "vap.") {
			t.Errorf("All should add no visibility condition: %s", q.SQL)
		}
	}
}

func TestCompileHeatmapSnapshot(t *testing.T) {
	q, err := CompileWidgetQuery(domain.WidgetQueryRequest{
		Spec:       heatmapSpec(),
		Visibility: domain.WidgetVisibility{TeamIDs: []int{3, 5}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "SELECT /*+ MAX_EXECUTION_TIME(5000) */ rs.likelihood AS `residual_likelihood`, rs.impact AS `residual_impact`, " +
		"COUNT(DISTINCT r.id) AS `risk_count` FROM risk r " +
		"LEFT JOIN risk_score rs ON rs.id = COALESCE((SELECT ra.score_id FROM risk_assessment ra WHERE ra.risk_id = r.id " +
		"ORDER BY ra.created_at DESC, ra.id DESC LIMIT 1), r.gross_score_id) " +
		"WHERE 1=1 AND ((r.source_register_id IN (?,?) OR r.assignment_team_id IN (?,?))) " +
		"GROUP BY rs.likelihood, rs.impact ORDER BY `residual_likelihood` ASC LIMIT ?"
	if q.SQL != want {
		t.Fatalf("SQL mismatch\n got: %s\nwant: %s", q.SQL, want)
	}
	if len(q.Args) != 5 || q.Args[4] != widgetDefaultLimit+1 || q.Limit != widgetDefaultLimit {
		t.Fatalf("args = %v, limit = %d", q.Args, q.Limit)
	}
	if len(q.Columns) != 3 || q.Columns[2].Role != "measure" || q.Columns[0].Role != "dimension" {
		t.Fatalf("columns = %+v", q.Columns)
	}
}

func TestCompileOverdueByActionOwner(t *testing.T) {
	q, err := CompileWidgetQuery(domain.WidgetQueryRequest{
		Spec: domain.WidgetSpec{
			Dataset:    "risks",
			Dimensions: []string{"action_owner"},
			Measures:   []string{"risk_count"},
			Filters: []domain.WidgetFilter{
				{Field: "workflow_status", Op: "in", Values: []string{"APPROVED"}},
				{Field: "is_overdue", Op: "is", Values: []string{"true"}},
			},
			Sort: &domain.WidgetSort{By: "risk_count", Desc: true},
		},
		Visibility: domain.WidgetVisibility{All: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, frag := range []string{
		"LEFT JOIN risk_action_plan ap ON ap.risk_id = r.id",
		"LEFT JOIN `user` au ON au.id = ap.action_owner_id",
		"au.uuid AS `action_owner`",
		"r.workflow_status IN (?)",
		"r.implementation_date < CURDATE()",
		"ORDER BY `risk_count` DESC",
	} {
		if !strings.Contains(q.SQL, frag) {
			t.Errorf("missing %q in %s", frag, q.SQL)
		}
	}
	if len(q.Args) != 2 || q.Args[0] != "APPROVED" {
		t.Fatalf("args = %v", q.Args)
	}
}

func TestCompileAISuggestionsExcludesSystemDecisions(t *testing.T) {
	q, err := CompileWidgetQuery(domain.WidgetQueryRequest{
		Spec:       domain.WidgetSpec{Dataset: "ai_suggestions", Dimensions: []string{"feature"}, Measures: []string{"suggestion_count", "acceptance_rate"}},
		Visibility: domain.WidgetVisibility{All: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(q.SQL, "s.status IN ('ACCEPTED', 'OVERRIDDEN')") || !strings.Contains(q.SQL, "NOT IN (?, ?)") {
		t.Fatalf("decided-only / system exclusion missing: %s", q.SQL)
	}
	if q.Args[0] != "system:likelihood-recheck-job" || q.Args[1] != "likelihood-recheck-job" {
		t.Fatalf("args = %v", q.Args)
	}
}

// Values from the spec are only ever bound, never written into the statement.
func TestCompileKeepsValuesOutOfSQL(t *testing.T) {
	evil := "x'; DROP TABLE risk; --"
	q, err := CompileWidgetQuery(domain.WidgetQueryRequest{
		Spec: domain.WidgetSpec{
			Dataset:  "risks",
			Measures: []string{"risk_count"},
			Filters:  []domain.WidgetFilter{{Field: "source_register", Op: "in", Values: []string{evil}}},
		},
		Visibility: domain.WidgetVisibility{All: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(q.SQL, "DROP") || strings.Contains(q.SQL, evil) {
		t.Fatalf("value leaked into SQL: %s", q.SQL)
	}
	if q.Args[0] != evil {
		t.Fatalf("value not bound: %v", q.Args)
	}
}

func TestCompileRejectsBadSpecs(t *testing.T) {
	all := domain.WidgetVisibility{All: true}
	count := []string{"risk_count"}
	cases := []struct {
		name string
		spec domain.WidgetSpec
	}{
		{"unknown dataset", domain.WidgetSpec{Dataset: "user", Measures: count}},
		{"no measures", domain.WidgetSpec{Dataset: "risks"}},
		{"too many measures", domain.WidgetSpec{Dataset: "risks", Measures: []string{"risk_count", "avg_gross_rating", "avg_residual_rating", "risk_count"}}},
		{"unknown measure", domain.WidgetSpec{Dataset: "risks", Measures: []string{"risk_title"}}},
		{"unknown dimension", domain.WidgetSpec{Dataset: "risks", Measures: count, Dimensions: []string{"owner_id"}}},
		{"column name is not a field", domain.WidgetSpec{Dataset: "risks", Measures: count, Dimensions: []string{"r.owner_id"}}},
		{"date is not a dimension", domain.WidgetSpec{Dataset: "risks", Measures: count, Dimensions: []string{"implementation_date"}}},
		{"flag is not a dimension", domain.WidgetSpec{Dataset: "risks", Measures: count, Dimensions: []string{"is_overdue"}}},
		{"too many dimensions", domain.WidgetSpec{Dataset: "risks", Measures: count, Dimensions: []string{"source_register", "assignment_team", "risk_year"}}},
		{"duplicate dimension", domain.WidgetSpec{Dataset: "risks", Measures: count, Dimensions: []string{"risk_year", "risk_year"}}},
		{"other dataset's field", domain.WidgetSpec{Dataset: "ai_suggestions", Measures: []string{"suggestion_count"}, Dimensions: []string{"workflow_status"}}},
		{"other dataset's measure", domain.WidgetSpec{Dataset: "ai_suggestions", Measures: count}},
		{"limit too big", domain.WidgetSpec{Dataset: "risks", Measures: count, Limit: widgetMaxLimit + 1}},
		{"negative limit", domain.WidgetSpec{Dataset: "risks", Measures: count, Limit: -1}},
		{"sort by something not selected", domain.WidgetSpec{Dataset: "risks", Measures: count, Sort: &domain.WidgetSort{By: "risk_year"}}},
		{"bad enum value", domain.WidgetSpec{Dataset: "risks", Measures: count, Filters: []domain.WidgetFilter{{Field: "workflow_status", Op: "in", Values: []string{"NOT_A_STATUS"}}}}},
		{"unsupported op for text", domain.WidgetSpec{Dataset: "risks", Measures: count, Filters: []domain.WidgetFilter{{Field: "source_register", Op: "gte", Values: []string{"a"}}}}},
		{"int out of range", domain.WidgetSpec{Dataset: "risks", Measures: count, Filters: []domain.WidgetFilter{{Field: "gross_likelihood", Op: "in", Values: []string{"4"}}}}},
		{"int not a number", domain.WidgetSpec{Dataset: "risks", Measures: count, Filters: []domain.WidgetFilter{{Field: "risk_year", Op: "gte", Values: []string{"2026 OR 1=1"}}}}},
		{"bad date", domain.WidgetSpec{Dataset: "risks", Measures: count, Filters: []domain.WidgetFilter{{Field: "implementation_date", Op: "lte", Values: []string{"tomorrow"}}}}},
		{"flag needs true or false", domain.WidgetSpec{Dataset: "risks", Measures: count, Filters: []domain.WidgetFilter{{Field: "is_overdue", Op: "is", Values: []string{"maybe"}}}}},
		{"is on a non-flag", domain.WidgetSpec{Dataset: "risks", Measures: count, Filters: []domain.WidgetFilter{{Field: "risk_year", Op: "is", Values: []string{"true"}}}}},
		{"filter without values", domain.WidgetSpec{Dataset: "risks", Measures: count, Filters: []domain.WidgetFilter{{Field: "source_register", Op: "in"}}}},
		{"value too long", domain.WidgetSpec{Dataset: "risks", Measures: count, Filters: []domain.WidgetFilter{{Field: "source_register", Op: "in", Values: []string{strings.Repeat("a", widgetMaxValueLen+1)}}}}},
		{"too many filters", domain.WidgetSpec{Dataset: "risks", Measures: count, Filters: make([]domain.WidgetFilter, widgetMaxFilters+1)}},
		{"average on rows a join repeats", domain.WidgetSpec{Dataset: "risks", Measures: []string{"avg_residual_rating"}, Dimensions: []string{"category"}}},
		{"average filtered on a repeating join", domain.WidgetSpec{Dataset: "risks", Measures: []string{"avg_gross_rating"}, Filters: []domain.WidgetFilter{{Field: "compliance_reference", Op: "in", Values: []string{"ISO 27001"}}}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := CompileWidgetQuery(domain.WidgetQueryRequest{Spec: c.spec, Visibility: all})
			if err == nil {
				t.Fatal("expected the spec to be rejected")
			}
			if _, ok := err.(*apierror.ValidationError); !ok {
				t.Fatalf("expected a validation error, got %T: %v", err, err)
			}
		})
	}
}

func TestCompileAllowsAverageWithoutRepeatingJoin(t *testing.T) {
	_, err := CompileWidgetQuery(domain.WidgetQueryRequest{
		Spec:       domain.WidgetSpec{Dataset: "risks", Measures: []string{"avg_residual_rating"}, Dimensions: []string{"assignment_team"}},
		Visibility: domain.WidgetVisibility{All: true},
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Every dimension of every dataset compiles to a statement that is a single
// SELECT with one bound value per placeholder.
func TestCompileEveryFieldIsWellFormed(t *testing.T) {
	for dsName, ds := range widgetDatasets {
		if ds.rows || ds.module != "" {
			continue // lists have their own test, TestCompileEveryListColumnIsWellFormed; audit datasets have theirs
		}
		var measure string
		for id, m := range ds.measures {
			if m.distinctSafe {
				measure = id
				break
			}
		}
		for id, f := range ds.fields {
			if !f.group {
				continue
			}
			q, err := CompileWidgetQuery(domain.WidgetQueryRequest{
				Spec:       domain.WidgetSpec{Dataset: dsName, Dimensions: []string{id}, Measures: []string{measure}},
				Visibility: domain.WidgetVisibility{TeamIDs: []int{1}},
			})
			if err != nil {
				t.Fatalf("%s.%s: %v", dsName, id, err)
			}
			if !strings.HasPrefix(q.SQL, "SELECT /*+ MAX_EXECUTION_TIME(") || strings.Contains(q.SQL, ";") {
				t.Errorf("%s.%s: not a single SELECT: %s", dsName, id, q.SQL)
			}
			if n := strings.Count(q.SQL, "?"); n != len(q.Args) {
				t.Errorf("%s.%s: %d placeholders, %d args", dsName, id, n, len(q.Args))
			}
		}
	}
}

func listSpec(columns ...string) domain.WidgetSpec {
	return domain.WidgetSpec{Dataset: "risk_list", Columns: columns}
}

func TestCompileRiskListSnapshot(t *testing.T) {
	spec := listSpec("risk_code", "risk_title", "residual_level")
	spec.Filters = []domain.WidgetFilter{{Field: "workflow_status", Op: "in", Values: []string{"APPROVED"}}}
	spec.Sort = &domain.WidgetSort{By: "risk_code", Desc: true}
	spec.Limit = 10
	q, err := CompileWidgetQuery(domain.WidgetQueryRequest{Spec: spec, Visibility: domain.WidgetVisibility{TeamIDs: []int{3}}})
	if err != nil {
		t.Fatal(err)
	}
	want := "SELECT /*+ MAX_EXECUTION_TIME(5000) */ DISTINCT r.risk_code AS `risk_code`, r.risk_title AS `risk_title`, rs.risk_level AS `residual_level` " +
		"FROM risk r " +
		"LEFT JOIN risk_score rs ON rs.id = COALESCE((SELECT ra.score_id FROM risk_assessment ra WHERE ra.risk_id = r.id " +
		"ORDER BY ra.created_at DESC, ra.id DESC LIMIT 1), r.gross_score_id) " +
		"WHERE 1=1 AND r.workflow_status IN (?) AND ((r.source_register_id IN (?) OR r.assignment_team_id IN (?))) " +
		"ORDER BY `risk_code` DESC LIMIT ?"
	if q.SQL != want {
		t.Fatalf("SQL mismatch\n got: %s\nwant: %s", q.SQL, want)
	}
	if len(q.Args) != 4 || q.Args[3] != 11 || q.Limit != 10 {
		t.Fatalf("args = %v, limit = %d", q.Args, q.Limit)
	}
	for _, c := range q.Columns {
		if c.Role != "column" {
			t.Errorf("column %q has role %q", c.ID, c.Role)
		}
	}
}

// A list is subject to the visibility rule exactly like a count.
func TestCompileRiskListAlwaysAppliesVisibility(t *testing.T) {
	one := widgetIntPtr(2)
	for name, vis := range map[string]domain.WidgetVisibility{
		"zero": {}, "teams": {TeamIDs: []int{4}}, "actionOwner": {ActionOwnerUserID: one},
	} {
		q, err := CompileWidgetQuery(domain.WidgetQueryRequest{Spec: listSpec("risk_code"), Visibility: vis})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		switch name {
		case "zero":
			if !strings.Contains(q.SQL, "1=0") {
				t.Errorf("no deny-all in %s", q.SQL)
			}
		case "teams":
			if !strings.Contains(q.SQL, "r.source_register_id IN") {
				t.Errorf("no team condition in %s", q.SQL)
			}
		case "actionOwner":
			if !strings.Contains(q.SQL, "vap.action_owner_id = ?") {
				t.Errorf("no action owner condition in %s", q.SQL)
			}
		}
	}
}

func TestCompileRiskListDefaults(t *testing.T) {
	q, err := CompileWidgetQuery(domain.WidgetQueryRequest{Spec: listSpec("risk_code", "risk_title"), Visibility: domain.WidgetVisibility{All: true}})
	if err != nil {
		t.Fatal(err)
	}
	if q.Limit != widgetMaxListRows || !strings.Contains(q.SQL, "ORDER BY `risk_code` ASC") {
		t.Fatalf("limit = %d, SQL = %s", q.Limit, q.SQL)
	}
	if strings.Contains(q.SQL, "risk_description") {
		t.Fatal("a list must never include the long description")
	}
}

func TestCompileRiskListCategoryFilterStaysOneRowPerRisk(t *testing.T) {
	spec := listSpec("risk_code")
	spec.Filters = []domain.WidgetFilter{{Field: "category", Op: "in", Values: []string{"Secrets"}}}
	q, err := CompileWidgetQuery(domain.WidgetQueryRequest{Spec: spec, Visibility: domain.WidgetVisibility{All: true}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(q.SQL, "DISTINCT") || !strings.Contains(q.SQL, "risk_category_reference") {
		t.Fatalf("expected DISTINCT with the category join: %s", q.SQL)
	}
}

func TestCompileRiskListRejectsBadSpecs(t *testing.T) {
	all := domain.WidgetVisibility{All: true}
	cases := map[string]domain.WidgetSpec{
		"no columns":              {Dataset: "risk_list"},
		"unknown column":          listSpec("risk_description"),
		"password-ish column":     listSpec("owner_id"),
		"duplicate column":        listSpec("risk_code", "risk_code"),
		"too many columns":        listSpec("risk_code", "risk_title", "source_register", "assignment_team", "workflow_status", "treatment_strategy", "gross_level", "residual_level", "implementation_date", "risk_year"),
		"measures on a list":      {Dataset: "risk_list", Columns: []string{"risk_code"}, Measures: []string{"risk_count"}},
		"dimensions on a list":    {Dataset: "risk_list", Columns: []string{"risk_code"}, Dimensions: []string{"risk_year"}},
		"limit above the cap":     {Dataset: "risk_list", Columns: []string{"risk_code"}, Limit: widgetMaxListRows + 1},
		"sort on an unselected":   {Dataset: "risk_list", Columns: []string{"risk_code"}, Sort: &domain.WidgetSort{By: "risk_title"}},
		"action owner filter":     {Dataset: "risk_list", Columns: []string{"risk_code"}, Filters: []domain.WidgetFilter{{Field: "action_owner", Op: "in", Values: []string{"x"}}}},
		"columns on an aggregate": {Dataset: "risks", Measures: []string{"risk_count"}, Columns: []string{"risk_code"}},
	}
	for name, spec := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := CompileWidgetQuery(domain.WidgetQueryRequest{Spec: spec, Visibility: all})
			if _, ok := err.(*apierror.ValidationError); !ok {
				t.Fatalf("expected a validation error, got %v", err)
			}
		})
	}
}

func TestCompileEveryListColumnIsWellFormed(t *testing.T) {
	for id := range widgetDatasets["risk_list"].columns {
		q, err := CompileWidgetQuery(domain.WidgetQueryRequest{Spec: listSpec(id), Visibility: domain.WidgetVisibility{TeamIDs: []int{1}}})
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if strings.Contains(q.SQL, ";") || strings.Count(q.SQL, "?") != len(q.Args) {
			t.Errorf("%s: malformed %s (%d args)", id, q.SQL, len(q.Args))
		}
	}
}

const (
	ann = "11111111-2222-3333-4444-555555555555"
	bob = "66666666-7777-8888-9999-000000000000"
)

func TestCompilePersonFilters(t *testing.T) {
	cases := []struct {
		field    string
		wantSQL  string
		wantArgs int
	}{
		{"owner_person", "r.owner_id IN (SELECT pu.id FROM `user` pu WHERE pu.uuid IN (?,?))", 2},
		{"assigner_person", "r.assigner_id IN (SELECT pu.id FROM `user` pu WHERE pu.uuid IN (?,?))", 2},
		{"action_owner_person", "pau.uuid IN (?,?)", 2},
		// Anyone named on the risk: owner, assigner, approver, or an action owner.
		{"involved_person", "r.management_approver_id IN (SELECT pu.id", 8},
	}
	for _, c := range cases {
		t.Run(c.field, func(t *testing.T) {
			spec := listSpec("risk_code")
			spec.Filters = []domain.WidgetFilter{{Field: c.field, Op: "in", Values: []string{ann, bob}}}
			q, err := CompileWidgetQuery(domain.WidgetQueryRequest{Spec: spec, Visibility: domain.WidgetVisibility{TeamIDs: []int{3}}})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(q.SQL, c.wantSQL) {
				t.Fatalf("missing %q in %s", c.wantSQL, q.SQL)
			}
			// The visibility condition is still there: a person filter narrows what
			// the caller may see, it never widens it.
			if !strings.Contains(q.SQL, "r.source_register_id IN") {
				t.Fatalf("visibility missing: %s", q.SQL)
			}
			// people args + (1 team id, twice) + the limit
			if want := c.wantArgs + 2 + 1; len(q.Args) != want {
				t.Fatalf("%d args, want %d", len(q.Args), want)
			}
			if strings.Count(q.SQL, "?") != len(q.Args) {
				t.Fatalf("placeholders and args differ: %s", q.SQL)
			}
		})
	}
}

func TestCompilePersonFiltersWorkOnCounts(t *testing.T) {
	q, err := CompileWidgetQuery(domain.WidgetQueryRequest{
		Spec: domain.WidgetSpec{
			Dataset: "risks", Measures: []string{"risk_count"},
			Filters: []domain.WidgetFilter{{Field: "involved_person", Op: "in", Values: []string{ann}}},
		},
		Visibility: domain.WidgetVisibility{All: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(q.SQL, "pu.uuid IN (?)") {
		t.Fatalf("%s", q.SQL)
	}
}

// A person is only ever an identity the platform already resolved: a name, SQL,
// or the wrong number of people is refused before it gets near a statement.
func TestCompilePersonFiltersRefuseAnythingButResolvedIdentities(t *testing.T) {
	all := domain.WidgetVisibility{All: true}
	bad := map[string]domain.WidgetFilter{
		"a name":          {Field: "involved_person", Op: "in", Values: []string{"tharushi"}},
		"an email":        {Field: "involved_person", Op: "in", Values: []string{"tharushi@wso2.com"}},
		"sql":             {Field: "owner_person", Op: "in", Values: []string{ann + "' OR '1'='1"}},
		"me":              {Field: "involved_person", Op: "in", Values: []string{"me"}},
		"no people":       {Field: "involved_person", Op: "in"},
		"too many people": {Field: "involved_person", Op: "in", Values: []string{ann, bob, ann, bob, ann, bob}},
		"the wrong op":    {Field: "involved_person", Op: "gte", Values: []string{ann}},
		"a flag op":       {Field: "involved_person", Op: "is", Values: []string{"true"}},
	}
	for name, flt := range bad {
		t.Run(name, func(t *testing.T) {
			spec := listSpec("risk_code")
			spec.Filters = []domain.WidgetFilter{flt}
			_, err := CompileWidgetQuery(domain.WidgetQueryRequest{Spec: spec, Visibility: all})
			if _, ok := err.(*apierror.ValidationError); !ok {
				t.Fatalf("expected a validation error, got %v", err)
			}
		})
	}
}

func TestPersonFieldsCannotBeDimensions(t *testing.T) {
	_, err := CompileWidgetQuery(domain.WidgetQueryRequest{
		Spec:       domain.WidgetSpec{Dataset: "risks", Measures: []string{"risk_count"}, Dimensions: []string{"involved_person"}},
		Visibility: domain.WidgetVisibility{All: true},
	})
	if err == nil {
		t.Fatal("a person must not be usable as a grouping")
	}
}
