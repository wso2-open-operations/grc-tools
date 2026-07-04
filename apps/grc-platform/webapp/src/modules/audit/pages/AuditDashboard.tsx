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
  Alert,
  Box,
  Chip,
  Divider,
  LinearProgress,
  Paper,
  Skeleton,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
} from "@mui/material";
import {
  CheckCircle,
  ClipboardList,
  FolderOpen,
} from "@wso2/oxygen-ui-icons-react";
import { PieChart, BarChart, Pie, Cell } from "@wso2/oxygen-ui-charts-react";
import { Sector } from "recharts";
import type { JSX } from "react";
import { useRef, useState } from "react";
import { useNavigate } from "react-router";
import { useGetDashboard } from "@modules/audit/api/useGetDashboard";
import { useAuditRole } from "@modules/audit/hooks/useAuditRole";
import type { AuditRole } from "@modules/audit/hooks/useAuditRole";
import { CONTROL_STATUS_COLORS, CONTROL_STATUS_LABELS } from "@modules/audit/utils/controlStatus";
import type { ControlStatus } from "@modules/audit/types/audit";
import type { ActionItem, OverdueControl, StatusCount, TeamCompletion } from "@modules/audit/types/dashboard";

function statusColor(s: string): string {
  return CONTROL_STATUS_COLORS[s as ControlStatus] ?? "#90A4AE";
}
function statusLabel(s: string): string {
  return CONTROL_STATUS_LABELS[s as ControlStatus] ?? s;
}

// ── Action label per role/status ─────────────────────────────────────────────

function actionLabel(status: string, role: AuditRole): string {
  switch (status) {
    case "EVIDENCE_PENDING":               return "Submit evidence";
    case "SUBMITTED_SAMPLE":              return "Submit evidence for sample";
    case "EVIDENCE_NEED_CLARIFICATION":   return "Resubmit evidence";
    case "POPULATION_PENDING":            return "Submit population";
    case "POPULATION_NEED_CLARIFICATION": return "Resubmit population";
    case "EVIDENCE_INTERNAL_REVIEW":
    case "POPULATION_INTERNAL_REVIEW":
      return role === "compliance_admin" ? "Review & approve" : "Pending review";
    case "EVIDENCE_UNDER_VALIDATION":     return "Approve / request resubmission";
    case "POPULATION_UNDER_VALIDATION":   return "Approve / reject population";
    case "POPULATION_COMPLETE":           return "Submit sample";
    case "AWAITING_SAMPLE":               return "Submit sample";
    default:                              return "Action required";
  }
}

// ── Role label ────────────────────────────────────────────────────────────────

const ROLE_LABELS: Record<AuditRole, string> = {
  compliance_admin: "Compliance Admin",
  compliance_team:  "Compliance Team",
  internal_team:    "Internal Team",
  external_auditor: "External Auditor",
  management:       "Management",
  unknown:          "",
};

// ── Active shape that doesn't expand (fixes the triangle/spike on hover) ─────

function StillSector(props: Record<string, unknown>): JSX.Element {
  // recharts passes the *expanded* outerRadius to activeShape; we subtract back
  // the default active offset (5px) so the sector stays the same size on hover.
  return (
    <Sector
      {...(props as Parameters<typeof Sector>[0])}
      outerRadius={((props.outerRadius as number) ?? 0) - 5}
    />
  );
}

// ── Status donut chart ────────────────────────────────────────────────────────

function StatusDonut({ data }: { data: StatusCount[] }): JSX.Element {
  // Merge API data into the full 12-status list so all statuses always appear.
  const countMap = Object.fromEntries(data.map((d) => [d.status, d.count]));
  const pieData = (Object.keys(CONTROL_STATUS_LABELS) as ControlStatus[]).map((status) => ({
    status,
    label: CONTROL_STATUS_LABELS[status],
    color: CONTROL_STATUS_COLORS[status],
    value: countMap[status] ?? 0,
  })).filter((s) => s.value > 0); // only show statuses that have controls

  const total = pieData.reduce((s, d) => s + d.value, 0);

  if (total === 0) {
    return (
      <Box sx={{ height: 240, display: "flex", alignItems: "center", justifyContent: "center" }}>
        <Typography variant="body2" color="text.secondary">No controls yet</Typography>
      </Box>
    );
  }

  return (
    <Box sx={{ display: "flex", gap: 3, alignItems: "center" }}>
      {/* Donut */}
      <Box sx={{ width: 220, height: 220, flexShrink: 0 }}>
        <PieChart
          legend={{ show: false }}
          margin={{ top: 5, right: 5, left: 5, bottom: 5 }}
          height={220}
          tooltip={{ show: true }}
        >
          <Pie
            data={pieData}
            dataKey="value"
            nameKey="label"
            cx="50%"
            cy="50%"
            innerRadius="50%"
            outerRadius="80%"
            paddingAngle={2}
            strokeWidth={0}
            activeShape={StillSector as unknown as React.FC}
          >
            {pieData.map((entry, i) => (
              <Cell key={entry.status} fill={entry.color} />
            ))}
          </Pie>
        </PieChart>
      </Box>

      {/* Legend — single column */}
      <Box sx={{ flex: 1, display: "flex", flexDirection: "column", gap: 0.9 }}>
        {pieData.map((entry) => (
          <Box key={entry.status} sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <Box sx={{ width: 10, height: 10, borderRadius: "50%", bgcolor: entry.color, flexShrink: 0 }} />
            <Typography variant="body2" sx={{ flex: 1, lineHeight: 1.3 }} noWrap title={entry.label}>
              {entry.label}
            </Typography>
            <Typography variant="body2" fontWeight={700} sx={{ ml: 0.5 }}>
              {total > 0 ? `${Math.round((entry.value / total) * 100)}%` : "0%"}
            </Typography>
          </Box>
        ))}
      </Box>
    </Box>
  );
}

// ── Team donut chart ──────────────────────────────────────────────────────────

function TeamDonut({ data }: { data: TeamCompletion[] }): JSX.Element {
  if (data.length === 0) {
    return (
      <Box sx={{ height: 240, display: "flex", alignItems: "center", justifyContent: "center" }}>
        <Typography variant="body2" color="text.secondary">No team data</Typography>
      </Box>
    );
  }

  const TEAM_COLORS = [
    "#1E88E5","#43A047","#FB8C00","#8E24AA","#E53935",
    "#039BE5","#FFB300","#AB47BC","#EF5350","#26A69A",
  ];

  const pieData = data.map((d, i) => ({
    name: d.team,
    value: d.total,
    color: TEAM_COLORS[i % TEAM_COLORS.length],
    completed: d.completed,
    total: d.total,
  }));
  const totalControls = data.reduce((s, d) => s + d.total, 0);

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
      {/* Donut */}
      <Box sx={{ width: "100%", height: 200 }}>
        <PieChart
          legend={{ show: false }}
          margin={{ top: 5, right: 5, left: 5, bottom: 5 }}
          height={200}
          tooltip={{ show: true }}
        >
          <Pie
            data={pieData}
            dataKey="value"
            nameKey="name"
            cx="50%"
            cy="50%"
            innerRadius="45%"
            outerRadius="75%"
            paddingAngle={2}
            strokeWidth={0}
            activeShape={StillSector as unknown as React.FC}
          >
            {pieData.map((entry) => (
              <Cell key={entry.name} fill={entry.color} />
            ))}
          </Pie>
        </PieChart>
      </Box>

      {/* Legend with completion bars */}
      <Box sx={{ display: "flex", flexDirection: "column", gap: 0.75 }}>
        {pieData.map((entry) => (
          <Box key={entry.name} sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <Box sx={{ width: 10, height: 10, borderRadius: "50%", bgcolor: entry.color, flexShrink: 0 }} />
            <Typography variant="body2" sx={{ width: 110, flexShrink: 0 }} noWrap title={entry.name}>
              {entry.name}
            </Typography>
            <LinearProgress
              variant="determinate"
              value={entry.total > 0 ? (entry.completed / entry.total) * 100 : 0}
              sx={{ flex: 1, height: 6, borderRadius: 3, bgcolor: "#E0E0E0",
                "& .MuiLinearProgress-bar": { bgcolor: entry.color } }}
            />
            <Typography variant="body2" color="text.secondary" sx={{ width: 44, textAlign: "right", flexShrink: 0 }}>
              {entry.completed}/{entry.total}
            </Typography>
          </Box>
        ))}
      </Box>
    </Box>
  );
}

// ── Overview banner (replaces 8 stat cards + completion bar) ─────────────────

interface OverviewBannerProps {
  auditStats: { totalAudits: number; activeAudits: number; completedAudits: number; archivedAudits: number };
  stats: { totalControls: number; completedControls: number; evidenceRequiredControls: number; overdueControls: number; completionPercent: number };
  onOverdueClick: () => void;
  onEvidenceClick: () => void;
}

function OverviewBanner({ auditStats, stats, onOverdueClick, onEvidenceClick }: OverviewBannerProps): JSX.Element {
  const { totalAudits, activeAudits, completedAudits, archivedAudits } = auditStats;
  const { totalControls, completedControls, evidenceRequiredControls, overdueControls, completionPercent } = stats;

  return (
    <Paper variant="outlined" sx={{ borderRadius: 2, display: "flex", overflow: "hidden" }}>
      {/* ── Audits panel ───────────────────────────────────────────────────── */}
      <Box sx={{ flex: 1, p: 3, display: "flex", flexDirection: "column", gap: 2 }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          <Box sx={{ width: 28, height: 28, borderRadius: 1, bgcolor: "#1E88E518", display: "flex", alignItems: "center", justifyContent: "center", color: "#1E88E5" }}>
            <FolderOpen size={16} />
          </Box>
          <Typography variant="overline" color="text.secondary" sx={{ lineHeight: 1 }}>Audits</Typography>
        </Box>

        {/* Primary number: Active */}
        <Box sx={{ display: "flex", alignItems: "baseline", gap: 1.5 }}>
          <Typography variant="h3" fontWeight={700} color="#1E88E5" lineHeight={1}>
            {activeAudits}
          </Typography>
          <Typography variant="body1" color="text.secondary">Active</Typography>
        </Box>

        {/* Secondary breakdown */}
        <Box sx={{ display: "flex", gap: 0, alignItems: "stretch" }}>
          <Box sx={{ textAlign: "center", px: 2, pl: 0 }}>
            <Typography variant="h5" fontWeight={700}>{totalAudits}</Typography>
            <Typography variant="body2" color="text.secondary">Total</Typography>
          </Box>
          <Divider orientation="vertical" flexItem />
          <Box sx={{ textAlign: "center", px: 2 }}>
            <Typography variant="h5" fontWeight={700} color="#43A047">{completedAudits}</Typography>
            <Typography variant="body2" color="text.secondary">Completed</Typography>
          </Box>
          <Divider orientation="vertical" flexItem />
          <Box sx={{ textAlign: "center", px: 2 }}>
            <Typography variant="h5" fontWeight={700} color="#78909C">{archivedAudits}</Typography>
            <Typography variant="body2" color="text.secondary">Archived</Typography>
          </Box>
        </Box>
      </Box>

      <Divider orientation="vertical" flexItem />

      {/* ── Controls panel ─────────────────────────────────────────────────── */}
      <Box sx={{ flex: 1.4, p: 3, display: "flex", flexDirection: "column", gap: 2 }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          <Box sx={{ width: 28, height: 28, borderRadius: 1, bgcolor: "#43A04718", display: "flex", alignItems: "center", justifyContent: "center", color: "#43A047" }}>
            <ClipboardList size={16} />
          </Box>
          <Typography variant="overline" color="text.secondary" sx={{ lineHeight: 1 }}>Controls</Typography>
        </Box>

        {/* Total + progress bar */}
        <Box sx={{ display: "flex", alignItems: "center", gap: 3 }}>
          <Box sx={{ flexShrink: 0 }}>
            <Typography variant="h3" fontWeight={700} lineHeight={1}>{totalControls}</Typography>
            <Typography variant="body2" color="text.secondary">Total</Typography>
          </Box>
          <Box sx={{ flex: 1 }}>
            <Box sx={{ display: "flex", justifyContent: "space-between", mb: 0.75 }}>
              <Typography variant="caption" color="text.secondary">Completion</Typography>
              <Typography variant="body2" fontWeight={700} color="primary.main">
                {completionPercent.toFixed(1)}%
              </Typography>
            </Box>
            <LinearProgress
              variant="determinate"
              value={Math.min(completionPercent, 100)}
              sx={{ height: 8, borderRadius: 4 }}
            />
          </Box>
        </Box>

        {/* 3 key metrics */}
        <Box sx={{ display: "flex", gap: 0, alignItems: "stretch" }}>
          <Box sx={{ textAlign: "center", px: 2, pl: 0 }}>
            <Typography variant="h5" fontWeight={700} color="#43A047">{completedControls}</Typography>
            <Typography variant="body2" color="text.secondary">Completed</Typography>
          </Box>
          <Divider orientation="vertical" flexItem />
          <Box
            onClick={onEvidenceClick}
            sx={{
              textAlign: "center", px: 2, borderRadius: 1.5, cursor: "pointer",
              transition: "background 0.15s",
              "&:hover": { bgcolor: "#FB8C0012" },
            }}
          >
            <Typography variant="h5" fontWeight={700} color="#FB8C00">{evidenceRequiredControls}</Typography>
            <Typography variant="body2" color="text.secondary">Evidence Required</Typography>
          </Box>
          <Divider orientation="vertical" flexItem />
          <Box
            onClick={onOverdueClick}
            sx={{
              textAlign: "center", px: 2, borderRadius: 1.5, cursor: "pointer",
              transition: "background 0.15s",
              "&:hover": { bgcolor: overdueControls > 0 ? "#E5393512" : "#78909C12" },
            }}
          >
            <Typography variant="h5" fontWeight={700} color={overdueControls > 0 ? "#E53935" : "#78909C"}>
              {overdueControls}
            </Typography>
            <Typography variant="body2" color="text.secondary">Overdue</Typography>
          </Box>
        </Box>
      </Box>
    </Paper>
  );
}

// ── Section card ──────────────────────────────────────────────────────────────

function SectionCard({ title, children }: { title: string; children: React.ReactNode }): JSX.Element {
  return (
    <Paper variant="outlined" sx={{ borderRadius: 2, overflow: "hidden" }}>
      <Box sx={{ px: 2.5, py: 1.5, borderBottom: 1, borderColor: "divider" }}>
        <Typography variant="subtitle1" fontWeight={700}>{title}</Typography>
      </Box>
      <Box sx={{ p: 2.5 }}>{children}</Box>
    </Paper>
  );
}

// ── Action items list ─────────────────────────────────────────────────────────

function ActionItemsList({ items, role }: { items: ActionItem[]; role: AuditRole }): JSX.Element {
  const navigate = useNavigate();

  if (items.length === 0) {
    return (
      <Box sx={{ py: 3, textAlign: "center" }}>
        <CheckCircle size={32} color="#43A047" />
        <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
          No pending actions — you're all caught up!
        </Typography>
      </Box>
    );
  }

  return (
    <TableContainer>
      <Table size="small">
        <TableHead>
          <TableRow>
            <TableCell sx={{ fontWeight: 600, whiteSpace: "nowrap" }}>Control</TableCell>
            <TableCell sx={{ fontWeight: 600 }}>Audit</TableCell>
            <TableCell sx={{ fontWeight: 600 }}>Action needed</TableCell>
            <TableCell sx={{ fontWeight: 600 }}>Status</TableCell>
            <TableCell sx={{ fontWeight: 600, whiteSpace: "nowrap" }}>Due date</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {items.map((item) => (
            <TableRow
              key={item.controlId}
              hover
              sx={{ cursor: "pointer" }}
              onClick={() => void navigate(`/audit/audits/${item.auditId}`)}
            >
              <TableCell sx={{ whiteSpace: "nowrap", fontWeight: 600 }}>{item.controlNumber}</TableCell>
              <TableCell sx={{ maxWidth: 200 }}>
                <Typography variant="body2" noWrap title={item.auditName}>{item.auditName}</Typography>
              </TableCell>
              <TableCell>
                <Typography variant="body2" color="primary.main">{actionLabel(item.status, role)}</Typography>
              </TableCell>
              <TableCell>
                <Chip
                  label={statusLabel(item.status)}
                  size="small"
                  sx={{ bgcolor: `${statusColor(item.status)}18`, color: statusColor(item.status), fontWeight: 600, fontSize: "0.7rem" }}
                />
              </TableCell>
              <TableCell sx={{ whiteSpace: "nowrap", color: item.dueDate ? "text.primary" : "text.disabled" }}>
                {item.dueDate || "—"}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  );
}

// ── Overdue controls list ─────────────────────────────────────────────────────

function OverdueList({ items }: { items: OverdueControl[] }): JSX.Element {
  const navigate = useNavigate();

  if (items.length === 0) {
    return (
      <Box sx={{ py: 3, textAlign: "center" }}>
        <CheckCircle size={32} color="#43A047" />
        <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
          No overdue controls
        </Typography>
      </Box>
    );
  }

  return (
    <TableContainer>
      <Table size="small">
        <TableHead>
          <TableRow>
            <TableCell sx={{ fontWeight: 600, whiteSpace: "nowrap" }}>Control</TableCell>
            <TableCell sx={{ fontWeight: 600 }}>Description</TableCell>
            <TableCell sx={{ fontWeight: 600 }}>Audit</TableCell>
            <TableCell sx={{ fontWeight: 600 }}>Status</TableCell>
            <TableCell sx={{ fontWeight: 600, whiteSpace: "nowrap" }}>Due date</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {items.map((item) => (
            <TableRow
              key={item.controlId}
              hover
              sx={{ cursor: "pointer" }}
              onClick={() => void navigate(`/audit/audits/${item.auditId}`)}
            >
              <TableCell sx={{ whiteSpace: "nowrap", fontWeight: 600 }}>{item.controlNumber}</TableCell>
              <TableCell sx={{ maxWidth: 260 }}>
                <Typography variant="body2" noWrap title={item.description}>{item.description}</Typography>
              </TableCell>
              <TableCell sx={{ maxWidth: 180 }}>
                <Typography variant="body2" noWrap title={item.auditName}>{item.auditName}</Typography>
              </TableCell>
              <TableCell>
                <Chip
                  label={statusLabel(item.status)}
                  size="small"
                  sx={{ bgcolor: `${statusColor(item.status)}18`, color: statusColor(item.status), fontWeight: 600, fontSize: "0.7rem" }}
                />
              </TableCell>
              <TableCell sx={{ whiteSpace: "nowrap", color: "error.main", fontWeight: 600 }}>{item.dueDate}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  );
}

// ── Skeleton ──────────────────────────────────────────────────────────────────

function DashboardSkeleton(): JSX.Element {
  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 3 }}>
      <Skeleton variant="rectangular" height={150} sx={{ borderRadius: 2 }} />
      <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 2 }}>
        <Skeleton variant="rectangular" height={300} sx={{ borderRadius: 2 }} />
        <Skeleton variant="rectangular" height={300} sx={{ borderRadius: 2 }} />
      </Box>
      <Skeleton variant="rectangular" height={240} sx={{ borderRadius: 2 }} />
      <Skeleton variant="rectangular" height={200} sx={{ borderRadius: 2 }} />
    </Box>
  );
}

// ── Main ──────────────────────────────────────────────────────────────────────

export default function AuditDashboard(): JSX.Element {
  const role = useAuditRole();
  const { data, isLoading, isError } = useGetDashboard();

  const overdueRef = useRef<HTMLDivElement>(null);
  const actionItemsRef = useRef<HTMLDivElement>(null);
  const [overdueHighlight, setOverdueHighlight] = useState(false);
  const [actionHighlight, setActionHighlight] = useState(false);

  const scrollAndHighlight = (ref: React.RefObject<HTMLDivElement | null>, setter: (v: boolean) => void) => {
    ref.current?.scrollIntoView({ behavior: "smooth", block: "start" });
    setter(true);
    setTimeout(() => setter(false), 1800);
  };

  if (isLoading) {
    return (
      <Box sx={{ p: 3 }}>
        <Skeleton variant="text" width={240} height={44} sx={{ mb: 0.5 }} />
        <Skeleton variant="text" width={180} height={22} sx={{ mb: 3 }} />
        <DashboardSkeleton />
      </Box>
    );
  }

  if (isError || !data) {
    return (
      <Box sx={{ p: 3 }}>
        <Alert severity="error">Failed to load dashboard. Please refresh the page.</Alert>
      </Box>
    );
  }

  const { auditStats, stats, statusDistribution, teamCompletion, actionItems, overdueControls } = data;
  const roleLabel = ROLE_LABELS[role];

  return (
    <Box sx={{ p: 3, display: "flex", flexDirection: "column", gap: 3 }}>

      {/* Header */}
      <Box sx={{ display: "flex", alignItems: "flex-start", justifyContent: "space-between" }}>
        <Box>
          <Typography variant="h4" fontWeight={700}>Dashboard</Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
            Overview of all active audits and controls
          </Typography>
        </Box>
        {roleLabel && (
          <Chip
            label={roleLabel}
            size="small"
            sx={{ bgcolor: "primary.50", color: "primary.main", fontWeight: 600 }}
          />
        )}
      </Box>

      {/* Overview banner — Audits + Controls in one panel */}
      <OverviewBanner
        auditStats={auditStats}
        stats={stats}
        onOverdueClick={() => scrollAndHighlight(overdueRef, setOverdueHighlight)}
        onEvidenceClick={() => scrollAndHighlight(actionItemsRef, setActionHighlight)}
      />

      {/* Charts */}
      <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 2 }}>
        <SectionCard title="Controls by Status">
          <StatusDonut data={statusDistribution} />
        </SectionCard>
        <SectionCard title="Controls by Team">
          <TeamDonut data={teamCompletion} />
        </SectionCard>
      </Box>

      {/* My action items */}
      {role !== "management" && (
        <Box
          ref={actionItemsRef}
          sx={{
            borderRadius: 2,
            outline: actionHighlight ? "2px solid #FB8C00" : "2px solid transparent",
            transition: "outline-color 0.3s",
          }}
        >
          <SectionCard title={`My Action Items (${actionItems.length})`}>
            <ActionItemsList items={actionItems} role={role} />
          </SectionCard>
        </Box>
      )}

      {/* Overdue controls */}
      <Box
        ref={overdueRef}
        sx={{
          borderRadius: 2,
          outline: overdueHighlight ? "2px solid #E53935" : "2px solid transparent",
          transition: "outline-color 0.3s",
        }}
      >
        <SectionCard title={`Overdue Controls (${overdueControls.length})`}>
          <OverdueList items={overdueControls} />
        </SectionCard>
      </Box>

    </Box>
  );
}
