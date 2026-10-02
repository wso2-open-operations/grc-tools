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

import { Box, Button, CircularProgress, Collapse, LinearProgress, Paper, Typography } from "@wso2/oxygen-ui";
import { AlertTriangle, Bot, ChevronDown, ChevronRight, Sparkles } from "@wso2/oxygen-ui-icons-react";
import { useState, type JSX } from "react";
import { useGetEvidence } from "@modules/audit/api/useGetEvidence";
import { useGetPopulation } from "@modules/audit/api/useGetPopulation";
import {
  isFreshPending,
  parseGaps,
  useGetAIValidation,
  useGetPopulationAIValidation,
  type AIGap,
  type AIValidationLog,
} from "@modules/audit/api/useGetAIValidation";
import { useAuditPrivileges } from "@modules/audit/hooks/useAuditPrivileges";
import { AuditPrivilege } from "@modules/audit/privileges";

const AI_PURPLE = "#7c3aed";
const AI_PURPLE_BG = "#faf5ff";

const SEVERITY_ORDER: AIGap["severity"][] = ["HIGH", "MEDIUM", "LOW"];
const SEVERITY_STYLE: Record<AIGap["severity"], { label: string; color: string }> = {
  HIGH:   { label: "High",   color: "#dc2626" },
  MEDIUM: { label: "Medium", color: "#b45309" },
  LOW:    { label: "Low",    color: "#6b7280" },
};

// Terminal/skipped result → row label + colour.
const RESULT_STYLE: Record<"PASS" | "FAIL" | "UNCERTAIN" | "SKIPPED", { label: string; color: string }> = {
  PASS:      { label: "AI: Looks Complete",                 color: "#16a34a" },
  FAIL:      { label: "AI: Issues Found",                    color: "#dc2626" },
  UNCERTAIN: { label: "AI: Needs Human Review",              color: "#b45309" },
  SKIPPED:   { label: "AI validation skipped by submitter",  color: "#6b7280" },
};

const PASS_NOTE = "Meets the requirement.";
const ADVISORY_REVIEWER = "Advisory only - your decision is authoritative.";

// Deploy-time switch mirroring the backend's AI_VALIDATION_ENABLED, so the card
// never promises a review the backend won't run.
const AI_VALIDATION_ENABLED = window.config?.GRC_PLATFORM_AI_VALIDATION_ENABLED === true;

interface AIValidationCardProps {
  auditId: number;
  controlId: number;
  variant: "submitter" | "reviewer";
  /** Which submission this card validates. Defaults to "evidence". */
  phase?: "evidence" | "population";
  /** Section heading; defaults to "AI Validation". */
  title?: string;
}

/**
 * AIValidationCard renders the advisory AI pre-review for a control's latest
 * evidence or population submission. Internal-only — an external caller
 * never sees this at all, regardless of caller.
 *
 * The content sits in an outlined "AI Validation" section box, like the
 * drawer's other sections. Inside it, the state is a single line - icon,
 * state, and (FAIL/UNCERTAIN, or PASS with LOW gaps) a chevron - collapsed by
 * default. PENDING, SKIPPED and ERROR rows have nothing further to show.
 * Clicking the chevron reveals a headline and the gaps grouped by severity -
 * manual click only, never auto-expanded. A clean PASS is the label plus a
 * generic "Meets the requirement." line. Submitter and reviewer see the same
 * text.
 */
export default function AIValidationCard({ auditId, controlId, variant, phase = "evidence", title }: AIValidationCardProps): JSX.Element | null {
  const { can, loading: privilegesLoading } = useAuditPrivileges();
  const isInternal = can(AuditPrivilege.ViewInternalComments);

  // Computed once; every phase-dependent pick below keys off this instead of
  // re-testing `phase` at each call site.
  const isPopulation = phase === "population";
  const evidenceEnabled = !isPopulation && AI_VALIDATION_ENABLED && isInternal;
  const populationEnabled = isPopulation && AI_VALIDATION_ENABLED && isInternal;

  const { data: submissions } = useGetEvidence(auditId, controlId, evidenceEnabled);
  const { data: population } = useGetPopulation(auditId, controlId, populationEnabled);
  const latestEvidenceId = evidenceEnabled ? (submissions?.[0]?.id ?? null) : null;
  const latestPopulationId = populationEnabled ? (population?.round?.id ?? null) : null;

  const evidenceValidations = useGetAIValidation(auditId, controlId, evidenceEnabled ? latestEvidenceId : null);
  const populationValidations = useGetPopulationAIValidation(
    auditId,
    controlId,
    populationEnabled ? latestPopulationId : null,
    populationEnabled ? (population?.round?.updatedAt ?? null) : null,
  );
  const { data: validations, isLoading, isError, refetch } = isPopulation ? populationValidations : evidenceValidations;
  const latestId = isPopulation ? latestPopulationId : latestEvidenceId;

  if (!AI_VALIDATION_ENABLED || privilegesLoading || !isInternal) return null;

  const latest = validations?.[0];

  // A failed fetch is not "no result" — say so instead of the submit prompt / hiding.
  if (latestId !== null && !latest && isError) {
    return (
      <AIBox title={title}>
        <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 1 }}>
          <StaticLine icon={<AlertTriangle size={14} color="#b45309" />} text="Couldn't load AI validation." />
          <Button size="small" onClick={() => void refetch()}>
            Retry
          </Button>
        </Box>
      </AIBox>
    );
  }

  // Reviewer variant stays out of the way until there is something to show.
  if (variant === "reviewer" && (latestId === null || !latest)) {
    return null;
  }

  // No submission yet: nothing has run, nothing to expand.
  if (latestId === null || (!latest && !isLoading)) {
    return (
      <AIBox title={title}>
        <StaticLine icon={<Bot size={14} />} text="AI review runs automatically after you submit." />
      </AIBox>
    );
  }
  if (!latest) {
    return (
      <AIBox title={title}>
        <StaticLine icon={<CircularProgress size={12} />} text="Loading AI review…" />
      </AIBox>
    );
  }

  return (
    <AIBox title={title}>
      {/* Keyed so a new result remounts collapsed instead of inheriting the old row's expanded state. */}
      <AIValidationRow key={`${phase}-${latest.id}`} latest={latest} showReviewerNote={can(AuditPrivilege.ReviewEvidence)} />
    </AIBox>
  );
}

/** Outlined section box matching the drawer's other sections (header + body). */
function AIBox({ children, title = "AI Validation" }: { children: React.ReactNode; title?: string }): JSX.Element {
  return (
    <Paper variant="outlined" sx={{ borderRadius: 2, overflow: "hidden", display: "flex", flexDirection: "column" }}>
      <Box sx={{ px: 2.5, py: 1.5, display: "flex", alignItems: "center", gap: 1.25, borderBottom: 1, borderColor: "divider", bgcolor: "action.hover" }}>
        <Box sx={{ width: 30, height: 30, borderRadius: 1.5, color: AI_PURPLE, display: "flex", alignItems: "center", justifyContent: "center", flexShrink: 0 }}>
          <Sparkles size={16} />
        </Box>
        <Typography variant="subtitle2" fontWeight={700}>
          {title}
        </Typography>
      </Box>
      <Box sx={{ p: 2.5 }}>{children}</Box>
    </Paper>
  );
}

/** A single muted line — icon + text, no box, no border. */
function StaticLine({ icon, text }: { icon: JSX.Element; text: string }): JSX.Element {
  return (
    <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
      <Box sx={{ display: "flex", color: "text.secondary", flexShrink: 0 }}>{icon}</Box>
      <Typography variant="body2" color="text.secondary">
        {text}
      </Typography>
    </Box>
  );
}

function AIValidationRow({ latest, showReviewerNote }: { latest: AIValidationLog; showReviewerNote: boolean }): JSX.Element {
  const [expanded, setExpanded] = useState(false);

  if (isFreshPending(latest)) {
    return (
      <Box sx={{ display: "flex", flexDirection: "column", gap: 0.5 }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          <Sparkles size={14} color={AI_PURPLE} />
          <Typography variant="body2" color="text.secondary">
            Analyzing evidence…
          </Typography>
        </Box>
        <LinearProgress
          sx={{
            height: 3,
            borderRadius: 1,
            "& .MuiLinearProgress-bar": { bgcolor: AI_PURPLE },
            bgcolor: AI_PURPLE_BG,
            "[data-color-scheme='dark'] &": { bgcolor: `${AI_PURPLE}33` },
          }}
        />
      </Box>
    );
  }

  // ERROR, or a PENDING row that never resolved (stale): unavailable.
  if (latest.result === "ERROR" || latest.result === "PENDING") {
    return <StaticLine icon={<AlertTriangle size={14} color="#b45309" />} text="AI validation unavailable - proceed as usual" />;
  }

  if (latest.result === "SKIPPED") {
    const style = RESULT_STYLE.SKIPPED;
    return <StaticLine icon={<Sparkles size={14} color={style.color} />} text={style.label} />;
  }

  const style = RESULT_STYLE[latest.result];
  const gaps = parseGaps(latest.gapsFound);

  // A clean PASS is the label plus one generic line — never per-rule detail.
  if (latest.result === "PASS" && gaps.length === 0) {
    return (
      <Box sx={{ display: "flex", flexDirection: "column", gap: 0.25 }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          <Sparkles size={14} color={style.color} />
          <Typography variant="body2" fontWeight={600} sx={{ color: style.color }}>
            {style.label}
          </Typography>
        </Box>
        <Typography variant="caption" color="text.secondary" sx={{ pl: 3 }}>
          {PASS_NOTE}
        </Typography>
      </Box>
    );
  }

  return (
    <Box>
      <Box
        role="button"
        tabIndex={0}
        aria-expanded={expanded}
        onClick={() => setExpanded((v) => !v)}
        onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); setExpanded((v) => !v); } }}
        sx={{ display: "flex", alignItems: "center", gap: 1, cursor: "pointer", border: "none", background: "none", p: 0, width: "100%", textAlign: "left" }}
      >
        <Sparkles size={14} color={style.color} />
        <Typography variant="body2" fontWeight={600} sx={{ color: style.color }}>
          {style.label}
        </Typography>
        {gaps.length > 0 && (
          <Typography variant="caption" color="text.secondary">· {gaps.length} {gaps.length === 1 ? "issue" : "issues"}</Typography>
        )}
        <Box sx={{ flex: 1 }} />
        {expanded ? <ChevronDown size={15} /> : <ChevronRight size={15} />}
      </Box>
      <Collapse in={expanded}>
        <Box sx={{ mt: 1.25, p: 1.75, borderRadius: 2, bgcolor: "action.hover", display: "flex", flexDirection: "column", gap: 1 }}>
          {latest.summary && (
            <Typography variant="body2" fontWeight={600} sx={{ lineHeight: 1.6 }}>
              {latest.summary}
            </Typography>
          )}
          <GapGroups gaps={gaps} />
          {showReviewerNote && (
            <Typography variant="caption" color="text.secondary">
              ⓘ {ADVISORY_REVIEWER}
            </Typography>
          )}
        </Box>
      </Collapse>
    </Box>
  );
}

/** Gaps grouped under High / Medium / Low headings, most severe first. */
function GapGroups({ gaps }: { gaps: AIGap[] }): JSX.Element | null {
  if (gaps.length === 0) return null;
  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
      {SEVERITY_ORDER.map((sev) => {
        const group = gaps.filter((g) => g.severity === sev);
        if (group.length === 0) return null;
        const style = SEVERITY_STYLE[sev];
        return (
          <Box key={sev}>
            <Typography variant="caption" fontWeight={700} sx={{ color: style.color, textTransform: "uppercase", letterSpacing: 0.5 }}>
              {style.label} · {group.length}
            </Typography>
            <Box sx={{ display: "flex", flexDirection: "column", gap: 1.25, mt: 0.5 }}>
              {group.map((g, i) => (
                <Box key={i} sx={{ display: "flex", gap: 1 }}>
                  <Box sx={{ mt: 0.65, width: 8, height: 8, borderRadius: "50%", bgcolor: style.color, flexShrink: 0 }} />
                  <Box>
                    <Typography variant="body2" sx={{ fontWeight: 600, lineHeight: 1.5 }}>
                      {g.requirementAspect}
                      {g.fileName ? (
                        <Typography component="span" variant="caption" color="text.secondary">
                          {" "}({g.fileName})
                        </Typography>
                      ) : null}
                    </Typography>
                    <Typography variant="body2" color="text.secondary" sx={{ lineHeight: 1.55 }}>
                      {g.issue}
                    </Typography>
                  </Box>
                </Box>
              ))}
            </Box>
          </Box>
        );
      })}
    </Box>
  );
}
