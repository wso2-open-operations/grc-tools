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

import { Box, Stack } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import type { DashboardSummary, RiskScore } from "../../api/riskApi";
import ChartCard from "./ChartCard";
import SummaryCards from "./SummaryCards";
import StatusPieChart from "./StatusPieChart";
import TreatmentByRegisterChart from "./TreatmentByRegisterChart";
import LevelCountChart from "./LevelCountChart";
import RiskHeatmap from "./RiskHeatmap";
import CertDistributionChart from "./CertDistributionChart";
import RegisterSection from "./RegisterSection";
import RepeatedRisksTable from "./RepeatedRisksTable";
import HighRisksTable from "./HighRisksTable";

interface DashboardViewProps {
  dashboard: DashboardSummary;
  scores: RiskScore[];
}

// Pure layout of the risk dashboard; RiskDashboard supplies fetched data.
export default function DashboardView({ dashboard, scores }: DashboardViewProps): JSX.Element {
  return (
    <Stack spacing={3}>
      <SummaryCards summary={dashboard.summary} />

      <Box
        sx={{
          display: "grid",
          gridTemplateColumns: { xs: "1fr", md: "5fr 7fr" },
          gap: 3,
        }}
      >
        <ChartCard title="Overall Risk Status Distribution">
          <StatusPieChart summary={dashboard.summary} />
        </ChartCard>
        <ChartCard title="Risk Treatment Strategy on Open Risks">
          <TreatmentByRegisterChart data={dashboard.treatment_by_register} />
        </ChartCard>
      </Box>

      <Box
        sx={{
          display: "grid",
          gridTemplateColumns: { xs: "1fr", md: "1fr 1fr" },
          gap: 3,
        }}
      >
        <ChartCard title="Count vs. Risk Level (Open Risks)">
          <LevelCountChart data={dashboard.level_counts} />
        </ChartCard>
        <ChartCard title="WSO2 Overall Risk Posture Based on Open Risks">
          <RiskHeatmap cells={dashboard.org_heatmap} scores={scores} />
        </ChartCard>
      </Box>

      <ChartCard title="Number of Open Risks against Compliance Certifications">
        <CertDistributionChart data={dashboard.cert_distribution} />
      </ChartCard>

      {dashboard.registers.map((register) => (
        <RegisterSection key={register.register_id} register={register} scores={scores} />
      ))}

      <ChartCard title="Repeated Risks Potentially Impacting Compliance Certs">
        <RepeatedRisksTable data={dashboard.repeated_compliance_risks} />
      </ChartCard>

      <ChartCard title="High Risk Detailed View">
        <HighRisksTable data={dashboard.high_risks} />
      </ChartCard>
    </Stack>
  );
}
