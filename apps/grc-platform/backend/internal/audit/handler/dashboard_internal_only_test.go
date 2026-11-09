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
	"testing"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

type countingDashboardService struct {
	service.DashboardService
	calls int
}

func (s *countingDashboardService) Get(context.Context, model.DashboardFilter) (*model.DashboardData, error) {
	s.calls++
	return &model.DashboardData{}, nil
}

func (s *countingDashboardService) GetWorkQueuePage(context.Context, model.DashboardFilter, model.WorkQueueTab, int, int) (*model.WorkQueuePage, error) {
	s.calls++
	return &model.WorkQueuePage{}, nil
}

// The dashboard and work queue are internal-only: the external auditor holds
// ViewAudits but not ViewInternalComments and must be refused before anything
// is loaded. Internal roles still get through.
func TestDashboardEndpoints_InternalOnly(t *testing.T) {
	external := map[string]bool{privilege.ViewAudits: true, privilege.ValidateEvidence: true,
		privilege.SelectSample: true, privilege.AddComment: true}
	internal := map[string]bool{privilege.ViewAudits: true, privilege.ViewAllAudits: true,
		privilege.ViewInternalComments: true}

	endpoints := map[string]func(*dashboardHandler) func(http.ResponseWriter, *http.Request){
		"dashboard":  func(h *dashboardHandler) func(http.ResponseWriter, *http.Request) { return h.getDashboard },
		"work queue": func(h *dashboardHandler) func(http.ResponseWriter, *http.Request) { return h.getWorkQueue },
	}
	for name, handle := range endpoints {
		for who, tc := range map[string]struct {
			privs map[string]bool
			code  int
			calls int
		}{
			"external auditor": {external, http.StatusForbidden, 0},
			"internal":         {internal, http.StatusOK, 1},
		} {
			t.Run(name+"/"+who, func(t *testing.T) {
				svc := &countingDashboardService{}
				rec := httptest.NewRecorder()
				handle(&dashboardHandler{svc: svc})(rec, requestAs(t, 7, tc.privs, "/api/v1/audits/dashboard", nil))
				if rec.Code != tc.code {
					t.Errorf("status = %d, want %d", rec.Code, tc.code)
				}
				if svc.calls != tc.calls {
					t.Errorf("service calls = %d, want %d", svc.calls, tc.calls)
				}
			})
		}
	}
}
