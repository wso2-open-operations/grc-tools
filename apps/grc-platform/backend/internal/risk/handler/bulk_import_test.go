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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/middleware"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/bulkimport"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// fakeBulkImporter stands in for bulkimport.Service: the pipeline has its own
// tests, so these cover what the HTTP layer adds — who may call, the dry-run-
// first rule, the one-at-a-time guard, and how a job is reported.
type fakeBulkImporter struct {
	target    bulkimport.Target
	targetErr error
	// report builds the report a run returns; nil means a clean, empty one.
	report func(dryRun bool) *bulkimport.Report
	runErr error
	// gate, when non-nil, holds every Run until it is closed.
	gate chan struct{}

	mu    sync.Mutex
	runs  []bulkimport.Request
	files [][]byte
}

func (f *fakeBulkImporter) LoadTarget(_ context.Context, _ int) (bulkimport.Target, error) {
	return f.target, f.targetErr
}

func (f *fakeBulkImporter) Run(_ context.Context, req bulkimport.Request) (*bulkimport.Result, error) {
	if f.gate != nil {
		<-f.gate
	}
	data, _ := io.ReadAll(req.CSV)
	f.mu.Lock()
	f.runs = append(f.runs, req)
	f.files = append(f.files, data)
	f.mu.Unlock()
	if f.runErr != nil {
		return nil, f.runErr
	}
	rep := bulkimport.NewReport()
	if f.report != nil {
		rep = f.report(req.DryRun)
	}
	return &bulkimport.Result{Outcome: bulkimport.OutcomeClean, Target: f.target, Rows: 2, Report: rep}, nil
}

func (f *fakeBulkImporter) runCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.runs)
}

func newTestBulkHandler(f *fakeBulkImporter) *bulkImportHandler {
	if f.target.RegisterID == 0 {
		f.target = bulkimport.Target{RegisterID: 3, Name: "Asgardeo", Template: "STANDARD"}
	}
	return newBulkImportHandler(f)
}

// complianceAdminGlobal is what the seed gives the Compliance Admin for this
// feature: the privilege held GLOBAL.
var complianceAdminGlobal = map[string]bool{privilege.BulkUploadRisks: true}

func adminCtx(t *testing.T) context.Context { return contextForGrants(t, complianceAdminGlobal, nil) }

type formField struct{ name, value string }

func multipartBody(t *testing.T, fileName, fileBody string, fields ...formField) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for _, f := range fields {
		if err := w.WriteField(f.name, f.value); err != nil {
			t.Fatal(err)
		}
	}
	if fileName != "" {
		fw, err := w.CreateFormFile("file", fileName)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = fw.Write([]byte(fileBody))
	}
	_ = w.Close()
	return &buf, w.FormDataContentType()
}

func startReq(t *testing.T, ctx context.Context, fileName, fileBody string, fields ...formField) *http.Request {
	t.Helper()
	body, ct := multipartBody(t, fileName, fileBody, fields...)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/risks/bulk-import", body).WithContext(ctx)
	req.Header.Set("Content-Type", ct)
	return req
}

func dryRunFields() []formField {
	return []formField{{"register_id", "3"}, {"mode", "dry_run"}}
}

func importFields(extra ...formField) []formField {
	return append([]formField{{"register_id", "3"}, {"mode", "import"}}, extra...)
}

const sampleCSV = "Year,Quarter\n2025,Q1\n"

func decodeJob(t *testing.T, rec *httptest.ResponseRecorder) jobResponse {
	t.Helper()
	var j jobResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &j); err != nil {
		t.Fatalf("response is not a job: %v\n%s", err, rec.Body.String())
	}
	return j
}

// start submits a file and returns the accepted job.
func start(t *testing.T, h *bulkImportHandler, ctx context.Context, body string, fields ...formField) jobResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	h.handleStart(rec, startReq(t, ctx, "register.csv", body, fields...))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("start: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	return decodeJob(t, rec)
}

// waitDone polls a job until it leaves the running state.
func waitDone(t *testing.T, h *bulkImportHandler, ctx context.Context, id string) jobResponse {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/risks/bulk-import/jobs/"+id, nil).WithContext(ctx)
		req.SetPathValue("id", id)
		rec := httptest.NewRecorder()
		h.handleJob(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("job: status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if j := decodeJob(t, rec); j.Status != bulkImportStatusRunning {
			return j
		}
		if time.Now().After(deadline) {
			t.Fatal("job did not finish")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func reportWith(findings ...bulkimport.Finding) func(bool) *bulkimport.Report {
	return func(bool) *bulkimport.Report {
		rep := bulkimport.NewReport()
		rep.Add(findings...)
		return rep
	}
}

var (
	rejectFinding = bulkimport.Finding{MigrationID: 2, CSVRow: 3, RiskTitle: "Bad", Severity: bulkimport.SevReject, Failure: "Quarter", Detail: "not Q1..Q4"}
	warnFinding   = bulkimport.Finding{MigrationID: 1, CSVRow: 2, RiskTitle: "Meh", Severity: bulkimport.SevWarn, Failure: "Action Steps", Detail: "placeholder"}
)

// ── who may call ────────────────────────────────────────────────────────────

func TestBulkImport_RequiresTheGlobalPrivilege(t *testing.T) {
	cases := []struct {
		name string
		ctx  context.Context
	}{
		{"no privileges", contextForGrants(t, map[string]bool{privilege.ViewRisks: true}, nil)},
		{"a register-scoped holder", contextForGrants(t, nil, map[int]map[string]bool{3: {privilege.BulkUploadRisks: true}})},
		{"a Compliance Team member", contextForGrants(t, complianceGlobal, nil)},
		{"unauthenticated", context.Background()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeBulkImporter{}
			h := newTestBulkHandler(f)

			routes := map[string]func(http.ResponseWriter, *http.Request){
				"template": h.handleTemplate, "start": h.handleStart, "job": h.handleJob, "artifact": h.handleArtifact,
			}
			for name, fn := range routes {
				var req *http.Request
				if name == "start" {
					req = startReq(t, tc.ctx, "r.csv", sampleCSV, dryRunFields()...)
				} else {
					req = httptest.NewRequest(http.MethodGet, "/x?register_id=3", nil).WithContext(tc.ctx)
					req.SetPathValue("id", "abc")
					req.SetPathValue("artifact", "errors")
				}
				rec := httptest.NewRecorder()
				fn(rec, req)
				if rec.Code != http.StatusForbidden && rec.Code != http.StatusUnauthorized {
					t.Errorf("%s: status = %d, want 403 (or 401 when unauthenticated)", name, rec.Code)
				}
			}
			if f.runCount() != 0 {
				t.Error("an unauthorised caller started a run")
			}
		})
	}
}

// A register-scoped grant is refused even for the register it names: the import
// writes GLOBAL role grants, so the privilege must be held GLOBAL.
func TestBulkImport_ScopedHolderIsRefusedForTheirOwnRegister(t *testing.T) {
	h := newTestBulkHandler(&fakeBulkImporter{})
	ctx := contextForGrants(t, nil, map[int]map[string]bool{3: {privilege.BulkUploadRisks: true}})
	rec := httptest.NewRecorder()
	h.handleStart(rec, startReq(t, ctx, "r.csv", sampleCSV, dryRunFields()...))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestBulkImport_NotConfiguredIsA503AfterTheAccessCheck(t *testing.T) {
	h := newBulkImportHandler(nil)

	rec := httptest.NewRecorder()
	h.handleStart(rec, startReq(t, contextForGrants(t, nil, nil), "r.csv", sampleCSV, dryRunFields()...))
	if rec.Code != http.StatusForbidden {
		t.Errorf("an unauthorised caller must see 403, not learn the feature is off: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.handleStart(rec, startReq(t, adminCtx(t), "r.csv", sampleCSV, dryRunFields()...))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}

// ── the template ────────────────────────────────────────────────────────────

func TestBulkImport_TemplateIsTheRegistersHeaderOnly(t *testing.T) {
	f := &fakeBulkImporter{target: bulkimport.Target{RegisterID: 9, Name: "WSO2 Cloud", Template: "AGGREGATED"}}
	h := newTestBulkHandler(f)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/risks/bulk-import/template?register_id=9", nil).WithContext(adminCtx(t))
	rec := httptest.NewRecorder()
	h.handleTemplate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.Contains(got, "risk-import-template-wso2-cloud.csv") {
		t.Errorf("Content-Disposition = %q", got)
	}
	rows := parseExport(t, rec)
	if len(rows) != 1 {
		t.Fatalf("the template must be a header row only, got %d rows (an example row left in would be imported)", len(rows))
	}
	header := strings.Join(rows[0], ",")
	if !strings.Contains(header, "Platform") || strings.Contains(header, "Customer") || strings.Contains(header, "Source Register") {
		t.Errorf("header = %s", header)
	}
}

func TestBulkImport_TemplateNeedsAValidRegister(t *testing.T) {
	h := newTestBulkHandler(&fakeBulkImporter{})
	for _, q := range []string{"", "?register_id=abc", "?register_id=0", "?register_id=-2"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/risks/bulk-import/template"+q, nil).WithContext(adminCtx(t))
		rec := httptest.NewRecorder()
		h.handleTemplate(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%q: status = %d, want 400", q, rec.Code)
		}
	}

	f := &fakeBulkImporter{targetErr: &bulkimport.InputError{Msg: "register not found"}}
	rec := httptest.NewRecorder()
	newTestBulkHandler(f).handleTemplate(rec, httptest.NewRequest(http.MethodGet, "/x?register_id=9", nil).WithContext(adminCtx(t)))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "register not found") {
		t.Errorf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

// ── validating the upload ───────────────────────────────────────────────────

func TestBulkImport_RejectsABadUpload(t *testing.T) {
	big := strings.Repeat("a,b\n", bulkImportMaxFileBytes/4+10)
	cases := []struct {
		name     string
		fileName string
		body     string
		fields   []formField
		want     int
	}{
		{"no file", "", "", dryRunFields(), http.StatusBadRequest},
		{"an xlsx", "register.xlsx", sampleCSV, dryRunFields(), http.StatusUnsupportedMediaType},
		{"a binary renamed to csv", "register.csv", "PK\x03\x04\x00\x00data", dryRunFields(), http.StatusUnsupportedMediaType},
		{"an empty file", "register.csv", "  \n", dryRunFields(), http.StatusBadRequest},
		{"an oversize file", "register.csv", big, dryRunFields(), http.StatusRequestEntityTooLarge},
		{"no register", "register.csv", sampleCSV, []formField{{"mode", "dry_run"}}, http.StatusBadRequest},
		{"a bad register", "register.csv", sampleCSV, []formField{{"register_id", "x"}, {"mode", "dry_run"}}, http.StatusBadRequest},
		{"no mode", "register.csv", sampleCSV, []formField{{"register_id", "3"}}, http.StatusBadRequest},
		{"an unknown mode", "register.csv", sampleCSV, []formField{{"register_id", "3"}, {"mode", "yolo"}}, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeBulkImporter{}
			h := newTestBulkHandler(f)
			rec := httptest.NewRecorder()
			h.handleStart(rec, startReq(t, adminCtx(t), tc.fileName, tc.body, tc.fields...))
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d (%s)", rec.Code, tc.want, rec.Body.String())
			}
			if f.runCount() != 0 {
				t.Error("a rejected upload started a run")
			}
		})
	}
}

func TestBulkImport_UnknownRegisterIsRefusedBeforeAJobExists(t *testing.T) {
	f := &fakeBulkImporter{targetErr: &bulkimport.InputError{Msg: "register not found"}}
	h := newTestBulkHandler(f)
	rec := httptest.NewRecorder()
	h.handleStart(rec, startReq(t, adminCtx(t), "r.csv", sampleCSV, dryRunFields()...))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	if len(h.jobs) != 0 {
		t.Error("a job was created for a register that does not exist")
	}
}

// ── a dry run ───────────────────────────────────────────────────────────────

func TestBulkImport_CleanDryRunReportsImportable(t *testing.T) {
	f := &fakeBulkImporter{}
	h := newTestBulkHandler(f)
	ctx := adminCtx(t)

	accepted := start(t, h, ctx, sampleCSV, dryRunFields()...)
	if accepted.Status != bulkImportStatusRunning || accepted.Mode != bulkImportModeDryRun || accepted.RegisterName != "Asgardeo" {
		t.Errorf("accepted = %+v", accepted)
	}
	job := waitDone(t, h, ctx, accepted.ID)

	if job.Status != bulkImportStatusDone || !job.Importable || job.ImportableUntil == "" {
		t.Errorf("job = %+v, want a finished, importable dry run", job)
	}
	if job.Counts == nil || job.Counts.Rejects != 0 {
		t.Errorf("counts = %+v", job.Counts)
	}
	if f.runs[0].DryRun != true || f.runs[0].MaxRows != bulkImportMaxRows || f.runs[0].RegisterID != 3 {
		t.Errorf("run request = %+v", f.runs[0])
	}
}

func TestBulkImport_DryRunWithARejectIsNotImportable(t *testing.T) {
	f := &fakeBulkImporter{report: reportWith(rejectFinding)}
	h := newTestBulkHandler(f)
	ctx := adminCtx(t)

	job := waitDone(t, h, ctx, start(t, h, ctx, sampleCSV, dryRunFields()...).ID)
	if job.Importable {
		t.Fatal("a dry run with a rejected row must not authorise an import")
	}
	if job.Counts.Rejects != 1 || len(job.Findings) != 1 || job.Findings[0].Failure != "Quarter" || job.Findings[0].CSVRow != 3 {
		t.Errorf("findings = %+v counts = %+v", job.Findings, job.Counts)
	}

	// And the import itself is refused outright for that file.
	rec := httptest.NewRecorder()
	h.handleStart(rec, startReq(t, ctx, "register.csv", sampleCSV, importFields()...))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("import after a dirty dry run: status = %d, want 422", rec.Code)
	}
	if f.runCount() != 1 {
		t.Errorf("runs = %d, want only the dry run", f.runCount())
	}
}

// ── the dry-run-first rule ──────────────────────────────────────────────────

func TestBulkImport_ImportNeedsADryRunOfTheSameFile(t *testing.T) {
	f := &fakeBulkImporter{}
	h := newTestBulkHandler(f)
	ctx := adminCtx(t)

	// No dry run at all.
	rec := httptest.NewRecorder()
	h.handleStart(rec, startReq(t, ctx, "register.csv", sampleCSV, importFields()...))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "dry run") {
		t.Fatalf("import with no dry run: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	waitDone(t, h, ctx, start(t, h, ctx, sampleCSV, dryRunFields()...).ID)

	// A changed file.
	rec = httptest.NewRecorder()
	h.handleStart(rec, startReq(t, ctx, "register.csv", sampleCSV+"2026,Q2\n", importFields()...))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("import of a changed file: status = %d, want 422", rec.Code)
	}

	// A different register.
	rec = httptest.NewRecorder()
	h.handleStart(rec, startReq(t, ctx, "register.csv", sampleCSV, formField{"register_id", "4"}, formField{"mode", "import"}))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("import into another register: status = %d, want 422", rec.Code)
	}

	// Someone else: another admin's dry run does not authorise this one's import.
	rec = httptest.NewRecorder()
	h.handleStart(rec, startReq(t, otherAdminCtx(t), "register.csv", sampleCSV, importFields()...))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("import authorised by someone else's dry run: status = %d, want 422", rec.Code)
	}

	if f.runCount() != 1 {
		t.Errorf("runs = %d, want only the one dry run", f.runCount())
	}

	// The same file, register and caller: accepted.
	job := waitDone(t, h, ctx, start(t, h, ctx, sampleCSV, importFields()...).ID)
	if job.Mode != bulkImportModeImport || job.Status != bulkImportStatusDone {
		t.Errorf("job = %+v", job)
	}
	if f.runs[1].DryRun {
		t.Error("the import ran as a dry run")
	}
	if !bytes.Equal(f.files[0], f.files[1]) {
		t.Error("the import must run the very bytes that were dry-run")
	}
}

func otherAdminCtx(t *testing.T) context.Context {
	t.Helper()
	return middleware.WithUserInfo(adminCtx(t), &middleware.UserInfo{Subject: "someone-else"})
}

func TestBulkImport_AnImportSpendsItsAuthorisation(t *testing.T) {
	f := &fakeBulkImporter{}
	h := newTestBulkHandler(f)
	ctx := adminCtx(t)

	waitDone(t, h, ctx, start(t, h, ctx, sampleCSV, dryRunFields()...).ID)
	waitDone(t, h, ctx, start(t, h, ctx, sampleCSV, importFields()...).ID)

	rec := httptest.NewRecorder()
	h.handleStart(rec, startReq(t, ctx, "register.csv", sampleCSV, importFields()...))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("a second import of the same file: status = %d, want 422 (run the dry run again)", rec.Code)
	}
}

func TestBulkImport_DryRunAuthorisationExpires(t *testing.T) {
	f := &fakeBulkImporter{}
	h := newTestBulkHandler(f)
	ctx := adminCtx(t)

	waitDone(t, h, ctx, start(t, h, ctx, sampleCSV, dryRunFields()...).ID)
	h.mu.Lock()
	h.now = func() time.Time { return time.Now().Add(bulkImportDryRunTTL + time.Minute) }
	h.mu.Unlock()

	rec := httptest.NewRecorder()
	h.handleStart(rec, startReq(t, ctx, "register.csv", sampleCSV, importFields()...))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("import after the window: status = %d, want 422", rec.Code)
	}
}

// Option 1: warnings do not block the import, but they must be accepted.
func TestBulkImport_WarningsMustBeAcknowledged(t *testing.T) {
	f := &fakeBulkImporter{report: reportWith(warnFinding)}
	h := newTestBulkHandler(f)
	ctx := adminCtx(t)

	job := waitDone(t, h, ctx, start(t, h, ctx, sampleCSV, dryRunFields()...).ID)
	if !job.Importable || job.Counts.Warnings != 1 {
		t.Fatalf("a dry run with only warnings is importable: %+v", job)
	}

	rec := httptest.NewRecorder()
	h.handleStart(rec, startReq(t, ctx, "register.csv", sampleCSV, importFields()...))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "warnings") {
		t.Fatalf("unacknowledged: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.handleStart(rec, startReq(t, ctx, "register.csv", sampleCSV, importFields(formField{"acknowledge_warnings", "true"})...))
	if rec.Code != http.StatusAccepted {
		t.Errorf("acknowledged: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	waitDone(t, h, ctx, decodeJob(t, rec).ID)
}

// ── one at a time ───────────────────────────────────────────────────────────

func TestBulkImport_SecondRunWhileOneIsRunningIs409(t *testing.T) {
	f := &fakeBulkImporter{gate: make(chan struct{})}
	h := newTestBulkHandler(f)
	ctx := adminCtx(t)

	first := start(t, h, ctx, sampleCSV, dryRunFields()...)

	rec := httptest.NewRecorder()
	h.handleStart(rec, startReq(t, ctx, "other.csv", "Year\n1\n", dryRunFields()...))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}

	close(f.gate)
	waitDone(t, h, ctx, first.ID)

	// The slot is free again.
	waitDone(t, h, ctx, start(t, h, ctx, "Year\n2\n", dryRunFields()...).ID)
}

func TestBulkImport_RunningJobReportsProgress(t *testing.T) {
	f := &fakeBulkImporter{gate: make(chan struct{})}
	h := newTestBulkHandler(f)
	ctx := adminCtx(t)

	job := start(t, h, ctx, sampleCSV, dryRunFields()...)
	req := httptest.NewRequest(http.MethodGet, "/x", nil).WithContext(ctx)
	req.SetPathValue("id", job.ID)
	rec := httptest.NewRecorder()
	h.handleJob(rec, req)
	got := decodeJob(t, rec)
	if got.Status != bulkImportStatusRunning || got.Counts != nil || got.Importable || len(got.Findings) != 0 {
		t.Errorf("a running job shows no result yet: %+v", got)
	}
	close(f.gate)
	waitDone(t, h, ctx, job.ID)
}

// ── failures ────────────────────────────────────────────────────────────────

func TestBulkImport_FailedRunKeepsInternalDetailOutOfTheResponse(t *testing.T) {
	f := &fakeBulkImporter{runErr: errors.Join(bulkimport.ErrStructural, errors.New("compliance-entity health check: dial tcp 10.0.0.7:8080: refused"))}
	h := newTestBulkHandler(f)
	ctx := adminCtx(t)

	job := waitDone(t, h, ctx, start(t, h, ctx, sampleCSV, dryRunFields()...).ID)
	if job.Status != bulkImportStatusFailed || job.Importable {
		t.Fatalf("job = %+v", job)
	}
	if strings.Contains(job.Error, "10.0.0.7") || strings.Contains(job.Error, "dial") {
		t.Errorf("an internal address reached the response: %q", job.Error)
	}
	if !strings.Contains(job.Error, "reference "+job.ID) {
		t.Errorf("a failed job must give the reference to find it in the log by: %q", job.Error)
	}
}

func TestBulkImport_InputProblemsAreShownToTheAdmin(t *testing.T) {
	f := &fakeBulkImporter{runErr: &bulkimport.InputError{Msg: "csv is missing column(s): Platform"}}
	h := newTestBulkHandler(f)
	ctx := adminCtx(t)

	job := waitDone(t, h, ctx, start(t, h, ctx, sampleCSV, dryRunFields()...).ID)
	if job.Status != bulkImportStatusFailed || job.Error != "csv is missing column(s): Platform" {
		t.Errorf("job = %+v", job)
	}
}

func TestBulkImport_UnknownJobIs404(t *testing.T) {
	h := newTestBulkHandler(&fakeBulkImporter{})
	req := httptest.NewRequest(http.MethodGet, "/x", nil).WithContext(adminCtx(t))
	req.SetPathValue("id", "nope")
	rec := httptest.NewRecorder()
	h.handleJob(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// ── downloads ───────────────────────────────────────────────────────────────

func TestBulkImport_Downloads(t *testing.T) {
	rep := func(bool) *bulkimport.Report {
		r := bulkimport.NewReport()
		r.Add(bulkimport.Finding{MigrationID: 1, CSVRow: 2, RiskTitle: "=SUM(A1)", Severity: bulkimport.SevWarn, Failure: "Action Steps", Detail: "x"})
		r.Migrated("APPROVED")
		r.AddRiskCode(bulkimport.RiskCodeEntry{MigrationID: 1, RiskCode: "2025-ASG-Q1-0001", RiskTitle: "T"})
		return r
	}
	f := &fakeBulkImporter{report: rep}
	h := newTestBulkHandler(f)
	ctx := adminCtx(t)

	job := waitDone(t, h, ctx, start(t, h, ctx, sampleCSV, dryRunFields()...).ID)
	if got := strings.Join(job.Downloads, ","); got != "errors,report,risk-codes" {
		t.Errorf("downloads = %q", got)
	}

	get := func(artifact string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/x", nil).WithContext(ctx)
		req.SetPathValue("id", job.ID)
		req.SetPathValue("artifact", artifact)
		rec := httptest.NewRecorder()
		h.handleArtifact(rec, req)
		return rec
	}

	errs := get("errors")
	if errs.Code != http.StatusOK || !strings.HasPrefix(errs.Header().Get("Content-Type"), "text/csv") {
		t.Fatalf("errors: status = %d, type = %q", errs.Code, errs.Header().Get("Content-Type"))
	}
	if strings.Contains(errs.Body.String(), ",=SUM") {
		t.Errorf("a sheet-authored cell reached the download as a live formula:\n%s", errs.Body.String())
	}
	if codes := get("risk-codes"); codes.Code != http.StatusOK || !strings.Contains(codes.Body.String(), "2025-ASG-Q1-0001") {
		t.Errorf("risk-codes: status = %d body = %s", codes.Code, codes.Body.String())
	}
	if txt := get("report"); txt.Code != http.StatusOK || !strings.HasPrefix(txt.Header().Get("Content-Type"), "text/plain") ||
		txt.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("report: status = %d headers = %v", txt.Code, txt.Header())
	}
	if get("secrets").Code != http.StatusNotFound {
		t.Error("an unknown artifact name must be 404")
	}
}

func TestBulkImport_FindingsAreCapped(t *testing.T) {
	many := make([]bulkimport.Finding, bulkImportMaxFindings+50)
	for i := range many {
		many[i] = warnFinding
	}
	f := &fakeBulkImporter{report: reportWith(many...)}
	h := newTestBulkHandler(f)
	ctx := adminCtx(t)

	job := waitDone(t, h, ctx, start(t, h, ctx, sampleCSV, dryRunFields()...).ID)
	if len(job.Findings) != bulkImportMaxFindings || !job.FindingsTruncated {
		t.Errorf("findings = %d truncated = %v", len(job.Findings), job.FindingsTruncated)
	}
	if job.Counts.Warnings != bulkImportMaxFindings+50 {
		t.Errorf("counts must still be complete: %+v", job.Counts)
	}
}

func TestBulkImport_OldJobsAreForgotten(t *testing.T) {
	f := &fakeBulkImporter{}
	h := newTestBulkHandler(f)
	ctx := adminCtx(t)

	first := waitDone(t, h, ctx, start(t, h, ctx, "Year\n1\n", dryRunFields()...).ID)
	h.mu.Lock()
	h.now = func() time.Time { return time.Now().Add(bulkImportJobTTL + time.Hour) }
	h.mu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/x", nil).WithContext(ctx)
	req.SetPathValue("id", first.ID)
	rec := httptest.NewRecorder()
	h.handleJob(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("a day-old job: status = %d, want 404", rec.Code)
	}
}
