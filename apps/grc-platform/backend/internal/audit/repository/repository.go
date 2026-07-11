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

// Package repository defines the data-access contracts for the Audit Hub module.
package repository

import (
	"context"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
)

// AuditRepository is the data-access contract for audit engagements.
type AuditRepository interface {
	List(ctx context.Context) ([]*model.Audit, error)
	GetByID(ctx context.Context, id int) (*model.Audit, error)
	Create(ctx context.Context, req model.CreateAuditRequest, createdBy string) (*model.Audit, error)
	Update(ctx context.Context, id int, req model.UpdateAuditRequest, updatedBy string) error
	Delete(ctx context.Context, id int, deletedBy string) error
}

// FrameworkControlRepository is the data-access contract for the versioned framework control library.
type FrameworkControlRepository interface {
	ListCurrent(ctx context.Context, frameworkID int) ([]*model.AuditFrameworkControl, error)
}

// FrameworkRepository is the data-access contract for audit frameworks.
type FrameworkRepository interface {
	List(ctx context.Context) ([]*model.AuditFramework, error)
	GetByID(ctx context.Context, id int) (*model.AuditFramework, error)
	Create(ctx context.Context, req model.CreateFrameworkRequest, createdBy string) (*model.AuditFramework, error)
}

// ProductRepository is the data-access contract for audit products.
type ProductRepository interface {
	List(ctx context.Context) ([]*model.AuditProduct, error)
	GetByID(ctx context.Context, id int) (*model.AuditProduct, error)
	Create(ctx context.Context, req model.CreateProductRequest, createdBy string) (*model.AuditProduct, error)
}

// ControlRepository is the data-access contract for audit controls.
type ControlRepository interface {
	List(ctx context.Context, auditID int) ([]*model.AuditControl, error)
	GetByID(ctx context.Context, auditID, controlID int) (*model.AuditControl, error)
	Create(ctx context.Context, auditID int, req model.AddControlRequest, createdBy string) (*model.AuditControl, error)
	BulkCreate(ctx context.Context, auditID int, reqs []model.AddControlRequest, createdBy string) ([]*model.AuditControl, error)
	Update(ctx context.Context, auditID, controlID int, req model.UpdateControlRequest, updatedBy string) error
	UpdateStatus(ctx context.Context, auditID, controlID int, status string, comment *string, updatedBy string) error
	Delete(ctx context.Context, auditID, controlID int) error
	// ListAssignedForEvidence returns all controls assigned to the team of userEmail
	// that are in a status requiring evidence submission.
	ListAssignedForEvidence(ctx context.Context, userEmail string) ([]*model.AssignedControlForEvidence, error)
}

// UserRepository is the data-access contract for the shared user list (owner/auditor dropdowns).
type UserRepository interface {
	List(ctx context.Context) ([]*model.UserRef, error)
}

// TeamRepository is the data-access contract for the audit team list.
type TeamRepository interface {
	List(ctx context.Context) ([]*model.AuditTeam, error)
}

// DashboardRepository aggregates cross-cutting dashboard stats and action items.
type DashboardRepository interface {
	Get(ctx context.Context, f model.DashboardFilter) (*model.DashboardData, error)
}

// EvidenceRepository is the data-access contract for audit evidence submissions.
type EvidenceRepository interface {
	// Create inserts a new evidence row for the given control and returns its ID.
	Create(ctx context.Context, auditID, controlID int, folderPath, createdBy string) (int, error)
	// AddFile inserts a single audit_evidence_file row linked to evidenceID.
	AddFile(ctx context.Context, evidenceID int, fileName, filePath string, fileType *string, fileSize *int64, createdBy string) error
	// ListByControl returns all evidence submissions for a control, newest first, with files pre-loaded.
	ListByControl(ctx context.Context, auditID, controlID int) ([]*model.AuditEvidence, error)
	// GetFileByID returns a single evidence file row by its ID (for downloads).
	GetFileByID(ctx context.Context, fileID int) (*model.AuditEvidenceFile, error)
}

// These remain empty — add methods as their handlers are implemented.
type PopulationRepository interface{}

// CommentRepository is the data-access contract for audit_comment (evidence-scoped).
type CommentRepository interface {
	Create(ctx context.Context, evidenceID int, content string, isInternal bool, parentCommentID *int, createdBy string) (*model.AuditComment, error)
	ListByEvidence(ctx context.Context, evidenceID int) ([]*model.AuditComment, error)
}
type AssignmentRepository interface{}
type NotificationRepository interface{}
type AIValidationLogRepository interface{}
type TrailRepository interface{}

// ReviewRepository is the data-access contract for audit_item_review.
// TODO: add Review/List/GetByID methods as the table schema is finalised.
type ReviewRepository interface{}
