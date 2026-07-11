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

import { Box, Button, Skeleton, Typography } from "@wso2/oxygen-ui";
import { ExternalLink, FileText } from "@wso2/oxygen-ui-icons-react";
import type { JSX } from "react";
import { useGetEvidence } from "@modules/audit/api/useGetEvidence";
import { useAuthApiClient } from "@hooks/useAuthApiClient";
import { BACKEND_BASE_URL } from "@config/apiConfig";

function sizeLabel(bytes: number | null): string {
  if (bytes === null) return "";
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

// createdAt is a full ISO timestamp (not a YYYY-MM-DD date), so parse it directly.
function formatTimestamp(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return (
    d.toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric" }) +
    " " +
    d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
  );
}

/**
 * Lists the files a team submitted for a control so a reviewer can open/download
 * each one. Uses the short-lived read SAS URL returned by the backend.
 */
export default function SubmittedEvidenceList({
  auditId,
  controlId,
}: {
  auditId: number;
  controlId: number;
}): JSX.Element {
  const { data, isLoading, isError } = useGetEvidence(auditId, controlId, true);
  const authFetch = useAuthApiClient();

  // readUrl is a backend download endpoint (not a public link). Fetch it with the
  // auth token, then open the returned bytes — the browser never contacts Azure.
  async function handleView(readUrl: string): Promise<void> {
    try {
      const res = await authFetch(`${BACKEND_BASE_URL}${readUrl}`);
      if (!res.ok) throw new Error(`download failed (${res.status})`);
      const blob = await res.blob();
      const objectUrl = URL.createObjectURL(blob);
      window.open(objectUrl, "_blank", "noopener,noreferrer");
      setTimeout(() => URL.revokeObjectURL(objectUrl), 60_000);
    } catch {
      // best-effort; the "View" simply does nothing if the download fails
    }
  }

  if (isLoading) {
    return <Skeleton variant="rectangular" height={56} sx={{ borderRadius: 1 }} />;
  }
  if (isError) {
    return <Typography variant="body2" color="error">Failed to load submitted evidence.</Typography>;
  }

  const submissions = data ?? [];
  // files can be null (a submission round with no files serialises to JSON null).
  const totalFiles = submissions.reduce((n, s) => n + (s.files?.length ?? 0), 0);

  if (totalFiles === 0) {
    return <Typography variant="body2" color="text.secondary">No evidence files submitted yet.</Typography>;
  }

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
      {submissions.map((sub) => (
        <Box key={sub.id} sx={{ display: "flex", flexDirection: "column", gap: 0.75 }}>
          <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600 }}>
            Submitted {formatTimestamp(sub.createdAt)}{sub.createdBy ? ` · ${sub.createdBy}` : ""}
          </Typography>
          {(sub.files ?? []).map((f) => (
            <Box
              key={f.id}
              sx={{ display: "flex", alignItems: "center", gap: 1, px: 1.25, py: 0.85, borderRadius: 1, border: "1px solid", borderColor: "divider", bgcolor: "action.hover" }}
            >
              <FileText size={15} />
              <Typography variant="body2" sx={{ flex: 1, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                {f.fileName}
              </Typography>
              {f.fileSize !== null && (
                <Typography variant="caption" color="text.secondary">{sizeLabel(f.fileSize)}</Typography>
              )}
              {f.readUrl ? (
                <Button
                  size="small"
                  onClick={() => { void handleView(f.readUrl as string); }}
                  startIcon={<ExternalLink size={13} />}
                  sx={{ textTransform: "none", minWidth: 0 }}
                >
                  View
                </Button>
              ) : (
                <Typography variant="caption" color="text.disabled">unavailable</Typography>
              )}
            </Box>
          ))}
        </Box>
      ))}
    </Box>
  );
}
