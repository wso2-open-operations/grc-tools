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

import { useCallback, useEffect, useState } from "react";
import {
  Alert,
  Box,
  Button,
  CircularProgress,
  PageContent,
  PageTitle,
  Stack,
} from "@wso2/oxygen-ui";
import type { JSX } from "react";
import { useAuthApiClient } from "@hooks/useAuthApiClient";
import {
  fetchDashboard,
  fetchRiskScores,
  type DashboardSummary,
  type RiskScore,
} from "../api/riskApi";
import ChartCard from "./dashboard/ChartCard";
import SummaryCards from "./dashboard/SummaryCards";
import StatusPieChart from "./dashboard/StatusPieChart";
import TreatmentByRegisterChart from "./dashboard/TreatmentByRegisterChart";
import LevelCountChart from "./dashboard/LevelCountChart";
import RiskHeatmap from "./dashboard/RiskHeatmap";
import CertDistributionChart from "./dashboard/CertDistributionChart";
import RegisterSection from "./dashboard/RegisterSection";
import RepeatedRisksTable from "./dashboard/RepeatedRisksTable";
import HighRisksTable from "./dashboard/HighRisksTable";

// Risk dashboard: current organisational risk posture built from a single
// GET /api/v1/dashboard payload, plus the 3×3 risk_score matrix that colors
// heatmap cells holding no risks.
export default function RiskDashboard(): JSX.Element {
  const authFetch = useAuthApiClient();
  const [dashboard, setDashboard] = useState<DashboardSummary | null>(null);
  const [scores, setScores] = useState<RiskScore[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async (): Promise<void> => {
    setLoading(true);
    setError(null);
    try {
      const [summary, scoreMatrix] = await Promise.all([
        fetchDashboard(authFetch),
        fetchRiskScores(authFetch).catch(() => [] as RiskScore[]),
      ]);
      setDashboard(summary);
      setScores(scoreMatrix);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load the dashboard.");
    } finally {
      setLoading(false);
    }
  }, [authFetch]);

  useEffect(() => {
    void load();
  }, [load]);

  return (
    <PageContent>
      <PageTitle>
        <PageTitle.Header>Risk Dashboard</PageTitle.Header>
      </PageTitle>

      {loading && (
        <Box sx={{ display: "flex", justifyContent: "center", py: 10 }}>
          <CircularProgress />
        </Box>
      )}

      {!loading && error && (
        <Alert
          severity="error"
          action={
            <Button color="inherit" size="small" onClick={() => void load()}>
              Retry
            </Button>
          }
        >
          {error}
        </Alert>
      )}

      {!loading && !error && dashboard && (
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
            <ChartCard title="Count vs. Risk Level">
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
            <RegisterSection
              key={register.register_id}
              register={register}
              scores={scores}
            />
          ))}

          <ChartCard title="Repeated Risks Potentially Impacting Compliance Certs">
            <RepeatedRisksTable data={dashboard.repeated_compliance_risks} />
          </ChartCard>

          <ChartCard title="High Risk Detailed View">
            <HighRisksTable data={dashboard.high_risks} />
          </ChartCard>
        </Stack>
      )}
    </PageContent>
  );
}
