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
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/bulkimport"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/csvexport"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// Bulk risk import: a Compliance Admin loads a historical risk register into one
// register from a CSV. The pipeline is internal/risk/bulkimport; this file is
// the HTTP side: who may run it and how a run reports back.
//
// A run is one request. The upload stays open and the response is a stream of
// newline-delimited JSON (see the event types below): progress lines while the
// run works, then one final result or error line. Nothing about a run is kept on
// the server between requests, so it does not matter which replica serves it.
//
// The rules the route enforces:
//
//   - It needs RISK_BULK_UPLOAD held GLOBAL. A register-scoped holder is
//     refused: the import provisions user rows and writes role grants, including
//     a GLOBAL Risk Management grant for an ACCEPT + HIGH row's approver, so
//     letting it run on a scoped grant would let that grant reach past its scope.
//   - An import re-validates the whole file read-only before writing anything,
//     exactly as a dry run does, and writes nothing if any row is rejected or if
//     there are warnings the caller has not acknowledged. So no file with a bad
//     row in it is ever imported, whatever the client did before.
//   - One run at a time per process. A run takes minutes and its numbering is
//     order-dependent, so a second concurrent run is refused with 409. With
//     several replicas two admins could still overlap, which the dialog warns
//     against; resume and the natural-key check keep that recoverable.
//   - A run lives and dies with its connection. If the client goes away the run
//     stops. An import is resumable: running the same file again skips what was
//     already written. A dry run writes nothing.
const (
	bulkImportMaxFileBytes    = 5 << 20 // 5 MiB
	bulkImportMaxRows         = 1000
	bulkImportRunTimeout      = 30 * time.Minute
	bulkImportMaxFindings     = 1000
	bulkImportMultipartMem    = 1 << 20
	bulkImportModeDryRun      = "dry_run"
	bulkImportModeImport      = "import"
	bulkImportHeartbeat       = 10 * time.Second
	bulkImportStreamMediaType = "application/x-ndjson"
)

// BulkImporter is the part of bulkimport.Service the handlers use.
type BulkImporter interface {
	LoadTarget(ctx context.Context, registerID int) (bulkimport.Target, error)
	Run(ctx context.Context, req bulkimport.Request) (*bulkimport.Result, error)
}

type bulkImportHandler struct {
	svc       BulkImporter
	heartbeat time.Duration

	mu      sync.Mutex
	running bool
}

func newBulkImportHandler(svc BulkImporter) *bulkImportHandler {
	return &bulkImportHandler{svc: svc, heartbeat: bulkImportHeartbeat}
}

// ready answers 503 when the import is not wired (no directory configured, as in
// local dev without SCIM credentials) and reports whether it is.
func (h *bulkImportHandler) ready(w http.ResponseWriter) bool {
	if h.svc == nil {
		response.WriteError(w, http.StatusServiceUnavailable, "Bulk risk import is not available in this environment.")
		return false
	}
	return true
}

// authorise is the gate every route starts with.
func (h *bulkImportHandler) authorise(w http.ResponseWriter, r *http.Request) (actor string, ok bool) {
	if !auth.RequireGlobalPrivilege(r.Context(), w, privilege.BulkUploadRisks) {
		return "", false
	}
	actor, ok = requireCallerUUID(w, r)
	if !ok {
		return "", false
	}
	return actor, h.ready(w)
}

// acquire takes the one-run-at-a-time slot.
func (h *bulkImportHandler) acquire() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.running {
		return false
	}
	h.running = true
	return true
}

func (h *bulkImportHandler) release() {
	h.mu.Lock()
	h.running = false
	h.mu.Unlock()
}

// ── GET /api/v1/risks/bulk-import/template?register_id= ─────────────────────

var unsafeFileChars = regexp.MustCompile(`[^a-z0-9]+`)

// handleTemplate serves the header row for the chosen register's template: only
// the columns that register needs. It carries no example row on purpose — an
// example left in by mistake would be imported.
func (h *bulkImportHandler) handleTemplate(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.authorise(w, r); !ok {
		return
	}
	registerID, err := strconv.Atoi(r.URL.Query().Get("register_id"))
	if err != nil || registerID <= 0 {
		response.WriteError(w, http.StatusBadRequest, "register_id must be a positive integer")
		return
	}
	tgt, err := h.svc.LoadTarget(r.Context(), registerID)
	if err != nil {
		h.writeRunError(r.Context(), w, err, "load register for template")
		return
	}

	name := strings.Trim(unsafeFileChars.ReplaceAllString(strings.ToLower(tgt.Name), "-"), "-")
	if name == "" {
		name = "register"
	}
	cw := csvexport.Begin(w, "risk-import-template-"+name+".csv", time.Minute)
	_ = cw.Write(bulkimport.TemplateHeaders(tgt.Template))
	cw.Flush()
}

// ── POST /api/v1/risks/bulk-import ──────────────────────────────────────────

// The response body is one JSON object per line. Every line has a "type":
//
//	progress  the run's phase and how far through it
//	ping      nothing happened; keeps idle proxies from closing the connection
//	result    the run finished; carries the counts, findings and report files
//	error     the run stopped; carries a message for the admin
//
// A well-formed stream ends with exactly one result or error line. A stream that
// ends without one was cut off, and the client should say so.
type progressEvent struct {
	Type  string `json:"type"`
	Phase string `json:"phase"`
	Done  int    `json:"done"`
	Total int    `json:"total"`
}

type pingEvent struct {
	Type string `json:"type"`
}

type errorEvent struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	// Reference is quoted to the platform team; the cause is in the log under it.
	Reference string `json:"reference,omitempty"`
}

type resultEvent struct {
	Type              string            `json:"type"`
	Mode              string            `json:"mode"`
	RegisterID        int               `json:"register_id"`
	RegisterName      string            `json:"register_name"`
	Template          string            `json:"template"`
	FileName          string            `json:"file_name"`
	FileSHA256        string            `json:"file_sha256"`
	Outcome           string            `json:"outcome"`
	Rows              int               `json:"rows"`
	Counts            bulkimport.Counts `json:"counts"`
	Findings          []findingResponse `json:"findings,omitempty"`
	FindingsTruncated bool              `json:"findings_truncated,omitempty"`
	Files             *resultFiles      `json:"files,omitempty"`
}

// resultFiles are the downloads, as text. A file is omitted when it has nothing
// to say, so the dialog offers only what exists.
type resultFiles struct {
	ErrorsCSV    string `json:"errors_csv,omitempty"`
	Report       string `json:"report,omitempty"`
	RiskCodesCSV string `json:"risk_codes_csv,omitempty"`
}

type findingResponse struct {
	MigrationID int    `json:"migration_id"`
	CSVRow      int    `json:"csv_row"`
	RiskTitle   string `json:"risk_title"`
	Severity    string `json:"severity"`
	Failure     string `json:"failure"`
	Detail      string `json:"detail"`
}

// handleRun accepts a CSV and runs a dry run or an import, streaming the run's
// progress and result back on the same response.
func (h *bulkImportHandler) handleRun(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.authorise(w, r)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, bulkImportMaxFileBytes+bulkImportMultipartMem)
	if err := r.ParseMultipartForm(bulkImportMultipartMem); err != nil { // #nosec G120 -- body already bounded by MaxBytesReader above
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			response.WriteError(w, http.StatusRequestEntityTooLarge,
				fmt.Sprintf("The file is too large. The limit is %d MB.", bulkImportMaxFileBytes>>20))
			return
		}
		response.WriteError(w, http.StatusBadRequest, "The upload could not be read. Send the CSV as a multipart form.")
		return
	}
	if r.MultipartForm != nil {
		defer func() { _ = r.MultipartForm.RemoveAll() }()
	}

	registerID, err := strconv.Atoi(strings.TrimSpace(r.FormValue("register_id")))
	if err != nil || registerID <= 0 {
		response.WriteError(w, http.StatusBadRequest, "register_id must be a positive integer")
		return
	}
	mode := strings.TrimSpace(r.FormValue("mode"))
	if mode != bulkImportModeDryRun && mode != bulkImportModeImport {
		response.WriteError(w, http.StatusBadRequest, "mode must be dry_run or import")
		return
	}
	acknowledged := r.FormValue("acknowledge_warnings") == "true"

	data, fileName, ok := readBulkImportFile(w, r)
	if !ok {
		return
	}
	sum := sha256.Sum256(data)
	fileSHA := hex.EncodeToString(sum[:])

	// The register is looked up before the stream starts, so a bad id is a
	// synchronous 4xx the admin sees at once, not an error line.
	tgt, err := h.svc.LoadTarget(r.Context(), registerID)
	if err != nil {
		h.writeRunError(r.Context(), w, err, "load register")
		return
	}

	if !h.acquire() {
		response.WriteError(w, http.StatusConflict, "Another import is running. Wait for it to finish, then try again.")
		return
	}
	defer h.release()

	ref := newJobID()
	email := ""
	if u := auth.FromContext(r.Context()); u != nil {
		email = u.Email
	}
	logAttrs := []any{
		"ref", ref, "mode", mode, "registerId", registerID, "register", tgt.Name,
		"template", tgt.Template, "file", fileSHA, "actor", actor, "actorEmail", email,
	}
	slog.InfoContext(r.Context(), "bulk risk import started", logAttrs...)

	// The server's read and write timeouts are sized for ordinary requests. This
	// one lives for minutes, so both are moved out for it. Without the read
	// extension the server's own deadline would cancel the request context after
	// 30 seconds, which would stop the run.
	rc := http.NewResponseController(w)
	until := time.Now().Add(bulkImportRunTimeout + time.Minute)
	_ = rc.SetReadDeadline(until)
	_ = rc.SetWriteDeadline(until)

	w.Header().Set("Content-Type", bulkImportStreamMediaType)
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no") // ask proxies not to buffer the stream
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)

	ctx, cancel := context.WithTimeout(r.Context(), bulkImportRunTimeout)
	defer cancel()
	out := &eventStream{enc: json.NewEncoder(w), rc: rc, onBroken: cancel}
	out.send(pingEvent{Type: "ping"}) // flushes the headers so the client sees the stream open

	res, runErr := h.run(ctx, out, bulkimport.Request{
		RegisterID:          registerID,
		CSV:                 bytes.NewReader(data),
		DryRun:              mode == bulkImportModeDryRun,
		MaxRows:             bulkImportMaxRows,
		AcknowledgeWarnings: acknowledged,
	})

	if runErr != nil {
		msg, safe := bulkimport.UserMessage(runErr)
		attrs := append(logAttrs, "err", runErr)
		if !safe {
			// The cause is in the server log under this reference; the admin gets
			// the reference to quote, not the detail, which can name internal addresses.
			msg = "The run could not complete. Nothing from it is lost: run the same file again to continue. If it keeps failing, contact the platform team and quote reference " + ref + "."
			slog.ErrorContext(r.Context(), "bulk risk import failed", attrs...)
		} else {
			slog.WarnContext(r.Context(), "bulk risk import refused", attrs...)
		}
		out.send(errorEvent{Type: "error", Message: msg, Reference: ref})
		return
	}

	ev := buildResultEvent(res, mode, tgt, fileName, fileSHA)
	slog.InfoContext(r.Context(), "bulk risk import finished", append(logAttrs,
		"outcome", ev.Outcome, "rows", ev.Rows, "migrated", ev.Counts.Migrated, "skipped", ev.Counts.Skipped,
		"rejects", ev.Counts.Rejects, "warnings", ev.Counts.Warnings, "mismatches", ev.Counts.Mismatches)...)
	out.send(ev)
}

// run executes the pipeline on its own goroutine and relays its progress to the
// stream, with a ping when it is quiet. It returns when the pipeline does. A
// panic in the pipeline is an error, not a crash of the server.
func (h *bulkImportHandler) run(ctx context.Context, out *eventStream, req bulkimport.Request) (*bulkimport.Result, error) {
	type finished struct {
		res *bulkimport.Result
		err error
	}
	progress := make(chan bulkimport.Progress, 32)
	done := make(chan finished, 1)

	req.Progress = func(p bulkimport.Progress) {
		select {
		case progress <- p:
		default: // the stream is behind; a later update supersedes this one
		}
	}
	go func() {
		defer func() {
			if p := recover(); p != nil {
				slog.Error("bulk risk import: panic", "panic", p)
				done <- finished{err: errors.New("internal error")}
			}
		}()
		res, err := h.svc.Run(ctx, req)
		done <- finished{res: res, err: err}
	}()

	tick := time.NewTicker(h.heartbeat)
	defer tick.Stop()
	for {
		select {
		case p := <-progress:
			out.send(progressEvent{Type: "progress", Phase: p.Phase, Done: p.Done, Total: p.Total})
		case <-tick.C:
			out.send(pingEvent{Type: "ping"})
		case f := <-done:
			// Updates still queued when the run ended come out before the result.
			for {
				select {
				case p := <-progress:
					out.send(progressEvent{Type: "progress", Phase: p.Phase, Done: p.Done, Total: p.Total})
				default:
					return f.res, f.err
				}
			}
		}
	}
}

func buildResultEvent(res *bulkimport.Result, mode string, tgt bulkimport.Target, fileName, fileSHA string) resultEvent {
	ev := resultEvent{
		Type: "result", Mode: mode, RegisterID: tgt.RegisterID, RegisterName: tgt.Name, Template: tgt.Template,
		FileName: fileName, FileSHA256: fileSHA, Outcome: string(res.Outcome), Rows: res.Rows,
	}
	if res.Report == nil {
		return ev
	}
	ev.Counts = res.Report.Counts()
	fs := res.Report.Findings()
	if len(fs) > bulkImportMaxFindings {
		fs, ev.FindingsTruncated = fs[:bulkImportMaxFindings], true
	}
	for _, f := range fs {
		ev.Findings = append(ev.Findings, findingResponse{
			MigrationID: f.MigrationID, CSVRow: f.CSVRow, RiskTitle: f.RiskTitle,
			Severity: string(f.Severity), Failure: f.Failure, Detail: f.Detail,
		})
	}

	var errs, narrative, codes bytes.Buffer
	res.Report.WriteErrorsCSV(&errs)
	res.Report.WriteNarrative(&narrative)
	res.Report.WriteRiskCodesCSV(&codes)
	// The report always has something to say (the plan, the codes); the errors
	// file is offered only when there are findings to list.
	files := &resultFiles{Report: narrative.String()}
	if len(ev.Findings) > 0 {
		files.ErrorsCSV = errs.String()
	}
	if ev.Counts.Migrated+ev.Counts.Skipped > 0 && codes.Len() > 0 {
		files.RiskCodesCSV = codes.String()
	}
	if *files != (resultFiles{}) {
		ev.Files = files
	}
	return ev
}

// eventStream writes events to the response, one JSON object per line, flushing
// after each. A failed write means the client is gone, so the run is cancelled
// rather than carried on for nobody. Only the handler goroutine calls send.
type eventStream struct {
	enc      *json.Encoder
	rc       *http.ResponseController
	onBroken func()
	broken   bool
}

func (s *eventStream) send(v any) {
	if s.broken {
		return
	}
	if err := s.enc.Encode(v); err != nil {
		s.broken = true
		s.onBroken()
		return
	}
	// Not every writer can flush (some test writers cannot); the data is still
	// written, just not pushed out at once.
	_ = s.rc.Flush()
}

// readBulkImportFile reads the uploaded CSV from the form, refusing anything
// that is not plainly a CSV text file.
func readBulkImportFile(w http.ResponseWriter, r *http.Request) (data []byte, name string, ok bool) {
	file, header, err := r.FormFile("file")
	if err != nil {
		response.WriteError(w, http.StatusBadRequest, "Attach the CSV file in the 'file' field.")
		return nil, "", false
	}
	defer func() { _ = file.Close() }()

	name = filepath.Base(header.Filename)
	if !strings.EqualFold(filepath.Ext(name), ".csv") {
		response.WriteError(w, http.StatusUnsupportedMediaType,
			"Upload a .csv file. Save the sheet as CSV (UTF-8) from your spreadsheet first.")
		return nil, "", false
	}
	data, err = io.ReadAll(io.LimitReader(file, bulkImportMaxFileBytes+1))
	if err != nil {
		response.WriteError(w, http.StatusBadRequest, "The file could not be read.")
		return nil, "", false
	}
	switch {
	case len(data) > bulkImportMaxFileBytes:
		response.WriteError(w, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("The file is too large. The limit is %d MB.", bulkImportMaxFileBytes>>20))
		return nil, "", false
	case len(bytes.TrimSpace(data)) == 0:
		response.WriteError(w, http.StatusBadRequest, "The file is empty.")
		return nil, "", false
	case bytes.IndexByte(data, 0) >= 0:
		// A NUL byte means a binary file renamed to .csv — an .xlsx, say.
		response.WriteError(w, http.StatusUnsupportedMediaType,
			"This does not look like a CSV text file. Save the sheet as CSV (UTF-8) and upload that.")
		return nil, "", false
	}
	return data, name, true
}

// newJobID is a short random reference for one run, used in the log and quoted
// to the admin when a run fails.
func newJobID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand does not fail on supported platforms; a timestamp keeps the
		// reference distinct if it ever did.
		return fmt.Sprintf("run-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// writeRunError maps an error from the import service to a response. Mistakes in
// what the admin supplied are 4xx with their own message; everything else is a
// generic 500/503 with the detail in the log.
func (h *bulkImportHandler) writeRunError(ctx context.Context, w http.ResponseWriter, err error, what string) {
	if msg, safe := bulkimport.UserMessage(err); safe {
		response.WriteError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	slog.ErrorContext(ctx, "bulk risk import: "+what, "err", err)
	response.WriteError(w, http.StatusServiceUnavailable, "The register list could not be read right now. Please try again.")
}
