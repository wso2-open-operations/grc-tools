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
// the HTTP side — who may run it, the dry-run-first rule, and the background job
// a run lives in.
//
// The rules the routes enforce:
//
//   - Every route needs RISK_BULK_UPLOAD held GLOBAL. A register-scoped holder is
//     refused: the import provisions user rows and writes role grants, including
//     a GLOBAL Risk Management grant for an ACCEPT + HIGH row's approver, so
//     letting it run on a scoped grant would let that grant reach past its scope.
//   - An import is accepted only for a file whose dry run, by the same caller
//     and into the same register, finished with nothing rejected. The match is on
//     the file's SHA-256, so a changed file needs a new dry run. The pipeline
//     re-checks, because reference data can change in between.
//   - One run at a time. A run takes minutes and its numbering is order-
//     dependent, so a second concurrent run is refused with 409. The guard is
//     per process: with several replicas two admins could still overlap, which
//     the dialog warns against; resume and the natural-key check keep that
//     recoverable rather than corrupting.
//
// Job state is in memory. A restart loses the report but not the work: an import
// is resumable, so running the same file again skips what was written.
const (
	bulkImportMaxFileBytes  = 5 << 20 // 5 MiB
	bulkImportMaxRows       = 1000
	bulkImportRunTimeout    = 30 * time.Minute
	bulkImportDryRunTTL     = 30 * time.Minute
	bulkImportJobTTL        = 24 * time.Hour
	bulkImportMaxJobs       = 20
	bulkImportMaxFindings   = 1000
	bulkImportMultipartMem  = 1 << 20
	bulkImportModeDryRun    = "dry_run"
	bulkImportModeImport    = "import"
	bulkImportStatusRunning = "running"
	bulkImportStatusDone    = "succeeded"
	bulkImportStatusFailed  = "failed"
)

// BulkImporter is the part of bulkimport.Service the handlers use.
type BulkImporter interface {
	LoadTarget(ctx context.Context, registerID int) (bulkimport.Target, error)
	Run(ctx context.Context, req bulkimport.Request) (*bulkimport.Result, error)
}

type bulkImportHandler struct {
	svc BulkImporter
	now func() time.Time

	mu      sync.Mutex
	jobs    map[string]*bulkJob
	order   []string // job ids, oldest first
	running string   // id of the job in flight, "" when none
	// cleanDry remembers dry runs that finished with nothing rejected, keyed by
	// caller + register + file hash. It is what authorises an import.
	cleanDry map[string]cleanDryRun
}

type cleanDryRun struct {
	at       time.Time
	warnings int
}

type bulkJob struct {
	id         string
	mode       string
	registerID int
	register   string
	template   string
	fileName   string
	fileSHA    string
	actor      string

	status   string
	phase    string
	done     int
	total    int
	outcome  bulkimport.Outcome
	counts   bulkimport.Counts
	rows     int
	errMsg   string
	started  time.Time
	finished time.Time
	// importableUntil is set on a dry run that authorises an import.
	importableUntil time.Time

	findings  []bulkimport.Finding
	truncated bool
	errorsCSV []byte
	report    []byte
	codesCSV  []byte
}

func newBulkImportHandler(svc BulkImporter) *bulkImportHandler {
	return &bulkImportHandler{
		svc:      svc,
		now:      time.Now,
		jobs:     map[string]*bulkJob{},
		cleanDry: map[string]cleanDryRun{},
	}
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

// jobResponse is a job as the API shows it.
type jobResponse struct {
	ID           string             `json:"id"`
	Mode         string             `json:"mode"`
	Status       string             `json:"status"`
	Phase        string             `json:"phase,omitempty"`
	Done         int                `json:"done"`
	Total        int                `json:"total"`
	RegisterID   int                `json:"register_id"`
	RegisterName string             `json:"register_name"`
	Template     string             `json:"template"`
	FileName     string             `json:"file_name"`
	FileSHA256   string             `json:"file_sha256"`
	Outcome      string             `json:"outcome,omitempty"`
	Rows         int                `json:"rows"`
	Error        string             `json:"error,omitempty"`
	StartedAt    string             `json:"started_at"`
	FinishedAt   string             `json:"finished_at,omitempty"`
	Counts       *bulkimport.Counts `json:"counts,omitempty"`
	// Importable is true on a finished dry run that has nothing rejected, until
	// ImportableUntil: the window in which the same file may be imported.
	Importable        bool              `json:"importable"`
	ImportableUntil   string            `json:"importable_until,omitempty"`
	Findings          []findingResponse `json:"findings,omitempty"`
	FindingsTruncated bool              `json:"findings_truncated,omitempty"`
	Downloads         []string          `json:"downloads,omitempty"`
}

type findingResponse struct {
	MigrationID int    `json:"migration_id"`
	CSVRow      int    `json:"csv_row"`
	RiskTitle   string `json:"risk_title"`
	Severity    string `json:"severity"`
	Failure     string `json:"failure"`
	Detail      string `json:"detail"`
}

// handleStart accepts a CSV and starts a dry run or an import as a background
// job. It answers 202 with the job; progress and the result are read from
// GET .../jobs/{id}.
func (h *bulkImportHandler) handleStart(w http.ResponseWriter, r *http.Request) {
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

	// The register is looked up before the job is accepted, so a bad id is a
	// synchronous 4xx the admin sees at once, not a failed job.
	tgt, err := h.svc.LoadTarget(r.Context(), registerID)
	if err != nil {
		h.writeRunError(r.Context(), w, err, "load register")
		return
	}

	h.mu.Lock()
	h.pruneLocked()
	if h.running != "" {
		h.mu.Unlock()
		response.WriteError(w, http.StatusConflict, "Another import is running. Wait for it to finish, then try again.")
		return
	}
	if mode == bulkImportModeImport {
		key := cleanDryKey(actor, registerID, fileSHA)
		dry, found := h.cleanDry[key]
		switch {
		case !found || h.now().Sub(dry.at) > bulkImportDryRunTTL:
			h.mu.Unlock()
			response.WriteError(w, http.StatusUnprocessableEntity,
				"Import this file only after a dry run of the same file, into the same register, finishes with nothing rejected. Run the dry run again.")
			return
		case dry.warnings > 0 && !acknowledged:
			h.mu.Unlock()
			response.WriteError(w, http.StatusUnprocessableEntity,
				"The dry run reported warnings. Review them and confirm that you accept them to import.")
			return
		}
	}

	job := &bulkJob{
		id: newJobID(), mode: mode, registerID: registerID, register: tgt.Name, template: tgt.Template,
		fileName: fileName, fileSHA: fileSHA, actor: actor,
		status: bulkImportStatusRunning, phase: "preflight", started: h.now(),
	}
	h.jobs[job.id] = job
	h.order = append(h.order, job.id)
	h.running = job.id
	h.mu.Unlock()

	slog.InfoContext(r.Context(), "bulk risk import started",
		"jobId", job.id, "mode", mode, "registerId", registerID, "template", tgt.Template,
		"file", fileSHA, "actor", actor)

	// Detached from the request: a run takes minutes and the server's write
	// timeout is 30s, and a closed browser tab must not abandon it half-written.
	go h.runJob(job, data) // #nosec G118 -- deliberately detached; bounded by bulkImportRunTimeout

	w.Header().Set("Location", "/api/v1/risks/bulk-import/jobs/"+job.id)
	response.WriteJSONValue(w, http.StatusAccepted, h.snapshot(job))
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

func cleanDryKey(actor string, registerID int, fileSHA string) string {
	return actor + "|" + strconv.Itoa(registerID) + "|" + fileSHA
}

func newJobID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand does not fail on supported platforms; a timestamp keeps the
		// id unique within the process if it ever did.
		return fmt.Sprintf("job-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// runJob executes one job to completion and records its outcome. It never
// returns an error: everything ends up on the job.
func (h *bulkImportHandler) runJob(job *bulkJob, data []byte) {
	defer func() {
		if p := recover(); p != nil {
			slog.Error("bulk risk import: panic", "jobId", job.id, "panic", p)
			h.finish(job, nil, errors.New("internal error"))
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), bulkImportRunTimeout)
	defer cancel()

	res, err := h.svc.Run(ctx, bulkimport.Request{
		RegisterID: job.registerID,
		CSV:        bytes.NewReader(data),
		DryRun:     job.mode == bulkImportModeDryRun,
		MaxRows:    bulkImportMaxRows,
		Progress: func(p bulkimport.Progress) {
			h.mu.Lock()
			job.phase, job.done, job.total = p.Phase, p.Done, p.Total
			h.mu.Unlock()
		},
	})
	h.finish(job, res, err)
}

// finish records a run's result on its job, frees the one-at-a-time slot, and,
// for a clean dry run, authorises the import of that file.
func (h *bulkImportHandler) finish(job *bulkJob, res *bulkimport.Result, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	job.finished = h.now()
	job.phase = ""
	if res != nil && res.Report != nil {
		job.rows = res.Rows
		job.outcome = res.Outcome
		job.counts = res.Report.Counts()
		fs := res.Report.Findings()
		if len(fs) > bulkImportMaxFindings {
			fs, job.truncated = fs[:bulkImportMaxFindings], true
		}
		job.findings = fs

		var errs, narrative, codes bytes.Buffer
		res.Report.WriteErrorsCSV(&errs)
		res.Report.WriteNarrative(&narrative)
		res.Report.WriteRiskCodesCSV(&codes)
		job.errorsCSV, job.report, job.codesCSV = errs.Bytes(), narrative.Bytes(), codes.Bytes()
	}

	switch {
	case err != nil:
		job.status = bulkImportStatusFailed
		job.outcome = bulkimport.OutcomeAborted
		if msg, safe := bulkimport.UserMessage(err); safe {
			job.errMsg = msg
		} else {
			// The cause is in the server log under this job id; the admin gets the
			// id to quote, not the detail, which can name internal addresses.
			job.errMsg = "The run could not complete. Nothing from it is lost: run the same file again to continue. If it keeps failing, contact the platform team and quote reference " + job.id + "."
		}
		slog.Error("bulk risk import failed", "jobId", job.id, "mode", job.mode, "registerId", job.registerID, "actor", job.actor, "err", err)
	default:
		job.status = bulkImportStatusDone
		// A dry run authorises the import of this exact file only when nothing
		// was rejected. Warnings do not block, but the import must acknowledge them.
		if job.mode == bulkImportModeDryRun && job.counts.Rejects == 0 && job.counts.Mismatches == 0 {
			h.cleanDry[cleanDryKey(job.actor, job.registerID, job.fileSHA)] = cleanDryRun{at: job.finished, warnings: job.counts.Warnings}
			job.importableUntil = job.finished.Add(bulkImportDryRunTTL)
		}
		if job.mode == bulkImportModeImport {
			// The authorisation is spent: a second import of this file needs a
			// fresh dry run, which also shows what the first one did.
			delete(h.cleanDry, cleanDryKey(job.actor, job.registerID, job.fileSHA))
		}
		slog.Info("bulk risk import finished", "jobId", job.id, "mode", job.mode, "registerId", job.registerID,
			"template", job.template, "file", job.fileSHA, "actor", job.actor, "outcome", string(job.outcome),
			"rows", job.rows, "migrated", job.counts.Migrated, "skipped", job.counts.Skipped,
			"rejects", job.counts.Rejects, "warnings", job.counts.Warnings, "mismatches", job.counts.Mismatches)
	}
	if h.running == job.id {
		h.running = ""
	}
}

// pruneLocked drops jobs past their TTL and, beyond the cap, the oldest finished
// ones, along with any dry-run authorisation past its window. Caller holds mu.
func (h *bulkImportHandler) pruneLocked() {
	now := h.now()
	for k, d := range h.cleanDry {
		if now.Sub(d.at) > bulkImportDryRunTTL {
			delete(h.cleanDry, k)
		}
	}
	keep := h.order[:0]
	for _, id := range h.order {
		j := h.jobs[id]
		expired := j.status != bulkImportStatusRunning && now.Sub(j.finished) > bulkImportJobTTL
		if expired {
			delete(h.jobs, id)
			continue
		}
		keep = append(keep, id)
	}
	h.order = keep
	for len(h.order) > bulkImportMaxJobs {
		oldest := h.order[0]
		if h.jobs[oldest].status == bulkImportStatusRunning {
			break
		}
		delete(h.jobs, oldest)
		h.order = h.order[1:]
	}
}

// ── GET .../jobs/{id} and .../jobs/{id}/{artifact} ──────────────────────────

func (h *bulkImportHandler) handleJob(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.authorise(w, r); !ok {
		return
	}
	h.mu.Lock()
	h.pruneLocked()
	job, found := h.jobs[r.PathValue("id")]
	var resp jobResponse
	if found {
		resp = h.snapshotLocked(job)
	}
	h.mu.Unlock()
	if !found {
		response.WriteError(w, http.StatusNotFound, "That import was not found. Results are kept for 24 hours and are lost if the service restarts.")
		return
	}
	response.WriteJSONValue(w, http.StatusOK, resp)
}

func (h *bulkImportHandler) handleArtifact(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.authorise(w, r); !ok {
		return
	}
	h.mu.Lock()
	job, found := h.jobs[r.PathValue("id")]
	var body []byte
	var fileName string
	var isCSV bool
	if found {
		switch r.PathValue("artifact") {
		case "errors":
			body, fileName, isCSV = job.errorsCSV, "errors.csv", true
		case "risk-codes":
			body, fileName, isCSV = job.codesCSV, "risk_codes.csv", true
		case "report":
			body, fileName = job.report, "report.txt"
		default:
			found = false
		}
		if len(body) == 0 {
			found = false
		}
	}
	h.mu.Unlock()
	if !found {
		response.WriteError(w, http.StatusNotFound, "That download is not available.")
		return
	}

	if isCSV {
		// Begin writes the headers and the BOM Excel needs for non-ASCII text; the
		// body is already CSV, with every user-authored cell neutralised.
		_ = csvexport.Begin(w, fileName, time.Minute)
		_, _ = w.Write(body)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", `attachment; filename="`+fileName+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// ── helpers ─────────────────────────────────────────────────────────────────

func (h *bulkImportHandler) snapshot(job *bulkJob) jobResponse {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.snapshotLocked(job)
}

func (h *bulkImportHandler) snapshotLocked(job *bulkJob) jobResponse {
	resp := jobResponse{
		ID: job.id, Mode: job.mode, Status: job.status, Phase: job.phase, Done: job.done, Total: job.total,
		RegisterID: job.registerID, RegisterName: job.register, Template: job.template,
		FileName: job.fileName, FileSHA256: job.fileSHA, Outcome: string(job.outcome), Rows: job.rows,
		Error: job.errMsg, StartedAt: job.started.UTC().Format(time.RFC3339),
	}
	if job.status == bulkImportStatusRunning {
		return resp
	}
	resp.FinishedAt = job.finished.UTC().Format(time.RFC3339)
	counts := job.counts
	resp.Counts = &counts
	if !job.importableUntil.IsZero() && h.now().Before(job.importableUntil) {
		// Re-checked against the authorisation itself: a later dry run of a changed
		// file, or the import having run, may have spent it.
		if _, ok := h.cleanDry[cleanDryKey(job.actor, job.registerID, job.fileSHA)]; ok {
			resp.Importable = true
			resp.ImportableUntil = job.importableUntil.UTC().Format(time.RFC3339)
		}
	}
	for _, f := range job.findings {
		resp.Findings = append(resp.Findings, findingResponse{
			MigrationID: f.MigrationID, CSVRow: f.CSVRow, RiskTitle: f.RiskTitle,
			Severity: string(f.Severity), Failure: f.Failure, Detail: f.Detail,
		})
	}
	resp.FindingsTruncated = job.truncated
	if len(job.errorsCSV) > 0 {
		resp.Downloads = append(resp.Downloads, "errors", "report")
	}
	if job.counts.Migrated+job.counts.Skipped > 0 && len(job.codesCSV) > 0 {
		resp.Downloads = append(resp.Downloads, "risk-codes")
	}
	return resp
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
