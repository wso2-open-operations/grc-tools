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

package aivalidation

import (
	"context"
	"fmt"
	"strings"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/llm"
)

// ControlSource is the subset of service.ControlService this package needs —
// control number/description/evidence requirement/requirement type.
type ControlSource interface {
	GetByID(ctx context.Context, auditID, controlID int) (*model.AuditControl, error)
}

// EvidenceSource is the subset of service.EvidenceService this package
// needs: fetching file bytes plus listing rounds for the "up to 2 previous
// submissions" context. Satisfied by service.EvidenceService.
type EvidenceSource interface {
	FileDownloader
	List(ctx context.Context, auditID, controlID int, includeRejected bool) ([]*model.AuditEvidence, error)
}

// PopulationSource is the subset of service.PopulationService this package
// needs: fetching file bytes, the current round's files, and (for the Sample
// Coverage rule) the control's latest population round.
type PopulationSource interface {
	FileDownloader
	ListFiles(ctx context.Context, populationID int) ([]*model.PopulationFile, error)
	LatestRound(ctx context.Context, auditID, controlID int) (*model.AuditPopulation, error)
}

// CommentSource is the subset of service.CommentService this package needs —
// always called with includeInternal=false: external-visible comments only,
// never internal reviewer notes.
type CommentSource interface {
	List(ctx context.Context, auditID, controlID int, includeInternal bool) ([]*model.AuditComment, error)
}

// evidenceFileRefs converts a round's files to fileRefs for buildFileContent.
func evidenceFileRefs(files []*model.AuditEvidenceFile) []fileRef {
	out := make([]fileRef, 0, len(files))
	for _, f := range files {
		out = append(out, fileRef{ID: f.ID, Name: f.FileName})
	}
	return out
}

// populationFileRefs converts a round's files to fileRefs, excluding
// SAMPLE-kind files — those are the auditor's own selection, not part of the
// team's population submission being validated.
func populationFileRefs(files []*model.PopulationFile) []fileRef {
	out := make([]fileRef, 0, len(files))
	for _, f := range files {
		if !strings.EqualFold(f.FileKind, "SAMPLE") {
			out = append(out, fileRef{ID: f.ID, Name: f.FileName})
		}
	}
	return out
}

// sampleFileRefs is populationFileRefs' complement — only SAMPLE-kind files,
// the auditor's own selection, for the Sample Coverage rule.
func sampleFileRefs(files []*model.PopulationFile) []fileRef {
	out := make([]fileRef, 0, len(files))
	for _, f := range files {
		if strings.EqualFold(f.FileKind, "SAMPLE") {
			out = append(out, fileRef{ID: f.ID, Name: f.FileName})
		}
	}
	return out
}

// sampleContext returns the external auditor's sample for an OE control's
// evidence validation — the free-text sample_reference note (held on the
// control itself, not the population row) plus any SAMPLE-kind files on the
// control's latest population round — as content blocks plus a labeled text
// section, so the Sample Coverage rule can check the evidence submission
// actually corresponds to what was sampled.
//
// Returns (nil, "") for a DESIGN control, a control with no population round
// yet, or one with neither a note nor sample files — the rule's own text
// says to skip entirely when this section is absent.
func sampleContext(ctx context.Context, popSvc PopulationSource, auditID, controlID int, control *model.AuditControl, budget *jobBudget) ([]llm.Block, string) {
	if !strings.EqualFold(control.RequirementType, "OE") {
		return nil, ""
	}
	round, err := popSvc.LatestRound(ctx, auditID, controlID)
	if err != nil || round == nil {
		return nil, ""
	}
	files, err := popSvc.ListFiles(ctx, round.ID)
	if err != nil {
		files = nil
	}
	refs := sampleFileRefs(files)

	note := ""
	if control.SampleReference != nil {
		note = strings.TrimSpace(*control.SampleReference)
	}
	if note == "" && len(refs) == 0 {
		return nil, ""
	}

	blocks, manifest := buildFileContent(ctx, popSvc, refs, budget)

	var b strings.Builder
	b.WriteString("## Sample selected by the external auditor\n\n")
	if note != "" {
		fmt.Fprintf(&b, "Auditor's note: %s\n\n", note)
	}
	b.WriteString(manifest)
	return blocks, b.String()
}

// previousEvidenceContext returns a text block describing up to 2 earlier
// evidence rounds (ids strictly before currentEvidenceID, newest first) plus
// external-visible reviewer comments on this control — never internal notes.
// Returns "" when there is nothing earlier to report.
func previousEvidenceContext(ctx context.Context, evSvc EvidenceSource, commentSvc CommentSource, auditID, controlID, currentEvidenceID int) string {
	rounds, err := evSvc.List(ctx, auditID, controlID, true)
	if err != nil {
		return ""
	}
	var previous []*model.AuditEvidence
	for _, r := range rounds {
		if r.ID < currentEvidenceID {
			previous = append(previous, r)
			if len(previous) == 2 {
				break
			}
		}
	}
	if len(previous) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Previous submissions on this control\n\n")
	for _, r := range previous {
		fmt.Fprintf(&b, "Round #%d (status: %s):\n", r.ID, r.Status)
		for _, f := range r.Files {
			fmt.Fprintf(&b, "- file: %s\n", f.FileName)
		}
		if r.Attestation != "" {
			fmt.Fprintf(&b, "- submitter note: %s\n", r.Attestation)
		}
	}
	b.WriteString("\n")

	comments, err := commentSvc.List(ctx, auditID, controlID, false)
	if err == nil && len(comments) > 0 {
		b.WriteString("## Reviewer comments visible to the external auditor\n\n")
		for _, c := range comments {
			fmt.Fprintf(&b, "- %s\n", c.Content)
		}
		b.WriteString("\n")
	}
	return b.String()
}
