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

// Package bulkimport imports a historical risk register into one register of the
// platform by driving the compliance-entity HTTP API. It is the in-app form of
// the former operations/risk-register-migration Choreo task and keeps that
// tool's pipeline: preflight, parse and map, resolve people, dry run, resume from
// the entity, write, verify. What changed is the entry point (a Service the
// Risk Hub handlers call, instead of a CLI), the identity lookup (the backend's
// SCIM client, instead of a directory snapshot), and the scope: one register per
// run, of any template.
//
// The marker below is written to created_by on every row and child row an
// import creates. It is the only handle for resume (state.go) and rollback
// (rollback.sql). The person who ran the import is not on the rows: it is in the
// server log (handler/bulk_import.go logs the actor with every run). The admin
// activity log would be the durable place, but its action and entity types are
// database enums, so recording an import there needs a schema change.
package bulkimport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"
)

const marker = "risk-sheet-migration"

// Options are the run-wide values the write pipeline needs.
type Options struct {
	// MigrationDate (YYYY-MM-DD) fills the closure fields no sheet records: the
	// compliance-approval date and, on a CLOSED row, the plan's completed date.
	MigrationDate string
}

// Outcome is how a run ended.
type Outcome string

const (
	// OutcomeClean means every row was accepted and nothing needed attention.
	OutcomeClean Outcome = "clean"
	// OutcomeFindings means the run finished but the report carries findings.
	OutcomeFindings Outcome = "completed_with_findings"
	// OutcomeAborted means the run stopped early; a real run can be re-run and
	// resumes where it stopped.
	OutcomeAborted Outcome = "aborted"
)

// Progress is reported while a run works through its rows.
type Progress struct {
	Phase string // "preflight" | "parse" | "resolve" | "write" | "verify"
	Done  int
	Total int
}

// Request is one import run.
type Request struct {
	RegisterID int
	CSV        io.Reader
	DryRun     bool
	// MigrationDate defaults to today (UTC) when empty.
	MigrationDate string
	// MaxRows rejects a file with more data rows than this before anything is
	// resolved or written. Zero means no limit.
	MaxRows int
	// AcknowledgeWarnings lets a real run proceed past warnings. Without it a real
	// run whose own validation finds any warning stops before writing anything.
	AcknowledgeWarnings bool
	Progress            func(Progress)
}

// Result is what a run produced. Report is set whenever the run got far enough
// to have one, including alongside an error from a run that aborted part-way.
type Result struct {
	Outcome Outcome
	Target  Target
	Rows    int // data rows read from the file
	Report  *Report
}

// ErrStructural marks a run that could not start or could not continue:
// preflight failed, the file is unusable, or the entity or directory failed
// part-way. Nothing partial is left intentionally; a real run can be repeated.
var ErrStructural = errors.New("bulk import aborted")

// ErrTooManyRows is returned when the file exceeds Request.MaxRows.
var ErrTooManyRows = errors.New("too many rows")

// InputError is a problem with what the admin supplied — the register, or the
// file's shape — rather than with the platform. Its message is written for the
// admin and is safe to show them, unlike an ErrStructural failure from the
// entity or the directory, whose detail belongs in the server log.
type InputError struct{ Msg string }

func (e *InputError) Error() string { return e.Msg }

// UserMessage returns the text to show the admin for err, and whether err is
// one whose message is safe to show.
func UserMessage(err error) (string, bool) {
	var ie *InputError
	if errors.As(err, &ie) {
		return ie.Msg, true
	}
	if errors.Is(err, ErrTooManyRows) {
		return err.Error(), true
	}
	return "", false
}

// Service runs imports. It holds no per-run state, so one value serves every
// run, though the caller allows only one at a time.
type Service struct {
	entity *EntityClient
	lookup EmailLookup
	log    *slog.Logger
}

// NewService builds a Service. lookup resolves an email to an Asgardeo uuid.
func NewService(entity *EntityClient, lookup EmailLookup, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{entity: entity, lookup: lookup, log: log}
}

// LoadTarget looks up the register an import would write into and reports its
// template. It reads only the register list, so the template download can use it
// without the full preflight.
func (s *Service) LoadTarget(ctx context.Context, registerID int) (Target, error) {
	teams, err := s.entity.ListRiskTeams(ctx)
	if err != nil {
		return Target{}, fmt.Errorf("list risk teams: %w", err)
	}
	for _, t := range teams {
		if t.ID != registerID {
			continue
		}
		if t.Code == nil || *t.Code == "" {
			return Target{}, &InputError{Msg: "this register has no code and cannot take risks"}
		}
		return Target{RegisterID: t.ID, Name: t.Name, Template: t.RegisterTemplate}, nil
	}
	return Target{}, &InputError{Msg: "register not found"}
}

// Run executes one import: a dry run validates and reports without writing; a
// real run does the same checks first and then writes. Both stop at the first
// problem that makes the run untrustworthy (ErrStructural); row-level problems
// are findings in the report.
//
// A real run never writes anything while a REJECT is outstanding, or a warning
// the caller has not acknowledged. It does not rely on an earlier dry run having
// been clean: it makes the same read-only pass itself first, so reference data
// that changed since is caught here.
func (s *Service) Run(ctx context.Context, req Request) (*Result, error) {
	report := req.Progress
	if report == nil {
		report = func(Progress) {}
	}
	date := req.MigrationDate
	if date == "" {
		date = time.Now().UTC().Format("2006-01-02")
	}
	opts := Options{MigrationDate: date}
	rep := NewReport()
	res := &Result{Outcome: OutcomeAborted, Report: rep}
	fail := func(err error) (*Result, error) {
		return res, fmt.Errorf("%w: %w", ErrStructural, err)
	}

	// ── Preflight (§7) — any failure aborts before a single write ─────────────
	report(Progress{Phase: "preflight"})
	refs, tgt, err := s.preflight(ctx, req.RegisterID)
	if err != nil {
		s.log.Error("bulk import preflight failed — nothing written", "err", err)
		return fail(err)
	}
	res.Target = tgt

	// ── Parse + validate + map the CSV (§4, §6) ──────────────────────────────
	report(Progress{Phase: "parse"})
	rows, parseFindings, err := parseSheet(req.CSV, refs, tgt)
	if err != nil {
		s.log.Warn("bulk import file is structurally unusable", "err", err)
		return fail(&InputError{Msg: err.Error()})
	}
	res.Rows = len(rows)
	if req.MaxRows > 0 && len(rows) > req.MaxRows {
		return res, fmt.Errorf("%w: the file has %d rows, the limit is %d", ErrTooManyRows, len(rows), req.MaxRows)
	}
	rep.Add(parseFindings...)

	// ── Validate: resolve people and check the rows against the entity ────────
	// Always read-only first, for a dry run and a real run alike. A dry run only
	// checks that each email maps to an account; it provisions user rows
	// (POST /users) only on a real run, and a real run does that only after this
	// pass has found nothing to reject: no file with a bad row in it may write
	// anything, not even a user row.
	report(Progress{Phase: "resolve", Total: len(rows)})
	lookup := cachedLookup(s.lookup)
	check := NewResolver(s.entity, lookup, true)
	resolveFindings, err := check.Apply(ctx, rows)
	if err != nil {
		s.log.Error("bulk import identity resolution failed — re-run to resume", "err", err)
		return fail(fmt.Errorf("identity resolution: %w", err))
	}
	rep.Add(resolveFindings...)

	// Rows that picked up a REJECT finding do not proceed. The same row-level
	// verdicts the real run reaches in reconstructState (rows that share a
	// natural key, rows that match two existing risks), plus the per-sequence
	// summary. Reads only.
	migratable, err := dryRunChecks(ctx, s.log, s.entity, refs, selectMigratable(rows, rep), rep)
	if err != nil {
		return fail(err)
	}
	s.log.Info("bulk import validated", "dryRun", req.DryRun, "migratable", len(migratable), "rejected", len(rows)-len(migratable))

	if req.DryRun {
		if rep.Counts().Rejects == 0 {
			if err := s.planRun(ctx, lookup, refs, opts, rows, rep); err != nil {
				return fail(err)
			}
		}
		res.Outcome = outcomeOf(rep)
		return res, nil
	}

	// A real run writes nothing while a row is rejected: no file with a bad row
	// in it is imported, whatever the caller did before.
	if rep.Counts().Rejects > 0 {
		s.log.Warn("bulk import has rejected rows — nothing written", "rejects", rep.Counts().Rejects)
		res.Outcome = OutcomeFindings
		return res, nil
	}

	// Warnings do not block, but the admin must have seen and accepted them. The
	// caller says so; this check is on what this run found, so a warning that was
	// not in the dry run (reference data changed) is not accepted unseen.
	if w := rep.Counts().Warnings; w > 0 && !req.AcknowledgeWarnings {
		return res, &InputError{Msg: fmt.Sprintf(
			"The file has %d warning(s). Run a dry run, review them and confirm that you accept them before importing. Nothing was written.", w)}
	}

	// ── Provision: email → uuid → user.id, creating user rows (§6) ───────────
	provision := NewResolver(s.entity, lookup, false)
	provisionFindings, err := provision.Apply(ctx, rows)
	if err != nil {
		s.log.Error("bulk import identity resolution failed — re-run to resume", "err", err)
		return fail(fmt.Errorf("identity resolution: %w", err))
	}
	rep.Add(provisionFindings...)
	if rep.Counts().Rejects > 0 {
		// The directory answered differently a moment later. Nothing has been
		// written but user rows, which are idempotent.
		res.Outcome = OutcomeFindings
		return res, nil
	}
	pending := selectMigratable(rows, rep)

	// ── Reconstruct resume state from the entity (§8) ────────────────────────
	state, err := reconstructState(ctx, s.entity, refs, opts.MigrationDate, pending, rep)
	if err != nil {
		s.log.Error("bulk import could not reconstruct resume state", "err", err)
		return fail(fmt.Errorf("resume state: %w", err))
	}
	// reconstructState rejects rows that collide on the natural key; the same
	// rule applies: nothing is written from a file with a rejected row.
	if rep.Counts().Rejects > 0 {
		res.Outcome = OutcomeFindings
		return res, nil
	}
	if err := summarizeCustomers(ctx, s.log, s.entity, refs, pending, state, rep); err != nil {
		return fail(err)
	}

	// ── Write, in Migration ID order (§9) ───────────────────────────────────
	for i, row := range pending {
		report(Progress{Phase: "write", Done: i, Total: len(pending)})
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		if err := migrateRow(ctx, s.entity, opts, refs, row, state[row.MigrationID], rep); err != nil {
			// Auth / network class errors are fatal; row-level problems are
			// recorded inside migrateRow and do not reach here.
			s.log.Error("bulk import fatal error during write — stopping; re-run to resume",
				"migrationId", row.MigrationID, "err", err)
			return fail(fmt.Errorf("row %d: %w", row.MigrationID, err))
		}
	}
	report(Progress{Phase: "write", Done: len(pending), Total: len(pending)})

	// ── Verify — read the entity back and diff it against the CSV,
	// unconditionally, regardless of what this run itself wrote ──────────────
	report(Progress{Phase: "verify", Total: len(pending)})
	if err := verifyMigration(ctx, s.log, s.entity, refs, opts.MigrationDate, rows, rep); err != nil {
		s.log.Error("bulk import verification failed", "err", err)
		return fail(fmt.Errorf("verification: %w", err))
	}

	res.Outcome = outcomeOf(rep)
	return res, nil
}

// planRun tells a dry run what a real run would do with the accepted rows: how
// many an earlier import already completed (skipped) and how many are left to
// write. It reads only. People are looked up but never provisioned, so a person
// with no user row yet has id 0 and their rows read as not yet complete, which
// is also what a real run would find.
//
// The report it builds along the way is thrown away: dryRunChecks has already
// recorded every finding this would repeat.
func (s *Service) planRun(ctx context.Context, lookup EmailLookup, rd RefData, opts Options, rows []Row, rep *Report) error {
	withIDs := append([]Row(nil), rows...)
	// The run's shared lookup, so this pass sees the same people validation did.
	// Anything it still rejects is kept: a plan must not call a row ready that a
	// real run would refuse.
	found, err := NewReadOnlyResolver(s.entity, lookup).Apply(ctx, withIDs)
	if err != nil {
		return fmt.Errorf("identity lookup: %w", err)
	}
	rep.Add(found...)
	pending := selectMigratable(withIDs, rep)
	state, err := reconstructState(ctx, s.entity, rd, opts.MigrationDate, pending, NewReport())
	if err != nil {
		return fmt.Errorf("resume state: %w", err)
	}
	done := 0
	for _, row := range pending {
		if state[row.MigrationID].Progress == ProgressComplete {
			done++
		}
	}
	rep.SetPlan(len(pending)-done, done)
	return nil
}

func outcomeOf(rep *Report) Outcome {
	if rep.HasFindings() {
		return OutcomeFindings
	}
	return OutcomeClean
}

// preflight runs the structural checks in plan §7 in order — the first failure
// returns an error and the run aborts before any write — and returns the
// reference-data lookups every row needs. It loads only the lists the target
// register's template uses.
func (s *Service) preflight(ctx context.Context, registerID int) (RefData, Target, error) {
	if err := s.entity.Health(ctx); err != nil {
		return RefData{}, Target{}, fmt.Errorf("compliance-entity health check: %w", err)
	}
	roles, err := s.entity.ListRoles(ctx)
	if err != nil {
		return RefData{}, Target{}, fmt.Errorf("list roles: %w", err)
	}
	teams, err := s.entity.ListRiskTeams(ctx)
	if err != nil {
		return RefData{}, Target{}, fmt.Errorf("list risk teams: %w", err)
	}
	cats, err := s.entity.ListRiskCategories(ctx)
	if err != nil {
		return RefData{}, Target{}, fmt.Errorf("list risk categories: %w", err)
	}
	scores, err := s.entity.ListRiskScores(ctx)
	if err != nil {
		return RefData{}, Target{}, fmt.Errorf("list risk scores: %w", err)
	}

	template := ""
	for _, t := range teams {
		if t.ID == registerID {
			template = t.RegisterTemplate
		}
	}

	var lk TemplateLookups
	switch template {
	case templateManagedServices:
		for _, l := range []struct {
			kind string
			into *[]RiskLookup
		}{
			{"customers", &lk.Customers},
			{"products", &lk.Products},
			{"deployment-types", &lk.DeploymentTypes},
		} {
			vals, err := s.entity.ListRiskLookups(ctx, l.kind)
			if err != nil {
				return RefData{}, Target{}, fmt.Errorf("list %s: %w", l.kind, err)
			}
			*l.into = vals
		}
	case templateAggregated:
		if lk.Platforms, err = s.entity.ListRiskLookups(ctx, "platforms"); err != nil {
			return RefData{}, Target{}, fmt.Errorf("list platforms: %w", err)
		}
	case templateStandard:
		if lk.ComplianceRefs, err = s.entity.ListComplianceRefs(ctx); err != nil {
			return RefData{}, Target{}, fmt.Errorf("list compliance references: %w", err)
		}
	}

	rd, err := buildRefData(teams, cats, scores, roles, lk)
	if err != nil {
		return RefData{}, Target{}, err
	}
	tgt, err := resolveTarget(rd, lk, registerID)
	if err != nil {
		return RefData{}, Target{}, &InputError{Msg: err.Error()}
	}
	return rd, tgt, nil
}

// summarizeCustomers fills the report's per-sequence block: how many risks each
// customer (or the register, off Managed Services) gets and the first and last
// risk code the run would assign. Read-only, so it runs on a dry run too; the
// numbers are read once, before anything is written.
func summarizeCustomers(ctx context.Context, log *slog.Logger, ec *EntityClient, rd RefData, rows []Row, progress map[int]ResumeState, rep *Report) error {
	summaries, err := computeCustomerSummaries(ctx, ec, rd, rows, progress)
	if err != nil {
		log.Error("could not compute the risk code summary — aborting", "err", err)
		return err
	}
	rep.SetCustomerSummaries(summaries)
	return nil
}

// selectMigratable returns the rows with no REJECT finding, in Migration ID order.
func selectMigratable(rows []Row, rep *Report) []Row {
	rejected := rep.RejectedMigrationIDs()
	out := make([]Row, 0, len(rows))
	for _, r := range rows {
		if _, bad := rejected[r.MigrationID]; bad {
			continue
		}
		out = append(out, r)
	}
	sortByMigrationID(out)
	return out
}
