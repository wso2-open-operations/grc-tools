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
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  FormControl,
  IconButton,
  InputAdornment,
  MenuItem,
  Select,
  Skeleton,
  Snackbar,
  TablePagination,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { Plus, Search, ShieldCheck, X } from "@wso2/oxygen-ui-icons-react";
import { useState, type JSX } from "react";
import { useNavigate, useSearchParams } from "react-router";
import AuditCard from "@modules/audit/components/AuditCard";
import { useGetAudits } from "@modules/audit/api/useGetAudits";
import { useDeleteAudit } from "@modules/audit/api/useDeleteAudit";
import { useUpdateAuditStatus } from "@modules/audit/api/useUpdateAuditStatus";
import EditAuditDialog from "@modules/audit/components/EditAuditDialog";
import { auditDisplayName } from "@modules/audit/utils/format";
import type { Audit, AuditStatus } from "@modules/audit/types/audit";
import { useAuditPrivileges } from "@modules/audit/hooks/useAuditPrivileges";
import { AuditPrivilege } from "@modules/audit/privileges";
import { auditPaths } from "@modules/audit/paths";

const PAGE_SIZE_OPTIONS = [12, 24, 48];
const ALL_FRAMEWORKS = "ALL";

type StatusFilter = "ACTIVE" | "COMPLETED" | "ARCHIVED" | "ALL";
const STATUS_FILTERS: StatusFilter[] = ["ACTIVE", "COMPLETED", "ARCHIVED", "ALL"];
const STATUS_FILTER_LABELS: Record<StatusFilter, string> = {
  ACTIVE: "Active",
  COMPLETED: "Completed",
  ARCHIVED: "Archived",
  ALL: "All",
};

/** Search matches the audit name, its display name or its framework's name. */
function matchesSearch(a: Audit, q: string): boolean {
  return (
    a.name.toLowerCase().includes(q) ||
    (a.displayName ?? "").toLowerCase().includes(q) ||
    a.framework.name.toLowerCase().includes(q)
  );
}

function filterByStatus(audits: Audit[], statusFilter: StatusFilter): Audit[] {
  const visible = audits.filter((a) => a.status !== "REMOVED");
  if (statusFilter === "ALL") return visible;
  return visible.filter((a) => a.status === (statusFilter as AuditStatus));
}

// ── Page ──────────────────────────────────────────────────────────────────────

export default function AuditsListPage(): JSX.Element {
  const navigate = useNavigate();
  const { can } = useAuditPrivileges();
  const canCreateAudit = can(AuditPrivilege.CreateAudit);
  const {
    data: auditsData,
    isLoading: auditsLoading,
    isError: auditsError,
    refetch: refetchAudits,
  } = useGetAudits();
  const deleteAudit = useDeleteAudit();
  const updateAuditStatus = useUpdateAuditStatus();
  const canUpdateAudit = can(AuditPrivilege.UpdateAudit);

  // The status filter lives in the URL so refresh/back/sharing all work.
  const [searchParams, setSearchParams] = useSearchParams();
  const statusParam = (searchParams.get("status") ?? "ACTIVE") as StatusFilter;
  const statusFilter: StatusFilter = STATUS_FILTERS.includes(statusParam) ? statusParam : "ACTIVE";

  // Framework filter: "ALL" or a framework id, also kept in the URL.
  const frameworkParam = searchParams.get("framework") ?? ALL_FRAMEWORKS;

  const [search, setSearchState] = useState("");
  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState(PAGE_SIZE_OPTIONS[0]);
  const [auditToDelete, setAuditToDelete] = useState<Audit | null>(null);
  const [auditToEdit, setAuditToEdit] = useState<Audit | null>(null);

  const allAudits = auditsData?.items ?? [];

  // Any change to what is being filtered sends the list back to its first page.
  function setSearch(value: string) {
    setSearchState(value);
    setPage(0);
  }

  function setStatusFilter(sf: StatusFilter) {
    const next = new URLSearchParams(searchParams);
    if (sf === "ACTIVE") next.delete("status");
    else next.set("status", sf);
    setSearchParams(next, { replace: true });
    setPage(0);
  }

  function setFrameworkFilter(value: string) {
    const next = new URLSearchParams(searchParams);
    if (value === ALL_FRAMEWORKS) next.delete("framework");
    else next.set("framework", value);
    setSearchParams(next, { replace: true });
    setPage(0);
  }

  const q = search.trim().toLowerCase();

  // Frameworks that actually have audits, for the filter dropdown.
  const frameworkOptions = [
    ...new Map(
      allAudits.filter((a) => a.status !== "REMOVED").map((a) => [a.framework.id, a.framework.name] as const),
    ),
  ].sort((a, b) => a[1].localeCompare(b[1]));
  // An unknown id in the URL (e.g. a framework with no audits) falls back to "all".
  const frameworkFilter = frameworkOptions.some(([id]) => String(id) === frameworkParam)
    ? frameworkParam
    : ALL_FRAMEWORKS;
  const inFramework =
    frameworkFilter === ALL_FRAMEWORKS
      ? allAudits
      : allAudits.filter((a) => String(a.framework.id) === frameworkFilter);

  const statusFiltered = filterByStatus(inFramework, statusFilter);
  const displayed = q ? statusFiltered.filter((a) => matchesSearch(a, q)) : statusFiltered;

  // Clamp in case the list shrank (e.g. an audit was deleted on the last page).
  const lastPage = Math.max(0, Math.ceil(displayed.length / pageSize) - 1);
  const currentPage = Math.min(page, lastPage);
  const pageItems = displayed.slice(currentPage * pageSize, (currentPage + 1) * pageSize);

  const statusCounts = Object.fromEntries(
    STATUS_FILTERS.map((sf) => [sf, filterByStatus(inFramework, sf).length]),
  ) as Record<StatusFilter, number>;

  const activeAuditsTotal = allAudits.filter((a) => a.status === "ACTIVE").length;
  const isLoading = auditsLoading;

  const handleDeleteConfirm = () => {
    if (!auditToDelete) return;
    deleteAudit.mutate(auditToDelete.id, {
      onSuccess: () => setAuditToDelete(null),
    });
  };

  // Archive is reversible, so no confirmation dialog — just a result snackbar.
  const [archiveSnack, setArchiveSnack] = useState<{ severity: "success" | "error"; message: string } | null>(null);
  const handleArchiveToggle = (audit: Audit) => {
    const archiving = audit.status !== "ARCHIVED";
    // We don't persist the pre-archive status, so infer the restore target from
    // completion: a fully-approved audit returns to Completed, otherwise Active.
    const { total, approved } = audit.controlCounts;
    const restoreStatus: AuditStatus = total > 0 && approved === total ? "COMPLETED" : "ACTIVE";
    const targetStatus: AuditStatus = archiving ? "ARCHIVED" : restoreStatus;
    updateAuditStatus.mutate(
      { auditId: audit.id, status: targetStatus },
      {
        onSuccess: () =>
          setArchiveSnack({
            severity: "success",
            message: archiving
              ? `"${auditDisplayName(audit)}" archived.`
              : `"${auditDisplayName(audit)}" restored to ${restoreStatus === "COMPLETED" ? "completed" : "active"}.`,
          }),
        onError: () =>
          setArchiveSnack({
            severity: "error",
            message: archiving ? "Failed to archive the audit." : "Failed to restore the audit.",
          }),
      },
    );
  };

  const searchField = (
    <TextField
      size="small"
      placeholder="Search by framework or audit…"
      value={search}
      onChange={(e) => setSearch(e.target.value)}
      sx={{ minWidth: 230 }}
      slotProps={{
        input: {
          startAdornment: (
            <InputAdornment position="start">
              <Search size={16} />
            </InputAdornment>
          ),
          endAdornment: search ? (
            <InputAdornment position="end">
              <IconButton size="small" edge="end" aria-label="Clear search" onClick={() => setSearch("")}>
                <X size={14} />
              </IconButton>
            </InputAdornment>
          ) : null,
        },
      }}
    />
  );

  return (
    <Box sx={{ p: { xs: 2, sm: 3 } }}>
      {/* Page header */}
      <Box
        sx={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          mb: 3,
          flexWrap: "wrap",
          gap: 2,
        }}
      >
        <Box>
          <Typography variant="h4" fontWeight={700}>
            Audits
          </Typography>
          {!isLoading && (
            <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
              {activeAuditsTotal} active audit{activeAuditsTotal === 1 ? "" : "s"}
            </Typography>
          )}
        </Box>

        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, flexWrap: "wrap" }}>
          {searchField}
          {canCreateAudit && (
            <Button
              variant="contained"
              startIcon={<Plus size={16} />}
              sx={{ textTransform: "none" }}
              onClick={() =>
                void navigate(auditPaths.create(frameworkFilter === ALL_FRAMEWORKS ? null : Number(frameworkFilter)))
              }
            >
              New Audit
            </Button>
          )}
        </Box>
      </Box>

      {/* Loading state */}
      {isLoading && (
        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: { xs: "1fr", sm: "repeat(2, 1fr)", md: "repeat(3, 1fr)" },
            gap: 2.5,
          }}
        >
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} variant="rectangular" height={180} sx={{ borderRadius: 2 }} />
          ))}
        </Box>
      )}

      {/* Audit cards */}
      {!isLoading && (
        <>
          {/* Status chips + search bar */}
          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              gap: 2,
              mb: 3,
              flexWrap: "wrap",
            }}
          >
            <Box sx={{ display: "flex", gap: 1 }}>
              {STATUS_FILTERS.map((sf) => (
                <Chip
                  key={sf}
                  clickable
                  label={sf === "ALL" ? STATUS_FILTER_LABELS[sf] : `${STATUS_FILTER_LABELS[sf]} (${statusCounts[sf]})`}
                  onClick={() => setStatusFilter(sf)}
                  color={statusFilter === sf ? "primary" : "default"}
                  variant={statusFilter === sf ? "filled" : "outlined"}
                  sx={{ fontWeight: statusFilter === sf ? 700 : 400 }}
                />
              ))}
            </Box>

            <FormControl size="small" sx={{ minWidth: 180 }}>
              <Select
                value={frameworkFilter}
                onChange={(e) => setFrameworkFilter(String(e.target.value))}
                inputProps={{ "aria-label": "Filter by framework" }}
                sx={{ fontSize: "0.875rem" }}
              >
                <MenuItem value={ALL_FRAMEWORKS}>All frameworks</MenuItem>
                {frameworkOptions.map(([id, name]) => (
                  <MenuItem key={id} value={String(id)}>
                    {name}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>

            <Typography variant="caption" color="text.secondary" sx={{ ml: "auto" }}>
              {displayed.length} {displayed.length === 1 ? "audit" : "audits"}
            </Typography>
          </Box>

          {/* Error */}
          {auditsError && (
            <Box
              sx={{ display: "flex", flexDirection: "column", alignItems: "center", py: 8, gap: 2 }}
            >
              <Typography variant="body1" color="text.secondary">
                Failed to load audits.
              </Typography>
              <Button variant="outlined" onClick={() => void refetchAudits()}>
                Try again
              </Button>
            </Box>
          )}

          {!auditsError && displayed.length > 0 && (
            <>
            <Box
              sx={{
                display: "grid",
                gridTemplateColumns: { xs: "1fr", sm: "repeat(2, 1fr)", md: "repeat(3, 1fr)" },
                gap: 2.5,
              }}
            >
              {pageItems.map((audit) => (
                <AuditCard
                  key={audit.id}
                  audit={audit}
                  onClick={() => void navigate(auditPaths.detail(audit.id))}
                  onDelete={() => setAuditToDelete(audit)}
                  canDelete={canCreateAudit}
                  onArchiveToggle={() => handleArchiveToggle(audit)}
                  canArchive={canUpdateAudit}
                  onEdit={() => setAuditToEdit(audit)}
                  canEdit={canUpdateAudit}
                />
              ))}
            </Box>
              <TablePagination
                component="div"
                count={displayed.length}
                page={currentPage}
                onPageChange={(_, p) => setPage(p)}
                rowsPerPage={pageSize}
                onRowsPerPageChange={(e) => {
                  setPageSize(Number(e.target.value));
                  setPage(0);
                }}
                rowsPerPageOptions={PAGE_SIZE_OPTIONS}
                labelRowsPerPage="Audits per page"
                sx={{ mt: 1 }}
              />
            </>
          )}

          {/* Empty state */}
          {!auditsError && displayed.length === 0 && (
            <Box
              sx={{
                display: "flex",
                flexDirection: "column",
                alignItems: "center",
                py: 10,
                gap: 2,
              }}
            >
              <ShieldCheck size={48} style={{ opacity: 0.25 }} />
              <Typography variant="h6">No audits found</Typography>
              <Typography variant="body2" color="text.secondary">
                {search.trim()
                  ? "No audits match your search."
                  : statusFilter === "ALL"
                  ? "No audits yet."
                  : `No ${STATUS_FILTER_LABELS[statusFilter].toLowerCase()} audits.`}
              </Typography>
              {statusFilter !== "ALL" && allAudits.some((a) => a.status !== "REMOVED") && (
                <Button
                  variant="outlined"
                  size="small"
                  onClick={() => setStatusFilter("ALL")}
                >
                  Show all audits
                </Button>
              )}
            </Box>
          )}
        </>
      )}

      {/* Delete confirmation dialog */}
      <Dialog
        open={Boolean(auditToDelete)}
        onClose={() => setAuditToDelete(null)}
        maxWidth="xs"
        fullWidth
      >
        <DialogTitle>Delete audit?</DialogTitle>
        <DialogContent>
          <DialogContentText>
            <strong>{auditToDelete ? auditDisplayName(auditToDelete) : ""}</strong> and all its controls will be permanently deleted.
            This cannot be undone.
          </DialogContentText>
          {deleteAudit.isError && (
            <Alert severity="error" sx={{ mt: 2 }}>
              Failed to delete the audit. Please try again.
            </Alert>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setAuditToDelete(null)} disabled={deleteAudit.isPending}>
            Cancel
          </Button>
          <Button
            variant="contained"
            color="error"
            onClick={handleDeleteConfirm}
            disabled={deleteAudit.isPending}
          >
            {deleteAudit.isPending ? "Deleting…" : "Delete"}
          </Button>
        </DialogActions>
      </Dialog>

      <EditAuditDialog
        audit={auditToEdit}
        onClose={() => setAuditToEdit(null)}
        onSaved={() => {
          setAuditToEdit(null);
          setArchiveSnack({ severity: "success", message: "Audit updated." });
        }}
      />

      {/* Archive / restore result snackbar */}
      <Snackbar
        open={Boolean(archiveSnack)}
        autoHideDuration={4000}
        onClose={() => setArchiveSnack(null)}
        anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
      >
        <Alert
          severity={archiveSnack?.severity ?? "success"}
          variant="filled"
          onClose={() => setArchiveSnack(null)}
        >
          {archiveSnack?.message}
        </Alert>
      </Snackbar>
    </Box>
  );
}
