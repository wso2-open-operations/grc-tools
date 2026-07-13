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

import { useState } from "react";
import {
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
} from "@wso2/oxygen-ui";
import type { JSX } from "react";
import type { HighRiskItem } from "../../api/riskApi";
import { formatDate, formatTreatment } from "./constants";

interface HighRisksTableProps {
  data: HighRiskItem[];
}

// Org-wide open HIGH (residual) risks, oldest identified first. Long
// descriptions are clamped to three lines; clicking a row toggles the clamp.
export default function HighRisksTable({ data }: HighRisksTableProps): JSX.Element {
  const [expanded, setExpanded] = useState<Set<number>>(new Set());

  if (data.length === 0) {
    return (
      <Typography variant="body2" color="text.secondary">
        No open high risks. 🎉
      </Typography>
    );
  }

  const toggle = (id: number): void => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  };

  return (
    <TableContainer>
      <Table size="small">
        <TableHead>
          <TableRow>
            <TableCell sx={{ minWidth: 320 }}>Risk Detail</TableCell>
            <TableCell>Source Register</TableCell>
            <TableCell>Risk Owner</TableCell>
            <TableCell>Identified Date</TableCell>
            <TableCell>Treatment</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {data.map((risk) => (
            <TableRow
              key={risk.id}
              hover
              role="button"
              tabIndex={0}
              aria-expanded={expanded.has(risk.id)}
              onClick={() => toggle(risk.id)}
              onKeyDown={(e) => {
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  toggle(risk.id);
                }
              }}
              sx={{ cursor: "pointer", verticalAlign: "top" }}
            >
              <TableCell>
                <Typography
                  variant="body2"
                  sx={
                    expanded.has(risk.id)
                      ? { whiteSpace: "pre-line" }
                      : {
                          display: "-webkit-box",
                          WebkitLineClamp: 3,
                          WebkitBoxOrient: "vertical",
                          overflow: "hidden",
                        }
                  }
                >
                  {risk.risk_description}
                </Typography>
              </TableCell>
              <TableCell>{risk.register_name}</TableCell>
              <TableCell>{risk.owner_name || "—"}</TableCell>
              <TableCell sx={{ whiteSpace: "nowrap" }}>
                {formatDate(risk.identified_date)}
              </TableCell>
              <TableCell sx={{ whiteSpace: "nowrap" }}>
                {formatTreatment(risk.treatment_strategy, risk.implementation_date)}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  );
}
