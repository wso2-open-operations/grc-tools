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
	audithandler "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/handler"
	auditentity "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository/entity"
	auditservice "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/file"
)

// buildAuditDeps wires Audit Hub dependencies. The audit module now reads/writes
// ALL data through the Compliance Entity (via ec) — no direct MySQL access.
func buildAuditDeps(fileSvc *file.Service, ec *entityclient.Client) audithandler.Deps {
	// ── Repositories (all Compliance Entity) ──────────────────────────────────
	auditRepo := auditentity.NewAuditRepository(ec)
	frameworkRepo := auditentity.NewFrameworkRepository(ec)
	frameworkControlRepo := auditentity.NewFrameworkControlRepository(ec)
	productRepo := auditentity.NewProductRepository(ec)
	userRepo := auditentity.NewUserRepository(ec)
	teamRepo := auditentity.NewTeamRepository(ec)
	commentRepo := auditentity.NewCommentRepository(ec)
	controlRepo := auditentity.NewControlRepository(ec)
	evidenceRepo := auditentity.NewEvidenceRepository(ec)
	dashboardRepo := auditentity.NewDashboardRepository(ec)

	// ── Services ──────────────────────────────────────────────────────────────
	auditSvc := auditservice.NewAuditService(auditRepo, frameworkRepo, productRepo)
	controlSvc := auditservice.NewControlService(controlRepo)
	frameworkSvc := auditservice.NewFrameworkService(frameworkRepo, productRepo, frameworkControlRepo)
	userSvc := auditservice.NewUserService(userRepo)
	teamSvc := auditservice.NewTeamService(teamRepo)
	dashboardSvc := auditservice.NewDashboardService(dashboardRepo)
	evidenceSvc := auditservice.NewEvidenceService(evidenceRepo, fileSvc)
	commentSvc := auditservice.NewCommentService(commentRepo)

	return audithandler.Deps{
		Audit:     auditSvc,
		Control:   controlSvc,
		Framework: frameworkSvc,
		User:      userSvc,
		Team:      teamSvc,
		Dashboard: dashboardSvc,
		Evidence:  evidenceSvc,
		Comment:   commentSvc,
		// Population, Review, Assignment, Trail, Notification
		// are wired here as their implementations are added.
	}
}
