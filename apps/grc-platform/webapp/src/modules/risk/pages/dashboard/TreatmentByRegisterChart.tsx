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
import { Box, Typography } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import type { RegisterTreatmentCount } from "../../api/riskApi";
import {
  TREATMENT_COLORS,
  TREATMENT_LABELS,
  TREATMENT_ORDER,
  labelColorOn,
  stackedSegmentAccessor,
} from "./constants";

interface TreatmentByRegisterChartProps {
  data: RegisterTreatmentCount[];
}

const CHART_HEIGHT = 320;

// Stacked bar of open risks per BU/register, segmented by treatment strategy.
// Zero counts are left undefined so recharts skips the segment and its label.
export default function TreatmentByRegisterChart({
  data,
}: TreatmentByRegisterChartProps): JSX.Element {
  if (data.length === 0) {
    return (
      <Typography variant="body2" color="text.secondary">
        No open risks.
      </Typography>
    );
  }

  const rows = new Map<string, Record<string, string | number>>();
  const present = new Set<string>();
  for (const d of data) {
    if (!rows.has(d.register_name)) rows.set(d.register_name, { register: d.register_name });
    rows.get(d.register_name)![d.treatment_strategy] = d.count;
    present.add(d.treatment_strategy);
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
    <Box sx={{ display: "flex", alignItems: "stretch", gap: 0.5 }}>
      {/* Custom axis title, centered on the whole chart height (bars + legend)
          rather than recharts' internal plot-only centering, which reads too
          high once the legend strip below the bars is accounted for. */}
      <Box
        sx={{
          height: CHART_HEIGHT,
          flexShrink: 0,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
        }}
      >
        <Typography
          variant="caption"
          color="text.secondary"
          noWrap
          sx={{ writingMode: "vertical-rl", transform: "rotate(180deg)" }}
        >
          Number of Open Risks
        </Typography>
      </Box>
      <Box sx={{ flex: 1, minWidth: 0 }}>
        <BarChart
          data={[...rows.values()]}
          xAxisDataKey="register"
          bars={bars}
          height={CHART_HEIGHT}
          maxBarSize={64}
          isAnimationActive={false}
          margin={{ top: 8, right: 16, left: 8, bottom: 0 }}
          yAxis={{ show: true }}
        />
      </Box>
    </Box>
  );
}
