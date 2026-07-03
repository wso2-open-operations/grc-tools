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
import type { RegisterCertShare } from "../../api/riskApi";
import { buildCertColorMap, labelColorOn } from "./constants";

interface CertDistributionChartProps {
  data: RegisterCertShare[];
}

// 100%-stacked bars per register: each certification's share of the register's
// open-risk cert tags. The backend guarantees segments per register total 100.
export default function CertDistributionChart({
  data,
}: CertDistributionChartProps): JSX.Element {
  if (data.length === 0) {
    return (
      <Typography variant="body2" color="text.secondary">
        No open risks are tagged with compliance certifications.
      </Typography>
    );
  }

  const certColors = buildCertColorMap(data.map((d) => d.cert_name));
  const rows = new Map<string, Record<string, string | number>>();
  for (const d of data) {
    if (!rows.has(d.register_name)) rows.set(d.register_name, { register: d.register_name });
    rows.get(d.register_name)![d.cert_name] = d.percentage;
  }

  const bars = [...certColors.entries()].map(([cert, color]) => ({
    dataKey: cert,
    name: cert,
    fill: color,
    stackId: "cert",
    radius: 0,
    label: {
      position: "center",
      fontSize: 11,
      fill: labelColorOn(color),
      formatter: (value: number) => (value >= 8 ? `${value}%` : ""),
    },
  }));

  return (
    <BarChart
      data={[...rows.values()]}
      xAxisDataKey="register"
      bars={bars}
      height={360}
      maxBarSize={72}
      xAxis={{ show: true, name: "Source Register" }}
      yAxis={{ show: true, name: "% of Cert Tags on Open Risks" }}
      tooltip={{ show: true, formatter: (value) => `${String(value)}%` }}
    />
  );
}
