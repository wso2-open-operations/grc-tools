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
import { extractErrorMessage } from "@modules/audit/api/apiError";

/**
 * One AI validation run against an evidence or population submission
 * (advisory only). Exactly one of evidenceId / populationId is set.
 */
export interface AIValidationLog {
  id: number;
  evidenceId: number | null;
  populationId: number | null;
  controlId: number;
  result: "PASS" | "FAIL" | "UNCERTAIN" | "PENDING" | "ERROR" | "SKIPPED";
  gapsFound: string | null; // JSON array of AIGap, stored as a string
  summary: string | null; // one-line headline; empty for a clean PASS
  createdBy: string | null;
  createdOn: string;
}

/** A single problem the AI flagged. */
export interface AIGap {
  requirementAspect: string;
  issue: string;
  severity: "HIGH" | "MEDIUM" | "LOW";
  fileName?: string;
}

interface AIValidationListResponse {
  validations: AIValidationLog[];
}

/** Minutes since an ISO timestamp (used to detect a stale PENDING row). */
export function ageMinutes(iso: string): number {
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return Number.POSITIVE_INFINITY;
  return (Date.now() - t) / 60000;
}

const STALE_PENDING_MINUTES = 10;

/** True when the latest row is a fresh PENDING (job is genuinely in progress). */
export function isFreshPending(latest: AIValidationLog | undefined): boolean {
  return latest?.result === "PENDING" && ageMinutes(latest.createdOn) < STALE_PENDING_MINUTES;
}

/** Parses the gapsFound JSON string; returns [] on absence or malformed data. */
export function parseGaps(gapsFound: string | null): AIGap[] {
  if (!gapsFound) return [];
  try {
    const parsed = JSON.parse(gapsFound);
    if (!Array.isArray(parsed)) return [];
    return parsed.filter(
      (e): e is AIGap =>
        typeof e === "object" &&
        e !== null &&
        typeof (e as Record<string, unknown>).requirementAspect === "string" &&
        typeof (e as Record<string, unknown>).issue === "string" &&
        typeof (e as Record<string, unknown>).severity === "string",
    );
  } catch {
    return [];
  }
}

export const aiValidationQueryKey = (evidenceId: number) =>
  ["audit", "ai-validation", "evidence", evidenceId] as const;

export const populationAIValidationQueryKey = (populationId: number) =>
  ["audit", "ai-validation", "population", populationId] as const;

/**
 * Fetches the AI validation rows for an evidence submission, latest first.
 * Polls every 5 s only while the latest row is a fresh PENDING — the interval
 * self-disables on any terminal state or once PENDING goes stale, so no global
 * polling is introduced.
 */
export function useGetAIValidation(auditId: number, controlId: number, evidenceId: number | null) {
  const authFetch = useAuthApiClient();

  return useQuery({
    queryKey: aiValidationQueryKey(evidenceId ?? 0),
    enabled: evidenceId !== null,
    queryFn: async (): Promise<AIValidationLog[]> => {
      const res = await authFetch(
        `${BACKEND_BASE_URL}/api/v1/audits/${auditId}/controls/${controlId}/evidence/${evidenceId}/ai-validations`,
      );
      if (!res.ok) {
        throw new Error(await extractErrorMessage(res, `Failed to load AI validation (${res.status})`));
      }
      const body = (await res.json()) as AIValidationListResponse;
      return body.validations ?? [];
    },
    refetchInterval: (query) => {
      const latest = query.state.data?.[0];
      return isFreshPending(latest) ? 5000 : false;
    },
  });
}

// The backend writes the first (PENDING) row from a detached goroutine after
// the submit returns, so a just-updated round can briefly have no rows yet.
const FIRST_ROW_WINDOW_MS = 60_000;

/**
 * useGetAIValidation for a population submission. Also polls, bounded to
 * FIRST_ROW_WINDOW_MS after `roundUpdatedAt`, while no row exists yet.
 */
export function useGetPopulationAIValidation(
  auditId: number,
  controlId: number,
  populationId: number | null,
  roundUpdatedAt: string | null = null,
) {
  const authFetch = useAuthApiClient();

  return useQuery({
    queryKey: populationAIValidationQueryKey(populationId ?? 0),
    enabled: populationId !== null,
    queryFn: async (): Promise<AIValidationLog[]> => {
      const res = await authFetch(
        `${BACKEND_BASE_URL}/api/v1/audits/${auditId}/controls/${controlId}/population/${populationId}/ai-validations`,
      );
      if (!res.ok) {
        throw new Error(await extractErrorMessage(res, `Failed to load AI validation (${res.status})`));
      }
      const body = (await res.json()) as AIValidationListResponse;
      return body.validations ?? [];
    },
    refetchInterval: (query) => {
      const data = query.state.data;
      if (data?.length === 0 && roundUpdatedAt) {
        // abs() so client/server clock skew can't make the window unbounded.
        const age = Date.now() - new Date(roundUpdatedAt).getTime();
        if (Math.abs(age) < FIRST_ROW_WINDOW_MS) return 5000;
      }
      return isFreshPending(data?.[0]) ? 5000 : false;
    },
  });
}
