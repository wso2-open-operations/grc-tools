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
  Button as MuiButton,
  Chip,
  Divider,
  IconButton,
  Paper,
  Skeleton,
  Stack,
  Tooltip,
} from "@mui/material";
import { Box, Typography } from "@wso2/oxygen-ui";
import {
  AlertCircle,
  CalendarDays,
  CheckCircle2,
  ChevronLeft,
  ClipboardList,
  ListChecks,
  Settings,
} from "@wso2/oxygen-ui-icons-react";
import { useEffect, useState, type JSX } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router";
import FilterPanel from "@modules/audit/components/FilterPanel";
import AuditStatusChip from "@modules/audit/components/AuditStatusChip";
import ControlsTable, {
  ColumnPicker,
  CONTROL_COLUMNS,
  DEFAULT_VISIBLE_CONTROL_COLUMNS,
  CONTROL_COLUMNS_STORAGE_KEY,
} from "@modules/audit/components/ControlsTable";
import ControlDrawer from "@modules/audit/components/ControlDrawer";
import ControlSettingsPanel from "@modules/audit/components/ControlSettingsPanel";
import { useAuditRole } from "@modules/audit/hooks/useAuditRole";
import { useGetAudit } from "@modules/audit/api/useGetAudit";
import { useGetControls } from "@modules/audit/api/useGetControls";
import { formatDateRange } from "@modules/audit/utils/format";
import {
  EMPTY_CONTROL_FILTERS,
  applyControlFilters,
  activeFilterCount,
} from "@modules/audit/utils/controlFilters";
import { CONTROL_STATUS_LABELS } from "@modules/audit/utils/controlStatus";
import type { AuditControl, ControlStatus } from "@modules/audit/types/audit";

// ── Active filter chip helpers ────────────────────────────────────────────────

const FILTER_COLUMN_LABELS: Record<string, string> = {
  status:          "Status",
  requirementType: "Req. Type",
  controlType:     "Control Type",
  scope:           "Scope",
  teamName:        "Team",
  auditorName:     "Auditor POC",
  ownerName:       "Process Owner",
};

const FILTER_VALUE_LABELS: Record<string, Record<string, string>> = {
  requirementType: { DESIGN: "Design", OE: "OE" },
  controlType:     { CONFIG: "Config", NON_CONFIG: "Non-Config" },
  scope:           { COMMON: "Common", PRODUCT_SPECIFIC: "Product Specific" },
};

function getFilterValueLabel(key: string, value: string): string {
  if (key === "status") {
    if (value === "OVERDUE") return "Overdue";
    return CONTROL_STATUS_LABELS[value as ControlStatus] ?? value;
  }
  return FILTER_VALUE_LABELS[key]?.[value] ?? value;
}

type QuickFilter = "all" | "approved" | "inProgress" | "overdue";

function applyQuickFilter(controls: AuditControl[], qf: QuickFilter): AuditControl[] {
  if (qf === "approved") return controls.filter((c) => c.status === "COMPLETE");
  if (qf === "overdue") return controls.filter((c) => c.isOverdue);
  if (qf === "inProgress")
    return controls.filter((c) => c.status !== "COMPLETE" && !c.isOverdue);
  return controls;
}

interface StatCardProps {
  icon: JSX.Element;
  label: string;
  value: number;
  iconBgColor?: string;
  iconColor?: string;
  onClick?: () => void;
  isActive?: boolean;
}

function StatCard({
  icon,
  label,
  value,
  iconBgColor = "#f1f5f9",
  iconColor = "#64748b",
  onClick,
  isActive = false,
}: StatCardProps): JSX.Element {
  return (
    <Paper
      variant="outlined"
      onClick={onClick}
      sx={{
        p: 2.5,
        borderRadius: 2,
        display: "flex",
        alignItems: "center",
        gap: 2,
        cursor: onClick ? "pointer" : "default",
        borderColor: isActive ? iconColor : undefined,
        borderWidth: isActive ? 2 : 1,
        transition: "border-color 0.15s, box-shadow 0.15s",
        "&:hover": onClick
          ? { borderColor: iconColor, boxShadow: `0 0 0 3px ${iconColor}22` }
          : undefined,
      }}
    >
      <Box
        sx={{
          width: 48,
          height: 48,
          borderRadius: 2,
          bgcolor: iconBgColor,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          color: iconColor,
          flexShrink: 0,
        }}
      >
        {icon}
      </Box>
      <Box>
        <Typography variant="h4" fontWeight={700} lineHeight={1}>
          {value}
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
          {label}
        </Typography>
      </Box>
    </Paper>
  );
}

export default function AuditDetailPage(): JSX.Element {
  const navigate = useNavigate();
  const { auditId: auditIdParam } = useParams<{ auditId: string }>();
  const auditId = parseInt(auditIdParam ?? "0", 10);

  const {
    data: audit,
    isLoading: auditLoading,
    isError: auditError,
  } = useGetAudit(auditId);

  const {
    data: controlsData,
    isLoading: controlsLoading,
    isError: controlsError,
  } = useGetControls(auditId);

  const [filters, setFilters] = useState<Record<string, string[]>>(EMPTY_CONTROL_FILTERS);
  const [search, setSearch] = useState("");
  const [activeQuickFilter, setActiveQuickFilter] = useState<QuickFilter | null>(null);
  const [selectedControl, setSelectedControl] = useState<AuditControl | null>(null);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const auditRole = useAuditRole();
  // TEMPORARY: role gating disabled so all controls are visible while roles &
  // privileges are seeded in the DB. Restore the check below once privileges
  // are wired: auditRole === "compliance_admin" || auditRole === "compliance_team".
  void auditRole;
  const canManageControls = true;

  const controls = controlsData?.items ?? [];

  // Deep link from the dashboard: ?control={id} opens that control's drawer once
  // controls have loaded, then the param is cleared so it doesn't re-open on close.
  const [searchParams, setSearchParams] = useSearchParams();
  useEffect(() => {
    const cid = searchParams.get("control");
    if (!cid || controls.length === 0) return;
    const match = controls.find((c) => c.id === Number(cid));
    if (match) {
      setSelectedControl(match);
      searchParams.delete("control");
      setSearchParams(searchParams, { replace: true });
    }
  }, [controls, searchParams, setSearchParams]);

  // Column visibility for the controls table — the picker lives in the filter bar.
  const [visibleColumnIds, setVisibleColumnIds] = useState<string[]>(() => {
    try {
      const stored = localStorage.getItem(CONTROL_COLUMNS_STORAGE_KEY);
      if (stored) return JSON.parse(stored) as string[];
    } catch { /* ignore malformed storage */ }
    return DEFAULT_VISIBLE_CONTROL_COLUMNS;
  });
  useEffect(() => {
    try { localStorage.setItem(CONTROL_COLUMNS_STORAGE_KEY, JSON.stringify(visibleColumnIds)); } catch { /* ignore */ }
  }, [visibleColumnIds]);

  // Quick filter (card click) takes precedence over panel filters
  const filteredControls =
    activeQuickFilter !== null
      ? applyQuickFilter(controls, activeQuickFilter)
      : applyControlFilters(controls, filters, search);

  const approvedCount = controls.filter((c) => c.status === "COMPLETE").length;
  const inProgressCount = controls.filter(
    (c) => c.status !== "COMPLETE" && !c.isOverdue,
  ).length;
  const overdueCount = controls.filter((c) => c.isOverdue).length;

  function handleQuickFilter(qf: QuickFilter) {
    // Toggle off if already active
    setActiveQuickFilter((prev) => (prev === qf ? null : qf));
    setFilters(EMPTY_CONTROL_FILTERS);
    setSearch("");
  }

  function handleFilterChange(newFilters: Record<string, string[]>) {
    setFilters(newFilters);
    setActiveQuickFilter(null);
  }

  function handleSearchChange(newSearch: string) {
    setSearch(newSearch);
    setActiveQuickFilter(null);
  }

  const isFiltered =
    activeQuickFilter !== null ||
    activeFilterCount(filters) > 0 ||
    search.trim().length > 0;

  const handleBack = () => void navigate("/audit/audits");

  if (auditError || controlsError) {
    return (
      <Box
        sx={{
          display: "flex",
          flexDirection: "column",
          alignItems: "center",
          py: 10,
          gap: 2,
        }}
      >
        <Typography variant="body1" color="text.secondary">
          Failed to load audit.
        </Typography>
        <MuiButton variant="outlined" onClick={handleBack}>
          Back to Audits
        </MuiButton>
      </Box>
    );
  }

  return (
    <Box sx={{ p: { xs: 2, sm: 3 } }}>
      {/* Back button */}
      <MuiButton
        startIcon={<ChevronLeft size={16} />}
        onClick={handleBack}
        sx={{ mb: 2, textTransform: "none", color: "text.secondary", pl: 0 }}
      >
        Audits
      </MuiButton>

      {/* Audit header */}
      {auditLoading ? (
        <Box sx={{ mb: 3 }}>
          <Skeleton variant="text" width={320} height={40} />
          <Skeleton variant="text" width={240} height={24} sx={{ mt: 0.5 }} />
        </Box>
      ) : (
        audit && (
          <Box sx={{ display: "flex", alignItems: "flex-start", justifyContent: "space-between", gap: 2, mb: 3, flexWrap: "wrap" }}>
            <Box>
              <Stack direction="row" spacing={1.5} alignItems="center" flexWrap="wrap" mb={0.75}>
                <Typography variant="h5" fontWeight={700}>
                  {audit.name}
                </Typography>
                <AuditStatusChip status={audit.status} />
              </Stack>
              <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap">
                <Typography variant="body2" color="text.secondary">
                  {audit.framework.name}
                  {audit.framework.version ? ` (${audit.framework.version})` : ""}
                </Typography>
                <Divider orientation="vertical" flexItem />
                <Typography variant="body2" color="text.secondary">
                  {audit.product.name}
                </Typography>
                <Divider orientation="vertical" flexItem />
                <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
                  <CalendarDays size={13} />
                  <Typography variant="body2" color="text.secondary">
                    {formatDateRange(audit.periodStart, audit.periodEnd)}
                  </Typography>
                </Box>
              </Stack>
            </Box>
            {canManageControls && (
              <Tooltip title="Manage controls">
                <IconButton
                  onClick={() => setSettingsOpen(true)}
                  sx={{
                    border: "1px solid",
                    borderColor: "divider",
                    borderRadius: 1.5,
                    p: 1,
                    flexShrink: 0,
                  }}
                >
                  <Settings size={20} />
                </IconButton>
              </Tooltip>
            )}
          </Box>
        )
      )}

      {/* Stat cards */}
      <Box
        sx={{
          display: "grid",
          gridTemplateColumns: { xs: "repeat(2, 1fr)", sm: "repeat(4, 1fr)" },
          gap: 2,
          mb: 3,
        }}
      >
        {controlsLoading ? (
          Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} variant="rectangular" height={80} sx={{ borderRadius: 2 }} />
          ))
        ) : (
          <>
            <StatCard
              icon={<ClipboardList size={22} />}
              label="Total Controls"
              value={controls.length}
              iconBgColor="#f1f5f9"
              iconColor="#475569"
              isActive={activeQuickFilter === "all"}
              onClick={() => handleQuickFilter("all")}
            />
            <StatCard
              icon={<CheckCircle2 size={22} />}
              label="Approved"
              value={approvedCount}
              iconBgColor="#dcfce7"
              iconColor="#16a34a"
              isActive={activeQuickFilter === "approved"}
              onClick={() => handleQuickFilter("approved")}
            />
            <StatCard
              icon={<ListChecks size={22} />}
              label="In Progress"
              value={inProgressCount}
              iconBgColor="#dbeafe"
              iconColor="#1d4ed8"
              isActive={activeQuickFilter === "inProgress"}
              onClick={() => handleQuickFilter("inProgress")}
            />
            <StatCard
              icon={<AlertCircle size={22} />}
              label="Overdue"
              value={overdueCount}
              iconBgColor="#fee2e2"
              iconColor="#dc2626"
              isActive={activeQuickFilter === "overdue"}
              onClick={() => handleQuickFilter("overdue")}
            />
          </>
        )}
      </Box>

      {/* Filter + search bar */}
      <Box sx={{ mb: 1.5 }}>
        <FilterPanel
          search={search}
          onSearchChange={handleSearchChange}
          searchPlaceholder="Search by control number or description..."
        />

        {/* Active filter chips — always rendered to prevent layout shift */}
        <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.75, mt: 1, alignItems: "center", minHeight: 28 }}>
          {activeFilterCount(filters) > 0 && (
            <>
              {Object.entries(filters).flatMap(([key, values]) =>
                values.map((value) => (
                  <Chip
                    key={`${key}:${value}`}
                    label={`${FILTER_COLUMN_LABELS[key]}: ${getFilterValueLabel(key, value)}`}
                    size="small"
                    onDelete={() =>
                      handleFilterChange({ ...filters, [key]: filters[key].filter((v) => v !== value) })
                    }
                    sx={{ fontSize: "0.78rem" }}
                  />
                ))
              )}
              <MuiButton
                size="small"
                onClick={() => handleFilterChange(EMPTY_CONTROL_FILTERS)}
                sx={{ textTransform: "none", fontSize: "0.78rem", color: "text.secondary" }}
              >
                Clear all
              </MuiButton>
              {isFiltered && (
                <Typography variant="caption" color="text.secondary" sx={{ ml: "auto" }}>
                  {filteredControls.length} of {controls.length} controls
                </Typography>
              )}
            </>
          )}
          <Box sx={{ ml: "auto" }}>
            <ColumnPicker
              columns={CONTROL_COLUMNS}
              visible={visibleColumnIds}
              onChange={setVisibleColumnIds}
              onReset={() => setVisibleColumnIds(DEFAULT_VISIBLE_CONTROL_COLUMNS)}
            />
          </Box>
        </Box>
      </Box>

      {/* Controls table */}
      <ControlsTable
        controls={filteredControls}
        allControls={controls}
        filters={filters}
        onFiltersChange={handleFilterChange}
        isLoading={controlsLoading}
        onRowClick={(control) => setSelectedControl(control)}
        visibleColumnIds={visibleColumnIds}
      />

      {/* Control detail drawer */}
      <ControlDrawer
        control={selectedControl}
        open={selectedControl !== null}
        onClose={() => setSelectedControl(null)}
      />

      {/* Control settings panel */}
      <ControlSettingsPanel
        auditId={auditId}
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
      />
    </Box>
  );
}
