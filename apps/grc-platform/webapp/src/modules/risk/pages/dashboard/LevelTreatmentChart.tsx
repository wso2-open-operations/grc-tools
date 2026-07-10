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

import { BarChart } from "@wso2/oxygen-ui-charts-react";
import { Typography } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import type { RegisterLevelTreatmentCount } from "../../api/riskApi";
import {
  LEVEL_LABELS,
  LEVEL_ORDER,
  TREATMENT_COLORS,
  TREATMENT_LABELS,
  TREATMENT_ORDER,
  labelColorOn,
  stackedSegmentAccessor,
} from "./constants";

interface LevelTreatmentChartProps {
  data: RegisterLevelTreatmentCount[];
}

// Per-register "Accept and Remediate" chart: open-risk count per residual
// level, stacked by treatment strategy.
export default function LevelTreatmentChart({ data }: LevelTreatmentChartProps): JSX.Element {
  if (data.length === 0) {
    return (
      <Typography variant="body2" color="text.secondary">
        No open risks.
      </Typography>
    );
  }

  const rows = new Map<string, Record<string, string | number>>();
  const present = new Set<string>();
  for (const level of LEVEL_ORDER) {
    for (const d of data) {
      if (d.risk_level !== level) continue;
      if (!rows.has(level)) rows.set(level, { level: LEVEL_LABELS[level] });
      rows.get(level)![d.treatment_strategy] = d.count;
      present.add(d.treatment_strategy);
    }
  }

  const bars = TREATMENT_ORDER.filter((s) => present.has(s)).map((strategy) => ({
    dataKey: strategy,
    name: TREATMENT_LABELS[strategy],
    fill: TREATMENT_COLORS[strategy],
    stackId: "treatment",
    radius: 0,
    label: {
      position: "center",
      fontSize: 11,
      fill: labelColorOn(TREATMENT_COLORS[strategy]),
      valueAccessor: stackedSegmentAccessor,
      formatter: (value: unknown) => (Number(value) > 0 ? Number(value) : ""),
    },
  }));

  return (
    <BarChart
      data={[...rows.values()]}
      xAxisDataKey="level"
      bars={bars}
      height={280}
      maxBarSize={56}
      isAnimationActive={false}
      margin={{ top: 8, right: 16, left: 0, bottom: 0 }}
    />
  );
}
