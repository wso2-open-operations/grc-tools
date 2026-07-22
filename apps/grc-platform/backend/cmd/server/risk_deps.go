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

package main

import (
	"database/sql"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/hrentity"
	riskhandler "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/handler"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/repository"
	riskentity "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/repository/entity"
	riskmysql "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/repository/mysql"
	riskservice "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/file"
)

// buildRiskDeps wires the full Risk Hub dependency graph:
// repositories → services → handler Deps struct.
// file is the shared Azure Blob service used by evidence uploads.
// hrClient talks to the HR entity GraphQL service for employee lookups —
// it is never backed by the GRC platform's own database.
//
// entityRepos names the repositories to serve from the Compliance Entity
// instead of MySQL (see config.RiskEntityRepos). It is empty by default, which
// keeps every repository on MySQL — the pre-migration behaviour. Both
// implementations satisfy the same interface, so this choice is invisible to
// the services and handlers below. Once every repository has been migrated,
// this parameter, the MySQL package and the db argument all go away together.
func buildRiskDeps(
	db *sql.DB,
	ec *entityclient.Client,
	fileSvc *file.Service,
	hrClient *hrentity.Client,
	entityRepos map[string]bool,
) riskhandler.Deps {
	var teamRepo repository.TeamRepository
	if entityRepos["team"] {
		teamRepo = riskentity.NewTeamRepository(ec)
	} else {
		teamRepo = riskmysql.NewTeamRepository(db)
	}

	var scoreRepo repository.RiskScoreRepository
	if entityRepos["score"] {
		scoreRepo = riskentity.NewRiskScoreRepository(ec)
	} else {
		scoreRepo = riskmysql.NewRiskScoreRepository(db)
	}

	var complianceRepo repository.ComplianceReferenceRepository
	if entityRepos["compliance"] {
		complianceRepo = riskentity.NewComplianceReferenceRepository(ec)
	} else {
		complianceRepo = riskmysql.NewComplianceReferenceRepository(db)
	}

	var assessmentRepo repository.RiskAssessmentRepository
	if entityRepos["assessment"] {
		assessmentRepo = riskentity.NewAssessmentRepository(ec)
	} else {
		assessmentRepo = riskmysql.NewAssessmentRepository(db)
	}

	var riskRepo repository.RiskRepository
	if entityRepos["risk"] {
		riskRepo = riskentity.NewRiskRepository(ec)
	} else {
		riskRepo = riskmysql.NewRiskRepository(db)
	}

	// Unimplemented features. These have no MySQL behaviour to preserve — every
	// method returns errNotImplemented on both sides — so they are wired to the
	// entity unconditionally rather than through RISK_ENTITY_REPOS. There is
	// nothing to roll back to.
	actionPlanRepo := riskentity.NewActionPlanRepository(ec)
	evidenceRepo := riskentity.NewRiskEvidenceRepository(ec)
	escalationRepo := riskentity.NewEscalationRepository(ec)
	notifRepo := riskentity.NewNotificationRepository(ec)

	analyticsSvc := riskservice.NewAnalyticsService(riskmysql.NewAnalyticsRepository(db))
	if entityRepos["analytics"] {
		analyticsSvc = riskservice.NewAssembledAnalyticsService(riskentity.NewAnalyticsRepository(ec))
	}

	// Dashboard: the entity returns the payload already assembled, so that path
	// uses a passthrough service rather than the fact-pivoting one.
	dashboardSvc := riskservice.NewDashboardService(riskmysql.NewDashboardRepository(db))
	if entityRepos["dashboard"] {
		dashboardSvc = riskservice.NewAssembledDashboardService(riskentity.NewDashboardRepository(ec))
	}

	return riskhandler.Deps{
		Risk:         riskservice.NewRiskService(riskRepo),
		Assessment:   riskservice.NewRiskAssessmentService(assessmentRepo),
		Team:         riskservice.NewTeamService(teamRepo),
		Score:        riskservice.NewRiskScoreService(scoreRepo),
		ActionPlan:   riskservice.NewActionPlanService(actionPlanRepo),
		Evidence:     riskservice.NewEvidenceService(evidenceRepo, fileSvc),
		Escalation:   riskservice.NewEscalationService(escalationRepo),
		Notification: riskservice.NewNotificationService(notifRepo),
		Compliance:   riskservice.NewComplianceReferenceService(complianceRepo),
		Analytics:    analyticsSvc,
		Dashboard:    dashboardSvc,
		Employee:     riskservice.NewEmployeeSearchService(hrClient),
	}
}
