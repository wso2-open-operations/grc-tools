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

package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/file"
)

// EvidenceService defines business operations for audit evidence submissions.
type EvidenceService interface {
	// GetUploadLink returns the folder path the client uses as a prefix for this
	// upload session. The client then POSTs each file to the upload endpoint.
	GetUploadLink(ctx context.Context, auditID, controlID int) (*model.UploadLinkResponse, error)

	// UploadFile stores one file into the session folder by proxying the bytes
	// through the backend to the Compliance Entity, which writes it to Azure.
	// No storage credential is handed to the client.
	UploadFile(ctx context.Context, folderPath, fileName, contentType string, data []byte) error

	// Submit reads all blobs at folderPath from Azure, records them in the DB as
	// a new evidence submission, and returns the created evidence record.
	// The caller (handler) is responsible for advancing the control status afterwards.
	Submit(ctx context.Context, auditID, controlID int, folderPath, submittedBy string) (*model.AuditEvidence, error)

	// List returns all evidence submissions for a control, newest first.
	List(ctx context.Context, auditID, controlID int) ([]*model.AuditEvidence, error)

	// DownloadFile returns one evidence file's bytes (proxied via the Compliance
	// Entity) plus its name and content type, by file ID.
	DownloadFile(ctx context.Context, fileID int) (data []byte, fileName, contentType string, err error)
}

type evidenceService struct {
	repo    repository.EvidenceRepository
	storage *file.Service
}

func NewEvidenceService(repo repository.EvidenceRepository, storage *file.Service) EvidenceService {
	return &evidenceService{repo: repo, storage: storage}
}

func (s *evidenceService) GetUploadLink(_ context.Context, auditID, controlID int) (*model.UploadLinkResponse, error) {
	folderPath := fmt.Sprintf("audits/%d/controls/%d/evidence/%d/",
		auditID, controlID, time.Now().Unix())
	return &model.UploadLinkResponse{
		FolderPath: folderPath,
		ExpiresAt:  time.Now().UTC().Add(4 * time.Hour),
	}, nil
}

func (s *evidenceService) UploadFile(ctx context.Context, folderPath, fileName, contentType string, data []byte) error {
	if folderPath == "" {
		return &apierror.Error{StatusCode: http.StatusBadRequest, Body: "folderPath is required"}
	}
	if strings.ContainsAny(fileName, "/\\") {
		return &apierror.Error{StatusCode: http.StatusBadRequest, Body: "fileName must not contain path separators"}
	}
	if len(data) == 0 {
		return &apierror.Error{StatusCode: http.StatusBadRequest, Body: "file is empty"}
	}
	blobName := folderPath + fileName
	return s.storage.UploadBlob(ctx, blobName, contentType, data)
}

func (s *evidenceService) Submit(ctx context.Context, auditID, controlID int, folderPath, submittedBy string) (*model.AuditEvidence, error) {
	if folderPath == "" {
		return nil, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "folderPath is required"}
	}

	blobs, err := s.storage.ListBlobs(ctx, folderPath)
	if err != nil {
		return nil, err
	}
	if len(blobs) == 0 {
		return nil, &apierror.Error{
			StatusCode: http.StatusUnprocessableEntity,
			Body:       "no files found at the specified folderPath — upload files first",
		}
	}

	evidenceID, err := s.repo.Create(ctx, auditID, controlID, folderPath, submittedBy)
	if err != nil {
		return nil, err
	}

	files := make([]*model.AuditEvidenceFile, 0, len(blobs))
	for _, blob := range blobs {
		// Store the blob's relative path; downloads are proxied through the entity.
		filePath := blob.Name
		ct := blob.ContentType
		sz := blob.Size
		if err := s.repo.AddFile(ctx, evidenceID, blob.FileName(), filePath, &ct, &sz, submittedBy); err != nil {
			return nil, err
		}
		files = append(files, &model.AuditEvidenceFile{
			EvidenceID: evidenceID,
			FileName:   blob.FileName(),
			FilePath:   filePath,
			FileType:   &ct,
			FileSize:   &sz,
			CreatedBy:  submittedBy,
		})
	}

	return &model.AuditEvidence{
		ID:        evidenceID,
		ControlID: controlID,
		Status:    "SUBMITTED",
		Files:     files,
		CreatedBy: submittedBy,
		CreatedAt: time.Now(),
	}, nil
}

func (s *evidenceService) List(ctx context.Context, auditID, controlID int) ([]*model.AuditEvidence, error) {
	evidence, err := s.repo.ListByControl(ctx, auditID, controlID)
	if err != nil {
		return nil, err
	}
	// Attach a backend download URL to each file. The reviewer's browser fetches
	// this authenticated endpoint, which proxies the bytes from the Compliance
	// Entity (the browser never contacts Azure directly).
	for _, e := range evidence {
		for _, f := range e.Files {
			if f.ID == 0 {
				continue
			}
			downloadURL := fmt.Sprintf("/api/v1/evidence/files/%d/download", f.ID)
			f.ReadURL = &downloadURL
		}
	}
	return evidence, nil
}

// DownloadFile fetches one evidence file's bytes (proxied via the Compliance
// Entity) by file ID, for the authenticated download endpoint.
func (s *evidenceService) DownloadFile(ctx context.Context, fileID int) (data []byte, fileName, contentType string, err error) {
	f, err := s.repo.GetFileByID(ctx, fileID)
	if err != nil {
		return nil, "", "", err
	}
	data, ct, err := s.storage.ReadBlob(ctx, f.FilePath)
	if err != nil {
		return nil, "", "", err
	}
	if ct == "" && f.FileType != nil {
		ct = *f.FileType
	}
	return data, f.FileName, ct, nil
}
