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

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

type stubDeadlineDaysService struct{ got int }

func (s *stubDeadlineDaysService) Get(_ context.Context, auditID int) (*model.DeadlineDays, error) {
	s.got = auditID
	return &model.DeadlineDays{}, nil
}

func (s *stubDeadlineDaysService) Update(_ context.Context, auditID int, _ model.UpdateDeadlineDaysRequest, _ string) (*model.DeadlineDays, error) {
	s.got = auditID
	return &model.DeadlineDays{}, nil
}

const deadlineDaysPath = "/api/v1/audits/2/settings/deadline-days"

// Reading the allowed days needs the same privilege as changing them: only the
// compliance admin holds ManageControls.
func TestDeadlineDays_OnlyManageControlsHolders(t *testing.T) {
	cases := map[string]struct {
		grants map[string]bool
		want   int
	}{
		"view only (internal team, management, external auditor)": {map[string]bool{privilege.ViewAudits: true}, http.StatusForbidden},
		"compliance team (review, no manage)":                     {complianceTeam, http.StatusForbidden},
		"compliance admin":                                        {map[string]bool{privilege.ViewAudits: true, privilege.ManageControls: true}, http.StatusOK},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			svc := &stubDeadlineDaysService{}
			h := &deadlineDaysHandler{svc: svc}

			get := httptest.NewRecorder()
			h.getDeadlineDays(get, requestAs(t, 1, tc.grants, deadlineDaysPath, nil))
			if get.Code != tc.want {
				t.Errorf("GET status = %d, want %d", get.Code, tc.want)
			}

			if tc.want != http.StatusForbidden {
				return
			}
			// The PUT checks the privilege before reading the body, so the same holders are refused.
			put := httptest.NewRecorder()
			h.updateDeadlineDays(put, requestAs(t, 1, tc.grants, deadlineDaysPath, nil))
			if put.Code != http.StatusForbidden {
				t.Errorf("PUT status = %d, want 403", put.Code)
			}
			if svc.got != 0 {
				t.Error("the service must not be reached without the privilege")
			}
		})
	}
}
