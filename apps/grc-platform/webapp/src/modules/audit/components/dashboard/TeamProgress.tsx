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

import { Box, LinearProgress, Typography, useTheme } from "@wso2/oxygen-ui";
import { ChevronRight } from "@wso2/oxygen-ui-icons-react";
import type { JSX } from "react";
import type { TeamCompletion } from "@modules/audit/types/dashboard";

// Bars use the active theme's primary colour (adapts to every theme, light and
// dark) and flip to the theme's success colour only at 100% — so the colour
// change itself signals completion rather than decorating team identity.

interface TeamProgressProps {
  data: TeamCompletion[];
  /** Opens the team insight dialog; color matches the clicked row's bar. */
  onTeamClick?: (team: TeamCompletion, color: string) => void;
}

// Horizontal completion bars per team — bars beat donuts for comparing ratios.
// Rows are clickable to open a focused, comparative team overview.
export default function TeamProgress({ data, onTeamClick }: TeamProgressProps): JSX.Element {
  const theme = useTheme();

  if (data.length === 0) {
    return (
      <Box sx={{ height: 240, display: "flex", alignItems: "center", justifyContent: "center" }}>
        <Typography variant="body2" color="text.secondary">No team data</Typography>
      </Box>
    );
  }

  const sorted = [...data].sort((a, b) => b.total - a.total);
  const clickable = Boolean(onTeamClick);

  return (
    <Box sx={{ display: "flex", flexDirection: "column", flex: "1 1 0", minHeight: 240 }}>
      {/* flex-basis 0: fills the row height set by the tallest sibling card and
          scrolls beyond it (see AuditProgressList for the same pattern). */}
      <Box sx={{ display: "flex", flexDirection: "column", gap: 1, flex: "1 1 0", minHeight: 0, overflowY: "auto", pr: 0.5 }}>
        {sorted.map((team) => {
          const pct = team.total > 0 ? (team.completed / team.total) * 100 : 0;
          const color = pct >= 100 ? theme.palette.success.main : theme.palette.primary.main;
          return (
            <Box
              key={team.team}
              {...(clickable
                ? {
                    role: "button",
                    tabIndex: 0,
                    onClick: () => onTeamClick?.(team, color),
                    onKeyDown: (e: React.KeyboardEvent) => {
                      if (e.key === "Enter" || e.key === " ") {
                        e.preventDefault();
                        onTeamClick?.(team, color);
                      }
                    },
                  }
                : {})}
              sx={{
                borderRadius: 1.5, px: 1, py: 0.75, mx: -1,
                cursor: clickable ? "pointer" : "default",
                transition: "background 0.15s",
                "&:hover .team-chevron": { opacity: clickable ? 0.6 : 0 },
                ...(clickable && { "&:hover": { bgcolor: "action.hover" } }),
              }}
            >
              <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 0.5 }}>
                <Typography variant="body2" fontWeight={600} noWrap title={team.team} sx={{ mr: 1 }}>
                  {team.team}
                </Typography>
                <Box sx={{ display: "flex", alignItems: "center", gap: 0.5, flexShrink: 0 }}>
                  <Typography variant="body2" color="text.secondary">
                    {team.completed}/{team.total} · {Math.round(pct)}%
                  </Typography>
                  {clickable && (
                    <ChevronRight className="team-chevron" size={15} style={{ opacity: 0, transition: "opacity 0.15s" }} />
                  )}
                </Box>
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

    </Box>
  );
}
