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

import { Box, Paper, Typography } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import type { RiskStatusSummary } from "../../api/riskApi";
import { CLOSED_COLOR, OPEN_COLOR } from "./constants";

interface SummaryCardsProps {
  summary: RiskStatusSummary;
}

interface TileProps {
  label: string;
  value: number;
  share?: number | null;
  dotColor?: string;
}

function Tile({ label, value, share, dotColor }: TileProps): JSX.Element {
  return (
    <Paper variant="outlined" sx={{ p: 2.5, flex: 1, minWidth: 160 }}>
      <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
        {dotColor && (
          <Box
            component="span"
            sx={{ width: 10, height: 10, borderRadius: "50%", bgcolor: dotColor }}
          />
        )}
        <Typography variant="body2" color="text.secondary">
          {label}
        </Typography>
      </Box>
      <Typography variant="h4" fontWeight={700} sx={{ mt: 0.5 }}>
        {value}
        {share != null && (
          <Typography component="span" variant="body1" color="text.secondary" sx={{ ml: 1 }}>
            ({share}%)
          </Typography>
        )}
      </Typography>
    </Paper>
  );
}

// The Total / Open / Closed stat tiles at the top of the dashboard.
export default function SummaryCards({ summary }: SummaryCardsProps): JSX.Element {
  const pct = (n: number): number | null =>
    summary.total > 0 ? Math.round((n / summary.total) * 100) : null;

  return (
    <Box sx={{ display: "flex", gap: 2, flexWrap: "wrap" }}>
      <Tile label="Total Risks" value={summary.total} />
      <Tile label="Open" value={summary.open} share={pct(summary.open)} dotColor={OPEN_COLOR} />
      <Tile
        label="Closed"
        value={summary.closed}
        share={pct(summary.closed)}
        dotColor={CLOSED_COLOR}
      />
    </Box>
  );
}
