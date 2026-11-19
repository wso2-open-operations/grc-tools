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

import { FormControl, MenuItem, Select } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import type { Audit } from "@modules/audit/types/audit";
import { ALL_PROGRAMS } from "@modules/audit/utils/assigneeProgress";
import { auditDisplayName } from "@modules/audit/utils/format";

interface AuditProgramFilterProps {
  audits: Audit[];
  /** Selected audit id, or ALL_PROGRAMS. */
  value: string;
  onChange: (value: string) => void;
}

// Audit-program dropdown shared by the dashboard sections. An audit is the
// program (e.g. "SOC 2 External"); "All programs" is the unfiltered default.
// Only ACTIVE audits are listed: the dashboard counts active audits only, so a
// completed or archived audit would always show an empty view.
export default function AuditProgramFilter({ audits, value, onChange }: AuditProgramFilterProps): JSX.Element {
  const sorted = audits.filter((a) => a.status === "ACTIVE").sort((a, b) => auditDisplayName(a).localeCompare(auditDisplayName(b)));
  return (
    <FormControl size="small" sx={{ width: 150 }}>
      <Select
        value={value}
        onChange={(e) => onChange(String(e.target.value))}
        inputProps={{ "aria-label": "Filter by audit program" }}
        sx={{ fontSize: "0.8rem" }}
      >
        <MenuItem value={ALL_PROGRAMS} dense>All programs</MenuItem>
        {sorted.map((a) => (
          <MenuItem key={a.id} value={String(a.id)} dense>{auditDisplayName(a)}</MenuItem>
        ))}
      </Select>
    </FormControl>
  );
}
