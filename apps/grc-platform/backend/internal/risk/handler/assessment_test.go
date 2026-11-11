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
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// fakeAssessmentSvc records what reached the assessment service.
type fakeAssessmentSvc struct {
	calls int
	last  model.CreateAssessmentRequest
}

func (f *fakeAssessmentSvc) Create(_ context.Context, _ int, req model.CreateAssessmentRequest, _ string) (*model.RiskAssessment, error) {
	f.calls++
	f.last = req
	return &model.RiskAssessment{ID: 1, ResidualLevel: "MEDIUM", Progress: req.Progress}, nil
}
func (f *fakeAssessmentSvc) ListByRiskID(context.Context, int) ([]model.RiskAssessment, error) {
	return nil, nil
}

// TestHandleAssessRisk covers the identity gates on top of the RISK_ASSESS
// grant: only the risk's owner or assigner may assess it, and only the assigner
// may change the owner, approver or implementation date while doing so.
func TestHandleAssessRisk(t *testing.T) {
	const (
		assignerID = 7
		ownerID    = 11
		date       = "2026-12-31"
	)
	assessIn := func(team int) map[int]map[string]bool {
		return map[int]map[string]bool{team: {privilege.ViewRisks: true, privilege.AssessRisk: true}}
	}
	complianceAdminIn := func(team int) map[int]map[string]bool {
		return map[int]map[string]bool{team: {
			privilege.ViewRisks: true, privilege.AssessRisk: true, privilege.ComplianceApproveRisk: true,
		}}
	}
	const base = `"likelihood":2,"impact":2,"progress":"ok","reassessment_date":"2027-01-01"`

	cases := []struct {
		name       string
		callerID   int
		byTeam     map[int]map[string]bool
		body       string
		wantStatus int
		wantCalled bool
		check      func(t *testing.T, got model.CreateAssessmentRequest)
	}{
		{"assigner assesses", assignerID, assessIn(asgardeo), `{` + base + `}`, http.StatusCreated, true, nil},
		{"owner assesses", ownerID, assessIn(asgardeo), `{` + base + `}`, http.StatusCreated, true, nil},
		{"unrelated user with RISK_ASSESS", 99, assessIn(asgardeo), `{` + base + `}`, http.StatusForbidden, false, nil},
		{"no RISK_ASSESS", assignerID, ownerIn(asgardeo), `{` + base + `}`, http.StatusForbidden, false, nil},
		{
			"assigner changes owner, approver and date", assignerID, assessIn(asgardeo),
			`{` + base + `,"owner_id":21,"management_approver_id":22,"implementation_date":"` + date + `"}`,
			http.StatusCreated, true,
			func(t *testing.T, got model.CreateAssessmentRequest) {
				if got.OwnerID == nil || *got.OwnerID != 21 || got.ManagementApproverID == nil || *got.ManagementApproverID != 22 ||
					got.ImplementationDate == nil || *got.ImplementationDate != date {
					t.Errorf("fields not passed through: %+v", got)
				}
				if got.Current == nil {
					t.Error("Current risk not attached for the change log")
				}
			},
		},
		{
			"compliance admin changes owner", 99, complianceAdminIn(asgardeo),
			`{` + base + `,"owner_id":21}`, http.StatusCreated, true, nil,
		},
		{
			"owner cannot change owner", ownerID, assessIn(asgardeo),
			`{` + base + `,"owner_id":21}`, http.StatusForbidden, false, nil,
		},
		{
			"owner cannot change implementation date", ownerID, assessIn(asgardeo),
			`{` + base + `,"implementation_date":"` + date + `"}`, http.StatusForbidden, false, nil,
		},
		{
			"assigner changes assigner and action owner", assignerID, assessIn(asgardeo),
			`{` + base + `,"assigner_id":31,"action_owner_id":32}`, http.StatusCreated, true,
			func(t *testing.T, got model.CreateAssessmentRequest) {
				if got.AssignerID == nil || *got.AssignerID != 31 || got.ActionOwnerID == nil || *got.ActionOwnerID != 32 {
					t.Errorf("fields not passed through: %+v", got)
				}
			},
		},
		{
			"owner cannot change assigner", ownerID, assessIn(asgardeo),
			`{` + base + `,"assigner_id":31}`, http.StatusForbidden, false, nil,
		},
		{
			"owner cannot change action owner", ownerID, assessIn(asgardeo),
			`{` + base + `,"action_owner_id":32}`, http.StatusForbidden, false, nil,
		},
		{
			"action owner needs an action plan", assignerID, assessIn(asgardeo),
			`{` + base + `,"action_owner_id":32}`, http.StatusConflict, false, nil,
		},
		{
			"owner echoing unchanged values is not a change", ownerID, assessIn(asgardeo),
			`{` + base + `,"owner_id":11,"management_approver_id":12,"implementation_date":"2026-11-30","assigner_id":7,"action_owner_id":9}`,
			http.StatusCreated, true,
			func(t *testing.T, got model.CreateAssessmentRequest) {
				if got.OwnerID != nil || got.ManagementApproverID != nil || got.ImplementationDate != nil ||
					got.AssignerID != nil || got.ActionOwnerID != nil {
					t.Errorf("unchanged fields were not dropped: %+v", got)
				}
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			current := "2026-11-30T00:00:00Z"
			plan := &model.ActionPlanDetail{ID: 1, ActionOwnerID: new(int)}
			*plan.ActionOwnerID = 9
			if c.name == "action owner needs an action plan" {
				plan = nil
			}
			risk := &fakeRiskSvc{byID: map[int]*model.RiskDetail{
				1: {
					ID: 1, SourceRegisterID: asgardeo, AssignmentTeamID: choreo,
					AssignerID: assignerID, OwnerID: ownerID, ManagementApproverID: 12, ImplementationDate: &current, ActionPlan: plan,
				},
			}}
			assess := &fakeAssessmentSvc{}
			d := (&Deps{Risk: risk, Assessment: assess, Users: fakeUserRepo{uuid: "test-caller-uuid", id: c.callerID}}).withNoopAISuggestions()

			req := httptest.NewRequest(http.MethodPost, "/api/v1/risks/1/assess", strings.NewReader(c.body)).
				WithContext(contextForGrants(t, nil, c.byTeam))
			req.Header.Set("Content-Type", "application/json")
			req.SetPathValue("id", "1")
			rec := httptest.NewRecorder()

			d.handleAssessRisk(rec, req)

			if rec.Code != c.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, c.wantStatus, rec.Body.String())
			}
			if called := assess.calls > 0; called != c.wantCalled {
				t.Fatalf("service called = %v, want %v", called, c.wantCalled)
			}
			if c.check != nil {
				c.check(t, assess.last)
			}
		})
	}
}
