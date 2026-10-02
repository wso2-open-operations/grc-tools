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

// Package aivalidation is the in-process AI Validation feature: it assembles
// the context for one evidence or population submission, calls the LLM with
// a single tool call, and writes the advisory result back through the
// Compliance Entity.
//
// Nothing here ever blocks a submission or changes evidence/population/
// control status — every entry point is fire-and-forget from the caller's
// point of view (see Service.TriggerEvidence / TriggerPopulation).
package aivalidation

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/llm"
)

// errSubmissionNotFound signals that the current round wasn't found among an
// evidence submission's own listing (should be impossible outside a race with
// a concurrent delete) — reported distinctly from a fetch failure.
var errSubmissionNotFound = errors.New("submission not found")

// submissionRef identifies the submission a job or log row belongs to; exactly
// one of EvidenceID / PopulationID is set (chk_ai_owner).
type submissionRef struct {
	EvidenceID   int
	PopulationID int
	ControlID    int
}

func evidenceRef(evidenceID, controlID int) submissionRef {
	return submissionRef{EvidenceID: evidenceID, ControlID: controlID}
}

func populationRef(populationID, controlID int) submissionRef {
	return submissionRef{PopulationID: populationID, ControlID: controlID}
}

func (r submissionRef) isPopulation() bool { return r.PopulationID != 0 }

// logAttr names the owning id under the field name a reader would grep for
// ("evidenceId" or "populationId"), rather than always logging both.
func (r submissionRef) logAttr() slog.Attr {
	if r.isPopulation() {
		return slog.Int("populationId", r.PopulationID)
	}
	return slog.Int("evidenceId", r.EvidenceID)
}

// createdBySentinel is CreatedBy for every row written here; no user triggers it.
const createdBySentinel = "system:ai-validation"

// JobTimeout bounds one job from the moment it holds a worker slot;
// maxConcurrent sizes the shared worker pool; writeTimeout bounds each log-row
// write. Constants, not env vars: they never differ per environment.
const (
	JobTimeout    = 180 * time.Second
	maxConcurrent = 4
	writeTimeout  = 15 * time.Second
)

// Service runs AI Validation jobs. It is safe for concurrent use — each
// Trigger* call spawns its own detached goroutine, and the number running
// the LLM call concurrently is bounded by a shared worker-pool semaphore.
type Service struct {
	llm        llm.Caller
	repo       repository.AIValidationLogRepository
	control    ControlSource
	evidence   EvidenceSource
	population PopulationSource
	comment    CommentSource

	sem   chan struct{}
	dedup *dedupe
	// enabled is the master switch — AI_VALIDATION_ENABLED plus a non-empty
	// ANTHROPIC_API_KEY (checked at construction; see cmd/server/audit_deps.go).
	// false makes every Trigger* call a no-op.
	enabled bool
}

// NewService constructs a Service. enabled=false makes every Trigger* call a
// no-op — the caller (audit_deps.go) is responsible for resolving the
// AI_VALIDATION_ENABLED + ANTHROPIC_API_KEY guard before calling this.
func NewService(
	caller llm.Caller,
	repo repository.AIValidationLogRepository,
	control ControlSource,
	evidence EvidenceSource,
	population PopulationSource,
	comment CommentSource,
	enabled bool,
) *Service {
	return &Service{
		llm:        caller,
		repo:       repo,
		control:    control,
		evidence:   evidence,
		population: population,
		comment:    comment,
		sem:        make(chan struct{}, maxConcurrent),
		dedup:      newDedupe(),
		enabled:    enabled,
	}
}

// TriggerEvidence starts (or skips) an advisory AI validation job for one
// evidence submission. Runs detached from the request context and never
// blocks or returns an error. A nil *Service is a no-op.
func (s *Service) TriggerEvidence(auditID, controlID, evidenceID int, actor string, skip bool) {
	if s == nil || !s.enabled {
		return
	}
	go s.runEvidence(auditID, controlID, evidenceID, actor, skip)
}

// TriggerPopulation is TriggerEvidence for a population submission.
func (s *Service) TriggerPopulation(auditID, controlID, populationID int, actor string, skip bool) {
	if s == nil || !s.enabled {
		return
	}
	go s.runPopulation(auditID, controlID, populationID, actor, skip)
}

// submissionFetch loads whatever is specific to one submission kind: its
// file blocks, the manifest text describing them, and any "previous rounds"
// context (evidence: prior rounds + comments; also, for an OE control,
// the Sample Coverage context — population has neither). control is the
// same one run already fetched, passed through so fetch doesn't need its
// own lookup. Returning errSubmissionNotFound distinguishes "the round
// vanished" from an ordinary fetch failure so run can report each with its
// original message.
type submissionFetch func(ctx context.Context, control *model.AuditControl) (blocks []llm.Block, manifest, previous string, err error)

func (s *Service) runEvidence(auditID, controlID, evidenceID int, actor string, skip bool) {
	s.run(auditID, evidenceRef(evidenceID, controlID), actor, skip, submissionEvidence, func(ctx context.Context, control *model.AuditControl) ([]llm.Block, string, string, error) {
		rounds, err := s.evidence.List(ctx, auditID, controlID, true)
		if err != nil {
			return nil, "", "", err
		}
		var current *model.AuditEvidence
		for _, r := range rounds {
			if r.ID == evidenceID {
				current = r
				break
			}
		}
		if current == nil {
			return nil, "", "", errSubmissionNotFound
		}
		budget := newJobBudget()
		blocks, manifest := buildFileContent(ctx, s.evidence, evidenceFileRefs(current.Files), budget)
		previous := previousEvidenceContext(ctx, s.evidence, s.comment, auditID, controlID, evidenceID)

		sampleBlocks, sampleText := sampleContext(ctx, s.population, auditID, controlID, control, budget)
		blocks = append(blocks, sampleBlocks...)
		if sampleText != "" {
			previous += "\n" + sampleText
		}
		return blocks, manifest, previous, nil
	})
}

func (s *Service) runPopulation(auditID, controlID, populationID int, actor string, skip bool) {
	s.run(auditID, populationRef(populationID, controlID), actor, skip, submissionPopulation, func(ctx context.Context, _ *model.AuditControl) ([]llm.Block, string, string, error) {
		files, err := s.population.ListFiles(ctx, populationID)
		if err != nil {
			return nil, "", "", err
		}
		blocks, manifest := buildFileContent(ctx, s.population, populationFileRefs(files), newJobBudget())
		// Population multi-round context assembly is an open item, not yet
		// settled, so no "previous rounds" text here.
		return blocks, manifest, "", nil
	})
}

// run is the shared job lifecycle for both an evidence and a population
// submission: opt-out check, then one job per submission at a time — a
// trigger while a job is queued is absorbed by it, one while it is running
// makes it run once more afterwards, so the newest row always reflects the
// latest file set. fetch supplies the one piece of behaviour that actually
// differs between the two submission kinds.
func (s *Service) run(auditID int, ref submissionRef, actor string, skip bool, kind submissionKind, fetch submissionFetch) {
	if skip {
		s.writeSkipped(context.Background(), ref)
		return
	}
	if !s.dedup.claim(ref) {
		return
	}
	for {
		s.runOnce(auditID, ref, actor, kind, fetch)
		if !s.dedup.release(ref) {
			return
		}
	}
}

// runOnce is one pass of a job: PENDING row, worker-pool slot, control
// lookup, fetch, the LLM call (unless an identical input was already
// validated), and the terminal row.
func (s *Service) runOnce(auditID int, ref submissionRef, actor string, kind submissionKind, fetch submissionFetch) {
	s.writePending(context.Background(), ref)
	s.sem <- struct{}{}
	defer func() { <-s.sem }()
	s.dedup.markStarted(ref)

	// Started only once a worker slot is held, so time spent queued behind
	// other jobs doesn't eat into this job's own budget.
	ctx, cancel := context.WithTimeout(context.Background(), JobTimeout)
	defer cancel()

	control, err := s.control.GetByID(ctx, auditID, ref.ControlID)
	if err != nil || control == nil {
		s.writeError(ctx, ref, "could not load the control")
		slog.Warn("ai validation: control lookup failed", ref.logAttr(), "controlId", ref.ControlID, "actor", actor, "err", err)
		return
	}

	blocks, manifest, previous, err := fetch(ctx, control)
	if err != nil {
		if errors.Is(err, errSubmissionNotFound) {
			s.writeError(ctx, ref, "submission not found")
			return
		}
		s.writeError(ctx, ref, "could not load the submission")
		slog.Warn("ai validation: submission fetch failed", ref.logAttr(), "actor", actor, "err", err)
		return
	}
	if len(blocks) == 0 {
		s.writeError(ctx, ref, "no readable files in this submission")
		return
	}

	dynamicPrompt := buildDynamicPrompt(control, kind)
	intro := buildIntro(manifest, previous)
	key := inputKey(dynamicPrompt, intro, blocks)
	if cached, ok := s.dedup.lookup(key); ok {
		// Same input as an earlier job — reuse its verdict; zero tokens
		// recorded since nothing was sent to the model.
		s.writeResult(ctx, ref, &cached, llm.Usage{})
		return
	}

	result, usage, err := s.call(ctx, dynamicPrompt, intro, blocks)
	if err != nil {
		s.writeError(ctx, ref, "AI validation could not complete")
		slog.Warn("ai validation: call failed", ref.logAttr(), "actor", actor, "err", err)
		return
	}
	s.dedup.store(key, *result)
	s.writeResult(ctx, ref, result, usage)
}

// buildIntro is the user turn's leading text: the manifest plus optional
// previous-rounds context.
func buildIntro(manifest, previousContext string) string {
	var intro strings.Builder
	intro.WriteString("Review this submission.\n\n")
	intro.WriteString(manifest)
	if previousContext != "" {
		intro.WriteString("\n")
		intro.WriteString(previousContext)
	}
	return intro.String()
}

// call makes the one tool call: static + dynamic system prompt, then the
// intro text followed by the file blocks.
func (s *Service) call(ctx context.Context, dynamicPrompt, intro string, fileBlocks []llm.Block) (*validationResult, llm.Usage, error) {
	content := make([]llm.Block, 0, len(fileBlocks)+1)
	content = append(content, llm.NewTextBlock(intro))
	content = append(content, fileBlocks...)

	res, err := s.llm.Call(ctx, llm.Request{
		SystemStatic:  staticSystemPrompt,
		SystemDynamic: dynamicPrompt,
		Content:       content,
		Tool:          submitValidationTool(),
	})
	if err != nil {
		return nil, llm.Usage{}, err
	}
	var vr validationResult
	if err := json.Unmarshal(res.ToolInput, &vr); err != nil {
		return nil, llm.Usage{}, err
	}
	vr.Result = strings.ToUpper(strings.TrimSpace(vr.Result))
	return &vr, res.Usage, nil
}

// writeStatus appends a lifecycle row (PENDING, SKIPPED, ERROR) with no verdict.
func (s *Service) writeStatus(ctx context.Context, ref submissionRef, result string, summary *string) {
	s.writeRow(ctx, ref, model.CreateAIValidationLogRequest{
		ControlID: ref.ControlID,
		Result:    result,
		Summary:   summary,
		CreatedBy: createdBySentinel,
	})
}

func (s *Service) writePending(ctx context.Context, ref submissionRef) {
	s.writeStatus(ctx, ref, "PENDING", nil)
}

func (s *Service) writeSkipped(ctx context.Context, ref submissionRef) {
	s.writeStatus(ctx, ref, "SKIPPED", nil)
}

func (s *Service) writeError(ctx context.Context, ref submissionRef, summary string) {
	s.writeStatus(ctx, ref, "ERROR", &summary)
}

func (s *Service) writeResult(ctx context.Context, ref submissionRef, vr *validationResult, usage llm.Usage) {
	gapsJSON, err := json.Marshal(vr.GapsFound)
	if err != nil || vr.GapsFound == nil {
		gapsJSON = []byte("[]")
	}
	gaps, summary := string(gapsJSON), strings.TrimSpace(vr.Summary)

	result := vr.Result
	if result != "PASS" && result != "FAIL" && result != "UNCERTAIN" {
		// The model didn't return one of the three verdicts the tool schema
		// enumerates — treat as UNCERTAIN rather than reject a validation the
		// job otherwise completed (advisory only; a human still decides).
		result = "UNCERTAIN"
	}

	s.writeRow(ctx, ref, model.CreateAIValidationLogRequest{
		ControlID:                ref.ControlID,
		Result:                   result,
		GapsFound:                &gaps,
		Summary:                  &summary,
		CreatedBy:                createdBySentinel,
		InputTokens:              &usage.InputTokens,
		OutputTokens:             &usage.OutputTokens,
		CacheReadInputTokens:     &usage.CacheReadInputTokens,
		CacheCreationInputTokens: &usage.CacheCreationInputTokens,
	})
}

// writeRow appends one row, best-effort. A failure here is logged and
// swallowed — there is no retry, and no way to surface it to anyone but the
// logs, since this always runs detached from a request.
func (s *Service) writeRow(ctx context.Context, ref submissionRef, req model.CreateAIValidationLogRequest) {
	// Detached from the job's deadline: the ERROR row for a timed-out job
	// must still land, or the submission is left showing PENDING.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), writeTimeout)
	defer cancel()

	var err error
	if ref.isPopulation() {
		err = s.repo.CreateForPopulation(ctx, ref.PopulationID, req)
	} else {
		err = s.repo.CreateForEvidence(ctx, ref.EvidenceID, req)
	}
	if err != nil {
		slog.Warn("ai validation: failed to write result row",
			ref.logAttr(), "result", req.Result, "err", err)
	}
}
