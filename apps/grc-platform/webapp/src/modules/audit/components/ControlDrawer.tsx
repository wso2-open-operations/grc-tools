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
  Button,
  Chip,
  Drawer,
  IconButton,
  Paper,
  Stack,
  Step,
  StepLabel,
  Stepper,
  Tab,
  Tabs,
} from "@wso2/oxygen-ui";
import { Box, Typography } from "@wso2/oxygen-ui";
import {
  AlertCircle,
  Bot,
  CalendarDays,
  CheckCircle2,
  Clock,
  ClipboardCheck,
  FileText,
  FileUp,
  History,
  MessageSquare,
  RotateCcw,
  Sparkles,
  Upload,
  Users,
  X,
  XCircle,
} from "@wso2/oxygen-ui-icons-react";
import { useEffect, useState, type JSX } from "react";
import ControlStatusChip from "@modules/audit/components/ControlStatusChip";
import UserAvatar from "@modules/audit/components/UserAvatar";
import { formatAuditDate } from "@modules/audit/utils/format";
import { useUpdateControlStatus } from "@modules/audit/api/useUpdateControlStatus";
import EvidenceUploadBox from "@modules/audit/components/EvidenceUploadBox";
import SubmittedEvidenceList from "@modules/audit/components/SubmittedEvidenceList";
import CommentsSection from "@modules/audit/components/CommentsSection";
import type { AuditControl, ControlStatus } from "@modules/audit/types/audit";
import { useAuditPrivileges } from "@modules/audit/hooks/useAuditPrivileges";
import { AuditPrivilege } from "@modules/audit/privileges";

interface ControlDrawerProps {
  control: AuditControl | null;
  open: boolean;
  onClose: () => void;
}

const REQ_TYPE_LABELS: Record<string, string> = {
  DESIGN: "Design",
  OE: "Operational Effectiveness",
};
const CTRL_TYPE_LABELS: Record<string, string> = {
  CONFIG: "Configuration",
  NON_CONFIG: "Non-Configuration",
};
const SCOPE_LABELS: Record<string, string> = {
  COMMON: "Common",
  PRODUCT_SPECIFIC: "Product Specific",
};

// ─── Info tile ────────────────────────────────────────────────────────────────

function InfoTile({ label, children }: { label: string; children: React.ReactNode }): JSX.Element {
  return (
    <Box
      sx={{
        px: 1.25,
        py: 1,
        borderRadius: 1,
        border: "1px solid",
        borderColor: "divider",
        bgcolor: "action.hover",
        display: "flex",
        flexDirection: "column",
        gap: 0.4,
      }}
    >
      <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 500, fontSize: "0.67rem", lineHeight: 1 }}>
        {label}
      </Typography>
      {children}
    </Box>
  );
}

// ─── Section card ─────────────────────────────────────────────────────────────

interface SectionCardProps {
  icon: React.ReactNode;
  iconColor?: string;
  iconBg?: string;
  title: string;
  children: React.ReactNode;
  noPad?: boolean;
  flexContent?: boolean;
}

function SectionCard({
  icon,
  iconColor = "#475569",
  iconBg = "#f1f5f9",
  title,
  children,
  noPad = false,
  flexContent = false,
}: SectionCardProps): JSX.Element {
  return (
    <Paper variant="outlined" sx={{ borderRadius: 2, overflow: "hidden", display: "flex", flexDirection: "column" }}>
      <Box
        sx={{
          px: 2.5,
          py: 1.5,
          display: "flex",
          alignItems: "center",
          gap: 1.25,
          borderBottom: 1,
          borderColor: "divider",
          bgcolor: "action.hover",
        }}
      >
        <Box
          sx={{
            width: 30,
            height: 30,
            borderRadius: 1.5,
            bgcolor: iconBg,
            color: iconColor,
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            flexShrink: 0,
          }}
        >
          {icon}
        </Box>
        <Typography variant="subtitle2" fontWeight={700}>
          {title}
        </Typography>
      </Box>
      <Box sx={noPad ? undefined : { p: 2.5, ...(flexContent && { display: "flex", flexDirection: "column", flex: 1 }) }}>{children}</Box>
    </Paper>
  );
}

// ─── Tab panel ────────────────────────────────────────────────────────────────

function TabPanel({ value, index, children }: { value: number; index: number; children: React.ReactNode }): JSX.Element {
  return (
    <Box
      role="tabpanel"
      hidden={value !== index}
      sx={{ flex: 1, overflowY: "auto", display: value === index ? "flex" : "none", flexDirection: "column" }}
    >
      {value === index && (
        <Box sx={{ p: 2.5, display: "flex", flexDirection: "column", gap: 2.5 }}>
          {children}
        </Box>
      )}
    </Box>
  );
}

// ─── OE evidence section ──────────────────────────────────────────────────────

const OE_STEPS = ["Submit Population", "Sample Selection", "Submit Evidence", "Review"] as const;

function oeActiveStep(status: ControlStatus): number {
  if (
    status === "POPULATION_PENDING" ||
    status === "POPULATION_INTERNAL_REVIEW" ||
    status === "POPULATION_UNDER_VALIDATION" ||
    status === "POPULATION_NEED_CLARIFICATION"
  ) return 0;
  if (status === "SUBMITTED_SAMPLE") return 1;
  if (status === "EVIDENCE_PENDING") return 2;
  if (status === "COMPLETE") return 4;
  return 3; // EVIDENCE_INTERNAL_REVIEW, EVIDENCE_UNDER_VALIDATION
}

// ─── Design evidence section ──────────────────────────────────────────────────

const DESIGN_STEPS = ["Evidence Pending", "Internal Review", "Under Validation", "Complete"] as const;

function designActiveStep(status: ControlStatus): number {
  if (status === "EVIDENCE_INTERNAL_REVIEW") return 1;
  if (status === "EVIDENCE_UNDER_VALIDATION") return 2;
  if (status === "COMPLETE") return 3;
  return 0;
}

function DesignEvidenceSection({
  control,
  onStatusChange,
  canSubmitEvidence,
}: {
  control: AuditControl;
  onStatusChange: (s: ControlStatus) => void;
  canSubmitEvidence: boolean;
}): JSX.Element {
  const activeStep = designActiveStep(control.status);
  return (
    <>
      <Paper variant="outlined" sx={{ borderRadius: 2, p: { xs: 1.5, sm: 2 }, overflow: "hidden" }}>
        <Stepper activeStep={activeStep} alternativeLabel sx={{ "& .MuiStepLabel-label": { fontSize: "0.72rem", mt: 0.5 } }}>
          {DESIGN_STEPS.map((label) => (
            <Step key={label}><StepLabel>{label}</StepLabel></Step>
          ))}
        </Stepper>
      </Paper>

      {activeStep === 0 && (
        <>
          {control.comments && (
            <SectionCard icon={<AlertCircle size={16} />} iconBg="#fee2e2" iconColor="#dc2626" title="Evidence Rejected">
              <Typography variant="body2" sx={{ lineHeight: 1.7 }}>{control.comments}</Typography>
            </SectionCard>
          )}
          <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" }, gap: 2 }}>
            {canSubmitEvidence && (
              <SectionCard icon={<FileUp size={16} />} iconBg="#dcfce7" iconColor="#16a34a" title="Evidence Submission" flexContent>
                <EvidenceUploadBox
                  auditId={control.auditId}
                  controlId={control.id}
                  hint="PDF, XLSX, PNG up to 50 MB"
                  buttonLabel="Submit Evidence"
                  onSubmitted={() => onStatusChange("EVIDENCE_INTERNAL_REVIEW")}
                />
              </SectionCard>
            )}
            <SectionCard icon={<Sparkles size={16} />} iconBg="#faf5ff" iconColor="#7c3aed" title="AI Validation">
              <Box sx={{ display: "flex", flexDirection: "column", alignItems: "center", textAlign: "center", gap: 2, py: 0.5 }}>
                <Box sx={{ width: 52, height: 52, borderRadius: "50%", bgcolor: "#faf5ff", display: "flex", alignItems: "center", justifyContent: "center", color: "#7c3aed" }}>
                  <Bot size={26} />
                </Box>
                <Box>
                  <Typography variant="body2" fontWeight={600} gutterBottom>Automated Evidence Check</Typography>
                  <Typography variant="caption" color="text.secondary" sx={{ lineHeight: 1.65 }}>AI reviews your uploaded files against the requirement and flags any gaps.</Typography>
                </Box>
                <Box sx={{ width: "100%", py: 1, px: 1.5, borderRadius: 1.5, bgcolor: "action.hover", display: "flex", alignItems: "center", gap: 1 }}>
                  <Box sx={{ width: 8, height: 8, borderRadius: "50%", bgcolor: "#9ca3af", flexShrink: 0 }} />
                  <Typography variant="caption" color="text.secondary">Not yet validated</Typography>
                </Box>
                <Button variant="outlined" fullWidth disabled startIcon={<Sparkles size={15} />} sx={{ textTransform: "none", fontWeight: 600 }}>Run AI Validation</Button>
              </Box>
            </SectionCard>
          </Box>
        </>
      )}

      {activeStep === 1 && (
        <SectionCard icon={<ClipboardCheck size={16} />} iconBg="#fff7ed" iconColor="#b45309" title="Evidence Under Internal Review">
          <Box sx={{ py: 1, px: 1.5, borderRadius: 1.5, bgcolor: "action.hover", display: "flex", alignItems: "center", gap: 1 }}>
            <Box sx={{ width: 8, height: 8, borderRadius: "50%", bgcolor: "#b45309", flexShrink: 0 }} />
            <Typography variant="body2" color="text.secondary">Evidence submitted — compliance team is reviewing internally.</Typography>
          </Box>
        </SectionCard>
      )}

      {activeStep === 2 && (
        <SectionCard icon={<ClipboardCheck size={16} />} iconBg="#f5f3ff" iconColor="#7c3aed" title="Evidence Under Auditor Validation">
          <Box sx={{ py: 1, px: 1.5, borderRadius: 1.5, bgcolor: "action.hover", display: "flex", alignItems: "center", gap: 1 }}>
            <Box sx={{ width: 8, height: 8, borderRadius: "50%", bgcolor: "#7c3aed", flexShrink: 0 }} />
            <Typography variant="body2" color="text.secondary">Passed internal review — external auditor is validating the evidence.</Typography>
          </Box>
        </SectionCard>
      )}

      {activeStep === 3 && (
        <SectionCard icon={<CheckCircle2 size={16} />} iconBg="#f0fdf4" iconColor="#16a34a" title="Control Complete">
          <Box sx={{ py: 1, px: 1.5, borderRadius: 1.5, bgcolor: "rgba(22,163,74,0.06)", display: "flex", alignItems: "center", gap: 1 }}>
            <CheckCircle2 size={14} color="#16a34a" />
            <Typography variant="body2" color="text.secondary">
              {control.comments ?? "All evidence reviewed and approved."}
            </Typography>
          </Box>
        </SectionCard>
      )}
    </>
  );
}

// ─── OE evidence section ──────────────────────────────────────────────────────

function UploadDropzone({ label, hint }: { label: string; hint: string }): JSX.Element {
  return (
    <Box
      sx={(theme) => ({
        border: "2px dashed",
        borderColor: theme.palette.mode === "dark" ? "rgba(255,255,255,0.15)" : "#d1d5db",
        borderRadius: 2,
        p: 3,
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        gap: 1,
        cursor: "pointer",
        textAlign: "center",
        mb: 1.5,
        "&:hover": { borderColor: "primary.main", bgcolor: "action.hover" },
      })}
    >
      <Box sx={{ width: 44, height: 44, borderRadius: "50%", bgcolor: "#f0fdf4", display: "flex", alignItems: "center", justifyContent: "center", color: "#16a34a" }}>
        <Upload size={20} />
      </Box>
      <Typography variant="body2" fontWeight={600}>{label}</Typography>
      <Typography variant="caption" color="text.secondary">{hint}</Typography>
    </Box>
  );
}

function SampleSelectionCard({ control }: { control: AuditControl }): JSX.Element {
  const hasNote = Boolean(control.sampleReference);

  return (
    <SectionCard
      icon={<ClipboardCheck size={16} />}
      iconBg="#dbeafe"
      iconColor="#1d4ed8"
      title="Sample Selected by Auditor"
    >
      {!hasNote && (
        <Typography variant="body2" color="text.secondary">
          Sample details will appear here once the auditor completes selection.
        </Typography>
      )}

      {hasNote && (
        <Box sx={{ p: 1.5, borderRadius: 1.5, bgcolor: "#eff6ff", border: "1px solid #bfdbfe" }}>
          <Typography variant="caption" color="text.secondary" fontWeight={600} sx={{ display: "block", mb: 0.5 }}>
            Auditor Note
          </Typography>
          <Typography variant="body2" sx={{ lineHeight: 1.7 }}>{control.sampleReference}</Typography>
        </Box>
      )}
    </SectionCard>
  );
}

function OEEvidenceSection({
  control,
  onStatusChange,
  canSubmitEvidence,
}: {
  control: AuditControl;
  onStatusChange: (s: ControlStatus) => void;
  canSubmitEvidence: boolean;
}): JSX.Element {
  const activeStep = oeActiveStep(control.status);

  return (
    <>
      <Paper variant="outlined" sx={{ borderRadius: 2, p: { xs: 1.5, sm: 2 }, overflow: "hidden" }}>
        <Stepper activeStep={activeStep} alternativeLabel sx={{ "& .MuiStepLabel-label": { fontSize: "0.72rem", mt: 0.5 } }}>
          {OE_STEPS.map((label) => (
            <Step key={label}><StepLabel>{label}</StepLabel></Step>
          ))}
        </Stepper>
      </Paper>

      {/* ── Step 0: Population phase ── */}
      {activeStep === 0 && (
        <>
          {control.status === "POPULATION_PENDING" && (
            <>
              {control.evidenceRequirement && (
                <SectionCard icon={<FileText size={16} />} iconBg="#f1f5f9" iconColor="#475569" title="Population Requirement">
                  <Typography variant="body2" sx={{ lineHeight: 1.8 }}>{control.evidenceRequirement}</Typography>
                </SectionCard>
              )}
              <SectionCard icon={<FileUp size={16} />} iconBg="#dcfce7" iconColor="#16a34a" title="Submit Population" flexContent>
                <UploadDropzone label="Drop population file here" hint="CSV or XLSX — complete list of in-scope items" />
                <Button variant="contained" fullWidth disableElevation startIcon={<FileUp size={15} />} sx={{ textTransform: "none", fontWeight: 600 }} onClick={() => onStatusChange("POPULATION_INTERNAL_REVIEW")}>
                  Submit Population
                </Button>
              </SectionCard>
            </>
          )}

          {control.status === "POPULATION_INTERNAL_REVIEW" && (
            <SectionCard icon={<Clock size={16} />} iconBg="#fff7ed" iconColor="#b45309" title="Population Under Internal Review">
              <Box sx={{ py: 2, display: "flex", flexDirection: "column", alignItems: "center", gap: 1.5, textAlign: "center" }}>
                <Box sx={{ width: 52, height: 52, borderRadius: "50%", bgcolor: "#fff7ed", display: "flex", alignItems: "center", justifyContent: "center" }}>
                  <Clock size={24} color="#b45309" />
                </Box>
                <Typography variant="body2" fontWeight={600}>Population submitted successfully</Typography>
                <Typography variant="caption" color="text.secondary" sx={{ maxWidth: 320, lineHeight: 1.65 }}>
                  The compliance team is reviewing your population file before it goes to the auditor.
                </Typography>
                <Chip size="small" label="Pending internal review" sx={{ bgcolor: "#fff7ed", color: "#92400e", fontWeight: 500 }} />
              </Box>
            </SectionCard>
          )}

          {control.status === "POPULATION_UNDER_VALIDATION" && (
            <SectionCard icon={<Clock size={16} />} iconBg="#f5f3ff" iconColor="#7c3aed" title="Population Under Auditor Validation">
              <Box sx={{ py: 2, display: "flex", flexDirection: "column", alignItems: "center", gap: 1.5, textAlign: "center" }}>
                <Box sx={{ width: 52, height: 52, borderRadius: "50%", bgcolor: "#f5f3ff", display: "flex", alignItems: "center", justifyContent: "center" }}>
                  <Clock size={24} color="#7c3aed" />
                </Box>
                <Typography variant="body2" fontWeight={600}>Population passed internal review</Typography>
                <Typography variant="caption" color="text.secondary" sx={{ maxWidth: 320, lineHeight: 1.65 }}>
                  The external auditor is reviewing your population and selecting a sample for evidence collection.
                </Typography>
                <Chip size="small" label="Waiting for auditor sample selection" sx={{ bgcolor: "#f5f3ff", color: "#6d28d9", fontWeight: 500 }} />
              </Box>
            </SectionCard>
          )}

          {control.status === "POPULATION_NEED_CLARIFICATION" && (
            <>
              <SectionCard icon={<AlertCircle size={16} />} iconBg="#fee2e2" iconColor="#dc2626" title="Population Clarification Required">
                <Typography variant="body2" sx={{ lineHeight: 1.7 }}>
                  {control.comments ?? "The auditor has requested clarification. Please review and resubmit your population."}
                </Typography>
              </SectionCard>
              <SectionCard icon={<FileUp size={16} />} iconBg="#dcfce7" iconColor="#16a34a" title="Resubmit Population" flexContent>
                <UploadDropzone label="Drop updated population file here" hint="CSV or XLSX — complete list of in-scope items" />
                <Button variant="contained" fullWidth disableElevation startIcon={<FileUp size={15} />} sx={{ textTransform: "none", fontWeight: 600 }} onClick={() => onStatusChange("POPULATION_INTERNAL_REVIEW")}>
                  Resubmit Population
                </Button>
              </SectionCard>
            </>
          )}
        </>
      )}

      {/* ── Step 1: Auditor selected samples → team submits evidence ── */}
      {activeStep === 1 && (
        <>
          <SampleSelectionCard control={control} />
          {canSubmitEvidence && (
            <SectionCard icon={<FileUp size={16} />} iconBg="#dcfce7" iconColor="#16a34a" title="Submit Evidence" flexContent>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5, lineHeight: 1.7 }}>
                Upload evidence covering all selected samples listed above.
              </Typography>
              <EvidenceUploadBox
                auditId={control.auditId}
                controlId={control.id}
                hint="PDF, XLSX, PNG up to 50 MB"
                buttonLabel="Submit Evidence"
                onSubmitted={() => onStatusChange("EVIDENCE_INTERNAL_REVIEW")}
              />
            </SectionCard>
          )}
        </>
      )}

      {/* ── Step 2: Evidence rejected → resubmit ── */}
      {activeStep === 2 && (
        <>
          {control.comments && (
            <SectionCard icon={<AlertCircle size={16} />} iconBg="#fee2e2" iconColor="#dc2626" title="Evidence Rejected">
              <Typography variant="body2" sx={{ lineHeight: 1.7 }}>{control.comments}</Typography>
            </SectionCard>
          )}
          <SampleSelectionCard control={control} />
          {canSubmitEvidence && (
            <SectionCard icon={<FileUp size={16} />} iconBg="#dcfce7" iconColor="#16a34a" title="Resubmit Evidence" flexContent>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5, lineHeight: 1.7 }}>
                Upload updated evidence addressing the rejection reason above.
              </Typography>
              <EvidenceUploadBox
                auditId={control.auditId}
                controlId={control.id}
                hint="PDF, XLSX, PNG up to 50 MB"
                buttonLabel="Resubmit Evidence"
                onSubmitted={() => onStatusChange("EVIDENCE_INTERNAL_REVIEW")}
              />
            </SectionCard>
          )}
        </>
      )}

      {/* ── Step 3+: Under review ── */}
      {activeStep >= 3 && control.status !== "COMPLETE" && (
        <>
          <SampleSelectionCard control={control} />
          <SectionCard
            icon={<Clock size={16} />}
            iconBg={control.status === "EVIDENCE_UNDER_VALIDATION" ? "#f5f3ff" : "#fff7ed"}
            iconColor={control.status === "EVIDENCE_UNDER_VALIDATION" ? "#7c3aed" : "#b45309"}
            title={control.status === "EVIDENCE_INTERNAL_REVIEW" ? "Evidence Under Internal Review" : "Evidence Under Auditor Validation"}
          >
            <Typography variant="body2" color="text.secondary" sx={{ lineHeight: 1.7 }}>
              {control.status === "EVIDENCE_INTERNAL_REVIEW"
                ? "The compliance team is reviewing your evidence submission."
                : "The external auditor is validating the submitted evidence."}
            </Typography>
          </SectionCard>
        </>
      )}

      {/* ── Complete ── */}
      {control.status === "COMPLETE" && (
        <>
          <SampleSelectionCard control={control} />
          <SectionCard icon={<CheckCircle2 size={16} />} iconBg="#f0fdf4" iconColor="#16a34a" title="Control Complete">
            <Typography variant="body2" color="text.secondary" sx={{ lineHeight: 1.7 }}>
              {control.comments ?? "All evidence reviewed and approved by the auditor."}
            </Typography>
          </SectionCard>
        </>
      )}
    </>
  );
}

// ─── Main component ───────────────────────────────────────────────────────────

export default function ControlDrawer({ control, open, onClose }: ControlDrawerProps): JSX.Element {
  const { can } = useAuditPrivileges();
  const canSubmitEvidence = can(AuditPrivilege.SubmitEvidence);
  const canComment = can(AuditPrivilege.AddComment);

  const [tab, setTab] = useState(0);
  const [localStatus, setLocalStatus] = useState<{ id: number; status: ControlStatus } | null>(null);

  // Reset to the Overview tab whenever a different control is opened, so the
  // drawer doesn't retain the previous control's active tab. Syncing tab state to
  // the opened control is a legitimate effect here.
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setTab(0);
  }, [control?.id]);
  const updateStatus = useUpdateControlStatus();

  // Use local override only when it belongs to the currently open control
  const displayStatus =
    localStatus !== null && control !== null && localStatus.id === control.id
      ? localStatus.status
      : control?.status;

  // Optimistically update the local status for instant UI feedback, then persist via API.
  // On failure, revert the optimistic value so the UI reflects the real server state.
  function handleStatusChange(c: AuditControl, newStatus: ControlStatus) {
    setLocalStatus({ id: c.id, status: newStatus });
    updateStatus.mutate(
      { auditId: c.auditId, controlId: c.id, status: newStatus },
      { onError: () => setLocalStatus(null) },
    );
  }

  return (
    <Drawer
      anchor="right"
      open={open}
      onClose={onClose}
      PaperProps={{
        sx: {
          width: { xs: "100vw", sm: 660, md: 720 },
          display: "flex",
          flexDirection: "column",
        },
      }}
    >
      {control && (
        // key resets all local state when a different control is opened
        <Box key={control.id} sx={{ display: "flex", flexDirection: "column", height: "100%" }}>

          {/* ── Header ── */}
          <Box sx={{ px: 3, pt: 2.5, pb: 2, flexShrink: 0 }}>
            <Box
              sx={{
                display: "flex",
                alignItems: "flex-start",
                justifyContent: "space-between",
                gap: 1,
                mb: 1.25,
              }}
            >
              <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap">
                <Typography variant="h5" fontWeight={700}>
                  {control.controlNumber}
                </Typography>
                <ControlStatusChip status={displayStatus ?? control.status} size="medium" />
                {control.isOverdue && (
                  <Chip
                    icon={<AlertCircle size={13} />}
                    label="Overdue"
                    size="small"
                    variant="outlined"
                    sx={{ color: "#dc2626", borderColor: "#dc2626", fontWeight: 500 }}
                  />
                )}
              </Stack>
              <IconButton size="small" onClick={onClose} aria-label="Close">
                <X size={18} />
              </IconButton>
            </Box>
            <Typography variant="body2" color="text.secondary" sx={{ lineHeight: 1.7 }}>
              {control.description}
            </Typography>
          </Box>

          {/* ── Tabs ── */}
          <Tabs
            value={tab}
            onChange={(_, v: number) => setTab(v)}
            sx={{ px: 2, borderBottom: 1, borderTop: 1, borderColor: "divider", flexShrink: 0, minHeight: 44 }}
          >
            <Tab
              icon={<ClipboardCheck size={15} />}
              iconPosition="start"
              label="Overview"
              sx={{ textTransform: "none", minHeight: 44, fontWeight: 600 }}
            />
            <Tab
              icon={<FileUp size={15} />}
              iconPosition="start"
              label="Evidence"
              sx={{ textTransform: "none", minHeight: 44, fontWeight: 600 }}
            />
            <Tab
              icon={<History size={15} />}
              iconPosition="start"
              label="History"
              sx={{ textTransform: "none", minHeight: 44, fontWeight: 600 }}
            />
          </Tabs>

          {/* ══ TAB 0 – OVERVIEW ══════════════════════════════════════════════ */}
          <TabPanel value={tab} index={0}>

            {/* Control details grid */}
            <SectionCard
              icon={<ClipboardCheck size={16} />}
              iconBg="#f1f5f9"
              iconColor="#475569"
              title="Control Details"
            >
              <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1 }}>

                <InfoTile label="Requirement Type">
                  <Typography variant="body2" fontWeight={600} fontSize="0.8rem">
                    {REQ_TYPE_LABELS[control.requirementType]}
                  </Typography>
                </InfoTile>

                <InfoTile label="Control Type">
                  <Typography variant="body2" fontWeight={600} fontSize="0.8rem">
                    {CTRL_TYPE_LABELS[control.controlType]}
                  </Typography>
                </InfoTile>

                <InfoTile label="Scope">
                  <Typography variant="body2" fontWeight={600} fontSize="0.8rem">
                    {SCOPE_LABELS[control.scope]}
                  </Typography>
                </InfoTile>

                <InfoTile label="Due Date">
                  <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
                    <CalendarDays size={13} color={control.isOverdue ? "#dc2626" : undefined} style={{ flexShrink: 0 }} />
                    <Typography variant="body2" fontWeight={600} fontSize="0.8rem" color={control.isOverdue ? "error.main" : "text.primary"}>
                      {control.dueDate ? formatAuditDate(control.dueDate) : "—"}
                    </Typography>
                  </Box>
                </InfoTile>

                <InfoTile label="Team">
                  <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
                    <Users size={13} style={{ flexShrink: 0, opacity: 0.55 }} />
                    <Typography variant="body2" fontWeight={600} fontSize="0.8rem">
                      {control.teamName ?? "—"}
                    </Typography>
                  </Box>
                </InfoTile>

                <InfoTile label="Sample Reference">
                  <Typography variant="body2" fontWeight={600} fontSize="0.8rem" noWrap>
                    {control.sampleReference ?? "—"}
                  </Typography>
                </InfoTile>

                <InfoTile label="Process Owner">
                  {control.ownerName ? (
                    <Box sx={{ display: "flex", alignItems: "center", gap: 0.75 }}>
                      <UserAvatar name={control.ownerName} size={22} />
                      <Typography variant="body2" fontWeight={600} fontSize="0.8rem" noWrap>
                        {control.ownerName}
                      </Typography>
                    </Box>
                  ) : (
                    <Typography variant="body2" color="text.disabled" fontSize="0.8rem">—</Typography>
                  )}
                </InfoTile>

                <InfoTile label="Auditor POC">
                  {control.auditorName ? (
                    <Box sx={{ display: "flex", alignItems: "center", gap: 0.75 }}>
                      <UserAvatar name={control.auditorName} size={22} />
                      <Typography variant="body2" fontWeight={600} fontSize="0.8rem" noWrap>
                        {control.auditorName}
                      </Typography>
                    </Box>
                  ) : (
                    <Typography variant="body2" color="text.disabled" fontSize="0.8rem">—</Typography>
                  )}
                </InfoTile>

              </Box>
            </SectionCard>

            {/* Evidence / population requirement preview */}
            {control.evidenceRequirement && (
              <SectionCard
                icon={<FileText size={16} />}
                iconBg="#f1f5f9"
                iconColor="#475569"
                title={control.requirementType === "OE" ? "Population Requirement" : "Evidence Requirement"}
              >
                <Typography variant="body2" sx={{ lineHeight: 1.8 }}>
                  {control.evidenceRequirement}
                </Typography>
              </SectionCard>
            )}

          </TabPanel>

          {/* ══ TAB 1 – EVIDENCE ══════════════════════════════════════════════ */}
          <TabPanel value={tab} index={1}>

            {control.requirementType === "OE" ? (
              <OEEvidenceSection
                control={{ ...control, status: displayStatus ?? control.status }}
                onStatusChange={(s) => handleStatusChange(control, s)}
                canSubmitEvidence={canSubmitEvidence}
              />
            ) : (
              <DesignEvidenceSection
                control={{ ...control, status: displayStatus ?? control.status }}
                onStatusChange={(s) => handleStatusChange(control, s)}
                canSubmitEvidence={canSubmitEvidence}
              />
            )}

            {/* Submitted evidence — reviewers open/download the files here */}
            <SectionCard
              icon={<FileText size={16} />}
              iconBg="#f1f5f9"
              iconColor="#475569"
              title="Submitted Evidence"
            >
              <SubmittedEvidenceList auditId={control.auditId} controlId={control.id} />
            </SectionCard>

            {/* Internal Review */}
            <SectionCard
              icon={<ClipboardCheck size={16} />}
              iconBg="#fff7ed"
              iconColor="#b45309"
              title="Internal Review"
            >
              <Typography variant="body2" color="text.secondary" sx={{ mb: 2, lineHeight: 1.7 }}>
                Review the submitted evidence internally before passing it to the auditor.
              </Typography>
              <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1 }}>
                <Button
                  variant="contained"
                  disableElevation
                  startIcon={<CheckCircle2 size={15} />}
                  onClick={() => handleStatusChange(control, "EVIDENCE_UNDER_VALIDATION")}
                  sx={{ textTransform: "none", fontWeight: 600, bgcolor: "#b45309", "&:hover": { bgcolor: "#92400e" } }}
                >
                  Approve
                </Button>
                <Button
                  variant="outlined"
                  startIcon={<XCircle size={15} />}
                  onClick={() => handleStatusChange(control, "EVIDENCE_PENDING")}
                  sx={{ textTransform: "none", fontWeight: 600, color: "#dc2626", borderColor: "#dc2626", "&:hover": { borderColor: "#b91c1c", bgcolor: "rgba(220,38,38,0.04)" } }}
                >
                  Reject
                </Button>
              </Box>
            </SectionCard>

            {/* Auditor Validation */}
            <SectionCard
              icon={<ClipboardCheck size={16} />}
              iconBg="#f5f3ff"
              iconColor="#7c3aed"
              title="Auditor Validation"
            >
              <Typography variant="body2" color="text.secondary" sx={{ mb: 2, lineHeight: 1.7 }}>
                Validate the submitted evidence and take a final decision on this control.
              </Typography>
              <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1 }}>
                <Button
                  variant="contained"
                  disableElevation
                  startIcon={<CheckCircle2 size={15} />}
                  onClick={() => handleStatusChange(control, "COMPLETE")}
                  sx={{ textTransform: "none", fontWeight: 600, bgcolor: "#7c3aed", "&:hover": { bgcolor: "#6d28d9" } }}
                >
                  Approve
                </Button>
                <Button
                  variant="outlined"
                  startIcon={<RotateCcw size={15} />}
                  onClick={() => handleStatusChange(control, "EVIDENCE_NEED_CLARIFICATION")}
                  sx={{ textTransform: "none", fontWeight: 600 }}
                >
                  Request Resubmission
                </Button>
              </Box>
            </SectionCard>

            {/* Comments */}
            <SectionCard
              icon={<MessageSquare size={16} />}
              iconBg="#fff7ed"
              iconColor="#ea580c"
              title="Comments"
            >
              <CommentsSection auditId={control.auditId} controlId={control.id} canComment={canComment} />
            </SectionCard>

          </TabPanel>

          {/* ══ TAB 2 – HISTORY ═══════════════════════════════════════════════ */}
          <TabPanel value={tab} index={2}>
            <Box
              sx={{
                display: "flex", flexDirection: "column",
                alignItems: "center", justifyContent: "center",
                py: 8, gap: 2, textAlign: "center",
              }}
            >
              <Box
                sx={{
                  width: 64, height: 64, borderRadius: "50%",
                  bgcolor: "action.hover",
                  display: "flex", alignItems: "center", justifyContent: "center",
                  color: "text.disabled",
                }}
              >
                <History size={32} />
              </Box>
              <Typography variant="h6" fontWeight={600}>No history yet</Typography>
              <Typography variant="body2" color="text.secondary" sx={{ maxWidth: 300 }}>
                Audit trail entries will appear here as actions are taken on this control.
              </Typography>
            </Box>
          </TabPanel>

        </Box>
      )}
    </Drawer>
  );
}
