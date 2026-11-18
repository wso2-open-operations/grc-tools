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
import { FRAMEWORKS_QUERY_KEY } from "@modules/audit/api/useGetFrameworks";
import { AUDITS_QUERY_KEY } from "@modules/audit/api/useGetAudits";
import { AUDIT_DASHBOARD_QUERY_KEY } from "@modules/audit/api/useGetDashboard";
import { extractErrorMessage } from "@modules/audit/api/apiError";
import type { AuditFramework } from "@modules/audit/types/audit";

/** Omitted fields are left as they are. INACTIVE archives the framework; ACTIVE restores it. */
export interface UpdateFrameworkFields {
  name?: string;
  status?: "ACTIVE" | "INACTIVE";
}

/** Renames, archives or restores a framework. Its audits are never touched. */
export function useUpdateFramework() {
  const authFetch = useAuthApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({ frameworkId, fields }: { frameworkId: number; fields: UpdateFrameworkFields }): Promise<AuditFramework> => {
      const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/audits/frameworks/${frameworkId}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(fields),
      });
      if (!res.ok) {
        throw new Error(await extractErrorMessage(res, `Failed to update framework (${res.status})`));
      }
      return res.json() as Promise<AuditFramework>;
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: FRAMEWORKS_QUERY_KEY });
      // Audit rows embed the framework name; the dashboard filters by it.
      void queryClient.invalidateQueries({ queryKey: AUDITS_QUERY_KEY });
      void queryClient.invalidateQueries({ queryKey: AUDIT_DASHBOARD_QUERY_KEY });
    },
  });
}
