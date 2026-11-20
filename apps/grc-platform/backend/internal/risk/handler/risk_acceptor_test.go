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

package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

func TestValidateCreateRiskRequestRiskAcceptor(t *testing.T) {
	email := "someone@wso2.com"
	valid := model.CreateRiskRequest{
		RiskTitle: "t", RiskDescription: "d", RiskIdentifiedDate: "2026-10-01", ImpactDescription: "i",
		ImplementationDate: "2026-12-01", ReassessmentDate: "2027-01-01",
		SourceRegisterID: 1, AssignmentTeamID: 1, AssignerID: 1, OwnerID: 1, ActionOwnerID: 1,
		Year: 2026, Quarter: "Q4", Likelihood: 1, Impact: 1,
		EmailSubject: "s", TreatmentStrategy: "REMEDIATE",
		ActionSteps:      []model.CreateActionStepRequest{{Description: "step"}},
		IdentifiedByType: model.IdentifiedByEmployee, IdentifiedByEmail: &email,
	}
	for _, c := range []struct {
		acceptor string
		wantErr  bool
	}{
		{"", false}, // omitted: a client that predates the field
		{model.RiskAcceptorCustomer, false},
		{model.RiskAcceptorWSO2, false},
		{"PARTNER", true},
		{"customer", true},
	} {
		req := valid
		req.RiskAcceptor = c.acceptor
		if err := validateCreateRiskRequest(req); (err != nil) != c.wantErr {
			t.Errorf("risk_acceptor %q: err = %v, wantErr %v", c.acceptor, err, c.wantErr)
		}
	}
}

func TestHandleUpdateRiskRiskAcceptor(t *testing.T) {
	const assignerID = 7
	byTeam := map[int]map[string]bool{asgardeo: {privilege.ViewRisks: true, privilege.UpdateRisk: true}}
	base := `"risk_title":"t","risk_description":"d","email_subject":"s"`

	for _, c := range []struct {
		name       string
		body       string
		wantStatus int
	}{
		{"omitted", `{` + base + `}`, http.StatusNoContent},
		{"CUSTOMER", `{` + base + `,"risk_acceptor":"CUSTOMER"}`, http.StatusNoContent},
		{"WSO2", `{` + base + `,"risk_acceptor":"WSO2"}`, http.StatusNoContent},
		{"unknown value", `{` + base + `,"risk_acceptor":"PARTNER"}`, http.StatusBadRequest},
	} {
		t.Run(c.name, func(t *testing.T) {
			risk := &fakeRiskSvc{byID: map[int]*model.RiskDetail{
				1: {ID: 1, SourceRegisterID: asgardeo, AssignmentTeamID: choreo, AssignerID: assignerID},
			}}
			d := (&Deps{Risk: risk, Users: fakeUserRepo{uuid: "test-caller-uuid", id: assignerID}}).withNoopAISuggestions()
			req := httptest.NewRequest(http.MethodPut, "/api/v1/risks/1", strings.NewReader(c.body)).
				WithContext(contextForGrants(t, nil, byTeam))
			req.Header.Set("Content-Type", "application/json")
			req.SetPathValue("id", "1")
			rec := httptest.NewRecorder()

			d.handleUpdateRisk(rec, req)

			if rec.Code != c.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, c.wantStatus, rec.Body.String())
			}
		})
	}
}
