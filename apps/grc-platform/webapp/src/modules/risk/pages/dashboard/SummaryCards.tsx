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

import { Box, Typography } from "@wso2/oxygen-ui";
import { Card, CardContent } from "@mui/material";
import { AlertTriangle, CheckCircle, Clock, ClipboardList } from "@wso2/oxygen-ui-icons-react";
import type { JSX } from "react";
import type * as React from "react";
import type { RiskStatusSummary } from "../../api/riskApi";

interface SummaryCardsProps {
  summary: RiskStatusSummary;
}

interface StatTileProps {
  count: number;
  pct?: number | null;
  label: string;
  icon: React.ReactNode;
  iconColor: "primary" | "error" | "success" | "warning";
}

function StatTile({ count, pct, label, icon, iconColor }: StatTileProps): JSX.Element {
  return (
    <Card variant="outlined">
      <CardContent>
        <Box sx={{ display: "flex", alignItems: "center", gap: 2 }}>
          <Box sx={{ color: `${iconColor}.main`, display: "flex", alignItems: "center" }}>
            {icon}
          </Box>
          <Box>
            <Box sx={{ display: "flex", alignItems: "baseline", gap: 0.75 }}>
              <Typography variant="h4" component="span" sx={{ fontWeight: 400, lineHeight: 1 }}>
                {count}
              </Typography>
              {pct != null && (
                <Typography variant="body2" component="span" color="text.secondary">
                  ({pct}%)
                </Typography>
              )}
            </Box>
            <Typography variant="body2" color="text.secondary">
              {label}
            </Typography>
          </Box>
        </Box>
      </CardContent>
    </Card>
  );
}

// The Total / Open / Closed stat tiles at the top of the dashboard.
export default function SummaryCards({ summary }: SummaryCardsProps): JSX.Element {
  const pct = (n: number): number | null =>
    summary.total > 0 ? Math.round((n / summary.total) * 100) : null;

  return (
    <Box
      sx={{
        display: "grid",
        gridTemplateColumns: { xs: "1fr", sm: "repeat(2, 1fr)", md: "repeat(4, 1fr)" },
        gap: 2,
      }}
    >
      <StatTile count={summary.total}   label="Total Risks" icon={<ClipboardList size={24} />} iconColor="primary" />
      <StatTile count={summary.open}    pct={pct(summary.open)}   label="Open"    icon={<AlertTriangle size={24} />} iconColor="error"   />
      <StatTile count={summary.closed}  pct={pct(summary.closed)} label="Closed"  icon={<CheckCircle size={24} />}   iconColor="success" />
      <StatTile count={summary.overdue} pct={pct(summary.overdue)} label="Overdue" icon={<Clock size={24} />}        iconColor="warning" />
    </Box>
  );
}
