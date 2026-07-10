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

import { Box, Chip, Paper, Stack, Typography } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import type { RegisterAnalytics, RiskScore } from "../../api/riskApi";
import { LEVEL_FALLBACK_COLORS, LEVEL_LABELS } from "./constants";
import LevelTreatmentChart from "./LevelTreatmentChart";
import RiskHeatmap from "./RiskHeatmap";

interface RegisterSectionProps {
  register: RegisterAnalytics;
  scores: RiskScore[];
}

// One per-register dashboard section: residual heatmap on the left, the
// level × treatment stacked bar on the right, level-count chips up top.
export default function RegisterSection({ register, scores }: RegisterSectionProps): JSX.Element {
  return (
    <Paper variant="outlined" sx={{ p: 2.5 }}>
      <Stack direction="row" alignItems="center" spacing={1.5} flexWrap="wrap" sx={{ mb: 2 }}>
        <Typography variant="subtitle1" fontWeight={600}>
          {register.register_name}
        </Typography>
        <Chip size="small" label={`${register.open_count} open`} variant="outlined" />
        {register.level_counts.map((lc) => (
          <Chip
            key={lc.risk_level}
            size="small"
            label={`${LEVEL_LABELS[lc.risk_level] ?? lc.risk_level}: ${lc.count}`}
            sx={{
              bgcolor: `${lc.color_code || LEVEL_FALLBACK_COLORS[lc.risk_level]}26`,
              border: `1px solid ${lc.color_code || LEVEL_FALLBACK_COLORS[lc.risk_level]}`,
            }}
          />
        ))}
      </Stack>
      <Box
        sx={{
          display: "grid",
          gridTemplateColumns: { xs: "1fr", md: "6fr 6fr" },
          gap: 3,
          alignItems: "center",
        }}
      >
        <RiskHeatmap cells={register.heatmap} scores={scores} />
        <LevelTreatmentChart data={register.level_treatments} />
      </Box>
    </Paper>
  );
}
