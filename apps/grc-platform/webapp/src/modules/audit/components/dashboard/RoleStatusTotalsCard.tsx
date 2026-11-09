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

import { Alert, Box, Skeleton, Typography } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import { useState } from "react";
import { useGetDashboard } from "@modules/audit/api/useGetDashboard";
import type { Audit, ControlStatus } from "@modules/audit/types/audit";
import type { AssigneeRole } from "@modules/audit/types/dashboard";
import { CONTROL_STATUS_COLORS, CONTROL_STATUS_LABELS } from "@modules/audit/utils/controlStatus";
import { ALL_PROGRAMS, auditIdsFor } from "@modules/audit/utils/assigneeProgress";
import AuditProgramFilter from "./AuditProgramFilter";
import SectionCard from "./SectionCard";

interface RoleStatusTotalsCardProps {
  title: string;
  role: AssigneeRole;
  /** The statuses this view counts, one row each. */
  statuses: readonly string[];
  audits: Audit[];
}

// Controls sitting in each tracked status for one role, summed across people
// (no per-person breakdown), filterable by audit program.
export default function RoleStatusTotalsCard({ title, role, statuses, audits }: RoleStatusTotalsCardProps): JSX.Element {
  const [program, setProgram] = useState(ALL_PROGRAMS);
  const { data, isLoading, isError, isPlaceholderData } = useGetDashboard(auditIdsFor(program));

  const distribution = data?.assigneeStatusDistribution;
  const totals = statuses.map((status) => ({
    status,
    count: (distribution ?? []).filter((r) => r.role === role && r.status === status).reduce((n, r) => n + r.count, 0),
  }));
  const max = Math.max(1, ...totals.map((t) => t.count));

  let body: JSX.Element;
  if (isLoading) {
    body = <Skeleton variant="rectangular" height={100} sx={{ borderRadius: 1 }} />;
  } else if (isError) {
    body = <Alert severity="error">Failed to load this view.</Alert>;
  } else if (!distribution) {
    body = <Typography variant="body2" color="text.secondary">Not available yet.</Typography>;
  } else {
    body = (
      <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
        {totals.map((t) => {
          const color = CONTROL_STATUS_COLORS[t.status as ControlStatus] ?? "#94A3B8";
          return (
            <Box key={t.status}>
              <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 0.5 }}>
                <Box sx={{ display: "flex", alignItems: "center", gap: 0.75 }}>
                  <Box sx={{ width: 8, height: 8, borderRadius: "50%", bgcolor: color }} />
                  <Typography variant="body2" fontWeight={600}>
                    {CONTROL_STATUS_LABELS[t.status as ControlStatus] ?? t.status}
                  </Typography>
                </Box>
                <Typography variant="body2" color="text.secondary">{t.count}</Typography>
              </Box>
              <Box sx={{ height: 8, borderRadius: 4, overflow: "hidden", bgcolor: "action.hover" }}>
                <Box sx={{ width: `${(t.count / max) * 100}%`, height: "100%", bgcolor: color }} />
              </Box>
            </Box>
          );
        })}
      </Box>
    );
  }

  return (
    <SectionCard
      title={title}
      action={<AuditProgramFilter audits={audits} value={program} onChange={setProgram} />}
    >
      <Box sx={{ opacity: isPlaceholderData ? 0.5 : 1, transition: "opacity 150ms" }}>{body}</Box>
    </SectionCard>
  );
}
