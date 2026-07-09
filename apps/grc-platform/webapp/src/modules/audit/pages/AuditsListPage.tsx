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
  Card,
  CardActionArea,
  CardContent,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  InputAdornment,
  Skeleton,
  TextField,
  ToggleButton,
  ToggleButtonGroup,
} from "@mui/material";
import { Box, Button, Typography } from "@wso2/oxygen-ui";
import { ChevronLeft, Plus, Search, ShieldCheck } from "@wso2/oxygen-ui-icons-react";
import { useState, type JSX } from "react";
import { useNavigate } from "react-router";
import AuditCard from "@modules/audit/components/AuditCard";
import { useGetAudits } from "@modules/audit/api/useGetAudits";
import { useGetFrameworks } from "@modules/audit/api/useGetFrameworks";
import { useDeleteAudit } from "@modules/audit/api/useDeleteAudit";
import type { Audit, AuditFramework } from "@modules/audit/types/audit";
import { useAuditPrivileges } from "@modules/audit/hooks/useAuditPrivileges";
import { AuditPrivilege } from "@modules/audit/privileges";

type StatusFilter = "ACTIVE" | "INACTIVE" | "ALL";

function filterByStatus(audits: Audit[], statusFilter: StatusFilter): Audit[] {
  const visible = audits.filter((a) => a.status !== "REMOVED");
  if (statusFilter === "ALL") return visible;
  if (statusFilter === "ACTIVE") return visible.filter((a) => a.status === "ACTIVE");
  return visible.filter((a) => a.status === "COMPLETED" || a.status === "ARCHIVED");
}

interface FrameworkCardProps {
  framework: AuditFramework;
  totalCount: number;
  activeCount: number;
  onClick: () => void;
}

function FrameworkCard({ framework, totalCount, activeCount, onClick }: FrameworkCardProps): JSX.Element {
  return (
    <Card
      variant="outlined"
      sx={{
        height: "100%",
        transition: "box-shadow 0.2s, border-color 0.2s",
        "&:hover": { boxShadow: 4, borderColor: "primary.main" },
      }}
    >
      <CardActionArea onClick={onClick} sx={{ height: "100%", alignItems: "flex-start" }}>
        <CardContent sx={{ display: "flex", flexDirection: "column", gap: 2, p: 3 }}>
          <Box
            sx={{
              width: 48,
              height: 48,
              borderRadius: 2,
              bgcolor: "primary.50",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              color: "primary.main",
            }}
          >
            <ShieldCheck size={24} />
          </Box>

          <Box>
            <Typography variant="h6" fontWeight={700} sx={{ lineHeight: 1.3 }}>
              {framework.name}
            </Typography>
          </Box>

          <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", mt: "auto" }}>
            <Chip
              label={`${totalCount} total`}
              size="small"
              variant="outlined"
              sx={{ height: 22, fontSize: "0.7rem" }}
            />
            {activeCount > 0 && (
              <Chip
                label={`${activeCount} active`}
                size="small"
                color="success"
                variant="outlined"
                sx={{ height: 22, fontSize: "0.7rem" }}
              />
            )}
            {totalCount > 0 && activeCount === 0 && (
              <Chip
                label="no active"
                size="small"
                variant="outlined"
                sx={{ height: 22, fontSize: "0.7rem", color: "text.disabled" }}
              />
            )}
          </Box>
        </CardContent>
      </CardActionArea>
    </Card>
  );
}

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
  const { data: frameworksData, isLoading: frameworksLoading } = useGetFrameworks();
  const deleteAudit = useDeleteAudit();

  const [selectedFrameworkId, setSelectedFrameworkId] = useState<number | null>(null);
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("ACTIVE");
  const [search, setSearch] = useState("");
  const [auditToDelete, setAuditToDelete] = useState<Audit | null>(null);

  const allAudits = auditsData?.items ?? [];
  const allFrameworks = frameworksData ?? [];

  const selectedFramework = allFrameworks.find((f) => f.id === selectedFrameworkId) ?? null;

  function frameworkCounts(fwId: number) {
    const fwAudits = allAudits.filter((a) => a.framework.id === fwId && a.status !== "REMOVED");
    return {
      total: fwAudits.length,
      active: fwAudits.filter((a) => a.status === "ACTIVE").length,
    };
  }

  const frameworkAudits = selectedFrameworkId !== null
    ? allAudits.filter((a) => a.framework.id === selectedFrameworkId)
    : [];
  const statusFiltered = filterByStatus(frameworkAudits, statusFilter);
  const q = search.trim().toLowerCase();
  const displayed = q ? statusFiltered.filter((a) => a.name.toLowerCase().includes(q)) : statusFiltered;

  const isLoading = auditsLoading || frameworksLoading;

  const handleDeleteConfirm = () => {
    if (!auditToDelete) return;
    deleteAudit.mutate(auditToDelete.id, {
      onSuccess: () => setAuditToDelete(null),
    });
  };

  function handleBackToFrameworks() {
    setSelectedFrameworkId(null);
    setStatusFilter("ACTIVE");
    setSearch("");
  }

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
        {selectedFramework ? (
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <MuiButton
              startIcon={<ChevronLeft size={16} />}
              onClick={handleBackToFrameworks}
              sx={{ textTransform: "none", color: "text.secondary", pl: 0, minWidth: 0 }}
            >
              Frameworks
            </MuiButton>
            <Typography variant="body2" color="text.secondary" sx={{ mx: 0.5 }}>
              /
            </Typography>
            <Typography variant="h5" fontWeight={700}>
              {selectedFramework.name}
            </Typography>
          </Box>
        ) : (
          <Typography variant="h4" fontWeight={700}>
            Audits
          </Typography>
        )}

        {canCreateAudit && (
          <Button
            variant="contained"
            startIcon={<Plus size={16} />}
            sx={{ textTransform: "none" }}
            onClick={() =>
              void navigate(
                selectedFrameworkId !== null
                  ? `/audit/audits/create?framework=${selectedFrameworkId}`
                  : "/audit/audits/create",
              )
            }
          >
            New Audit
          </Button>
        )}
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

      {/* Framework overview */}
      {!isLoading && !selectedFramework && (
        <>
          {allFrameworks.length === 0 ? (
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
              <Typography variant="h6">No frameworks yet</Typography>
              <Typography variant="body2" color="text.secondary">
                Create a framework when creating your first audit.
              </Typography>
              {canCreateAudit && (
                <Button
                  variant="contained"
                  startIcon={<Plus size={16} />}
                  sx={{ textTransform: "none" }}
                  onClick={() => void navigate("/audit/audits/create")}
                >
                  New Audit
                </Button>
              )}
            </Box>
          ) : (
            <>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                Select a framework to view its audits
              </Typography>
              <Box
                sx={{
                  display: "grid",
                  gridTemplateColumns: {
                    xs: "1fr",
                    sm: "repeat(2, 1fr)",
                    md: "repeat(3, 1fr)",
                    lg: "repeat(4, 1fr)",
                  },
                  gap: 2.5,
                }}
              >
                {allFrameworks.map((fw) => {
                  const { total, active } = frameworkCounts(fw.id);
                  return (
                    <FrameworkCard
                      key={fw.id}
                      framework={fw}
                      totalCount={total}
                      activeCount={active}
                      onClick={() => {
                        setSelectedFrameworkId(fw.id);
                        setStatusFilter("ACTIVE");
                        setSearch("");
                      }}
                    />
                  );
                })}
              </Box>
            </>
          )}
        </>
      )}

      {/* Drilled framework view */}
      {!isLoading && selectedFramework && (
        <>
          {/* Status toggle + search bar */}
          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              gap: 2,
              mb: 3,
              flexWrap: "wrap",
            }}
          >
            <ToggleButtonGroup
              value={statusFilter}
              exclusive
              onChange={(_e, val: StatusFilter | null) => {
                if (val) setStatusFilter(val);
              }}
              size="small"
            >
              <ToggleButton value="ACTIVE" sx={{ textTransform: "none", px: 2 }}>
                Active
              </ToggleButton>
              <ToggleButton value="INACTIVE" sx={{ textTransform: "none", px: 2 }}>
                Inactive
              </ToggleButton>
              <ToggleButton value="ALL" sx={{ textTransform: "none", px: 2 }}>
                All
              </ToggleButton>
            </ToggleButtonGroup>

            <TextField
              size="small"
              placeholder="Search audits…"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              sx={{ minWidth: 220 }}
              InputProps={{
                startAdornment: (
                  <InputAdornment position="start">
                    <Search size={16} />
                  </InputAdornment>
                ),
              }}
            />

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
              <MuiButton variant="outlined" onClick={() => void refetchAudits()}>
                Try again
              </MuiButton>
            </Box>
          )}

          {/* Audit cards */}
          {!auditsError && displayed.length > 0 && (
            <Box
              sx={{
                display: "grid",
                gridTemplateColumns: { xs: "1fr", sm: "repeat(2, 1fr)", md: "repeat(3, 1fr)" },
                gap: 2.5,
              }}
            >
              {displayed.map((audit) => (
                <AuditCard
                  key={audit.id}
                  audit={audit}
                  onClick={() => void navigate(`/audit/audits/${audit.id}`)}
                  onDelete={() => setAuditToDelete(audit)}
                  canDelete={canCreateAudit}
                />
              ))}
            </Box>
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
                  : statusFilter === "ACTIVE"
                  ? `No active ${selectedFramework.name} audits.`
                  : statusFilter === "INACTIVE"
                  ? `No inactive ${selectedFramework.name} audits.`
                  : `No ${selectedFramework.name} audits yet.`}
              </Typography>
              {statusFilter === "ACTIVE" && frameworkAudits.filter((a) => a.status !== "REMOVED").length > 0 && (
                <MuiButton
                  variant="outlined"
                  size="small"
                  onClick={() => setStatusFilter("ALL")}
                >
                  Show all audits
                </MuiButton>
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
            <strong>{auditToDelete?.name}</strong> and all its controls will be permanently deleted.
            This cannot be undone.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <MuiButton onClick={() => setAuditToDelete(null)} disabled={deleteAudit.isPending}>
            Cancel
          </MuiButton>
          <MuiButton
            variant="contained"
            color="error"
            onClick={handleDeleteConfirm}
            disabled={deleteAudit.isPending}
          >
            {deleteAudit.isPending ? "Deleting…" : "Delete"}
          </MuiButton>
        </DialogActions>
      </Dialog>
    </Box>
  );
}
