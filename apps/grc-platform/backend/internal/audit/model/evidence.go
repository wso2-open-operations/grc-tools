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

package model

import "time"

// AuditEvidenceFile represents a single uploaded file attached to an evidence submission.
type AuditEvidenceFile struct {
	ID         int       `json:"id"`
	EvidenceID int       `json:"evidenceId"`
	FileName   string    `json:"fileName"`
	FilePath   string    `json:"filePath"`
	FileType   *string   `json:"fileType"`
	FileSize   *int64    `json:"fileSize"`
	CreatedBy  string    `json:"createdBy"`
	CreatedAt  time.Time `json:"createdAt"`
}

// AuditEvidence represents one submission round for a control.
// Each resubmission creates a new row; Files holds all blobs in that round.
type AuditEvidence struct {
	ID         int                  `json:"id"`
	ControlID  int                  `json:"controlId"`
	Status     string               `json:"status"`
	FolderPath string               `json:"folderPath"`
	Files      []*AuditEvidenceFile `json:"files"`
	CreatedBy  string               `json:"createdBy"`
	CreatedAt  time.Time            `json:"createdAt"`
}

// AssignedControlForEvidence is returned by GET /api/v1/evidence-app/controls.
// It gives the evidence capture agent the list of controls it needs to act on,
// along with the Azure Blob base prefix for each control's evidence folder.
type AssignedControlForEvidence struct {
	AuditID       int    `json:"auditId"`
	AuditName     string `json:"auditName"`
	ControlID     int    `json:"controlId"`
	ControlNumber string `json:"controlNumber"`
	Description   string `json:"description"`
	Status        string `json:"status"`
	// BaseFolderPath is the control-level Azure Blob prefix.
	// e.g. "audits/5/controls/12/evidence/"
	// The agent appends a session timestamp when calling /evidence/file-url.
	BaseFolderPath string `json:"baseFolderPath"`
}

// UploadLinkResponse is returned by GET .../evidence/upload-link.
// It gives the agent the folder path to use as a prefix when requesting
// per-file upload URLs and when calling the submit endpoint.
type UploadLinkResponse struct {
	// FolderPath is the Azure Blob prefix for this upload session.
	// e.g. "audits/5/controls/12/evidence/1751500000/"
	FolderPath string    `json:"folderPath"`
	ExpiresAt  time.Time `json:"expiresAt"`
}

// FileUploadURLRequest is the body for POST .../evidence/file-url.
// The agent calls this once per file to get a blob-scoped upload URL.
type FileUploadURLRequest struct {
	FileName   string `json:"fileName"`
	FolderPath string `json:"folderPath"`
}

// FileUploadURLResponse is returned by POST .../evidence/file-url.
// UploadURL is a pre-signed PUT URL scoped to exactly one blob.
// Agent: PUT {UploadURL} with body=file bytes and header x-ms-blob-type: BlockBlob.
type FileUploadURLResponse struct {
	UploadURL string    `json:"uploadUrl"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// SubmitEvidenceRequest is the body for POST .../evidence/submit.
type SubmitEvidenceRequest struct {
	// FolderPath must match exactly what was returned by the upload-link endpoint.
	FolderPath string `json:"folderPath"`
}
