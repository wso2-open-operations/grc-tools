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
	"encoding/csv"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

const exportPath = "/api/v1/audits/2/controls/export"

type stubExportControlService struct {
	service.ControlService
	controls []*model.AuditControl
	calls    int
	scope    model.Scope
	userID   int
}

func (s *stubExportControlService) ListScoped(_ context.Context, _ int, scope model.Scope, userID int, _ []int) ([]*model.AuditControl, error) {
	s.calls++
	s.scope, s.userID = scope, userID
	return s.controls, nil
}

type stubExportAuditService struct {
	service.AuditService
	inScope bool
}

func (s stubExportAuditService) InScope(context.Context, int, model.Scope, int, []int) (bool, error) {
	return s.inScope, nil
}

func (s stubExportAuditService) GetByID(context.Context, int) (*model.Audit, error) {
	return &model.Audit{ID: 2, Name: "HIPAA GRC Platform 2027"}, nil
}

type exportedTrail struct {
	auditID int
	action  string
	actor   string
	details map[string]any
}

type stubTrailService struct {
	service.TrailService
	recorded []exportedTrail
	err      error
}

func (s *stubTrailService) RecordAuditAction(_ context.Context, auditID int, action, actor string, details map[string]any) error {
	s.recorded = append(s.recorded, exportedTrail{auditID, action, actor, details})
	return s.err
}

func sampleControls() []*model.AuditControl {
	team, owner, auditor := "Platform", "Owen Owner", "Ada Auditor"
	due := "2026-11-05T00:00:00Z"
	return []*model.AuditControl{
		{ControlNumber: "CA-01", Description: "Access is reviewed", RequirementType: "DESIGN", ControlType: "NON_CONFIG",
			Scope: "COMMON", Status: "COMPLETE", TeamName: &team, OwnerName: &owner, AuditorName: &auditor, DueDate: &due},
		{ControlNumber: "CA-02", Description: "=cmd|' /C calc'!A0", RequirementType: "OE", ControlType: "CONFIG",
			Scope: "PRODUCT_SPECIFIC", Status: "EVIDENCE_PENDING", IsOverdue: true, TeamName: &team},
		{ControlNumber: "CB-03", Description: `Backups, "tested" quarterly`, RequirementType: "OE", ControlType: "CONFIG",
			Scope: "COMMON", Status: "POPULATION_INTERNAL_REVIEW"},
	}
}

func newExportHandler(controls []*model.AuditControl, inScope bool) (*controlHandler, *stubExportControlService, *stubTrailService) {
	svc := &stubExportControlService{controls: controls}
	trail := &stubTrailService{}
	return &controlHandler{svc: svc, auditSvc: stubExportAuditService{inScope: inScope}, trail: trail}, svc, trail
}

// complianceTeam is what Compliance Admin and Compliance Team hold that matters
// here: org-wide read, and internal review (see shared_seed_data.sql).
var complianceTeam = map[string]bool{
	privilege.ViewAudits: true, privilege.ViewAllAudits: true,
	privilege.ReviewEvidence: true, privilege.SubmitEvidence: true,
}

var viewAll = complianceTeam

func exportAs(t *testing.T, h *controlHandler, userID int, global map[string]bool, query string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.exportControls(rec, requestAs(t, userID, global, exportPath+query, nil))
	return rec
}

func exportRows(t *testing.T, rec *httptest.ResponseRecorder) [][]string {
	t.Helper()
	body := strings.TrimPrefix(rec.Body.String(), "\xEF\xBB\xBF")
	rows, err := csv.NewReader(strings.NewReader(body)).ReadAll()
	if err != nil {
		t.Fatalf("response is not valid CSV: %v", err)
	}
	return rows
}

func codes(controls []*model.AuditControl) string {
	var out []string
	for _, c := range controls {
		out = append(out, c.ControlNumber)
	}
	return strings.Join(out, ",")
}

// TestFilterControls mirrors the audit detail page's own filtering
// (utils/controlFilters.ts and applyQuickFilter in the web app).
func TestFilterControls(t *testing.T) {
	controls := sampleControls()
	for name, tc := range map[string]struct {
		query string
		want  string
	}{
		"no filters is everything":          {"", "CA-01,CA-02,CB-03"},
		"approved tab":                      {"tab=approved", "CA-01"},
		"all pending tab":                   {"tab=allPending", "CA-02,CB-03"},
		"overdue tab":                       {"tab=overdue", "CA-02"},
		"in progress tab":                   {"tab=inProgress", "CB-03"},
		"a tab ignores search and filters":  {"tab=approved&search=backups&status=EVIDENCE_PENDING", "CA-01"},
		"search matches the number":         {"search=cb-", "CB-03"},
		"search matches the description":    {"search=ACCESS", "CA-01"},
		"search is trimmed":                 {"search=%20backups%20", "CB-03"},
		"status":                            {"status=COMPLETE", "CA-01"},
		"status is OR within the field":     {"status=COMPLETE&status=EVIDENCE_PENDING", "CA-01,CA-02"},
		"OVERDUE pseudo-status":             {"status=OVERDUE", "CA-02"},
		"OVERDUE together with a status":    {"status=OVERDUE&status=COMPLETE", "CA-01,CA-02"},
		"requirement type":                  {"requirement_type=OE", "CA-02,CB-03"},
		"control type":                      {"control_type=CONFIG", "CA-02,CB-03"},
		"scope":                             {"scope=PRODUCT_SPECIFIC", "CA-02"},
		"team by name":                      {"team=Platform", "CA-01,CA-02"},
		"unassigned team is the empty name": {"team=", "CB-03"},
		"auditor by name":                   {"auditor=Ada+Auditor", "CA-01"},
		"owner by name":                     {"owner=Owen+Owner", "CA-01"},
		"fields AND together":               {"requirement_type=OE&team=Platform", "CA-02"},
		"nothing matches":                   {"search=zzz", ""},
	} {
		t.Run(name, func(t *testing.T) {
			q, err := url.ParseQuery(tc.query)
			if err != nil {
				t.Fatal(err)
			}
			p, ok := parseControlExportParams(q)
			if !ok {
				t.Fatalf("params rejected: %v", q)
			}
			if got := codes(filterControls(controls, p)); got != tc.want {
				t.Errorf("filtered = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestExportControls_CSV(t *testing.T) {
	h, svc, _ := newExportHandler(sampleControls(), true)

	rec := exportAs(t, h, 1, viewAll, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/csv; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	disposition := rec.Header().Get("Content-Disposition")
	want := "attachment; filename=audit-controls-hipaa-grc-platform-2027-" + time.Now().UTC().Format("2006-01-02") + ".csv"
	if disposition != want {
		t.Errorf("Content-Disposition = %q, want %q", disposition, want)
	}
	if !strings.HasPrefix(rec.Body.String(), "\xEF\xBB\xBF") {
		t.Error("missing UTF-8 BOM")
	}
	if svc.scope != model.ScopeAll {
		t.Errorf("list scope = %v, want all for a ViewAllAudits holder", svc.scope)
	}

	rows := exportRows(t, rec)
	if len(rows) != 4 {
		t.Fatalf("rows = %d, want header + 3", len(rows))
	}
	if strings.Join(rows[0], "|") != strings.Join(controlExportHeader, "|") {
		t.Errorf("header = %v", rows[0])
	}
	// Display labels, a date, and the name columns come through as the table shows them.
	first := strings.Join(rows[1], "|")
	if first != "CA-01|Access is reviewed||Design|Non-Config|Complete|No|Common|Platform|Owen Owner|Ada Auditor|2026-11-05||||||||||" {
		t.Errorf("first row = %q", first)
	}
	if rows[2][1] != "'=cmd|' /C calc'!A0" {
		t.Errorf("formula was not neutralised: %q", rows[2][1])
	}
	if rows[2][5] != "Evidence Pending" || rows[2][6] != "Yes" || rows[2][7] != "Product Specific" {
		t.Errorf("labels = %v", rows[2])
	}
	if rows[3][1] != `Backups, "tested" quarterly` {
		t.Errorf("quoting lost: %q", rows[3][1])
	}
}

// The export carries the page's tab, search and filters, and nothing else.
func TestExportControls_AppliesTheView(t *testing.T) {
	h, _, _ := newExportHandler(sampleControls(), true)

	rows := exportRows(t, exportAs(t, h, 1, viewAll, "?requirement_type=OE&search=backups"))

	if len(rows) != 2 || rows[1][0] != "CB-03" {
		t.Errorf("rows = %v, want header + CB-03", rows)
	}
}

func TestExportControls_EmptyViewIsHeaderOnly(t *testing.T) {
	h, _, _ := newExportHandler(sampleControls(), true)

	rec := exportAs(t, h, 1, viewAll, "?search=zzz")

	if rec.Code != http.StatusOK || len(exportRows(t, rec)) != 1 {
		t.Errorf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
}

func TestExportControls_RejectsAnUnknownTab(t *testing.T) {
	h, svc, _ := newExportHandler(sampleControls(), true)

	rec := exportAs(t, h, 1, viewAll, "?tab=everything")

	if rec.Code != http.StatusBadRequest || svc.calls != 0 {
		t.Errorf("status = %d, list calls = %d", rec.Code, svc.calls)
	}
}

// A caller who holds ReviewEvidence but not org-wide read is listed under their
// own row scope, so the file holds only the controls the page would show them.
func TestExportControls_IsScopedToTheCaller(t *testing.T) {
	h, svc, _ := newExportHandler(sampleControls(), true)
	team := map[string]bool{privilege.ViewAudits: true, privilege.ReviewEvidence: true, privilege.SubmitEvidence: true}

	rec := exportAs(t, h, 7, team, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if svc.scope != model.ScopeOwned || svc.userID != 7 {
		t.Errorf("listed with scope %v / user %d, want owned / 7", svc.scope, svc.userID)
	}
}

// Only Compliance Admin and Compliance Team may export. Every other audit role
// is refused before anything is listed, logged or written.
func TestExportControls_OnlyComplianceMayExport(t *testing.T) {
	for name, privs := range map[string]map[string]bool{
		"management": {privilege.ViewAudits: true, privilege.ViewAllAudits: true, privilege.ViewInternalComments: true},
		"internal team": {privilege.ViewAudits: true, privilege.SubmitEvidence: true,
			privilege.AddComment: true, privilege.ViewInternalComments: true},
		"external auditor": {privilege.ViewAudits: true, privilege.ValidateEvidence: true,
			privilege.SelectSample: true, privilege.AddComment: true},
		"no audit privileges": {},
	} {
		t.Run(name, func(t *testing.T) {
			h, svc, trail := newExportHandler(sampleControls(), true)

			rec := exportAs(t, h, visAuditorUserID, privs, "")

			if rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, want 403", rec.Code)
			}
			if strings.Contains(rec.Body.String(), "CA-01") || strings.Contains(rec.Header().Get("Content-Type"), "text/csv") {
				t.Error("a refused caller received CSV data")
			}
			if svc.calls != 0 || len(trail.recorded) != 0 {
				t.Errorf("list calls = %d, trail entries = %d, want none", svc.calls, len(trail.recorded))
			}
		})
	}
}

// An audit outside the caller's scope is a 404 — never an empty file, which
// would confirm the audit exists — and nothing is listed.
func TestExportControls_AuditOutOfScope(t *testing.T) {
	h, svc, trail := newExportHandler(sampleControls(), false)
	team := map[string]bool{privilege.ViewAudits: true, privilege.ReviewEvidence: true, privilege.SubmitEvidence: true}

	rec := exportAs(t, h, 999, team, "")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if svc.calls != 0 || len(trail.recorded) != 0 {
		t.Errorf("list calls = %d, trail entries = %d, want none", svc.calls, len(trail.recorded))
	}
}

func TestExportControls_TooManyControls(t *testing.T) {
	many := make([]*model.AuditControl, controlExportMaxRows+1)
	for i := range many {
		many[i] = &model.AuditControl{ControlNumber: "C"}
	}
	h, _, trail := newExportHandler(many, true)

	rec := exportAs(t, h, 1, viewAll, "")

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	if strings.Contains(rec.Header().Get("Content-Type"), "text/csv") || len(trail.recorded) != 0 {
		t.Error("an over-cap export must not be a CSV response or be logged as an export")
	}
}

func TestExportControls_RecordsTheExport(t *testing.T) {
	h, _, trail := newExportHandler(sampleControls(), true)

	exportAs(t, h, 1, viewAll, "?requirement_type=OE&team=Platform")

	if len(trail.recorded) != 1 {
		t.Fatalf("trail entries = %d, want 1", len(trail.recorded))
	}
	got := trail.recorded[0]
	if got.auditID != 2 || got.action != "EXPORTED" || got.actor != "uuid" {
		t.Errorf("entry = %+v", got)
	}
	if got.details["rows"] != 1 {
		t.Errorf("rows = %v, want 1", got.details["rows"])
	}
	if _, ok := got.details["search"]; ok {
		t.Error("an unused filter should not be recorded")
	}
	if team, _ := got.details["team"].([]string); len(team) != 1 || team[0] != "Platform" {
		t.Errorf("team = %v", got.details["team"])
	}
}

// The file has already been sent by the time the entry is written, so a failure
// to log it must not turn the export into an error.
func TestExportControls_TrailFailureDoesNotFailTheExport(t *testing.T) {
	h, _, trail := newExportHandler(sampleControls(), true)
	trail.err = errors.New("trail down")

	rec := exportAs(t, h, 1, viewAll, "")

	if rec.Code != http.StatusOK || len(exportRows(t, rec)) != 4 {
		t.Errorf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
}

func TestControlExportFileName(t *testing.T) {
	day := time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)
	for in, want := range map[string]string{
		"HIPAA GRC Platform 2027":         "audit-controls-hipaa-grc-platform-2027-2026-10-28.csv",
		`../etc/passwd "quoted" & <tags>`: "audit-controls-etc-passwd-quoted-tags-2026-10-28.csv",
		"":                                "audit-controls-audit-2026-10-28.csv",
		"日本語":                             "audit-controls-audit-2026-10-28.csv",
		strings.Repeat("a", 100):          "audit-controls-" + strings.Repeat("a", 60) + "-2026-10-28.csv",
	} {
		if got := controlExportFileName(in, day); got != want {
			t.Errorf("controlExportFileName(%q) = %q, want %q", in, got, want)
		}
	}
}
