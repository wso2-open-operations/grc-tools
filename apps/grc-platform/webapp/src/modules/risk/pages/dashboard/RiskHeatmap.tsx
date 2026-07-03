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

import { Box, Tooltip, Typography } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import type { HeatmapCell, RiskScore } from "../../api/riskApi";
import { LEVEL_LABELS } from "./constants";

interface RiskHeatmapProps {
  cells: HeatmapCell[];
  // The full 3×3 risk_score matrix, used to color cells that hold no risks.
  scores: RiskScore[];
}

const AXIS_VALUES = [1, 2, 3];

// Custom 3×3 likelihood × impact heatmap (residual risk). Likelihood increases
// upward, impact rightward; each cell shows its open-risk count tinted with
// the matrix cell's severity color.
export default function RiskHeatmap({ cells, scores }: RiskHeatmapProps): JSX.Element {
  const countAt = new Map(cells.map((c) => [`${c.likelihood}-${c.impact}`, c.count]));
  const scoreAt = new Map(scores.map((s) => [`${s.likelihood}-${s.impact}`, s]));

  return (
    <Box sx={{ display: "flex", alignItems: "stretch", gap: 1 }}>
      <Box
        sx={{
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          writingMode: "vertical-rl",
          transform: "rotate(180deg)",
        }}
      >
        <Typography variant="caption" color="text.secondary">
          Likelihood (Residual Risk)
        </Typography>
      </Box>
      <Box sx={{ flex: 1 }}>
        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: "auto repeat(3, 1fr)",
            gap: "2px",
          }}
        >
          {[...AXIS_VALUES].reverse().map((likelihood) => (
            <Box key={likelihood} sx={{ display: "contents" }}>
              <Box sx={{ display: "flex", alignItems: "center", pr: 1 }}>
                <Typography variant="caption" color="text.secondary">
                  {likelihood}
                </Typography>
              </Box>
              {AXIS_VALUES.map((impact) => {
                const score = scoreAt.get(`${likelihood}-${impact}`);
                const count = countAt.get(`${likelihood}-${impact}`) ?? 0;
                const color = score?.color_code ?? "#9e9e9e";
                const levelName = score ? (LEVEL_LABELS[score.risk_level] ?? score.risk_level) : "";
                return (
                  <Tooltip
                    key={impact}
                    title={`Likelihood ${likelihood} × Impact ${impact}${levelName ? ` — ${levelName}` : ""}: ${count} open risk${count === 1 ? "" : "s"}`}
                  >
                    <Box
                      sx={{
                        aspectRatio: "1.6",
                        minHeight: 56,
                        display: "flex",
                        alignItems: "center",
                        justifyContent: "center",
                        borderRadius: 1,
                        bgcolor: `${color}26`,
                        border: `2px solid ${color}`,
                      }}
                    >
                      <Typography
                        variant="h6"
                        fontWeight={700}
                        color={count === 0 ? "text.disabled" : "text.primary"}
                      >
                        {count}
                      </Typography>
                    </Box>
                  </Tooltip>
                );
              })}
            </Box>
          ))}
          <Box />
          {AXIS_VALUES.map((impact) => (
            <Box key={impact} sx={{ textAlign: "center", pt: 0.5 }}>
              <Typography variant="caption" color="text.secondary">
                {impact}
              </Typography>
            </Box>
          ))}
        </Box>
        <Typography
          variant="caption"
          color="text.secondary"
          sx={{ display: "block", textAlign: "center", mt: 0.5 }}
        >
          Impact (Residual Risk)
        </Typography>
      </Box>
    </Box>
  );
}
