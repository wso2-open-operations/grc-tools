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
  InputAdornment,
  Skeleton,
  Snackbar,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { Plus, Search } from "@wso2/oxygen-ui-icons-react";
import { useState, type JSX } from "react";
import { useGetAudits } from "@modules/audit/api/useGetAudits";
import { useGetFrameworks } from "@modules/audit/api/useGetFrameworks";
import { useUpdateFramework } from "@modules/audit/api/useUpdateFramework";
import { useDeleteFramework } from "@modules/audit/api/useDeleteFramework";
import FrameworkFormDialog from "@modules/audit/components/FrameworkFormDialog";
import FrameworkManageCard from "@modules/audit/components/FrameworkManageCard";
import { useAuditPrivileges } from "@modules/audit/hooks/useAuditPrivileges";
import { AuditPrivilege } from "@modules/audit/privileges";
import type { AuditFramework } from "@modules/audit/types/audit";

type StatusFilter = "ACTIVE" | "ARCHIVED" | "ALL";
const STATUS_FILTERS: StatusFilter[] = ["ACTIVE", "ARCHIVED", "ALL"];
const STATUS_LABELS: Record<StatusFilter, string> = { ACTIVE: "Active", ARCHIVED: "Archived", ALL: "All" };

function matchesStatus(f: AuditFramework, sf: StatusFilter): boolean {
  if (sf === "ALL") return true;
  return sf === "ARCHIVED" ? f.status === "ARCHIVED" : f.status !== "ARCHIVED";
}

export default function FrameworksPage(): JSX.Element {
  const { can } = useAuditPrivileges();
  const canManage = can(AuditPrivilege.ManageFrameworks);

  const { data: frameworksData, isLoading: frameworksLoading, isError: frameworksError } = useGetFrameworks("ALL");
  const { data: auditsData, isLoading: auditsLoading } = useGetAudits();
  const updateFramework = useUpdateFramework();
  const deleteFramework = useDeleteFramework();

  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("ACTIVE");
  const [formOpen, setFormOpen] = useState(false);
  const [frameworkToEdit, setFrameworkToEdit] = useState<AuditFramework | null>(null);
  const [frameworkToDelete, setFrameworkToDelete] = useState<AuditFramework | null>(null);
  const [snack, setSnack] = useState<{ severity: "success" | "error"; message: string } | null>(null);

  const frameworks = frameworksData ?? [];
  // REMOVED audits are soft-deleted, so they don't count towards what a user sees.
  const audits = (auditsData?.items ?? []).filter((a) => a.status !== "REMOVED");
  const auditsOf = (id: number) => audits.filter((a) => a.framework.id === id);

  const q = search.trim().toLowerCase();
  const bySearch = frameworks.filter((f) => !q || f.name.toLowerCase().includes(q));
  const displayed = bySearch.filter((f) => matchesStatus(f, statusFilter));
  const countFor = (sf: StatusFilter) => bySearch.filter((f) => matchesStatus(f, sf)).length;
  const isLoading = frameworksLoading || auditsLoading;

  // Archive is reversible, so no confirmation dialog — just a result snackbar.
  const handleArchiveToggle = (framework: AuditFramework) => {
    const archiving = framework.status !== "ARCHIVED";
    updateFramework.mutate(
      { frameworkId: framework.id, fields: { status: archiving ? "ARCHIVED" : "ACTIVE" } },
      {
        onSuccess: () =>
          setSnack({
            severity: "success",
            message: archiving ? `"${framework.name}" archived.` : `"${framework.name}" restored.`,
          }),
        onError: (e) => setSnack({ severity: "error", message: e.message }),
      },
    );
  };

  const deleteTargetAudits = frameworkToDelete ? auditsOf(frameworkToDelete.id).length : 0;
  const deleteBlocked = deleteTargetAudits > 0;
  const closeDelete = () => {
    setFrameworkToDelete(null);
    deleteFramework.reset();
  };
  const handleDeleteConfirm = () => {
    if (!frameworkToDelete) return;
    const name = frameworkToDelete.name;
    deleteFramework.mutate(frameworkToDelete.id, {
      onSuccess: () => {
        closeDelete();
        setSnack({ severity: "success", message: `"${name}" deleted.` });
      },
    });
  };

  return (
    <Box sx={{ p: 3 }}>
      <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", flexWrap: "wrap", gap: 2, mb: 3 }}>
        <Box>
          <Typography variant="h5" sx={{ fontWeight: 700 }}>
            Frameworks
          </Typography>
          {!isLoading && (
            <Typography variant="body2" color="text.secondary">
              {countFor("ACTIVE")} active · {countFor("ARCHIVED")} archived
            </Typography>
          )}
        </Box>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, flexWrap: "wrap" }}>
          <TextField
            size="small"
            placeholder="Search frameworks…"
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
              },
            }}
          />
          {canManage && (
            <Button
              variant="contained"
              startIcon={<Plus size={16} />}
              sx={{ textTransform: "none" }}
              onClick={() => {
                setFrameworkToEdit(null);
                setFormOpen(true);
              }}
            >
              New Framework
            </Button>
          )}
        </Box>
      </Box>

      <Box sx={{ display: "flex", gap: 1, mb: 3, flexWrap: "wrap" }}>
        {STATUS_FILTERS.map((sf) => (
          <Chip
            key={sf}
            label={`${STATUS_LABELS[sf]} (${countFor(sf)})`}
            color={statusFilter === sf ? "primary" : "default"}
            variant={statusFilter === sf ? "filled" : "outlined"}
            onClick={() => setStatusFilter(sf)}
          />
        ))}
      </Box>

      {frameworksError && <Alert severity="error">Failed to load frameworks.</Alert>}

      {isLoading && (
        <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "repeat(2, 1fr)", md: "repeat(3, 1fr)" }, gap: 2.5 }}>
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} variant="rectangular" height={150} sx={{ borderRadius: 2 }} />
          ))}
        </Box>
      )}

      {!isLoading && !frameworksError && displayed.length === 0 && (
        <Typography variant="body2" color="text.secondary">
          {q ? `No frameworks matching “${search.trim()}”.` : "No frameworks to show."}
        </Typography>
      )}

      {!isLoading && displayed.length > 0 && (
        <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "repeat(2, 1fr)", md: "repeat(3, 1fr)" }, gap: 2.5 }}>
          {displayed.map((f) => {
            return (
              <FrameworkManageCard
                key={f.id}
                framework={f}
                audits={auditsOf(f.id)}
                canManage={canManage}
                onEdit={() => {
                  setFrameworkToEdit(f);
                  setFormOpen(true);
                }}
                onArchiveToggle={() => handleArchiveToggle(f)}
                onDelete={() => setFrameworkToDelete(f)}
              />
            );
          })}
        </Box>
      )}

      <FrameworkFormDialog
        open={formOpen}
        framework={frameworkToEdit}
        onClose={() => setFormOpen(false)}
        onSaved={(message) => {
          setFormOpen(false);
          setSnack({ severity: "success", message });
        }}
      />

      {/* Delete confirmation — only an empty framework can go; otherwise point at Archive. */}
      <Dialog open={Boolean(frameworkToDelete)} onClose={closeDelete} maxWidth="xs" fullWidth>
        <DialogTitle>Delete framework?</DialogTitle>
        <DialogContent>
          <DialogContentText>
            {deleteBlocked ? (
              <>
                <strong>{frameworkToDelete?.name}</strong> has {deleteTargetAudits} audit
                {deleteTargetAudits === 1 ? "" : "s"} and cannot be deleted. Archive it instead — its existing audits
                are kept and it is hidden from the new-audit form.
              </>
            ) : (
              <>
                <strong>{frameworkToDelete?.name}</strong> will be deleted and no longer shown. Creating a framework with
                the same name later brings it back.
              </>
            )}
          </DialogContentText>
          {deleteFramework.isError && (
            <Alert severity="error" sx={{ mt: 2 }}>
              {deleteFramework.error.message}
            </Alert>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={closeDelete} disabled={deleteFramework.isPending}>
            Cancel
          </Button>
          <Button
            color="error"
            variant="contained"
            onClick={handleDeleteConfirm}
            disabled={deleteBlocked || deleteFramework.isPending}
          >
            {deleteFramework.isPending ? "Deleting…" : "Delete"}
          </Button>
        </DialogActions>
      </Dialog>

      <Snackbar open={Boolean(snack)} autoHideDuration={4000} onClose={() => setSnack(null)}>
        {snack ? (
          <Alert severity={snack.severity} onClose={() => setSnack(null)}>
            {snack.message}
          </Alert>
        ) : undefined}
      </Snackbar>
    </Box>
  );
}
