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

import { useQuery } from "@tanstack/react-query";
import { useAuthApiClient } from "@hooks/useAuthApiClient";
import { BACKEND_BASE_URL } from "@config/apiConfig";
import type { AuditFramework } from "@modules/audit/types/audit";

export const FRAMEWORKS_QUERY_KEY = ["audit", "frameworks"] as const;

/**
 * ACTIVE (default) is what pickers want: archived frameworks take no new audits.
 * ALL also returns archived ones, for the Frameworks page and for views that
 * must keep reaching audits that already exist under an archived framework.
 */
export type FrameworkStatusFilter = "ACTIVE" | "ALL";

export function useGetFrameworks(status: FrameworkStatusFilter = "ACTIVE") {
  const authFetch = useAuthApiClient();

  return useQuery({
    // Prefix-matches FRAMEWORKS_QUERY_KEY, so one invalidation covers both variants.
    queryKey: [...FRAMEWORKS_QUERY_KEY, status],
    queryFn: async (): Promise<AuditFramework[]> => {
      const url = `${BACKEND_BASE_URL}/api/v1/audits/frameworks${status === "ALL" ? "?status=ALL" : ""}`;
      const res = await authFetch(url);
      if (!res.ok) throw new Error(`Failed to load frameworks (${res.status})`);
      return res.json() as Promise<AuditFramework[]>;
    },
    staleTime: 10 * 60 * 1000,
  });
}
