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
  Divider,
  IconButton,
  LinearProgress,
  ToggleButton,
  ToggleButtonGroup,
  Typography,
} from "@wso2/oxygen-ui";
import { X } from "@wso2/oxygen-ui-icons-react";
import type { JSX } from "react";
import { useState } from "react";
import CompletionRing from "./CompletionRing";
import {
  CONTROL_STATUS_COLORS,
  CONTROL_STATUS_LABELS,
  PHASE_COLORS,
  PHASE_LABELS,
  PHASE_ORDER,
  STATUS_PHASE,
} from "@modules/audit/utils/controlStatus";
import type { ControlStatus } from "@modules/audit/types/audit";
import type { TeamCompletion, TeamStatusCount } from "@modules/audit/types/dashboard";

const REMAINING_COLOR = "#FB8C00";
const COMPLETED_COLOR = "#22C55E";
const OVERDUE_COLOR = "#E53935";

type ViewMode = "phase" | "detailed";

interface Row {
  key: string;
  label: string;
  color: string;
  count: number;
}

interface TeamInsightDialogProps {
  /** Selected team, or null to keep the dialog closed. */
  team: TeamCompletion | null;
  /** Accent color carried from the clicked row so the dot matches. */
  color: string;
  /** Per-team per-status counts from the dashboard payload. */
  teamStatusDistribution: TeamStatusCount[];
  onClose: () => void;
}

function StatTile({ label, value, color }: { label: string; value: number; color?: string }): JSX.Element {
  return (
    <Box sx={{ flex: 1, textAlign: "center" }}>
      <Typography variant="h5" fontWeight={700} sx={{ color: color ?? "text.primary", lineHeight: 1.1 }}>
        {value}
      </Typography>
      <Typography variant="caption" color="text.secondary">{label}</Typography>
    </Box>
  );
}

// Focused overview for one team: completion KPIs plus the team's own
// controls-by-status breakdown (the parent chart already handles comparison
// between teams, so none is repeated here).
export default function TeamInsightDialog({ team, color, teamStatusDistribution, onClose }: TeamInsightDialogProps): JSX.Element {
  const open = team !== null;
  const [mode, setMode] = useState<ViewMode>("phase");

  const completed = team?.completed ?? 0;
  const total = team?.total ?? 0;
  const remaining = total - completed;
  const overdue = team?.overdue ?? 0;
  const hasOverdue = team?.overdue !== undefined;
  const pct = total > 0 ? Math.round((completed / total) * 100) : 0;

  // This team's per-status counts.
  const teamCounts: Record<string, number> = {};
  for (const r of teamStatusDistribution) {
    if (team && r.team === team.team) teamCounts[r.status] = (teamCounts[r.status] ?? 0) + r.count;
  }
  const teamTotal = Object.values(teamCounts).reduce((s, c) => s + c, 0);

  const rows: Row[] =
    mode === "phase"
      ? PHASE_ORDER
          .map((phase) => ({
            key: phase,
            label: PHASE_LABELS[phase],
            color: PHASE_COLORS[phase],
            count: (Object.keys(STATUS_PHASE) as ControlStatus[])
              .filter((s) => STATUS_PHASE[s] === phase)
              .reduce((sum, s) => sum + (teamCounts[s] ?? 0), 0),
          }))
          .filter((r) => r.count > 0)
      : (Object.keys(CONTROL_STATUS_LABELS) as ControlStatus[])
          .map((s) => ({
            key: s,
            label: CONTROL_STATUS_LABELS[s],
            color: CONTROL_STATUS_COLORS[s],
            count: teamCounts[s] ?? 0,
          }))
          .filter((r) => r.count > 0)
          .sort((a, b) => b.count - a.count);

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
            <Typography variant="h6" fontWeight={700} lineHeight={1.2}>{team?.team}</Typography>
            <Typography variant="caption" color="text.secondary">
              Control completion across all active audits
            </Typography>
          </Box>
        </Box>
        <IconButton aria-label="Close" onClick={onClose} sx={{ position: "absolute", top: 12, right: 12 }}>
          <X size={18} />
        </IconButton>
      </DialogTitle>

      <DialogContent dividers>
        {/* KPI band */}
        <Box sx={{ display: "flex", alignItems: "center", gap: 3, mb: 3 }}>
          <CompletionRing percent={pct} size={104} />
          <Box sx={{ flex: 1, display: "flex", gap: 1 }}>
            <StatTile label="Completed" value={completed} color={COMPLETED_COLOR} />
            <Divider orientation="vertical" flexItem />
            <StatTile label="Remaining" value={remaining} color={remaining > 0 ? REMAINING_COLOR : undefined} />
            {hasOverdue && (
              <>
                <Divider orientation="vertical" flexItem />
                <StatTile label="Overdue" value={overdue} color={overdue > 0 ? OVERDUE_COLOR : undefined} />
              </>
            )}
            <Divider orientation="vertical" flexItem />
            <StatTile label="Total" value={total} />
          </Box>
        </Box>

        {/* Per-team status breakdown */}
        <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 1.5 }}>
          <Typography variant="subtitle2" fontWeight={700}>Controls by status</Typography>
          <ToggleButtonGroup
            size="small"
            exclusive
            value={mode}
            onChange={(_, v: ViewMode | null) => { if (v) setMode(v); }}
          >
            <ToggleButton value="phase" sx={{ px: 1.5, py: 0.25, textTransform: "none" }}>Phases</ToggleButton>
            <ToggleButton value="detailed" sx={{ px: 1.5, py: 0.25, textTransform: "none" }}>Detailed</ToggleButton>
          </ToggleButtonGroup>
        </Box>

        {rows.length === 0 ? (
          <Typography variant="body2" color="text.secondary" sx={{ py: 2 }}>
            No status breakdown available for this team.
          </Typography>
        ) : (
          <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5, maxHeight: 300, overflowY: "auto", pr: 0.5 }}>
            {rows.map((r) => {
              const share = teamTotal > 0 ? Math.round((r.count / teamTotal) * 100) : 0;
              return (
                <Box key={r.key}>
                  <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 0.5 }}>
                    <Box sx={{ width: 10, height: 10, borderRadius: "50%", bgcolor: r.color, flexShrink: 0 }} />
                    <Typography variant="body2" sx={{ flex: 1, minWidth: 0 }} noWrap title={r.label}>{r.label}</Typography>
                    <Typography variant="body2" color="text.secondary" sx={{ flexShrink: 0 }}>{r.count}</Typography>
                    <Typography variant="body2" fontWeight={700} sx={{ width: 42, textAlign: "right", flexShrink: 0 }}>
                      {share}%
                    </Typography>
                  </Box>
                  <LinearProgress
                    variant="determinate"
                    value={share}
                    sx={{
                      height: 6, borderRadius: 3, bgcolor: "#E0E0E0",
                      "[data-color-scheme='dark'] &": { bgcolor: "rgba(255,255,255,0.12)" },
                      "& .MuiLinearProgress-bar": { bgcolor: r.color, borderRadius: 3 },
                    }}
                  />
                </Box>
              );
            })}
          </Box>
        )}
      </DialogContent>
    </Dialog>
  );
}
