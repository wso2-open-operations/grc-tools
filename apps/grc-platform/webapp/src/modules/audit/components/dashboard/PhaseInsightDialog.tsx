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

import {
  Box,
  Dialog,
  DialogContent,
  DialogTitle,
  IconButton,
  LinearProgress,
  Typography,
} from "@wso2/oxygen-ui";
import { X } from "@wso2/oxygen-ui-icons-react";
import type { JSX } from "react";
import {
  CONTROL_STATUS_COLORS,
  CONTROL_STATUS_LABELS,
  PHASE_COLORS,
  PHASE_LABELS,
  STATUS_PHASE,
  type ControlPhase,
} from "@modules/audit/utils/controlStatus";
import type { ControlStatus } from "@modules/audit/types/audit";
import type { StatusCount } from "@modules/audit/types/dashboard";

// Plain-language meaning + "what's next" for each phase.
const PHASE_DESCRIPTION: Record<ControlPhase, string> = {
  NOT_STARTED: "Work hasn't begun — population or evidence collection is still pending.",
  IN_PROGRESS: "Underway — populations or evidence are being prepared, reviewed, or validated.",
  BLOCKED:     "Waiting on clarification before work can continue.",
  COMPLETE:    "Fully reviewed and approved - no further action needed.",
};

const PHASE_NEXT: Record<ControlPhase, string> = {
  NOT_STARTED: "Controls move to In Progress once population or evidence work begins.",
  IN_PROGRESS: "Controls leave In Progress once evidence is approved, or get flagged for clarification.",
  BLOCKED:     "Resolve the clarification to return a control to In Progress.",
  COMPLETE:    "These controls count toward your overall completion percentage.",
};

interface PhaseInsightDialogProps {
  /** Selected phase, or null to keep the dialog closed. */
  phase: ControlPhase | null;
  /** Global per-status counts from the dashboard payload. */
  statusDistribution: StatusCount[];
  onClose: () => void;
}

// Focused overview for one phase: its share of all controls and the individual
// statuses that roll into it. Derived entirely from statusDistribution.
export default function PhaseInsightDialog({ phase, statusDistribution, onClose }: PhaseInsightDialogProps): JSX.Element {
  const open = phase !== null;
  const color = phase ? PHASE_COLORS[phase] : "#000";

  const countMap = Object.fromEntries(statusDistribution.map((d) => [d.status, d.count]));
  const grandTotal = statusDistribution.reduce((s, d) => s + d.count, 0);

  // Constituent statuses for this phase, largest first.
  const statuses = phase
    ? (Object.keys(CONTROL_STATUS_LABELS) as ControlStatus[])
        .filter((s) => STATUS_PHASE[s] === phase)
        .map((s) => ({
          status: s,
          label: CONTROL_STATUS_LABELS[s],
          color: CONTROL_STATUS_COLORS[s],
          count: (countMap[s] as number | undefined) ?? 0,
        }))
        .filter((s) => s.count > 0)
        .sort((a, b) => b.count - a.count)
    : [];

  const phaseTotal = statuses.reduce((s, d) => s + d.count, 0);
  const shareOfAll = grandTotal > 0 ? Math.round((phaseTotal / grandTotal) * 100) : 0;

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="sm"
      fullWidth
      // Dark scheme lightens Dialog paper via an elevation-overlay gradient —
      // strip it so the surface matches the dashboard cards.
      slotProps={{ paper: { sx: { backgroundImage: "none", borderRadius: 2 } } }}
    >
      <DialogTitle sx={{ pr: 6 }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.25 }}>
          <Box sx={{ width: 12, height: 12, borderRadius: "50%", bgcolor: color, flexShrink: 0 }} />
          <Box>
            <Typography variant="h6" fontWeight={700} lineHeight={1.2}>
              {phase ? PHASE_LABELS[phase] : ""}
            </Typography>
            <Typography variant="caption" color="text.secondary">
              {phase ? PHASE_DESCRIPTION[phase] : ""}
            </Typography>
          </Box>
        </Box>
        <IconButton aria-label="Close" onClick={onClose} sx={{ position: "absolute", top: 12, right: 12 }}>
          <X size={18} />
        </IconButton>
      </DialogTitle>

      <DialogContent dividers>
        {/* KPI band — phase count + share of all controls */}
        <Box sx={{ display: "flex", alignItems: "baseline", gap: 1, mb: 0.5 }}>
          <Typography variant="h4" fontWeight={700} sx={{ color }}>{phaseTotal}</Typography>
          <Typography variant="body1" color="text.secondary">
            control{phaseTotal === 1 ? "" : "s"} · {shareOfAll}% of all {grandTotal}
          </Typography>
        </Box>
        <LinearProgress
          variant="determinate"
          value={shareOfAll}
          sx={{
            height: 8, borderRadius: 4, mb: 3, bgcolor: "#E0E0E0",
            "[data-color-scheme='dark'] &": { bgcolor: "rgba(255,255,255,0.12)" },
            "& .MuiLinearProgress-bar": { bgcolor: color, borderRadius: 4 },
          }}
        />

        {/* Constituent status breakdown */}
        <Typography variant="subtitle2" fontWeight={700} sx={{ mb: 1.5 }}>
          Breakdown by status
        </Typography>
        <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5, mb: 2.5 }}>
          {statuses.map((s) => {
            const pctOfPhase = phaseTotal > 0 ? Math.round((s.count / phaseTotal) * 100) : 0;
            return (
              <Box key={s.status}>
                <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 0.5 }}>
                  <Box sx={{ width: 10, height: 10, borderRadius: "50%", bgcolor: s.color, flexShrink: 0 }} />
                  <Typography variant="body2" sx={{ flex: 1, minWidth: 0 }} noWrap title={s.label}>{s.label}</Typography>
                  <Typography variant="body2" color="text.secondary" sx={{ flexShrink: 0 }}>{s.count}</Typography>
                  <Typography variant="body2" fontWeight={700} sx={{ width: 42, textAlign: "right", flexShrink: 0 }}>
                    {pctOfPhase}%
                  </Typography>
                </Box>
                <LinearProgress
                  variant="determinate"
                  value={pctOfPhase}
                  sx={{
                    height: 6, borderRadius: 3, bgcolor: "#E0E0E0",
                    "[data-color-scheme='dark'] &": { bgcolor: "rgba(255,255,255,0.12)" },
                    "& .MuiLinearProgress-bar": { bgcolor: s.color, borderRadius: 3 },
                  }}
                />
              </Box>
            );
          })}
        </Box>

        <Typography variant="caption" color="text.secondary">
          {phase ? PHASE_NEXT[phase] : ""}
        </Typography>
      </DialogContent>
    </Dialog>
  );
}
