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

import { Box, LinearProgress, Typography } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import type { TeamCompletion } from "@modules/audit/types/dashboard";

const TEAM_COLORS = [
  "#1E88E5", "#43A047", "#FB8C00", "#8E24AA", "#E53935",
  "#039BE5", "#FFB300", "#AB47BC", "#EF5350", "#26A69A",
];

// Horizontal completion bars per team — bars beat donuts for comparing ratios.
export default function TeamProgress({ data }: { data: TeamCompletion[] }): JSX.Element {
  if (data.length === 0) {
    return (
      <Box sx={{ height: 240, display: "flex", alignItems: "center", justifyContent: "center" }}>
        <Typography variant="body2" color="text.secondary">No team data</Typography>
      </Box>
    );
  }

  const sorted = [...data].sort((a, b) => b.total - a.total);

  return (
    // flex-basis 0: fills the row height set by the tallest sibling card and
    // scrolls beyond it (see AuditProgressList for the same pattern).
    <Box sx={{ display: "flex", flexDirection: "column", gap: 1.75, flex: "1 1 0", minHeight: 240, overflowY: "auto", pr: 0.5 }}>
      {sorted.map((team, i) => {
        const color = TEAM_COLORS[i % TEAM_COLORS.length];
        const pct = team.total > 0 ? (team.completed / team.total) * 100 : 0;
        return (
          <Box key={team.team}>
            <Box sx={{ display: "flex", justifyContent: "space-between", mb: 0.5 }}>
              <Typography variant="body2" fontWeight={600} noWrap title={team.team} sx={{ mr: 1 }}>
                {team.team}
              </Typography>
              <Typography variant="body2" color="text.secondary" sx={{ flexShrink: 0 }}>
                {team.completed}/{team.total} · {Math.round(pct)}%
              </Typography>
            </Box>
            <LinearProgress
              variant="determinate"
              value={pct}
              sx={{
                height: 8, borderRadius: 4, bgcolor: "#E0E0E0",
                "[data-color-scheme='dark'] &": { bgcolor: "rgba(255,255,255,0.12)" },
                "& .MuiLinearProgress-bar": { bgcolor: color, borderRadius: 4 },
              }}
            />
          </Box>
        );
      })}
    </Box>
  );
}
