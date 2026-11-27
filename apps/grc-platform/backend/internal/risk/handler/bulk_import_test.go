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
	"bufio"
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
// tests, so these cover what the HTTP layer adds: who may call, how a run is
// streamed, the one-at-a-time guard, and what happens when the client goes away.
type fakeBulkImporter struct {
	target    bulkimport.Target
	targetErr error
	// report builds the report a run returns; nil means a clean, empty one.
	report func(dryRun bool) *bulkimport.Report
	runErr error
	// progress is reported, in order, before the run finishes.
	progress []bulkimport.Progress
	// pause holds a run for this long before it finishes.
	pause time.Duration
	// gate, when non-nil, holds every Run until it is closed.
	gate chan struct{}
	// holdUntilCancel makes Run wait for its context to end, as a long run does.
	holdUntilCancel bool
	// started is closed when the first Run begins.
	started chan struct{}
	// panicRun makes Run panic.
	panicRun bool

	mu      sync.Mutex
	runs    []bulkimport.Request
	files   [][]byte
	ctxErrs []error
}

func (f *fakeBulkImporter) LoadTarget(_ context.Context, _ int) (bulkimport.Target, error) {
	return f.target, f.targetErr
}

func (f *fakeBulkImporter) Run(ctx context.Context, req bulkimport.Request) (*bulkimport.Result, error) {
	data, _ := io.ReadAll(req.CSV)
	f.mu.Lock()
	f.runs = append(f.runs, req)
	f.files = append(f.files, data)
	if f.started != nil {
		select {
		case <-f.started:
		default:
			close(f.started)
		}
	}
	f.mu.Unlock()

	if f.panicRun {
		panic("boom")
	}
	for _, p := range f.progress {
		req.Progress(p)
	}
	if f.gate != nil {
		<-f.gate
	}
	if f.holdUntilCancel {
		<-ctx.Done()
		f.mu.Lock()
		f.ctxErrs = append(f.ctxErrs, ctx.Err())
		f.mu.Unlock()
		return nil, errors.Join(bulkimport.ErrStructural, ctx.Err())
	}
	if f.pause > 0 {
		time.Sleep(f.pause)
	}
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

func (f *fakeBulkImporter) lastRun() bulkimport.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.runs[len(f.runs)-1]
}

func newTestBulkHandler(f *fakeBulkImporter) *bulkImportHandler {
	if f.target.RegisterID == 0 {
		f.target = bulkimport.Target{RegisterID: 3, Name: "Asgardeo", Template: "STANDARD"}
	}
	h := newBulkImportHandler(f)
	h.heartbeat = time.Hour // a test that wants pings sets its own
	return h
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

func runReq(t *testing.T, ctx context.Context, fileName, fileBody string, fields ...formField) *http.Request {
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

// streamLine is one line of a run's response.
type streamLine struct {
	Type string
	raw  []byte
}

func (l streamLine) as(t *testing.T, v any) {
	t.Helper()
	if err := json.Unmarshal(l.raw, v); err != nil {
		t.Fatalf("line is not the expected event: %v\n%s", err, l.raw)
	}
}

// events reads a finished run's response as its lines.
func events(t *testing.T, rec *httptest.ResponseRecorder) []streamLine {
	t.Helper()
	var out []streamLine
	sc := bufio.NewScanner(bytes.NewReader(rec.Body.Bytes()))
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		raw := append([]byte(nil), sc.Bytes()...)
		var probe struct{ Type string }
		if err := json.Unmarshal(raw, &probe); err != nil {
			t.Fatalf("a stream line is not JSON: %v\n%s", err, raw)
		}
		out = append(out, streamLine{Type: probe.Type, raw: raw})
	}
	return out
}

// run submits a file and returns the response once the run has finished.
func run(t *testing.T, h *bulkImportHandler, ctx context.Context, body string, fields ...formField) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.handleRun(rec, runReq(t, ctx, "register.csv", body, fields...))
	return rec
}

// finalLine is the stream's last line, which must be a result or an error.
func finalLine(t *testing.T, rec *httptest.ResponseRecorder) streamLine {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	ls := events(t, rec)
	if len(ls) == 0 {
		t.Fatal("the stream is empty")
	}
	last := ls[len(ls)-1]
	if last.Type != "result" && last.Type != "error" {
		t.Fatalf("the stream did not end with a result or an error: %s", last.raw)
	}
	return last
}

func resultOf(t *testing.T, rec *httptest.ResponseRecorder) resultEvent {
	t.Helper()
	last := finalLine(t, rec)
	if last.Type != "result" {
		t.Fatalf("want a result, got %s", last.raw)
	}
	var ev resultEvent
	last.as(t, &ev)
	return ev
}

func errorOf(t *testing.T, rec *httptest.ResponseRecorder) errorEvent {
	t.Helper()
	last := finalLine(t, rec)
	if last.Type != "error" {
		t.Fatalf("want an error, got %s", last.raw)
	}
	var ev errorEvent
	last.as(t, &ev)
	return ev
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
				"template": h.handleTemplate, "run": h.handleRun,
			}
			for name, fn := range routes {
				var req *http.Request
				if name == "run" {
					req = runReq(t, tc.ctx, "r.csv", sampleCSV, dryRunFields()...)
				} else {
					req = httptest.NewRequest(http.MethodGet, "/x?register_id=3", nil).WithContext(tc.ctx)
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
	h.handleRun(rec, runReq(t, ctx, "r.csv", sampleCSV, dryRunFields()...))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestBulkImport_NotConfiguredIsA503AfterTheAccessCheck(t *testing.T) {
	h := newBulkImportHandler(nil)

	rec := httptest.NewRecorder()
	h.handleRun(rec, runReq(t, contextForGrants(t, nil, nil), "r.csv", sampleCSV, dryRunFields()...))
	if rec.Code != http.StatusForbidden {
		t.Errorf("an unauthorised caller must see 403, not learn the feature is off: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.handleRun(rec, runReq(t, adminCtx(t), "r.csv", sampleCSV, dryRunFields()...))
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
			h.handleRun(rec, runReq(t, adminCtx(t), tc.fileName, tc.body, tc.fields...))
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d (%s)", rec.Code, tc.want, rec.Body.String())
			}
			if f.runCount() != 0 {
				t.Error("a rejected upload started a run")
			}
		})
	}
}

// A bad register is a plain 4xx before the stream opens, not an error line.
func TestBulkImport_UnknownRegisterIsRefusedBeforeTheStreamOpens(t *testing.T) {
	f := &fakeBulkImporter{targetErr: &bulkimport.InputError{Msg: "register not found"}}
	h := newTestBulkHandler(f)
	rec := run(t, h, adminCtx(t), sampleCSV, dryRunFields()...)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "register not found") {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if f.runCount() != 0 {
		t.Error("a run started for a register that does not exist")
	}
}

// ── a run ───────────────────────────────────────────────────────────────────

func TestBulkImport_CleanDryRunStreamsAResult(t *testing.T) {
	f := &fakeBulkImporter{}
	h := newTestBulkHandler(f)

	rec := run(t, h, adminCtx(t), sampleCSV, dryRunFields()...)
	if got := rec.Header().Get("Content-Type"); got != bulkImportStreamMediaType {
		t.Errorf("Content-Type = %q", got)
	}
	if ls := events(t, rec); ls[0].Type != "ping" {
		t.Errorf("the stream should open with a ping so the client sees it start, got %s", ls[0].raw)
	}

	ev := resultOf(t, rec)
	if ev.Mode != bulkImportModeDryRun || ev.RegisterID != 3 || ev.RegisterName != "Asgardeo" || ev.Template != "STANDARD" ||
		ev.FileName != "register.csv" || len(ev.FileSHA256) != 64 || ev.Rows != 2 || ev.Outcome != "clean" {
		t.Errorf("result = %+v", ev)
	}
	if ev.Counts.Rejects != 0 || len(ev.Findings) != 0 {
		t.Errorf("counts = %+v findings = %+v", ev.Counts, ev.Findings)
	}
	if ev.Files == nil || ev.Files.ErrorsCSV != "" || ev.Files.Report == "" || ev.Files.RiskCodesCSV != "" {
		t.Errorf("a clean run offers the report only: %+v", ev.Files)
	}
	if r := f.lastRun(); !r.DryRun || r.MaxRows != bulkImportMaxRows || r.RegisterID != 3 || r.AcknowledgeWarnings {
		t.Errorf("run request = %+v", r)
	}
	if !bytes.Equal(f.files[0], []byte(sampleCSV)) {
		t.Error("the run must get the uploaded bytes")
	}
}

func TestBulkImport_RejectedRowsAreReportedAsFindings(t *testing.T) {
	f := &fakeBulkImporter{report: reportWith(rejectFinding)}
	h := newTestBulkHandler(f)

	ev := resultOf(t, run(t, h, adminCtx(t), sampleCSV, dryRunFields()...))
	if ev.Counts.Rejects != 1 || len(ev.Findings) != 1 || ev.Findings[0].Failure != "Quarter" || ev.Findings[0].CSVRow != 3 {
		t.Errorf("findings = %+v counts = %+v", ev.Findings, ev.Counts)
	}
	if ev.Files == nil || ev.Files.ErrorsCSV == "" || ev.Files.Report == "" {
		t.Errorf("a run with findings offers the errors file and the report: %+v", ev.Files)
	}
}

// The import is a run of its own: the server never trusts that a dry run came
// first, so it carries the acknowledgement into the pipeline, which re-validates
// and decides.
func TestBulkImport_ImportRunsForRealAndCarriesTheAcknowledgement(t *testing.T) {
	f := &fakeBulkImporter{}
	h := newTestBulkHandler(f)
	ctx := adminCtx(t)

	ev := resultOf(t, run(t, h, ctx, sampleCSV, importFields()...))
	if ev.Mode != bulkImportModeImport {
		t.Errorf("mode = %q", ev.Mode)
	}
	if r := f.lastRun(); r.DryRun || r.AcknowledgeWarnings {
		t.Errorf("an import is a real run, unacknowledged unless the caller said so: %+v", r)
	}

	resultOf(t, run(t, h, ctx, sampleCSV, importFields(formField{"acknowledge_warnings", "true"})...))
	if r := f.lastRun(); r.DryRun || !r.AcknowledgeWarnings {
		t.Errorf("the acknowledgement must reach the pipeline: %+v", r)
	}
}

func TestBulkImport_ProgressIsStreamedBeforeTheResult(t *testing.T) {
	f := &fakeBulkImporter{progress: []bulkimport.Progress{
		{Phase: "preflight"}, {Phase: "write", Done: 1, Total: 4}, {Phase: "write", Done: 4, Total: 4},
	}}
	h := newTestBulkHandler(f)

	rec := run(t, h, adminCtx(t), sampleCSV, importFields()...)
	var got []progressEvent
	for _, l := range events(t, rec) {
		if l.Type == "progress" {
			var p progressEvent
			l.as(t, &p)
			got = append(got, p)
		}
	}
	if len(got) != 3 || got[0].Phase != "preflight" || got[1].Done != 1 || got[2].Done != 4 || got[2].Total != 4 {
		t.Errorf("progress = %+v", got)
	}
	resultOf(t, rec)
}

func TestBulkImport_QuietRunsKeepTheConnectionAlive(t *testing.T) {
	f := &fakeBulkImporter{pause: 120 * time.Millisecond}
	h := newTestBulkHandler(f)
	h.heartbeat = 10 * time.Millisecond

	rec := run(t, h, adminCtx(t), sampleCSV, dryRunFields()...)
	pings := 0
	for _, l := range events(t, rec) {
		if l.Type == "ping" {
			pings++
		}
	}
	if pings < 3 { // the opening one plus at least two heartbeats
		t.Errorf("pings = %d, want a heartbeat while the run is quiet", pings)
	}
	resultOf(t, rec)
}

// ── one at a time ───────────────────────────────────────────────────────────

func TestBulkImport_SecondRunWhileOneIsRunningIs409(t *testing.T) {
	f := &fakeBulkImporter{gate: make(chan struct{}), started: make(chan struct{})}
	h := newTestBulkHandler(f)
	ctx := adminCtx(t)

	var wg sync.WaitGroup
	first := httptest.NewRecorder()
	wg.Add(1)
	go func() {
		defer wg.Done()
		h.handleRun(first, runReq(t, ctx, "register.csv", sampleCSV, dryRunFields()...))
	}()
	<-f.started

	rec := httptest.NewRecorder()
	h.handleRun(rec, runReq(t, ctx, "other.csv", "Year\n1\n", dryRunFields()...))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}

	close(f.gate)
	wg.Wait()
	resultOf(t, first)

	// The slot is free again.
	resultOf(t, run(t, h, ctx, "Year\n2\n", dryRunFields()...))
}

// ── failures ────────────────────────────────────────────────────────────────

func TestBulkImport_FailedRunKeepsInternalDetailOutOfTheResponse(t *testing.T) {
	f := &fakeBulkImporter{runErr: errors.Join(bulkimport.ErrStructural, errors.New("compliance-entity health check: dial tcp 10.0.0.7:8080: refused"))}
	h := newTestBulkHandler(f)

	ev := errorOf(t, run(t, h, adminCtx(t), sampleCSV, dryRunFields()...))
	if strings.Contains(ev.Message, "10.0.0.7") || strings.Contains(ev.Message, "dial") {
		t.Errorf("an internal address reached the response: %q", ev.Message)
	}
	if ev.Reference == "" || !strings.Contains(ev.Message, "reference "+ev.Reference) {
		t.Errorf("a failed run must give the reference to find it in the log by: %+v", ev)
	}
}

func TestBulkImport_InputProblemsAreShownToTheAdmin(t *testing.T) {
	f := &fakeBulkImporter{runErr: &bulkimport.InputError{Msg: "csv is missing column(s): Platform"}}
	h := newTestBulkHandler(f)

	ev := errorOf(t, run(t, h, adminCtx(t), sampleCSV, dryRunFields()...))
	if ev.Message != "csv is missing column(s): Platform" {
		t.Errorf("error = %+v", ev)
	}
}

func TestBulkImport_APanicInARunIsAnErrorNotACrash(t *testing.T) {
	f := &fakeBulkImporter{panicRun: true}
	h := newTestBulkHandler(f)
	ctx := adminCtx(t)

	ev := errorOf(t, run(t, h, ctx, sampleCSV, dryRunFields()...))
	if strings.Contains(ev.Message, "boom") || ev.Reference == "" {
		t.Errorf("error = %+v", ev)
	}

	// The slot was released.
	f.panicRun = false
	resultOf(t, run(t, h, ctx, sampleCSV, dryRunFields()...))
}

// ── the client going away ───────────────────────────────────────────────────

func TestBulkImport_ClosingTheConnectionStopsTheRun(t *testing.T) {
	f := &fakeBulkImporter{holdUntilCancel: true, started: make(chan struct{})}
	h := newTestBulkHandler(f)
	ctx, cancel := context.WithCancel(adminCtx(t))
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		h.handleRun(httptest.NewRecorder(), runReq(t, ctx, "register.csv", sampleCSV, importFields()...))
	}()
	<-f.started
	cancel() // the browser went away

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("the run kept going after the client left")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.ctxErrs) != 1 || !errors.Is(f.ctxErrs[0], context.Canceled) {
		t.Errorf("the run's context was not cancelled: %v", f.ctxErrs)
	}
	if !h.acquire() {
		t.Error("the slot was not released")
	}
}

// brokenWriter is a response whose client has gone: every write fails.
type brokenWriter struct{ header http.Header }

func (w *brokenWriter) Header() http.Header       { return w.header }
func (w *brokenWriter) WriteHeader(int)           {}
func (w *brokenWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }

func TestBulkImport_AFailedWriteStopsTheRun(t *testing.T) {
	f := &fakeBulkImporter{holdUntilCancel: true}
	h := newTestBulkHandler(f)

	done := make(chan struct{})
	go func() {
		defer close(done)
		h.handleRun(&brokenWriter{header: http.Header{}}, runReq(t, adminCtx(t), "register.csv", sampleCSV, importFields()...))
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("the run kept going after a write to the client failed")
	}
	if !h.acquire() {
		t.Error("the slot was not released")
	}
}

// ── downloads ───────────────────────────────────────────────────────────────

func TestBulkImport_ResultCarriesTheDownloads(t *testing.T) {
	rep := func(bool) *bulkimport.Report {
		r := bulkimport.NewReport()
		r.Add(bulkimport.Finding{MigrationID: 1, CSVRow: 2, RiskTitle: "=SUM(A1)", Severity: bulkimport.SevWarn, Failure: "Action Steps", Detail: "x"})
		r.Migrated("APPROVED")
		r.AddRiskCode(bulkimport.RiskCodeEntry{MigrationID: 1, RiskCode: "2025-ASG-Q1-0001", RiskTitle: "T"})
		return r
	}
	f := &fakeBulkImporter{report: rep}
	h := newTestBulkHandler(f)

	ev := resultOf(t, run(t, h, adminCtx(t), sampleCSV, importFields()...))
	if ev.Files == nil || ev.Files.ErrorsCSV == "" || ev.Files.Report == "" || !strings.Contains(ev.Files.RiskCodesCSV, "2025-ASG-Q1-0001") {
		t.Fatalf("files = %+v", ev.Files)
	}
	if strings.Contains(ev.Files.ErrorsCSV, ",=SUM") {
		t.Errorf("a sheet-authored cell reached the download as a live formula:\n%s", ev.Files.ErrorsCSV)
	}
}

func TestBulkImport_FindingsAreCapped(t *testing.T) {
	many := make([]bulkimport.Finding, bulkImportMaxFindings+50)
	for i := range many {
		many[i] = warnFinding
	}
	f := &fakeBulkImporter{report: reportWith(many...)}
	h := newTestBulkHandler(f)

	ev := resultOf(t, run(t, h, adminCtx(t), sampleCSV, dryRunFields()...))
	if len(ev.Findings) != bulkImportMaxFindings || !ev.FindingsTruncated {
		t.Errorf("findings = %d truncated = %v", len(ev.Findings), ev.FindingsTruncated)
	}
	if ev.Counts.Warnings != bulkImportMaxFindings+50 {
		t.Errorf("counts must still be complete: %+v", ev.Counts)
	}
	if ev.Files == nil || ev.Files.ErrorsCSV == "" {
		t.Error("the errors file has every finding")
	}
}

// The logger middleware wraps every response. A streamed run needs to flush
// through it and move the server's deadlines, which http.ResponseController can
// only do if the wrapper unwraps.
func TestBulkImport_StreamsThroughTheLoggerMiddleware(t *testing.T) {
	f := &fakeBulkImporter{}
	h := newTestBulkHandler(f)
	wrapped := middleware.Logger(http.HandlerFunc(h.handleRun))

	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, runReq(t, adminCtx(t), "register.csv", sampleCSV, dryRunFields()...))
	resultOf(t, rec)
	if !rec.Flushed {
		t.Error("the stream was not flushed through the middleware")
	}
}
