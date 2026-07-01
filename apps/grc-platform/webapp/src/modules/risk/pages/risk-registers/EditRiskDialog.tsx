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

import { useEffect, useState } from "react";
import {
  AdapterDateFns,
  Alert,
  Box,
  Button,
  Chip,
  DatePickers,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  FormControl,
  IconButton,
  InputLabel,
  MenuItem,
  Select,
  Stack,
  TextField,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Plus, Trash2 } from "@wso2/oxygen-ui-icons-react";
import { toDateOnlyString } from "@utils/dateTime";
import type { JSX } from "react";
import type {
  ComplianceReference,
  RiskDetail,
  RiskScore,
  RiskTeam,
  UpdateRiskPayload,
  UserOption,
} from "../../api/riskApi";
import { TREATMENT_STRATEGIES } from "../add-risk/constants";

const { DatePicker, LocalizationProvider } = DatePickers;

// ── Mode ──────────────────────────────────────────────────────────────────────
// "full"       — all fields editable (PENDING_RISK_OWNER_APPROVAL / PENDING_AMENDMENT)
// "restricted" — only implementation_date, action_steps, email_subject (post-approval)

export type EditMode = "full" | "restricted";

// ── Helpers ───────────────────────────────────────────────────────────────────

function SectionLabel({ children, restricted }: { children: React.ReactNode; restricted?: boolean }): JSX.Element {
  return (
    <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1.5 }}>
      <Typography variant="subtitle2" fontWeight={700}>
        {children}
      </Typography>
      {restricted && (
        <Chip label="Requires Re-approval" color="warning" size="small" />
      )}
    </Box>
  );
}

const LEVEL_COLORS: Record<string, string> = {
  LOW: "#00B050",
  MEDIUM: "#FF9900",
  HIGH: "#FF0000",
};

// ── Props ─────────────────────────────────────────────────────────────────────

interface EditRiskDialogProps {
  open: boolean;
  detail: RiskDetail;
  mode: EditMode;
  assignmentTeams: RiskTeam[];
  // Required for full mode only:
  users?: UserOption[];
  riskScores?: RiskScore[];
  complianceRefs?: ComplianceReference[];
  onClose: () => void;
  onSave: (payload: UpdateRiskPayload) => Promise<void>;
}

// ── Component ─────────────────────────────────────────────────────────────────

export default function EditRiskDialog({
  open,
  detail,
  mode,
  assignmentTeams,
  users = [],
  riskScores = [],
  complianceRefs = [],
  onClose,
  onSave,
}: EditRiskDialogProps): JSX.Element {
  // ── All fields (full mode) ─────────────────────────────────────────────────
  const [riskTitle, setRiskTitle] = useState(detail.risk_title);
  const [riskDescription, setRiskDescription] = useState(detail.risk_description);
  const [impactDescription, setImpactDescription] = useState(detail.impact_description ?? "");
  const [riskIdentifiedDate, setRiskIdentifiedDate] = useState<Date | null>(
    detail.risk_identified_date ? new Date(detail.risk_identified_date) : null,
  );
  const [identifiedByType, setIdentifiedByType] = useState(detail.identified_by_type ?? "");
  const [identifiedByUserId, setIdentifiedByUserId] = useState<number | "">(
    detail.identified_by_user_id ?? "",
  );
  const [identifiedByName, setIdentifiedByName] = useState(detail.identified_by_name ?? "");
  const [assignerId, setAssignerId] = useState<number | "">(detail.assigner_id || "");
  const [ownerId, setOwnerId] = useState<number | "">(detail.owner_id || "");
  const [selectedRefIds, setSelectedRefIds] = useState<number[]>(
    detail.compliance_references.map((r) => r.id),
  );
  const [grossScoreId, setGrossScoreId] = useState<number | null>(
    detail.gross_score?.id ?? null,
  );
  const [reassessmentDate, setReassessmentDate] = useState<Date | null>(
    detail.reassessment_date ? new Date(detail.reassessment_date) : null,
  );
  const [treatmentStrategy, setTreatmentStrategy] = useState(detail.treatment_strategy ?? "");
  const [assignmentTeamId, setAssignmentTeamId] = useState<number>(detail.assignment_team_id);
  const [actionOwnerId, setActionOwnerId] = useState<number | "">(
    detail.action_plan?.action_owner_id ?? "",
  );
  const [actionPlanDescription, setActionPlanDescription] = useState(
    detail.action_plan?.description ?? "",
  );
  const [progress, setProgress] = useState(detail.progress ?? "");
  const [gitIssueUrl, setGitIssueUrl] = useState(detail.git_issue_url ?? "");
  const [remarks, setRemarks] = useState(detail.remarks ?? "");

  // ── Fields available in both modes ────────────────────────────────────────
  const [implementationDate, setImplementationDate] = useState<Date | null>(
    detail.implementation_date ? new Date(detail.implementation_date) : null,
  );
  const [actionSteps, setActionSteps] = useState<string[]>(
    detail.action_plan?.steps.map((s) => s.description) ?? [],
  );
  const [emailSubject, setEmailSubject] = useState(detail.email_subject ?? "");

  const [errors, setErrors] = useState<Record<string, string>>({});
  const [submitting, setSubmitting] = useState(false);
  const [apiError, setApiError] = useState("");

  useEffect(() => {
    if (!open) return;
    setRiskTitle(detail.risk_title);
    setRiskDescription(detail.risk_description);
    setImpactDescription(detail.impact_description ?? "");
    setRiskIdentifiedDate(detail.risk_identified_date ? new Date(detail.risk_identified_date) : null);
    setIdentifiedByType(detail.identified_by_type ?? "");
    setIdentifiedByUserId(detail.identified_by_user_id ?? "");
    setIdentifiedByName(detail.identified_by_name ?? "");
    setAssignerId(detail.assigner_id || "");
    setOwnerId(detail.owner_id || "");
    setSelectedRefIds(detail.compliance_references.map((r) => r.id));
    setGrossScoreId(detail.gross_score?.id ?? null);
    setReassessmentDate(detail.reassessment_date ? new Date(detail.reassessment_date) : null);
    setTreatmentStrategy(detail.treatment_strategy ?? "");
    setAssignmentTeamId(detail.assignment_team_id);
    setActionOwnerId(detail.action_plan?.action_owner_id ?? "");
    setActionPlanDescription(detail.action_plan?.description ?? "");
    setProgress(detail.progress ?? "");
    setGitIssueUrl(detail.git_issue_url ?? "");
    setRemarks(detail.remarks ?? "");
    setImplementationDate(detail.implementation_date ? new Date(detail.implementation_date) : null);
    setActionSteps(detail.action_plan?.steps.map((s) => s.description) ?? []);
    setEmailSubject(detail.email_subject ?? "");
    setErrors({});
    setApiError("");
  }, [open, detail]);

  const validate = (): boolean => {
    const e: Record<string, string> = {};
    if (mode === "full") {
      if (!riskTitle.trim()) e.riskTitle = "Risk title is required.";
      if (!riskDescription.trim()) e.riskDescription = "Risk description is required.";
    }
    if (!emailSubject.trim()) e.emailSubject = "Email subject is required.";
    actionSteps.forEach((s, i) => {
      if (!s.trim()) e[`step_${i}`] = "Step description cannot be empty.";
    });
    setErrors(e);
    return Object.keys(e).length === 0;
  };

  const handleSave = async () => {
    if (!validate()) return;
    setSubmitting(true);
    setApiError("");
    try {
      const payload: UpdateRiskPayload = {
        // Always sent (backend requires these)
        risk_title: mode === "full" ? riskTitle.trim() : detail.risk_title,
        risk_description: mode === "full" ? riskDescription.trim() : detail.risk_description,
        email_subject: emailSubject.trim(),
        implementation_date: toDateOnlyString(implementationDate),
        action_steps: actionSteps.length > 0
          ? actionSteps.map((d) => ({ description: d.trim() }))
          : undefined,
      };

      if (mode === "full") {
        payload.impact_description = impactDescription.trim() || undefined;
        payload.risk_identified_date = toDateOnlyString(riskIdentifiedDate);
        payload.identified_by_type = identifiedByType || undefined;
        payload.identified_by_user_id = identifiedByUserId !== "" ? Number(identifiedByUserId) : undefined;
        payload.identified_by_name = identifiedByName.trim() || undefined;
        payload.assigner_id = assignerId !== "" ? Number(assignerId) : undefined;
        payload.owner_id = ownerId !== "" ? Number(ownerId) : undefined;
        payload.compliance_reference_ids = selectedRefIds;
        payload.gross_score_id = grossScoreId ?? undefined;
        payload.reassessment_date = toDateOnlyString(reassessmentDate);
        payload.treatment_strategy = treatmentStrategy || undefined;
        payload.assignment_team_id = assignmentTeamId !== detail.assignment_team_id ? assignmentTeamId : undefined;
        payload.action_owner_id = actionOwnerId !== "" ? Number(actionOwnerId) : undefined;
        payload.action_plan_description = actionPlanDescription.trim() || undefined;
        payload.progress = progress.trim() || undefined;
        payload.git_issue_url = gitIssueUrl.trim() || undefined;
        payload.remarks = remarks.trim() || undefined;
      }

      await onSave(payload);
      onClose();
    } catch (e: unknown) {
      setApiError(e instanceof Error ? e.message : "Failed to save changes.");
    } finally {
      setSubmitting(false);
    }
  };

  const datepickerPaperSx = {
    backdropFilter: "none",
    backgroundColor: "#fff",
    "[data-color-scheme='dark'] &": { backgroundColor: "#1e1e1e" },
  };

  // ── Action steps (shared between both modes) ───────────────────────────────
  const actionStepsSection = (
    <Box>
      <SectionLabel>Action Steps</SectionLabel>
      <Stack gap={1.5}>
        {actionSteps.map((step, idx) => (
          <Box key={idx} sx={{ display: "flex", gap: 1, alignItems: "flex-start" }}>
            <Typography variant="body2" color="text.secondary" fontWeight={600} sx={{ mt: 1.5, minWidth: 24 }}>
              {idx + 1}.
            </Typography>
            <TextField
              size="small"
              fullWidth
              value={step}
              onChange={(e) => {
                const next = [...actionSteps];
                next[idx] = e.target.value;
                setActionSteps(next);
                if (errors[`step_${idx}`]) setErrors((p) => ({ ...p, [`step_${idx}`]: "" }));
              }}
              error={!!errors[`step_${idx}`]}
              helperText={errors[`step_${idx}`]}
              disabled={submitting}
            />
            <Tooltip title="Remove step">
              <IconButton
                size="small"
                onClick={() => setActionSteps(actionSteps.filter((_, i) => i !== idx))}
                disabled={submitting || actionSteps.length === 1}
                sx={{ mt: 0.25 }}
              >
                <Trash2 size={16} />
              </IconButton>
            </Tooltip>
          </Box>
        ))}
        <Button
          size="small"
          startIcon={<Plus size={14} />}
          onClick={() => setActionSteps([...actionSteps, ""])}
          disabled={submitting}
          sx={{ alignSelf: "flex-start" }}
        >
          Add Step
        </Button>
      </Stack>
    </Box>
  );

  return (
    <Dialog
      open={open}
      onClose={() => !submitting && onClose()}
      maxWidth="md"
      fullWidth
      PaperProps={{
        sx: {
          backdropFilter: "none",
          backgroundImage: "none",
          backgroundColor: "#ffffff",
          "[data-color-scheme='dark'] &": { backgroundColor: "#1a1a24" },
        },
      }}
    >
      <DialogTitle>
        <Typography variant="h6" fontWeight={700}>
          Edit Risk
        </Typography>
        <Typography variant="caption" color="text.secondary">
          {detail.risk_code}
        </Typography>
      </DialogTitle>

      <DialogContent dividers>
        <Stack gap={3} sx={{ py: 1 }}>
          {apiError && <Alert severity="error">{apiError}</Alert>}

          {mode === "restricted" && (
            <Box>
              <Chip label="Requires Re-approval" color="warning" size="small" sx={{ mb: 0.5 }} />
              <Typography variant="caption" color="text.secondary" display="block">
                All three fields — implementation date, action steps, and email subject — require re-approval.
              </Typography>
            </Box>
          )}

          {/* ── RESTRICTED mode: only 3 fields ── */}
          {mode === "restricted" && (
            <>
              <Box>
                <SectionLabel>Implementation Date</SectionLabel>
                <LocalizationProvider dateAdapter={AdapterDateFns}>
                  <DatePicker
                    label="Implementation Date"
                    value={implementationDate}
                    onChange={(d) => setImplementationDate(d)}
                    slotProps={{
                      desktopPaper: { sx: datepickerPaperSx },
                      textField: { fullWidth: true, disabled: submitting },
                    }}
                  />
                </LocalizationProvider>
              </Box>
              <Divider />
              {actionStepsSection}
              <Divider />
              <Box>
                <SectionLabel>Email Subject</SectionLabel>
                <TextField
                  label="Email Subject"
                  fullWidth
                  required
                  value={emailSubject}
                  onChange={(e) => { setEmailSubject(e.target.value); if (errors.emailSubject) setErrors((p) => ({ ...p, emailSubject: "" })); }}
                  error={!!errors.emailSubject}
                  helperText={errors.emailSubject}
                  disabled={submitting}
                />
              </Box>
            </>
          )}

          {/* ── FULL mode: all fields ── */}
          {mode === "full" && (
            <>
              {/* Basic Information */}
              <Box>
                <SectionLabel>Basic Information</SectionLabel>
                <Stack gap={2}>
                  <TextField
                    label="Risk Title"
                    fullWidth
                    required
                    value={riskTitle}
                    onChange={(e) => { setRiskTitle(e.target.value); if (errors.riskTitle) setErrors((p) => ({ ...p, riskTitle: "" })); }}
                    error={!!errors.riskTitle}
                    helperText={errors.riskTitle}
                    disabled={submitting}
                  />
                  <TextField
                    label="Risk Description"
                    fullWidth
                    required
                    multiline
                    rows={3}
                    value={riskDescription}
                    onChange={(e) => { setRiskDescription(e.target.value); if (errors.riskDescription) setErrors((p) => ({ ...p, riskDescription: "" })); }}
                    error={!!errors.riskDescription}
                    helperText={errors.riskDescription}
                    disabled={submitting}
                  />
                  <TextField
                    label="Impact Description"
                    fullWidth
                    multiline
                    rows={2}
                    value={impactDescription}
                    onChange={(e) => setImpactDescription(e.target.value)}
                    disabled={submitting}
                  />
                  <LocalizationProvider dateAdapter={AdapterDateFns}>
                    <DatePicker
                      label="Risk Identified Date"
                      value={riskIdentifiedDate}
                      onChange={(d) => setRiskIdentifiedDate(d)}
                      slotProps={{ desktopPaper: { sx: datepickerPaperSx }, textField: { fullWidth: true, disabled: submitting } }}
                    />
                  </LocalizationProvider>
                  <FormControl fullWidth disabled={submitting}>
                    <InputLabel>Identified By Type</InputLabel>
                    <Select
                      label="Identified By Type"
                      value={identifiedByType}
                      onChange={(e) => { setIdentifiedByType(e.target.value as string); setIdentifiedByUserId(""); setIdentifiedByName(""); }}
                    >
                      <MenuItem value="EMPLOYEE">Employee</MenuItem>
                      <MenuItem value="EXTERNAL_PERSON">External Person</MenuItem>
                      <MenuItem value="TOOL">Tool</MenuItem>
                    </Select>
                  </FormControl>
                  {identifiedByType === "EMPLOYEE" && (
                    <FormControl fullWidth disabled={submitting}>
                      <InputLabel>Identified By (Employee)</InputLabel>
                      <Select
                        label="Identified By (Employee)"
                        value={identifiedByUserId}
                        onChange={(e) => setIdentifiedByUserId(Number(e.target.value))}
                      >
                        {users.map((u) => <MenuItem key={u.id} value={u.id}>{u.display_name}</MenuItem>)}
                      </Select>
                    </FormControl>
                  )}
                  {(identifiedByType === "EXTERNAL_PERSON" || identifiedByType === "TOOL") && (
                    <TextField
                      label="Identified By (Name)"
                      fullWidth
                      value={identifiedByName}
                      onChange={(e) => setIdentifiedByName(e.target.value)}
                      disabled={submitting}
                    />
                  )}
                  <FormControl fullWidth disabled={submitting}>
                    <InputLabel>Risk Assigner</InputLabel>
                    <Select label="Risk Assigner" value={assignerId} onChange={(e) => setAssignerId(Number(e.target.value))}>
                      {users.map((u) => <MenuItem key={u.id} value={u.id}>{u.display_name}</MenuItem>)}
                    </Select>
                  </FormControl>
                  <FormControl fullWidth disabled={submitting}>
                    <InputLabel>Risk Owner</InputLabel>
                    <Select label="Risk Owner" value={ownerId} onChange={(e) => setOwnerId(Number(e.target.value))}>
                      {users.map((u) => <MenuItem key={u.id} value={u.id}>{u.display_name}</MenuItem>)}
                    </Select>
                  </FormControl>
                  {/* Compliance References */}
                  <FormControl fullWidth disabled={submitting}>
                    <InputLabel>Compliance References</InputLabel>
                    <Select
                      label="Compliance References"
                      multiple
                      value={selectedRefIds}
                      onChange={(e) => setSelectedRefIds(e.target.value as number[])}
                      renderValue={(selected) =>
                        (selected as number[])
                          .map((id) => complianceRefs.find((r) => r.id === id)?.name ?? id)
                          .join(", ")
                      }
                    >
                      {complianceRefs.map((ref) => (
                        <MenuItem key={ref.id} value={ref.id}>{ref.name}</MenuItem>
                      ))}
                    </Select>
                  </FormControl>
                </Stack>
              </Box>

              <Divider />

              {/* Risk Assessment */}
              <Box>
                <SectionLabel>Risk Assessment</SectionLabel>
                <Stack gap={2}>
                  {/* Gross Score Matrix */}
                  <Box>
                    <Typography variant="caption" color="text.secondary" fontWeight={600} display="block" sx={{ mb: 1 }}>
                      Gross Risk Score
                    </Typography>
                    <Box sx={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 0.75, maxWidth: 300 }}>
                      {[3, 2, 1].map((likelihood) =>
                        [1, 2, 3].map((impact) => {
                          const score = riskScores.find((s) => s.likelihood === likelihood && s.impact === impact);
                          if (!score) return null;
                          const selected = grossScoreId === score.id;
                          return (
                            <Box
                              key={score.id}
                              onClick={() => !submitting && setGrossScoreId(score.id)}
                              sx={{
                                p: 1,
                                textAlign: "center",
                                borderRadius: 1,
                                cursor: submitting ? "default" : "pointer",
                                bgcolor: score.color_code,
                                opacity: selected ? 1 : 0.35,
                                border: selected ? "2px solid #000" : "2px solid transparent",
                                transform: selected ? "scale(1.06)" : "scale(1)",
                                transition: "all 0.1s",
                              }}
                            >
                              <Typography variant="caption" fontWeight={700} color="#fff">
                                {score.risk_rating}
                              </Typography>
                            </Box>
                          );
                        })
                      )}
                    </Box>
                    {grossScoreId && (() => {
                      const s = riskScores.find((sc) => sc.id === grossScoreId);
                      return s ? (
                        <Typography variant="caption" sx={{ mt: 0.5, display: "block", color: LEVEL_COLORS[s.risk_level] ?? "text.secondary" }}>
                          L{s.likelihood} × I{s.impact} = {s.risk_rating} ({s.risk_level})
                        </Typography>
                      ) : null;
                    })()}
                  </Box>
                  <LocalizationProvider dateAdapter={AdapterDateFns}>
                    <DatePicker
                      label="Implementation Date"
                      value={implementationDate}
                      onChange={(d) => setImplementationDate(d)}
                      slotProps={{ desktopPaper: { sx: datepickerPaperSx }, textField: { fullWidth: true, disabled: submitting } }}
                    />
                  </LocalizationProvider>
                  <LocalizationProvider dateAdapter={AdapterDateFns}>
                    <DatePicker
                      label="Reassessment Date"
                      value={reassessmentDate}
                      onChange={(d) => setReassessmentDate(d)}
                      slotProps={{ desktopPaper: { sx: datepickerPaperSx }, textField: { fullWidth: true, disabled: submitting } }}
                    />
                  </LocalizationProvider>
                </Stack>
              </Box>

              <Divider />

              {/* Action Plan */}
              <Box>
                <SectionLabel>Action Plan</SectionLabel>
                <Stack gap={2}>
                  <FormControl fullWidth disabled={submitting}>
                    <InputLabel>Treatment Strategy</InputLabel>
                    <Select label="Treatment Strategy" value={treatmentStrategy} onChange={(e) => setTreatmentStrategy(e.target.value as string)}>
                      {TREATMENT_STRATEGIES.map((s) => <MenuItem key={s.value} value={s.value}>{s.label}</MenuItem>)}
                    </Select>
                  </FormControl>
                  <FormControl fullWidth disabled={submitting}>
                    <InputLabel>Assignment Team</InputLabel>
                    <Select label="Assignment Team" value={assignmentTeamId} onChange={(e) => setAssignmentTeamId(Number(e.target.value))}>
                      {assignmentTeams.map((t) => <MenuItem key={t.id} value={t.id}>{t.name}</MenuItem>)}
                    </Select>
                  </FormControl>
                  <FormControl fullWidth disabled={submitting}>
                    <InputLabel>Action Owner</InputLabel>
                    <Select label="Action Owner" value={actionOwnerId} onChange={(e) => setActionOwnerId(Number(e.target.value))}>
                      {users.map((u) => <MenuItem key={u.id} value={u.id}>{u.display_name}</MenuItem>)}
                    </Select>
                  </FormControl>
                  <TextField
                    label="Action Plan Description"
                    fullWidth
                    multiline
                    rows={2}
                    value={actionPlanDescription}
                    onChange={(e) => setActionPlanDescription(e.target.value)}
                    disabled={submitting}
                  />
                </Stack>
              </Box>

              <Divider />

              {/* Action Steps */}
              {actionStepsSection}

              <Divider />

              {/* Other */}
              <Box>
                <SectionLabel>Other</SectionLabel>
                <Stack gap={2}>
                  <TextField
                    label="Email Subject"
                    fullWidth
                    required
                    value={emailSubject}
                    onChange={(e) => { setEmailSubject(e.target.value); if (errors.emailSubject) setErrors((p) => ({ ...p, emailSubject: "" })); }}
                    error={!!errors.emailSubject}
                    helperText={errors.emailSubject}
                    disabled={submitting}
                  />
                  <TextField
                    label="Progress"
                    fullWidth
                    multiline
                    rows={2}
                    value={progress}
                    onChange={(e) => setProgress(e.target.value)}
                    disabled={submitting}
                  />
                  <TextField
                    label="Git Issue URL"
                    fullWidth
                    value={gitIssueUrl}
                    onChange={(e) => setGitIssueUrl(e.target.value)}
                    disabled={submitting}
                  />
                  <TextField
                    label="Remarks"
                    fullWidth
                    multiline
                    rows={2}
                    value={remarks}
                    onChange={(e) => setRemarks(e.target.value)}
                    disabled={submitting}
                  />
                </Stack>
              </Box>
            </>
          )}
        </Stack>
      </DialogContent>

      <DialogActions sx={{ px: 3, py: 2 }}>
        <Button onClick={() => !submitting && onClose()} disabled={submitting} color="inherit">
          Cancel
        </Button>
        <Button onClick={handleSave} disabled={submitting} variant="contained">
          {submitting ? "Saving..." : "Save Changes"}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
