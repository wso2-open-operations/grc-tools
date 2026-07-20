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
  Button,
  Checkbox,
  Chip,
  CircularProgress,
  FormControlLabel,
  IconButton,
  Popover,
  Tab,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TablePagination,
  TableRow,
  Tabs,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { CheckCircle, Filter, Search, X } from "@wso2/oxygen-ui-icons-react";
import type { JSX } from "react";
import { useState } from "react";
import { useNavigate } from "react-router";
import { CONTROL_STATUS_COLORS, CONTROL_STATUS_LABELS } from "@modules/audit/utils/controlStatus";
import type { ControlStatus } from "@modules/audit/types/audit";
import type { ActionItem } from "@modules/audit/types/dashboard";
import { useGetWorkQueue, type WorkQueueTab } from "@modules/audit/api/useGetWorkQueue";
import { useGetTeams } from "@modules/audit/api/useGetTeams";
import { useGetUsers } from "@modules/audit/api/useGetUsers";
import { dueInfo } from "./dueDate";

function statusColor(s: string): string {
  return CONTROL_STATUS_COLORS[s as ControlStatus] ?? "#90A4AE";
}
function statusLabel(s: string): string {
  return CONTROL_STATUS_LABELS[s as ControlStatus] ?? s;
}

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

// ── Column filter ─────────────────────────────────────────────────────────────

interface FilterOption {
  id: number;
  label: string;
}

interface ColFilterProps {
  label: string;
  options: FilterOption[];
  selected: number[];
  onChange: (v: number[]) => void;
}

function ColFilter({ label, options, selected, onChange }: ColFilterProps): JSX.Element {
  const [anchor, setAnchor] = useState<HTMLElement | null>(null);
  const [query, setQuery] = useState("");
  const isActive = selected.length > 0;
  const visible = query.trim()
    ? options.filter((o) => o.label.toLowerCase().includes(query.toLowerCase()))
    : options;

  function toggle(id: number) {
    onChange(selected.includes(id) ? selected.filter((x) => x !== id) : [...selected, id]);
  }

  return (
    <>
      <IconButton
        size="small"
        aria-label={`Filter by ${label}`}
        onClick={(e) => { e.stopPropagation(); setAnchor(e.currentTarget); }}
        sx={{
          ml: 0.25, p: 0.25, borderRadius: 0.75,
          color: isActive ? "primary.main" : "action.disabled",
          bgcolor: isActive ? "rgba(25,118,210,0.08)" : "transparent",
          "&:hover": { color: isActive ? "primary.main" : "text.secondary", bgcolor: isActive ? "rgba(25,118,210,0.12)" : "action.hover" },
        }}
      >
        <Filter size={12} />
      </IconButton>

      <Popover
        open={Boolean(anchor)}
        anchorEl={anchor}
        onClose={() => { setAnchor(null); setQuery(""); }}
        anchorOrigin={{ vertical: "bottom", horizontal: "left" }}
        transformOrigin={{ vertical: "top", horizontal: "left" }}
        slotProps={{ paper: { sx: { width: 230, borderRadius: 2, mt: 0.5 } } }}
        onClick={(e) => e.stopPropagation()}
      >
        <Box sx={{ p: 1.25 }}>
          <TextField
            size="small" fullWidth placeholder="Search..." value={query}
            onChange={(e) => setQuery(e.target.value)} autoFocus sx={{ mb: 0.75 }}
            slotProps={{
              input: {
                startAdornment: <Search size={14} style={{ marginRight: 4 }} />,
                endAdornment: query ? (
                  <IconButton size="small" edge="end" aria-label="Clear search" onClick={() => setQuery("")}><X size={12} /></IconButton>
                ) : null,
              },
            }}
          />
          {isActive && (
            <Button size="small" onClick={() => onChange([])}
              sx={{ textTransform: "none", fontSize: "0.72rem", py: 0.25, mb: 0.5, display: "block" }}>
              Clear ({selected.length} selected)
            </Button>
          )}
          <Box sx={{ maxHeight: 260, overflowY: "auto" }}>
            {visible.length === 0 ? (
              <Typography variant="caption" color="text.secondary" sx={{ px: 1, py: 1, display: "block" }}>No matches</Typography>
            ) : visible.map((opt) => (
              <FormControlLabel
                key={opt.id}
                control={<Checkbox size="small" checked={selected.includes(opt.id)} onChange={() => toggle(opt.id)} disableRipple sx={{ p: 0.5 }} />}
                label={<Typography variant="body2" sx={{ fontSize: "0.82rem", lineHeight: 1.4 }}>{opt.label || "—"}</Typography>}
                sx={{ display: "flex", alignItems: "center", px: 0.5, py: 0.1, borderRadius: 1, mx: 0, width: "100%", "&:hover": { bgcolor: "action.hover" } }}
              />
            ))}
          </Box>
        </Box>
      </Popover>
    </>
  );
}

// ── Paginated tab panel ────────────────────────────────────────────────────────

interface TabPanelProps {
  tab: WorkQueueTab;
  canApprove: boolean;
  emptyText: string;
}

function TabPanel({ tab, canApprove, emptyText }: TabPanelProps): JSX.Element {
  const navigate = useNavigate();
  const [page, setPage] = useState(0); // 0-based for MUI, 1-based for API
  const [teamFilter, setTeamFilter] = useState<number[]>([]);
  const [ownerFilter, setOwnerFilter] = useState<number[]>([]);

  const { data, isLoading, isError } = useGetWorkQueue(tab, page + 1, teamFilter, ownerFilter);
  const { data: teamsData } = useGetTeams();
  const { data: usersData } = useGetUsers();

  const items: ActionItem[] = data?.items ?? [];
  const total = data?.total ?? 0;
  const limit = data?.limit ?? 25;

  // Source filter options from the full unfiltered lists so all values are
  // selectable regardless of which page is currently displayed.
  const teams: FilterOption[] = (teamsData ?? [])
    .map((t) => ({ id: t.id, label: t.name }))
    .sort((a, b) => a.label.localeCompare(b.label));

  const owners: FilterOption[] = (usersData ?? [])
    .filter((u) => u.userType === "INTERNAL")
    .map((u) => ({ id: u.id, label: u.displayName }))
    .sort((a, b) => a.label.localeCompare(b.label));

  const hasFilters = teamFilter.length > 0 || ownerFilter.length > 0;

  if (isLoading) {
    return (
      <Box sx={{ py: 4, display: "flex", justifyContent: "center" }}>
        <CircularProgress size={28} />
      </Box>
    );
  }

  if (isError) {
    return <Alert severity="error" sx={{ m: 1 }}>Failed to load items. Please refresh.</Alert>;
  }

  if (total === 0 && !hasFilters) {
    return (
      <Box sx={{ py: 4, textAlign: "center" }}>
        <CheckCircle size={32} color="#43A047" />
        <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>{emptyText}</Typography>
      </Box>
    );
  }

  return (
    <Box>
      {hasFilters && (
        <Box sx={{ px: 0.5, pb: 1, display: "flex", flexWrap: "wrap", gap: 0.5, alignItems: "center" }}>
          {teamFilter.map((id) => (
            <Chip key={id} label={teams.find((t) => t.id === id)?.label ?? String(id)} size="small" onDelete={() => { setTeamFilter((p) => p.filter((x) => x !== id)); setPage(0); }} />
          ))}
          {ownerFilter.map((id) => (
            <Chip key={id} label={owners.find((o) => o.id === id)?.label ?? String(id)} size="small" onDelete={() => { setOwnerFilter((p) => p.filter((x) => x !== id)); setPage(0); }} />
          ))}
          <Button size="small" onClick={() => { setTeamFilter([]); setOwnerFilter([]); setPage(0); }}
            sx={{ textTransform: "none", fontSize: "0.75rem", py: 0.25 }}>
            Clear all
          </Button>
        </Box>
      )}

      {total === 0 ? (
        <Box sx={{ py: 4, textAlign: "center" }}>
          <Typography variant="body2" color="text.secondary">No matches for the current filters.</Typography>
        </Box>
      ) : null}

      <TableContainer sx={{ display: total === 0 ? "none" : undefined }}>
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell sx={{ fontWeight: 600, whiteSpace: "nowrap" }}>Control</TableCell>
              <TableCell sx={{ fontWeight: 600 }}>Audit</TableCell>
              <TableCell sx={{ fontWeight: 600 }}>Action needed</TableCell>
              <TableCell sx={{ fontWeight: 600 }}>Status</TableCell>
              <TableCell sx={{ fontWeight: 600, whiteSpace: "nowrap" }}>Due date</TableCell>
              <TableCell sx={{ fontWeight: 600 }}>
                <Box sx={{ display: "flex", alignItems: "center" }}>
                  Team
                  {teams.length > 0 && (
                    <ColFilter label="Team" options={teams} selected={teamFilter} onChange={(v) => { setTeamFilter(v); setPage(0); }} />
                  )}
                </Box>
              </TableCell>
              <TableCell sx={{ fontWeight: 600 }}>
                <Box sx={{ display: "flex", alignItems: "center" }}>
                  Process Owner
                  {owners.length > 0 && (
                    <ColFilter label="Process Owner" options={owners} selected={ownerFilter} onChange={(v) => { setOwnerFilter(v); setPage(0); }} />
                  )}
                </Box>
              </TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {items.map((item) => {
              const due = dueInfo(item.dueDate);
              return (
                <TableRow
                  key={item.controlId}
                  hover tabIndex={0} sx={{ cursor: "pointer" }}
                  onClick={() => void navigate(`/audit/audits/${item.auditId}?control=${item.controlId}`)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") {
                      e.preventDefault();
                      void navigate(`/audit/audits/${item.auditId}?control=${item.controlId}`);
                    }
                  }}
                >
                  <TableCell sx={{ whiteSpace: "nowrap", fontWeight: 600 }}>{item.controlNumber}</TableCell>
                  <TableCell sx={{ maxWidth: 180 }}>
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
                  <TableCell>
                    <Typography variant="body2" noWrap>{item.team || "—"}</Typography>
                  </TableCell>
                  <TableCell>
                    <Typography variant="body2" noWrap>{item.processOwner || "—"}</Typography>
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      </TableContainer>

      {total > 0 && (
        <TablePagination
          component="div"
          count={total}
          page={page}
          onPageChange={(_, p) => setPage(p)}
          rowsPerPage={limit}
          rowsPerPageOptions={[limit]}
          sx={{ borderTop: 1, borderColor: "divider" }}
        />
      )}
    </Box>
  );
}

// ── WorkQueue ─────────────────────────────────────────────────────────────────

export const QUEUE_TAB_AWAITING = 0;
export const QUEUE_TAB_DUE_SOON = 1;
export const QUEUE_TAB_OVERDUE = 2;

interface WorkQueueProps {
  totalActionItems: number;
  totalDueSoonItems: number;
  totalOverdueControls: number;
  canApprove: boolean;
  queueTitle: string;
  tab: number;
  onTabChange: (tab: number) => void;
}

export default function WorkQueue({
  totalActionItems, totalDueSoonItems, totalOverdueControls,
  canApprove, queueTitle, tab, onTabChange,
}: WorkQueueProps): JSX.Element {
  return (
    <Box>
      <Tabs
        value={tab}
        onChange={(_, v: number) => onTabChange(v)}
        sx={{ borderBottom: 1, borderColor: "divider", minHeight: 40, "& .MuiTab-root": { minHeight: 40, textTransform: "none", fontWeight: 600 } }}
      >
        <Tab label={`${queueTitle} (${totalActionItems})`} />
        <Tab label={`Due Soon (${totalDueSoonItems})`} />
        <Tab
          label={`Overdue (${totalOverdueControls})`}
          sx={totalOverdueControls > 0 ? { color: "#E53935", "&.Mui-selected": { color: "#E53935" } } : undefined}
        />
      </Tabs>
      <Box sx={{ pt: 1 }}>
        {tab === 0 && <TabPanel tab="action-items" canApprove={canApprove} emptyText="No pending actions — you're all caught up!" />}
        {tab === 1 && <TabPanel tab="due-soon" canApprove={canApprove} emptyText="Nothing due in the next 7 days" />}
        {tab === 2 && <TabPanel tab="overdue" canApprove={canApprove} emptyText="No overdue controls" />}
      </Box>
    </Box>
  );
}
