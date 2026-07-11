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
  Chip,
  Tab,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Tabs,
  Typography,
} from "@wso2/oxygen-ui";
import { CheckCircle } from "@wso2/oxygen-ui-icons-react";
import type { JSX } from "react";
import { useNavigate } from "react-router";
import { CONTROL_STATUS_COLORS, CONTROL_STATUS_LABELS } from "@modules/audit/utils/controlStatus";
import type { ControlStatus } from "@modules/audit/types/audit";
import type { ActionItem, OverdueControl } from "@modules/audit/types/dashboard";
import { DUE_SOON_DAYS, dueInfo } from "./dueDate";

function statusColor(s: string): string {
  return CONTROL_STATUS_COLORS[s as ControlStatus] ?? "#90A4AE";
}
function statusLabel(s: string): string {
  return CONTROL_STATUS_LABELS[s as ControlStatus] ?? s;
}

// Role-aware "what should I do with this row" label.
function actionLabel(status: string, canApprove: boolean): string {
  switch (status) {
    case "EVIDENCE_PENDING":              return "Submit evidence";
    case "SUBMITTED_SAMPLE":              return "Submit evidence for sample";
    case "EVIDENCE_NEED_CLARIFICATION":   return "Resubmit evidence";
    case "POPULATION_PENDING":            return "Submit population";
    case "POPULATION_NEED_CLARIFICATION": return "Resubmit population";
    case "EVIDENCE_INTERNAL_REVIEW":
    case "POPULATION_INTERNAL_REVIEW":
      return canApprove ? "Review & approve" : "Pending review";
    case "EVIDENCE_UNDER_VALIDATION":     return "Approve / request resubmission";
    case "POPULATION_UNDER_VALIDATION":   return "Approve / reject population";
    case "POPULATION_COMPLETE":           return "Submit sample";
    case "AWAITING_SAMPLE":               return "Submit sample";
    default:                              return "Action required";
  }
}

// ActionItem and OverdueControl share this row shape.
type QueueRow = ActionItem | OverdueControl;

function QueueTable({ rows, canApprove, emptyText }: {
  rows: QueueRow[];
  canApprove: boolean;
  emptyText: string;
}): JSX.Element {
  const navigate = useNavigate();

  if (rows.length === 0) {
    return (
      <Box sx={{ py: 4, textAlign: "center" }}>
        <CheckCircle size={32} color="#43A047" />
        <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
          {emptyText}
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
          {[...rows].sort((a, b) => dueInfo(a.dueDate).sortKey - dueInfo(b.dueDate).sortKey).map((item) => {
            const due = dueInfo(item.dueDate);
            return (
              <TableRow
                key={item.controlId}
                hover
                sx={{ cursor: "pointer" }}
                onClick={() => void navigate(`/audit/audits/${item.auditId}?control=${item.controlId}`)}
              >
                <TableCell sx={{ whiteSpace: "nowrap", fontWeight: 600 }}>{item.controlNumber}</TableCell>
                <TableCell sx={{ maxWidth: 200 }}>
                  <Typography variant="body2" noWrap title={item.auditName}>{item.auditName}</Typography>
                </TableCell>
                <TableCell>
                  <Typography variant="body2" color="primary.main">{actionLabel(item.status, canApprove)}</Typography>
                </TableCell>
                <TableCell>
                  <Chip
                    label={statusLabel(item.status)}
                    size="small"
                    sx={{
                      bgcolor: `${statusColor(item.status)}18`,
                      "[data-color-scheme='dark'] &": { bgcolor: `${statusColor(item.status)}40` },
                      color: statusColor(item.status), fontWeight: 600, fontSize: "0.7rem",
                    }}
                  />
                </TableCell>
                <TableCell sx={{ whiteSpace: "nowrap" }}>
                  <Typography variant="body2" sx={{ color: due.color, fontWeight: due.sortKey <= 3 ? 600 : 400 }}>
                    {due.label}
                  </Typography>
                </TableCell>
              </TableRow>
            );
          })}
        </TableBody>
      </Table>
    </TableContainer>
  );
}

export const QUEUE_TAB_AWAITING = 0;
export const QUEUE_TAB_DUE_SOON = 1;
export const QUEUE_TAB_OVERDUE = 2;

interface WorkQueueProps {
  actionItems: ActionItem[];
  overdueControls: OverdueControl[];
  canApprove: boolean;
  /** Tab label for the "awaiting you" tab — role-specific ("Review Queue", "My Tasks", …). */
  queueTitle: string;
  /** Controlled tab index so hero chips / KPI cards can jump to a specific tab. */
  tab: number;
  onTabChange: (tab: number) => void;
}

// Tabbed work queue: Awaiting You / Due Soon (≤7 days) / Overdue.
export default function WorkQueue({ actionItems, overdueControls, canApprove, queueTitle, tab, onTabChange }: WorkQueueProps): JSX.Element {
  const dueSoon = actionItems.filter((i) => {
    const d = dueInfo(i.dueDate).days;
    return d >= 0 && d <= DUE_SOON_DAYS;
  });

  return (
    <Box>
      <Tabs
        value={tab}
        onChange={(_, v: number) => onTabChange(v)}
        sx={{ borderBottom: 1, borderColor: "divider", minHeight: 40, "& .MuiTab-root": { minHeight: 40, textTransform: "none", fontWeight: 600 } }}
      >
        <Tab label={`${queueTitle} (${actionItems.length})`} />
        <Tab label={`Due Soon (${dueSoon.length})`} />
        <Tab
          label={`Overdue (${overdueControls.length})`}
          sx={overdueControls.length > 0 ? { color: "#E53935", "&.Mui-selected": { color: "#E53935" } } : undefined}
        />
      </Tabs>
      <Box sx={{ pt: 1 }}>
        {tab === 0 && <QueueTable rows={actionItems} canApprove={canApprove} emptyText="No pending actions — you're all caught up!" />}
        {tab === 1 && <QueueTable rows={dueSoon} canApprove={canApprove} emptyText={`Nothing due in the next ${DUE_SOON_DAYS} days`} />}
        {tab === 2 && <QueueTable rows={overdueControls} canApprove={canApprove} emptyText="No overdue controls" />}
      </Box>
    </Box>
  );
}
