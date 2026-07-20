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

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useAuthApiClient } from "@hooks/useAuthApiClient";
import { BACKEND_BASE_URL } from "@config/apiConfig";
import { controlsQueryKey } from "@modules/audit/api/useGetControls";
import { evidenceQueryKey } from "@modules/audit/api/useGetEvidence";

interface SubmitEvidencePayload {
  auditId: number;
  controlId: number;
  files: File[];
}

async function errText(res: Response, action: string): Promise<string> {
  const msg = await res.text().catch(() => "");
  return msg || `Failed to ${action} (${res.status})`;
}

/**
 * Submits evidence for a control via the backend-proxied flow:
 *   1. GET  .../evidence/upload-link              -> folderPath for this round
 *   2. per file: POST .../evidence/upload          -> multipart; backend validates
 *                (multipart/form-data)                size/type and streams to Azure
 *   3. POST .../evidence/submit { folderPath }      -> backend records files + advances status
 *
 * File bytes go browser -> backend -> Azure. No SAS is handed to the client; the
 * backend authenticates to Azure with its own account key and enforces size/type.
 */
export function useSubmitEvidence() {
  const authFetch = useAuthApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({ auditId, controlId, files }: SubmitEvidencePayload): Promise<void> => {
      if (files.length === 0) throw new Error("Select at least one file to submit.");
      const base = `${BACKEND_BASE_URL}/api/v1/audits/${auditId}/controls/${controlId}/evidence`;

      // 1. Folder path for this submission round.
      const linkRes = await authFetch(`${base}/upload-link`);
      if (!linkRes.ok) throw new Error(await errText(linkRes, "start evidence upload"));
      const { folderPath } = (await linkRes.json()) as { folderPath: string };

      // 2. Proxy each file through the backend (multipart). The browser sets the
      //    multipart boundary; authFetch leaves the Content-Type alone for FormData.
      for (const file of files) {
        const form = new FormData();
        form.append("folderPath", folderPath);
        form.append("file", file, file.name);

        const upRes = await authFetch(`${base}/upload`, { method: "POST", body: form });
        if (!upRes.ok) throw new Error(await errText(upRes, `upload ${file.name}`));
      }

      // 3. Record the submission — the backend lists the blobs and advances status.
      const submitRes = await authFetch(`${base}/submit`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ folderPath }),
      });
      if (!submitRes.ok) throw new Error(await errText(submitRes, "submit evidence"));
    },

    onSuccess: (_data, { auditId, controlId }) => {
      void queryClient.invalidateQueries({ queryKey: controlsQueryKey(auditId) });
      void queryClient.invalidateQueries({ queryKey: evidenceQueryKey(auditId, controlId) });
    },
  });
}
