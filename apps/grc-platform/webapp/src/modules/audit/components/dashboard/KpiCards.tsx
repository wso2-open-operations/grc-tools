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
import { AlertTriangle, CheckCircle, ClipboardList, Inbox } from "@wso2/oxygen-ui-icons-react";
import type { JSX, ReactNode } from "react";

interface KpiCardProps {
  icon: ReactNode;
  iconColor: string;
  value: number;
  label: string;
  sub?: string;
  onClick?: () => void;
  valueColor?: string;
}

function KpiCard({ icon, iconColor, value, label, sub, onClick, valueColor }: KpiCardProps): JSX.Element {
  return (
    <Paper
      variant="outlined"
      onClick={onClick}
      sx={{
        borderRadius: 2,
        p: 2.5,
        flex: 1,
        minWidth: 160,
        display: "flex",
        alignItems: "center",
        gap: 2,
        ...(onClick && {
          cursor: "pointer",
          transition: "border-color 0.15s, box-shadow 0.15s",
          "&:hover": { borderColor: iconColor, boxShadow: 1 },
        }),
      }}
    >
      <Box
        sx={{
          width: 44, height: 44, borderRadius: 1.5, flexShrink: 0,
          display: "flex", alignItems: "center", justifyContent: "center",
          color: iconColor, bgcolor: `${iconColor}18`,
          "[data-color-scheme='dark'] &": { bgcolor: `${iconColor}33` },
        }}
      >
        {icon}
      </Box>
      <Box sx={{ minWidth: 0 }}>
        <Typography variant="h4" fontWeight={700} lineHeight={1.1} color={valueColor}>
          {value}
        </Typography>
        <Typography variant="body2" color="text.secondary" noWrap>{label}</Typography>
        {sub && (
          <Typography variant="caption" color="text.secondary" noWrap sx={{ display: "block" }}>
            {sub}
          </Typography>
        )}
      </Box>
    </Paper>
  );
}

interface KpiCardsProps {
  totalControls: number;
  completedControls: number;
  completionPercent: number;
  overdueControls: number;
  /** null hides the awaiting card entirely (e.g. management). */
  awaitingCount: number | null;
  awaitingLabel: string;
  onAwaitingClick: () => void;
  onOverdueClick: () => void;
}

export default function KpiCards({
  totalControls,
  completedControls,
  completionPercent,
  overdueControls,
  awaitingCount,
  awaitingLabel,
  onAwaitingClick,
  onOverdueClick,
}: KpiCardsProps): JSX.Element {
  return (
    <Box sx={{ display: "flex", gap: 2, flexWrap: "wrap" }}>
      <KpiCard
        icon={<ClipboardList size={22} />}
        iconColor="#1E88E5"
        value={totalControls}
        label="Total Controls"
        sub="across active audits"
      />
      <KpiCard
        icon={<CheckCircle size={22} />}
        iconColor="#22C55E"
        value={completedControls}
        label="Completed"
        sub={`${completionPercent.toFixed(1)}% of scope`}
        valueColor="#22C55E"
      />
      {awaitingCount !== null && (
        <KpiCard
          icon={<Inbox size={22} />}
          iconColor="#FB8C00"
          value={awaitingCount}
          label={awaitingLabel}
          sub="click to view"
          onClick={onAwaitingClick}
          valueColor={awaitingCount > 0 ? "#FB8C00" : undefined}
        />
      )}
      <KpiCard
        icon={<AlertTriangle size={22} />}
        iconColor={overdueControls > 0 ? "#E53935" : "#78909C"}
        value={overdueControls}
        label="Overdue"
        sub={overdueControls > 0 ? "needs attention" : "all on schedule"}
        onClick={onOverdueClick}
        valueColor={overdueControls > 0 ? "#E53935" : undefined}
      />
    </Box>
  );
}
