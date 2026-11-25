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

package bulkimport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func strptr(s string) *string { return &s }

func goodRoles() []Role {
	return []Role{
		{ID: 10, RoleName: roleRiskOwner, Module: "RISK", Status: "ACTIVE"},
		{ID: 11, RoleName: roleRiskAssigner, Module: "RISK", Status: "ACTIVE"},
		{ID: 12, RoleName: roleRiskManagement, Module: "RISK", Status: "ACTIVE"},
		{ID: 99, RoleName: "grc-platform-audit-lead", Module: "AUDIT", Status: "ACTIVE"},
	}
}

func goodRefInputs() (teams []RiskTeam, cats []RiskCategory, scores []RiskScore, lk TemplateLookups) {
	teams = msTeams()
	cats = []RiskCategory{{ID: 3, Name: "Access Control & Credentials"}}
	scores = []RiskScore{{ID: 7, Likelihood: 3, Impact: 3}, {ID: 4, Likelihood: 1, Impact: 2}}
	lk = msLookups()
	return
}

func TestBuildRefData_HappyPath(t *testing.T) {
	teams, cats, scores, lk := goodRefInputs()

	rd, err := buildRefData(teams, cats, scores, goodRoles(), lk)
	if err != nil {
		t.Fatalf("buildRefData: %v", err)
	}
	if rd.TeamIDByKey["managed services"] != idRegMS || rd.TeamIDByKey["ms"] != idRegMS {
		t.Errorf("team keyed by name and code -> id: %+v", rd.TeamIDByKey)
	}
	if rd.TeamIDByKey["sre one"] != idTeamSRE {
		t.Errorf("codeless team still keyed by name: %+v", rd.TeamIDByKey)
	}
	if rd.TeamCodeByID[idRegMS] != "MS" || rd.TeamCodeByID[idTeamSRE] != "" {
		t.Errorf("TeamCodeByID: %+v", rd.TeamCodeByID)
	}
	if rd.CategoryIDByName["access control & credentials"] != 3 {
		t.Errorf("category keyed by lowercased name: %+v", rd.CategoryIDByName)
	}
	if rd.RoleIDByName[roleRiskOwner] != 10 || rd.RoleIDByName[roleRiskAssigner] != 11 || rd.RoleIDByName[roleRiskManagement] != 12 {
		t.Errorf("RoleIDByName: %+v", rd.RoleIDByName)
	}
}

func TestBuildRefData_Rejections(t *testing.T) {
	tests := []struct {
		name string
		mut  func(*[]RiskTeam, *[]RiskCategory, *[]RiskScore, *[]Role)
		want string
	}{
		{
			name: "empty categories",
			mut:  func(_ *[]RiskTeam, c *[]RiskCategory, _ *[]RiskScore, _ *[]Role) { *c = nil },
			want: "reference data incomplete",
		},
		{
			name: "empty scores",
			mut:  func(_ *[]RiskTeam, _ *[]RiskCategory, s *[]RiskScore, _ *[]Role) { *s = nil },
			want: "reference data incomplete",
		},
		{
			name: "no team has a code",
			mut: func(tm *[]RiskTeam, _ *[]RiskCategory, _ *[]RiskScore, _ *[]Role) {
				for i := range *tm {
					(*tm)[i].Code = nil
				}
			},
			want: "no risk_team carries a code",
		},
		{
			name: "risk role missing",
			mut: func(_ *[]RiskTeam, _ *[]RiskCategory, _ *[]RiskScore, r *[]Role) {
				*r = (*r)[1:] // drop grc-platform-risk-owner
			},
			want: `risk role "grc-platform-risk-owner" not found`,
		},
		{
			name: "risk role inactive",
			mut: func(_ *[]RiskTeam, _ *[]RiskCategory, _ *[]RiskScore, r *[]Role) {
				(*r)[2].Status = "INACTIVE" // grc-platform-risk-management
			},
			want: `risk role "grc-platform-risk-management" has status "INACTIVE"`,
		},
		{
			name: "team key collision",
			mut: func(tm *[]RiskTeam, _ *[]RiskCategory, _ *[]RiskScore, _ *[]Role) {
				*tm = append(*tm, RiskTeam{ID: 42, Name: "asgardeo", Code: strptr("X"), Status: "ACTIVE"})
			},
			want: "maps to both id",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			teams, cats, scores, lk := goodRefInputs()
			roles := goodRoles()
			tc.mut(&teams, &cats, &scores, &roles)

			_, err := buildRefData(teams, cats, scores, roles, lk)
			if err == nil {
				t.Fatalf("want an error containing %q, got nil", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %q, want it to contain %q", err.Error(), tc.want)
			}
		})
	}
}

// entityRefStub answers the preflight calls with canned reference data.
func entityRefStub(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "GET /roles":
			_, _ = w.Write([]byte(`{"roles":[
				{"id":10,"roleName":"grc-platform-risk-owner","module":"RISK","status":"ACTIVE"},
				{"id":11,"roleName":"grc-platform-risk-assigner","module":"RISK","status":"ACTIVE"},
				{"id":12,"roleName":"grc-platform-risk-management","module":"RISK","status":"ACTIVE"}
			]}`))
		case "POST /risk/teams/search":
			_, _ = w.Write([]byte(`{"total":4,"teams":[
				{"id":1,"name":"Asgardeo","code":"ASG","status":"ACTIVE","registerTemplate":"STANDARD"},
				{"id":10,"name":"Managed Services","code":"MS","status":"ACTIVE","registerTemplate":"MANAGED_SERVICES"},
				{"id":11,"name":"Cloud","code":"CLD","status":"ACTIVE","registerTemplate":"AGGREGATED"},
				{"id":12,"name":"Legal","status":"ACTIVE","registerTemplate":"STANDARD"}
			]}`))
		case "GET /risk/categories":
			_, _ = w.Write([]byte(`{"categories":[{"id":3,"name":"Access Control & Credentials"}]}`))
		case "GET /risk/customers":
			_, _ = w.Write([]byte(`{"values":[{"id":21,"name":"BankOne","code":"BO","status":"ACTIVE"}]}`))
		case "GET /risk/products":
			_, _ = w.Write([]byte(`{"values":[{"id":31,"name":"APIM","status":"ACTIVE"}]}`))
		case "GET /risk/deployment-types":
			_, _ = w.Write([]byte(`{"values":[{"id":41,"name":"Private Cloud","status":"ACTIVE"}]}`))
		case "GET /risk/platforms":
			_, _ = w.Write([]byte(`{"values":[
				{"id":61,"name":"Choreo","status":"ACTIVE"},
				{"id":62,"name":"Choreo Console","status":"ACTIVE","parentId":61}
			]}`))
		case "POST /risk/compliance-references/search":
			_, _ = w.Write([]byte(`{"total":3,"references":[{"id":51,"name":"SOC2"},{"id":52,"name":"HIPAA"},{"id":53,"name":"PCI DSS"}]}`))
		case "GET /risk/scores":
			_, _ = w.Write([]byte(`{"scores":[{"id":7,"likelihood":3,"impact":3}]}`))
		default:
			t.Errorf("unexpected entity call %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func newTestService(t *testing.T, h http.HandlerFunc) *Service {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return NewService(NewEntityClient(srv.URL, 5*time.Second), snapshotLookup(nil), nil)
}

func TestPreflight_ManagedServicesRegister(t *testing.T) {
	svc := newTestService(t, entityRefStub(t))

	rd, tgt, err := svc.preflight(context.Background(), 10)
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	if tgt.Template != templateManagedServices || tgt.Name != "Managed Services" || tgt.RegisterID != 10 {
		t.Fatalf("target = %+v", tgt)
	}
	if rd.TeamIDByKey["asg"] != 1 || rd.RoleIDByName[roleRiskManagement] != 12 || rd.CustomerIDByName["bankone"] != 21 {
		t.Fatalf("RefData not fully populated: %+v", rd)
	}
}

func TestPreflight_StandardRegisterLoadsComplianceRefsNotCustomers(t *testing.T) {
	seen := map[string]bool{}
	inner := entityRefStub(t)
	svc := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		seen[r.Method+" "+r.URL.Path] = true
		inner(w, r)
	})

	rd, tgt, err := svc.preflight(context.Background(), 1)
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	if tgt.Template != templateStandard {
		t.Fatalf("target = %+v", tgt)
	}
	if rd.ComplianceIDByName["SOC2"] != 51 {
		t.Errorf("compliance references not loaded: %+v", rd.ComplianceIDByName)
	}
	for _, called := range []string{"GET /risk/customers", "GET /risk/products", "GET /risk/platforms"} {
		if seen[called] {
			t.Errorf("a STANDARD register's preflight should not call %s", called)
		}
	}
}

func TestPreflight_AggregatedRegisterLoadsPlatforms(t *testing.T) {
	svc := newTestService(t, entityRefStub(t))

	rd, tgt, err := svc.preflight(context.Background(), 11)
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	if tgt.Template != templateAggregated {
		t.Fatalf("target = %+v", tgt)
	}
	if ids := rd.PlatformIDByName["choreo"]; len(ids) != 1 || ids[0] != 61 {
		t.Errorf("platforms not loaded: %+v", rd.PlatformIDByName)
	}
	if rd.PlatformParentByID[62] != 61 {
		t.Errorf("sub-platform parent not indexed: %+v", rd.PlatformParentByID)
	}
}

func TestPreflight_Refusals(t *testing.T) {
	svc := newTestService(t, entityRefStub(t))
	for _, tc := range []struct {
		name     string
		register int
		want     string
	}{
		{"unknown register", 999, "not found"},
		{"register without a code", 12, "no code"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := svc.preflight(context.Background(), tc.register)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestLoadTarget(t *testing.T) {
	svc := newTestService(t, entityRefStub(t))
	tgt, err := svc.LoadTarget(context.Background(), 11)
	if err != nil || tgt.Template != templateAggregated || tgt.Name != "Cloud" {
		t.Fatalf("LoadTarget = %+v, %v", tgt, err)
	}
	if _, err := svc.LoadTarget(context.Background(), 12); err == nil {
		t.Error("a register with no code cannot take risks")
	}
}
