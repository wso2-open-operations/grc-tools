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

// Package handler contains the HTTP handlers for the Audit Hub module.
package handler

import (
	"net/http"

	auditservice "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/service"
)

// Deps holds all service dependencies for Audit Hub handlers.
// Handlers call service methods; services call repositories.
type Deps struct {
	Audit        auditservice.AuditService
	Control      auditservice.ControlService
	Framework    auditservice.FrameworkService
	User         auditservice.UserService
	Team         auditservice.TeamService
	Dashboard    auditservice.DashboardService
	Evidence     auditservice.EvidenceService
	Population   auditservice.PopulationService
	Comment      auditservice.CommentService
	Notification auditservice.NotificationService
	Assignment   auditservice.AssignmentService
	Trail        auditservice.TrailService
}

// RegisterRoutes mounts all Audit Hub routes onto mux.
func RegisterRoutes(mux *http.ServeMux, deps Deps) {
	ah := &auditHandler{svc: deps.Audit}
	ch := &controlHandler{svc: deps.Control}
	fh := &frameworkHandler{svc: deps.Framework}
	uh := &userHandler{svc: deps.User}
	th := &teamHandler{svc: deps.Team}
	dh := &dashboardHandler{svc: deps.Dashboard}
	eh := &evidenceHandler{svc: deps.Evidence, controlSvc: deps.Control}
	cmh := &commentHandler{svc: deps.Comment}

	// Dashboard.
	mux.HandleFunc("GET /api/v1/audit/dashboard", dh.getDashboard)

	// Lookup data for Create Audit form dropdowns.
	mux.HandleFunc("GET /api/v1/audit/frameworks", fh.listFrameworks)
	mux.HandleFunc("POST /api/v1/audit/frameworks", fh.createFramework)
	mux.HandleFunc("GET /api/v1/audit/frameworks/{id}/controls", fh.listFrameworkControls)
	mux.HandleFunc("GET /api/v1/audit/products", fh.listProducts)
	mux.HandleFunc("POST /api/v1/audit/products", fh.createProduct)
	mux.HandleFunc("GET /api/v1/audit/users", uh.listUsers)
	mux.HandleFunc("GET /api/v1/audit/teams", th.listTeams)

	// Audit CRUD.
	mux.HandleFunc("GET /api/v1/audits", ah.listAudits)
	mux.HandleFunc("POST /api/v1/audits", ah.createAudit)
	mux.HandleFunc("GET /api/v1/audits/{id}", ah.getAudit)
	mux.HandleFunc("PUT /api/v1/audits/{id}", ah.updateAudit)
	mux.HandleFunc("DELETE /api/v1/audits/{id}", ah.deleteAudit)

	// Control CRUD + status transitions.
	// Note: /bulk must be registered before /{controlId} so the router matches it first.
	mux.HandleFunc("GET /api/v1/audits/{id}/controls", ch.listControls)
	mux.HandleFunc("POST /api/v1/audits/{id}/controls", ch.addControl)
	mux.HandleFunc("POST /api/v1/audits/{id}/controls/bulk", ch.bulkAddControls)
	mux.HandleFunc("GET /api/v1/audits/{id}/controls/{controlId}", ch.getControl)
	mux.HandleFunc("PUT /api/v1/audits/{id}/controls/{controlId}", ch.updateControl)
	mux.HandleFunc("DELETE /api/v1/audits/{id}/controls/{controlId}", ch.deleteControl)
	mux.HandleFunc("PATCH /api/v1/audits/{id}/controls/{controlId}/status", ch.updateControlStatus)

	// Evidence submission (backend-proxied upload flow).
	// Note: /upload-link, /upload and /submit must be registered before the plain
	// /evidence list route so the router matches their literal suffixes first.
	// File bytes are proxied through the backend (POST /upload, multipart) — no
	// write SAS is handed to the client.
	mux.HandleFunc("GET /api/v1/evidence-app/controls", eh.getAssignedControls)
	mux.HandleFunc("GET /api/v1/audits/{id}/controls/{controlId}/evidence/upload-link", eh.getUploadLink)
	mux.HandleFunc("POST /api/v1/audits/{id}/controls/{controlId}/evidence/upload", eh.uploadEvidence)
	mux.HandleFunc("POST /api/v1/audits/{id}/controls/{controlId}/evidence/submit", eh.submitEvidence)
	mux.HandleFunc("GET /api/v1/audits/{id}/controls/{controlId}/evidence", eh.listEvidence)
	// Proxied file download by file ID (bytes streamed via the Compliance Entity).
	mux.HandleFunc("GET /api/v1/evidence/files/{fileId}/download", eh.downloadEvidenceFile)

	// Evidence comments (evidence-scoped; is_internal hides from external auditors)
	mux.HandleFunc("GET /api/v1/evidence/{evidenceId}/comments", cmh.listComments)
	mux.HandleFunc("POST /api/v1/evidence/{evidenceId}/comments", cmh.addComment)
}
