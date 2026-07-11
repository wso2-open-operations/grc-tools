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

import type { ControlStatus } from "@modules/audit/types/audit";

export const CONTROL_STATUS_LABELS: Record<ControlStatus, string> = {
  POPULATION_PENDING:            "Population Pending",
  POPULATION_INTERNAL_REVIEW:    "Population Internal Review",
  POPULATION_UNDER_VALIDATION:   "Population Under Validation",
  POPULATION_NEED_CLARIFICATION: "Population Need Clarification",
  POPULATION_COMPLETE:           "Population Complete",
  AWAITING_SAMPLE:               "Awaiting Sample",
  SUBMITTED_SAMPLE:              "Submitted Sample",
  EVIDENCE_PENDING:              "Evidence Pending",
  EVIDENCE_INTERNAL_REVIEW:      "Evidence Internal Review",
  EVIDENCE_UNDER_VALIDATION:     "Evidence Under Validation",
  EVIDENCE_NEED_CLARIFICATION:   "Evidence Need Clarification",
  COMPLETE:                      "Complete",
};

// ── 4-phase rollup for the dashboard donut ───────────────────────────────────
// Groups the 12 statuses into scannable phases; the donut offers a "Detailed"
// toggle that switches back to the full per-status breakdown.

export type ControlPhase = "NOT_STARTED" | "IN_PROGRESS" | "BLOCKED" | "COMPLETE";

export const STATUS_PHASE: Record<ControlStatus, ControlPhase> = {
  POPULATION_PENDING:            "NOT_STARTED",
  EVIDENCE_PENDING:              "NOT_STARTED",
  POPULATION_INTERNAL_REVIEW:    "IN_PROGRESS",
  POPULATION_UNDER_VALIDATION:   "IN_PROGRESS",
  POPULATION_COMPLETE:           "IN_PROGRESS",
  AWAITING_SAMPLE:               "IN_PROGRESS",
  SUBMITTED_SAMPLE:              "IN_PROGRESS",
  EVIDENCE_INTERNAL_REVIEW:      "IN_PROGRESS",
  EVIDENCE_UNDER_VALIDATION:     "IN_PROGRESS",
  POPULATION_NEED_CLARIFICATION: "BLOCKED",
  EVIDENCE_NEED_CLARIFICATION:   "BLOCKED",
  COMPLETE:                      "COMPLETE",
};

export const PHASE_LABELS: Record<ControlPhase, string> = {
  NOT_STARTED: "Not Started",
  IN_PROGRESS: "In Progress",
  BLOCKED:     "Needs Clarification",
  COMPLETE:    "Complete",
};

export const PHASE_COLORS: Record<ControlPhase, string> = {
  NOT_STARTED: "#94A3B8",
  IN_PROGRESS: "#3B82F6",
  BLOCKED:     "#EF4444",
  COMPLETE:    "#22C55E",
};

export const PHASE_ORDER: ControlPhase[] = ["NOT_STARTED", "IN_PROGRESS", "BLOCKED", "COMPLETE"];

export const CONTROL_STATUS_COLORS: Record<ControlStatus, string> = {
  // OE population phase
  POPULATION_PENDING:            "#6b7280",
  POPULATION_INTERNAL_REVIEW:    "#b45309",
  POPULATION_UNDER_VALIDATION:   "#7c3aed",
  POPULATION_NEED_CLARIFICATION: "#dc2626",
  POPULATION_COMPLETE:           "#0891b2",
  AWAITING_SAMPLE:               "#0369a1",
  SUBMITTED_SAMPLE:              "#0284c7",
  // Evidence phase
  EVIDENCE_PENDING:              "#ea580c",
  EVIDENCE_INTERNAL_REVIEW:      "#b45309",
  EVIDENCE_UNDER_VALIDATION:     "#7c3aed",
  EVIDENCE_NEED_CLARIFICATION:   "#dc2626",
  COMPLETE:                      "#16a34a",
};
