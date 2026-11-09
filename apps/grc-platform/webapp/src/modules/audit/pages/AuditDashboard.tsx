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

import { Alert, Box, Skeleton, useTheme } from "@wso2/oxygen-ui";
import { AlertTriangle, CheckCircle, ClipboardList, Clock, Inbox, ShieldCheck, Upload } from "@wso2/oxygen-ui-icons-react";
import type { JSX } from "react";
import { useRef, useState } from "react";
import { useSearchParams } from "react-router";
import { useGetAudits } from "@modules/audit/api/useGetAudits";
import { useGetDashboard } from "@modules/audit/api/useGetDashboard";
import { useAuditPrivileges } from "@modules/audit/hooks/useAuditPrivileges";
import { AuditPrivilege } from "@modules/audit/privileges";
import { useIdTokenClaims } from "@hooks/useIdTokenClaims";
import TabBar, { type TabOption } from "@components/tab-bar/TabBar";
import AuditProgressList from "@modules/audit/components/dashboard/AuditProgressList";
import HeroBand from "@modules/audit/components/dashboard/HeroBand";
import {
  isQueueTabVisible,
  QUEUE_TAB_ALL_PENDING,
  QUEUE_TAB_AWAITING,
  QUEUE_TAB_COMPLETED,
  QUEUE_TAB_DUE_SOON,
  QUEUE_TAB_OVERDUE,
  QUEUE_TAB_PENDING,
  QUEUE_TAB_VALIDATION,
} from "@modules/audit/utils/workQueueTabs";
import KpiCards, { type KpiItem } from "@modules/audit/components/dashboard/KpiCards";
import PhaseDonut from "@modules/audit/components/dashboard/PhaseDonut";
import AuditProgramFilter from "@modules/audit/components/dashboard/AuditProgramFilter";
import { ALL_PROGRAMS, auditIdsFor } from "@modules/audit/utils/assigneeProgress";
import PersonStatusTable from "@modules/audit/components/dashboard/PersonStatusTable";
import RoleProgressCard from "@modules/audit/components/dashboard/RoleProgressCard";
import RoleStatusTotalsCard from "@modules/audit/components/dashboard/RoleStatusTotalsCard";
import SectionCard from "@modules/audit/components/dashboard/SectionCard";
import PhaseInsightDialog from "@modules/audit/components/dashboard/PhaseInsightDialog";
import TeamProgress from "@modules/audit/components/dashboard/TeamProgress";
import TeamInsightDialog from "@modules/audit/components/dashboard/TeamInsightDialog";
import WorkQueue from "@modules/audit/components/dashboard/WorkQueue";
import FrameworkReadinessTab from "@modules/audit/components/dashboard/framework/FrameworkReadinessTab";
import type { ControlPhase } from "@modules/audit/utils/controlStatus";
import type { Audit } from "@modules/audit/types/audit";
import type { TeamCompletion } from "@modules/audit/types/dashboard";

const TAB_OVERVIEW = "overview";
const TAB_FRAMEWORKS = "frameworks";

// ── Filtered chart cards ──────────────────────────────────────────────────────
// Each card owns its audit-program filter and fetches the dashboard for just
// that selection (react-query shares the unfiltered "All programs" result), so
// phase and team numbers can be viewed per audit independently.

function PhaseCard({ audits }: { audits: Audit[] }): JSX.Element {
  const [program, setProgram] = useState(ALL_PROGRAMS);
  const [selectedPhase, setSelectedPhase] = useState<ControlPhase | null>(null);
  const { data, isLoading, isPlaceholderData } = useGetDashboard(auditIdsFor(program));
  const distribution = data?.statusDistribution ?? [];
  return (
    <SectionCard title="Controls by Phase" action={<AuditProgramFilter audits={audits} value={program} onChange={setProgram} />}>
      {isLoading ? <Skeleton variant="rectangular" height={240} sx={{ borderRadius: 1 }} /> : (
        <Box sx={{ opacity: isPlaceholderData ? 0.5 : 1, transition: "opacity 150ms" }}>
          <PhaseDonut data={distribution} onPhaseClick={setSelectedPhase} />
        </Box>
      )}
      <PhaseInsightDialog phase={selectedPhase} statusDistribution={distribution} onClose={() => setSelectedPhase(null)} />
    </SectionCard>
  );
}

function TeamCard({ audits }: { audits: Audit[] }): JSX.Element {
  const [program, setProgram] = useState(ALL_PROGRAMS);
  const [selectedTeam, setSelectedTeam] = useState<{ team: TeamCompletion; color: string } | null>(null);
  const { data, isLoading, isPlaceholderData } = useGetDashboard(auditIdsFor(program));
  return (
    <SectionCard title="Team Progress" action={<AuditProgramFilter audits={audits} value={program} onChange={setProgram} />}>
      {isLoading ? <Skeleton variant="rectangular" height={240} sx={{ borderRadius: 1 }} /> : (
        <Box sx={{ opacity: isPlaceholderData ? 0.5 : 1, transition: "opacity 150ms" }}>
          <TeamProgress
            data={data?.teamCompletion ?? []}
            onTeamClick={(team, color) => setSelectedTeam({ team, color })}
          />
        </Box>
      )}
      <TeamInsightDialog
        team={selectedTeam?.team ?? null}
        color={selectedTeam?.color ?? "#1E88E5"}
        teamStatusDistribution={data?.teamStatusDistribution ?? []}
        onClose={() => setSelectedTeam(null)}
      />
    </SectionCard>
  );
}

// Statuses each role's view tracks — the conditions the stakeholders asked for.
const AUDITOR_STATUSES = ["EVIDENCE_UNDER_VALIDATION", "POPULATION_UNDER_VALIDATION", "POPULATION_COMPLETE"] as const;
const OWNER_STATUSES = [
  "POPULATION_PENDING", "EVIDENCE_PENDING", "EVIDENCE_NEED_CLARIFICATION",
  "POPULATION_NEED_CLARIFICATION", "OVERDUE",
] as const;
const COMPLIANCE_STATUSES = ["EVIDENCE_INTERNAL_REVIEW", "POPULATION_INTERNAL_REVIEW"] as const;

// ── Skeleton ──────────────────────────────────────────────────────────────────

function DashboardSkeleton(): JSX.Element {
  return (
    <Box sx={{ p: 3, display: "flex", flexDirection: "column", gap: 3 }}>
      <Skeleton variant="rectangular" height={140} sx={{ borderRadius: 2 }} />
      <Box sx={{ display: "flex", gap: 2 }}>
        {[0, 1, 2, 3].map((i) => (
          <Skeleton key={i} variant="rectangular" height={92} sx={{ borderRadius: 2, flex: 1 }} />
        ))}
      </Box>
      <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", lg: "1fr 1fr 1fr" }, gap: 2 }}>
        <Skeleton variant="rectangular" height={300} sx={{ borderRadius: 2 }} />
        <Skeleton variant="rectangular" height={300} sx={{ borderRadius: 2 }} />
        <Skeleton variant="rectangular" height={300} sx={{ borderRadius: 2 }} />
      </Box>
      <Skeleton variant="rectangular" height={260} sx={{ borderRadius: 2 }} />
    </Box>
  );
}

// ── Main ──────────────────────────────────────────────────────────────────────

export default function AuditDashboard(): JSX.Element {
  const theme = useTheme();
  const { can } = useAuditPrivileges();
  const { data, isLoading, isError } = useGetDashboard();
  const { data: auditsData } = useGetAudits();
  const claims = useIdTokenClaims();

  // The Frameworks tab is an org-wide readiness view — shown only to org-wide
  // readers (admin, compliance team, management). Internal team and external
  // auditors, who lack AUDIT_VIEW_ALL_AUDITS, never see it.
  const canViewAll = can(AuditPrivilege.ViewAllAudits);

  // Tab state lives in the URL (?tab=frameworks) so a status email can link
  // straight to the Frameworks view; Overview is the default landing tab.
  const [searchParams, setSearchParams] = useSearchParams();
  const activeTab =
    canViewAll && searchParams.get("tab") === TAB_FRAMEWORKS ? TAB_FRAMEWORKS : TAB_OVERVIEW;
  const setActiveTab = (tabId: string) => {
    const next = new URLSearchParams(searchParams);
    if (tabId === TAB_FRAMEWORKS) next.set("tab", TAB_FRAMEWORKS);
    else next.delete("tab");
    setSearchParams(next, { replace: true });
  };

  const tabs: TabOption[] = [
    { id: TAB_OVERVIEW, label: "Overview" },
    ...(canViewAll ? [{ id: TAB_FRAMEWORKS, label: "Frameworks" }] : []),
  ];

  const queueRef = useRef<HTMLDivElement>(null);
  const [queueTab, setQueueTab] = useState(QUEUE_TAB_AWAITING);
  const [queueHighlight, setQueueHighlight] = useState(false);


  const jumpToQueue = (tab: number) => {
    setQueueTab(tab);
    queueRef.current?.scrollIntoView({ behavior: "smooth", block: "start" });
    setQueueHighlight(true);
    setTimeout(() => setQueueHighlight(false), 1800);
  };

  if (isLoading) return <DashboardSkeleton />;

  if (isError || !data) {
    return (
      <Box sx={{ p: 3 }}>
        <Alert severity="error">Failed to load dashboard. Please refresh the page.</Alert>
      </Box>
    );
  }

  const { auditStats, stats } = data;

  // Privilege-driven gating (never role names).
  const canSubmit = can(AuditPrivilege.SubmitEvidence);
  const canApprove = can(AuditPrivilege.ReviewEvidence);
  const canValidate = can(AuditPrivilege.ValidateEvidence);
  const hasQueue = canSubmit || canApprove || canValidate;
  const queueTitle = canApprove
    ? "Review Queue"
    : canValidate
      ? "Validation Queue"
      : canSubmit
        ? "My Tasks"
        : "Action Items";
  const awaitingCount = hasQueue ? (stats.totalActionItems ?? null) : null;

  const dueSoonCount = data.dueSoonItems?.length ?? 0;
  const pendingCount = data.pendingCount ?? 0;
  const validationCount = data.validationCount ?? 0;
  const allPendingCount = data.allPendingCount ?? 0;
  // An entity that predates completedCount still reports stats.completedControls.
  const completedCount = data.completedCount ?? stats.completedControls;
  const audits = auditsData?.items ?? [];

  const access = { canViewAll, canApprove, canSubmit, canValidate };
  const neutral = theme.palette.grey[500];
  const kpiDefs: (KpiItem & { tab: number })[] = [
    {
      tab: QUEUE_TAB_ALL_PENDING, key: "all-pending", icon: <ClipboardList size={22} />, iconColor: theme.palette.primary.main,
      value: allPendingCount, label: "All Pending", sub: `of ${stats.totalControls} total controls`,
    },
    {
      tab: QUEUE_TAB_AWAITING, key: "awaiting", icon: <Inbox size={22} />, iconColor: theme.palette.warning.main,
      value: stats.totalActionItems ?? 0, label: queueTitle, sub: "click to view",
      valueColor: (stats.totalActionItems ?? 0) > 0 ? theme.palette.warning.main : undefined,
    },
    {
      tab: QUEUE_TAB_DUE_SOON, key: "due-soon", icon: <Clock size={22} />, iconColor: theme.palette.info.main,
      value: dueSoonCount, label: "Due Soon", sub: "next 7 days",
    },
    {
      tab: QUEUE_TAB_PENDING, key: "pending", icon: <Upload size={22} />, iconColor: theme.palette.warning.main,
      value: pendingCount, label: "Pending Submission", sub: "awaiting owners",
    },
    {
      tab: QUEUE_TAB_VALIDATION, key: "validation", icon: <ShieldCheck size={22} />, iconColor: theme.palette.secondary.main,
      value: validationCount, label: "Under Validation", sub: "awaiting auditors",
    },
    {
      tab: QUEUE_TAB_OVERDUE, key: "overdue", icon: <AlertTriangle size={22} />,
      iconColor: stats.overdueControls > 0 ? theme.palette.error.main : neutral,
      value: stats.overdueControls, label: "Overdue", sub: stats.overdueControls > 0 ? "needs attention" : "all on schedule",
      valueColor: stats.overdueControls > 0 ? theme.palette.error.main : undefined,
    },
    {
      tab: QUEUE_TAB_COMPLETED, key: "completed", icon: <CheckCircle size={22} />, iconColor: theme.palette.success.main,
      value: completedCount, label: "Completed", sub: `${stats.completionPercent.toFixed(1)}% of scope`,
      valueColor: theme.palette.success.main,
    },
  ];
  const kpiItems: KpiItem[] = kpiDefs
    .filter((d) => isQueueTabVisible(d.tab, access))
    .map(({ tab, ...item }) => ({ ...item, onClick: () => jumpToQueue(tab) }));

  const userName =
    (claims?.given_name as string | undefined) ??
    (claims?.username as string | undefined)?.split("@")[0] ??
    null;

  return (
    <Box sx={{ p: 3, display: "flex", flexDirection: "column", gap: 3 }}>

      <TabBar tabs={tabs} activeTab={activeTab} onTabChange={setActiveTab} sx={{ mb: 0 }} />

      {activeTab === TAB_FRAMEWORKS ? (
        <FrameworkReadinessTab audits={auditsData?.items ?? []} />
      ) : (
        <>

      {/* ① Hero band — greeting, completion ring, attention chips */}
      <HeroBand
        userName={userName}
        completionPercent={stats.completionPercent}
        activeAudits={auditStats.activeAudits}
        totalControls={stats.totalControls}
        overdueCount={stats.overdueControls}
        dueSoonCount={dueSoonCount}
        awaitingCount={awaitingCount}
        awaitingLabel={canApprove ? "to review" : canValidate ? "to validate" : "to submit"}
        onOverdueClick={() => jumpToQueue(QUEUE_TAB_OVERDUE)}
        onQueueClick={() => jumpToQueue(QUEUE_TAB_AWAITING)}
      />

      {/* ② KPI cards — one per work-queue tab, each jumping to its tab */}
      <KpiCards items={kpiItems} />

      {/* ③ Charts row */}
      <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", lg: "1fr 1fr 1fr" }, gap: 2 }}>
        <PhaseCard audits={audits} />
        <SectionCard title="Audit Progress">
          <AuditProgressList audits={audits} />
        </SectionCard>
        <TeamCard audits={audits} />
      </Box>

      {/* ④ Role progress views — auditor and compliance as cards, process owner
          as a full-width table (one row per person, a column per status). */}
      <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", lg: "1fr 1fr" }, gap: 2 }}>
        <RoleProgressCard title="Auditor Team" role="auditor" statuses={AUDITOR_STATUSES} audits={audits} />
        <RoleStatusTotalsCard title="Compliance Team" role="compliance" statuses={COMPLIANCE_STATUSES} audits={audits} />
      </Box>
      <PersonStatusTable title="Process Owner Team" role="owner" statuses={OWNER_STATUSES} personLabel="Process Owner" audits={audits} />

      {/* ⑤ Work queue — tabbed (awaiting you / due soon / overdue). Also shown
          read-only for viewers without submit/review since the overdue tab is
          the monitoring view (their awaiting tab will simply be empty). */}
      <Box
        ref={queueRef}
        sx={{
          borderRadius: 2,
          outline: queueHighlight ? "2px solid #FB8C00" : "2px solid transparent",
          transition: "outline-color 0.3s",
        }}
      >
        <SectionCard title="Work Queue">
          <WorkQueue
            totalActionItems={stats.totalActionItems ?? 0}
            totalDueSoonItems={dueSoonCount}
            totalPendingItems={pendingCount}
            totalValidationItems={validationCount}
            totalOverdueControls={stats.overdueControls}
            totalAllPendingItems={allPendingCount}
            totalCompletedItems={completedCount}
            canViewAll={canViewAll}
            canApprove={canApprove}
            canSubmit={canSubmit}
            canValidate={canValidate}
            queueTitle={queueTitle}
            tab={queueTab}
            onTabChange={setQueueTab}
          />
        </SectionCard>
      </Box>

        </>
      )}

    </Box>
  );
}
