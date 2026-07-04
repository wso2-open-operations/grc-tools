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

	audithandler "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/handler"
	auditmysql "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository/mysql"
	auditservice "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/file"
)

// buildAuditDeps wires Audit Hub dependencies.
func buildAuditDeps(db *sql.DB, fileSvc *file.Service) audithandler.Deps {
	// ── Repositories ──────────────────────────────────────────────────────────
	auditRepo     := auditmysql.NewAuditRepository(db)
	controlRepo   := auditmysql.NewControlRepository(db)
	frameworkRepo := auditmysql.NewFrameworkRepository(db)
	productRepo   := auditmysql.NewProductRepository(db)
	userRepo      := auditmysql.NewUserRepository(db)
	teamRepo      := auditmysql.NewTeamRepository(db)
	dashboardRepo := auditmysql.NewDashboardRepository(db)
	evidenceRepo  := auditmysql.NewEvidenceRepository(db)

	// ── Services ──────────────────────────────────────────────────────────────
	auditSvc     := auditservice.NewAuditService(auditRepo, frameworkRepo, productRepo)
	controlSvc   := auditservice.NewControlService(controlRepo)
	frameworkSvc := auditservice.NewFrameworkService(frameworkRepo, productRepo)
	userSvc      := auditservice.NewUserService(userRepo)
	teamSvc      := auditservice.NewTeamService(teamRepo)
	dashboardSvc := auditservice.NewDashboardService(dashboardRepo)
	evidenceSvc  := auditservice.NewEvidenceService(evidenceRepo, fileSvc)

	return audithandler.Deps{
		Audit:     auditSvc,
		Control:   controlSvc,
		Framework: frameworkSvc,
		User:      userSvc,
		Team:      teamSvc,
		Dashboard: dashboardSvc,
		Evidence:  evidenceSvc,
		// Population, Comment, Review, Assignment, Trail, Notification
		// are wired here as their implementations are added.
	}
}
