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

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/middleware"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/grant"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// stubBulkFrameworkService records the bulk call; every other method panics
// (nil embedded interface), so a test fails loudly if the handler strays.
type stubBulkFrameworkService struct {
	service.FrameworkService
	calls      int
	frameworkI int
	got        []model.CreateFrameworkControlRequest
	err        error
}

func (s *stubBulkFrameworkService) BulkCreateFrameworkControls(_ context.Context, frameworkID int, reqs []model.CreateFrameworkControlRequest, _ string) ([]*model.AuditFrameworkControl, error) {
	s.calls++
	s.frameworkI, s.got = frameworkID, reqs
	if s.err != nil {
		return nil, s.err
	}
	out := make([]*model.AuditFrameworkControl, len(reqs))
	for i, r := range reqs {
		out[i] = &model.AuditFrameworkControl{ID: i + 1, FrameworkID: frameworkID, ControlNumber: r.ControlNumber}
	}
	return out, nil
}

// bulkRequestAs builds POST /frameworks/controls/bulk for a caller holding the
// given global privileges. The privilege map is non-nil so the caller is judged
// by their grants, not the "no privilege store" allow-all mode.
func bulkRequestAs(global map[string]bool, body string) *http.Request {
	set := grant.NewForTest(global, nil, 1)
	ctx := middleware.WithUserInfo(context.Background(), &middleware.UserInfo{Subject: "a@b.c", UserID: 1})
	ctx = grant.WithContext(ctx, set)
	ctx = privilege.WithContext(ctx, set.PrivilegeMap())
	return httptest.NewRequest(http.MethodPost, "/api/v1/audits/frameworks/controls/bulk", strings.NewReader(body)).WithContext(ctx)
}

const bulkBody = `{"frameworkId":3,"controls":[{"controlNumber":"A-1","description":"d","requirementType":"DESIGN","controlType":"NON_CONFIG","scope":"COMMON"}]}`

func TestBulkCreateFrameworkControls_RequiresManageFrameworks(t *testing.T) {
	callers := map[string]map[string]bool{
		"external auditor": externalAuditor,
		"internal team":    internalTeam,
		"viewer only":      {privilege.ViewAudits: true},
		"no privileges":    {},
	}
	for name, privs := range callers {
		svc := &stubBulkFrameworkService{}
		h := &frameworkHandler{svc: svc}
		w := httptest.NewRecorder()
		h.bulkCreateFrameworkControls(w, bulkRequestAs(privs, bulkBody))
		if w.Code != http.StatusForbidden {
			t.Errorf("%s: status = %d, want 403", name, w.Code)
		}
		if svc.calls != 0 {
			t.Errorf("%s: the service must not run for a caller without ManageFrameworks", name)
		}
	}
}

func TestBulkCreateFrameworkControls_ManagerCreates(t *testing.T) {
	svc := &stubBulkFrameworkService{}
	h := &frameworkHandler{svc: svc}
	w := httptest.NewRecorder()
	h.bulkCreateFrameworkControls(w, bulkRequestAs(map[string]bool{privilege.ManageFrameworks: true}, bulkBody))
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body %s)", w.Code, w.Body.String())
	}
	if svc.calls != 1 || svc.frameworkI != 3 || len(svc.got) != 1 || svc.got[0].ControlNumber != "A-1" {
		t.Errorf("service saw framework %d, controls %+v", svc.frameworkI, svc.got)
	}
	if !strings.Contains(w.Body.String(), `"total":1`) {
		t.Errorf("response = %s, want total 1", w.Body.String())
	}
}

func TestBulkCreateFrameworkControls_ValidationErrorIsPassedThrough(t *testing.T) {
	svc := &stubBulkFrameworkService{err: &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "row 2: scope must be COMMON or PRODUCT_SPECIFIC"}}
	h := &frameworkHandler{svc: svc}
	w := httptest.NewRecorder()
	h.bulkCreateFrameworkControls(w, bulkRequestAs(map[string]bool{privilege.ManageFrameworks: true}, bulkBody))
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "row 2") {
		t.Errorf("status = %d body = %s, want 422 naming row 2", w.Code, w.Body.String())
	}
}

func TestBulkCreateFrameworkControls_RejectsMalformedJSON(t *testing.T) {
	svc := &stubBulkFrameworkService{}
	h := &frameworkHandler{svc: svc}
	w := httptest.NewRecorder()
	h.bulkCreateFrameworkControls(w, bulkRequestAs(map[string]bool{privilege.ManageFrameworks: true}, `{not json`))
	if w.Code != http.StatusBadRequest || svc.calls != 0 {
		t.Errorf("status = %d calls = %d, want 400 and no service call", w.Code, svc.calls)
	}
}
