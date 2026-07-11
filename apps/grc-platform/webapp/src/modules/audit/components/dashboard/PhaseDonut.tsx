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

import { Box, ToggleButton, ToggleButtonGroup, Typography } from "@wso2/oxygen-ui";
import { PieChart } from "@wso2/oxygen-ui-charts-react";
import type { JSX } from "react";
import { useState } from "react";
import {
  CONTROL_STATUS_COLORS,
  CONTROL_STATUS_LABELS,
  PHASE_COLORS,
  PHASE_LABELS,
  PHASE_ORDER,
  STATUS_PHASE,
} from "@modules/audit/utils/controlStatus";
import type { ControlStatus } from "@modules/audit/types/audit";
import type { StatusCount } from "@modules/audit/types/dashboard";

type ViewMode = "phase" | "detailed";

interface Slice {
  key: string;
  label: string;
  color: string;
  value: number;
}

function toPhaseSlices(data: StatusCount[]): Slice[] {
  const totals: Record<string, number> = {};
  for (const d of data) {
    const phase = STATUS_PHASE[d.status as ControlStatus];
    if (phase) totals[phase] = (totals[phase] ?? 0) + d.count;
  }
  return PHASE_ORDER
    .map((phase) => ({ key: phase, label: PHASE_LABELS[phase], color: PHASE_COLORS[phase], value: totals[phase] ?? 0 }))
    .filter((s) => s.value > 0);
}

function toDetailedSlices(data: StatusCount[]): Slice[] {
  const countMap = Object.fromEntries(data.map((d) => [d.status, d.count]));
  return (Object.keys(CONTROL_STATUS_LABELS) as ControlStatus[])
    .map((status) => ({
      key: status,
      label: CONTROL_STATUS_LABELS[status],
      color: CONTROL_STATUS_COLORS[status],
      value: countMap[status] ?? 0,
    }))
    .filter((s) => s.value > 0);
}

export default function PhaseDonut({ data }: { data: StatusCount[] }): JSX.Element {
  const [mode, setMode] = useState<ViewMode>("phase");

  const slices = mode === "phase" ? toPhaseSlices(data) : toDetailedSlices(data);
  const total = slices.reduce((s, d) => s + d.value, 0);

  if (total === 0) {
    return (
      <Box sx={{ height: 240, display: "flex", alignItems: "center", justifyContent: "center" }}>
        <Typography variant="body2" color="text.secondary">No controls yet</Typography>
      </Box>
    );
  }

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
      <Box sx={{ display: "flex", justifyContent: "flex-end" }}>
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

      {/* Donut on top, legend below — full card width for labels at any zoom */}
      <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
        <Box sx={{ width: 180, height: 180, position: "relative", alignSelf: "center" }}>
          <PieChart
            data={slices}
            pies={[{ dataKey: "value", nameKey: "label", cx: "50%", cy: "50%", innerRadius: "55%", outerRadius: "85%", paddingAngle: 2 }]}
            colors={slices.map((s) => s.color)}
            legend={{ show: false }}
            tooltip={{ show: true }}
            margin={{ top: 5, right: 5, left: 5, bottom: 5 }}
            height={180}
          />
          {/* Center total (pointer-events none so tooltips still work) */}
          <Box sx={{ position: "absolute", inset: 0, display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", pointerEvents: "none" }}>
            <Typography variant="h5" fontWeight={700} lineHeight={1}>{total}</Typography>
            <Typography variant="caption" color="text.secondary">controls</Typography>
          </Box>
        </Box>

        <Box sx={{ display: "flex", flexDirection: "column", gap: 0.75, maxHeight: 170, overflowY: "auto", pr: 0.5 }}>
          {slices.map((entry) => (
            <Box key={entry.key} sx={{ display: "flex", alignItems: "center", gap: 1 }}>
              <Box sx={{ width: 10, height: 10, borderRadius: "50%", bgcolor: entry.color, flexShrink: 0 }} />
              <Typography variant="body2" sx={{ flex: 1, lineHeight: 1.3, minWidth: 0 }} noWrap title={entry.label}>
                {entry.label}
              </Typography>
              <Typography variant="body2" color="text.secondary" sx={{ flexShrink: 0 }}>{entry.value}</Typography>
              <Typography variant="body2" fontWeight={700} sx={{ width: 40, textAlign: "right", flexShrink: 0 }}>
                {`${Math.round((entry.value / total) * 100)}%`}
              </Typography>
            </Box>
          ))}
        </Box>
      </Box>
    </Box>
  );
}
