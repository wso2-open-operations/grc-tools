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

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

func auditReq(spec domain.WidgetSpec, scope domain.Scope, userID int, teams ...int) domain.AuditWidgetQueryRequest {
	return domain.AuditWidgetQueryRequest{Spec: spec, Scope: scope, UserID: userID, ScopeTeamIDs: teams}
}

var controlsByTeam = domain.WidgetSpec{Dataset: "controls", Measures: []string{"control_count"}, Dimensions: []string{"team"}}

func TestAuditVisibilityIsFailClosed(t *testing.T) {
	cases := []struct {
		name string
		req  domain.AuditWidgetQueryRequest
		want string
	}{
		{"all sees every control", auditReq(controlsByTeam, domain.ScopeAll, 0), ""},
		{"empty scope sees nothing", auditReq(controlsByTeam, "", 5), " AND 1=0"},
		{"unknown scope sees nothing", auditReq(controlsByTeam, "everything", 5), " AND 1=0"},
		{"none sees nothing", auditReq(controlsByTeam, domain.ScopeNone, 5), " AND 1=0"},
		{"owned without a user sees nothing", auditReq(controlsByTeam, domain.ScopeOwned, 0), " AND 1=0"},
		{"team without a user sees nothing", auditReq(controlsByTeam, domain.ScopeTeam, 0, 3), " AND 1=0"},
		{"owned is the caller's own controls", auditReq(controlsByTeam, domain.ScopeOwned, 5), " AND c.owner_id = ?"},
		{"assigned is the controls the caller audits", auditReq(controlsByTeam, domain.ScopeAssigned, 5), " AND c.auditor_id = ?"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _, err := auditWidgetVisibilityClause(c.req)
			if err != nil || got != c.want {
				t.Fatalf("got %q, %v; want %q", got, err, c.want)
			}
		})
	}
	if _, _, err := auditWidgetVisibilityClause(auditReq(controlsByTeam, domain.ScopeTeam, 5, 3, 0)); err == nil {
		t.Fatal("a non-positive team was accepted")
	}
}

// Whatever the spec asks for, the statement carries the scope unless the caller
// sees everything.
func TestAuditCompileAlwaysAppliesScope(t *testing.T) {
	specs := []domain.WidgetSpec{
		controlsByTeam,
		{Dataset: "controls", Measures: []string{"control_count", "overdue_count"}},
		{Dataset: "controls", Measures: []string{"control_count"}, Dimensions: []string{"owner", "control_status"}},
		{Dataset: "evidence_rounds", Measures: []string{"evidence_count"}, Dimensions: []string{"evidence_status"}},
		{Dataset: "control_list", Columns: []string{"control_number", "control_status"}},
	}
	for _, spec := range specs {
		q, err := CompileAuditWidgetQuery(auditReq(spec, domain.ScopeOwned, 5))
		if err != nil {
			t.Fatalf("%s: %v", spec.Dataset, err)
		}
		if !strings.Contains(strings.Join(strings.Fields(q.SQL), " "), "c.owner_id = ?") {
			t.Errorf("%s: scope is missing from %s", spec.Dataset, q.SQL)
		}
		if !strings.Contains(q.SQL, "a.status <> 'REMOVED'") {
			t.Errorf("%s: removed audits are not left out", spec.Dataset)
		}
		none, _ := CompileAuditWidgetQuery(auditReq(spec, "", 5))
		if !strings.Contains(none.SQL, "1=0") {
			t.Errorf("%s: an empty scope did not match nothing", spec.Dataset)
		}
		if strings.Count(q.SQL, "?") != len(q.Args) {
			t.Errorf("%s: %d placeholders, %d args", spec.Dataset, strings.Count(q.SQL, "?"), len(q.Args))
		}
	}
}

func TestAuditTeamScopeKeepsTheCallersOwnWork(t *testing.T) {
	q, err := CompileAuditWidgetQuery(auditReq(controlsByTeam, domain.ScopeTeam, 5, 3, 4))
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.Join(strings.Fields(q.SQL), " ")
	for _, want := range []string{"c.team_id IN (?,?)", "c.owner_id = ?", "c.auditor_id = ?"} {
		if !strings.Contains(sql, want) {
			t.Errorf("team scope lacks %q: %s", want, q.SQL)
		}
	}
}

// Each hub's endpoint reaches only its own datasets.
func TestWidgetModulesAreSeparate(t *testing.T) {
	if _, err := CompileWidgetQuery(domain.WidgetQueryRequest{
		Spec: domain.WidgetSpec{Dataset: "controls", Measures: []string{"control_count"}}, Visibility: domain.WidgetVisibility{All: true},
	}); err == nil {
		t.Fatal("the risk endpoint reached an audit dataset")
	}
	if _, err := CompileAuditWidgetQuery(auditReq(domain.WidgetSpec{Dataset: "risks", Measures: []string{"risk_count"}}, domain.ScopeAll, 1)); err == nil {
		t.Fatal("the audit endpoint reached the risk register")
	}
	if _, err := CompileAuditWidgetQuery(auditReq(domain.WidgetSpec{Dataset: "ai_suggestions", Measures: []string{"suggestion_count"}}, domain.ScopeAll, 1)); err == nil {
		t.Fatal("the audit endpoint reached risk AI suggestions")
	}
}

// No audit dataset can select free text, files or the audit's own storage path.
func TestAuditCatalogOffersNoFreeText(t *testing.T) {
	banned := []string{"description", "evidence_requirement", "comments", "attestation", "folder_path", "audit_evidence_file", "scope_description", "file_name", "audit_comment", "a.name,"}
	for id, ds := range widgetDatasets {
		if ds.module != widgetModuleAudit {
			continue
		}
		var exprs []string
		for _, f := range ds.fields {
			exprs = append(exprs, f.expr)
		}
		for _, c := range ds.columns {
			exprs = append(exprs, c.expr)
		}
		for _, m := range ds.measures {
			exprs = append(exprs, m.expr)
		}
		for _, j := range ds.joins {
			exprs = append(exprs, j.sql)
		}
		exprs = append(exprs, ds.from)
		all := strings.ToLower(strings.Join(exprs, " | "))
		for _, b := range banned {
			if strings.Contains(all, b) {
				t.Errorf("dataset %s touches %q", id, b)
			}
		}
	}
}

func TestAuditPersonFilterNeedsResolvedIdentities(t *testing.T) {
	spec := domain.WidgetSpec{
		Dataset: "control_list", Columns: []string{"control_number"},
		Filters: []domain.WidgetFilter{{Field: "owner_person", Op: "in", Values: []string{"Ann Silva"}}},
	}
	if _, err := CompileAuditWidgetQuery(auditReq(spec, domain.ScopeAll, 1)); err == nil {
		t.Fatal("a name reached the person filter")
	}
	spec.Filters[0].Values = []string{"11111111-1111-4111-8111-111111111111"}
	q, err := CompileAuditWidgetQuery(auditReq(spec, domain.ScopeAll, 1))
	if err != nil || !strings.Contains(q.SQL, "c.owner_id IN (SELECT pu.id FROM `user` pu") {
		t.Fatalf("%v %v", err, q)
	}
	if strings.Contains(q.SQL, "11111111") {
		t.Fatal("the identity was written into the SQL instead of bound")
	}
}

func TestAuditOverdueAndLimits(t *testing.T) {
	spec := domain.WidgetSpec{
		Dataset: "controls", Measures: []string{"control_count"}, Dimensions: []string{"owner"},
		Filters: []domain.WidgetFilter{{Field: "is_overdue", Op: "is", Values: []string{"true"}}},
	}
	q, err := CompileAuditWidgetQuery(auditReq(spec, domain.ScopeAll, 1))
	if err != nil || !strings.Contains(q.SQL, "c.status <> 'COMPLETE'") || !strings.Contains(q.SQL, "ow.uuid") {
		t.Fatalf("%v %v", err, q)
	}
	spec.Limit = widgetMaxLimit + 1
	if _, err := CompileAuditWidgetQuery(auditReq(spec, domain.ScopeAll, 1)); err == nil {
		t.Fatal("an over-large limit was accepted")
	}
	list := domain.WidgetSpec{Dataset: "control_list", Columns: []string{"control_number"}, Limit: widgetMaxListRows + 1}
	if _, err := CompileAuditWidgetQuery(auditReq(list, domain.ScopeAll, 1)); err == nil {
		t.Fatal("a list over its row cap was accepted")
	}
}

// Every group-by field and every list column of every audit dataset compiles to
// a single SELECT with one bound value per placeholder.
func TestAuditEveryFieldIsWellFormed(t *testing.T) {
	check := func(label string, q *CompiledWidgetQuery, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		if !strings.HasPrefix(q.SQL, "SELECT /*+ MAX_EXECUTION_TIME(") || strings.Contains(q.SQL, ";") {
			t.Errorf("%s: not a single SELECT: %s", label, q.SQL)
		}
		if n := strings.Count(q.SQL, "?"); n != len(q.Args) {
			t.Errorf("%s: %d placeholders, %d args", label, n, len(q.Args))
		}
	}
	for name, ds := range widgetDatasets {
		if ds.module != widgetModuleAudit {
			continue
		}
		if ds.rows {
			for id := range ds.columns {
				q, err := CompileAuditWidgetQuery(auditReq(domain.WidgetSpec{Dataset: name, Columns: []string{id}}, domain.ScopeTeam, 1, 2))
				check(name+"."+id, q, err)
			}
			continue
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
			q, err := CompileAuditWidgetQuery(auditReq(domain.WidgetSpec{Dataset: name, Dimensions: []string{id}, Measures: []string{measure}}, domain.ScopeTeam, 1, 2))
			check(name+"."+id, q, err)
		}
	}
}

// An audit is visible only through a control the caller may see.
func TestAuditListsAreScopedThroughTheirControls(t *testing.T) {
	for _, spec := range []domain.WidgetSpec{
		{Dataset: "audit_list", Columns: []string{"audit", "framework"}},
		{Dataset: "audits", Measures: []string{"audit_count"}, Dimensions: []string{"framework"}},
	} {
		q, err := CompileAuditWidgetQuery(auditReq(spec, domain.ScopeOwned, 5))
		if err != nil {
			t.Fatal(err)
		}
		sql := strings.Join(strings.Fields(q.SQL), " ")
		if !strings.Contains(sql, "EXISTS (SELECT 1 FROM audit_control c WHERE c.audit_id = a.id AND c.owner_id = ?)") {
			t.Errorf("%s: scope missing: %s", spec.Dataset, sql)
		}
		if !strings.Contains(sql, "a.status <> 'REMOVED'") || strings.Count(q.SQL, "?") != len(q.Args) {
			t.Errorf("%s: %s", spec.Dataset, sql)
		}
		none, _ := CompileAuditWidgetQuery(auditReq(spec, "", 5))
		if !strings.Contains(none.SQL, "1=0") {
			t.Errorf("%s: an empty scope did not match nothing", spec.Dataset)
		}
		all, _ := CompileAuditWidgetQuery(auditReq(spec, domain.ScopeAll, 0))
		if strings.Contains(all.SQL, "EXISTS") {
			t.Errorf("%s: org-wide read should not be narrowed: %s", spec.Dataset, all.SQL)
		}
	}
}
