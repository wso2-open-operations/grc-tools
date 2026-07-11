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
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/storage"
)

// validBlobPath enforces the allowed Azure Blob layout. The pattern is fully
// anchored (^ … $) and restricts each segment to a safe character set so that
// dot-dot sequences, query separators (#, ?), and percent-signs cannot appear.
// Dot-dot is additionally rejected explicitly because the character set allows
// individual dots: "report..final.pdf" is fine; ".." as a traversal segment is not.
var validBlobPath = regexp.MustCompile(`^audits/\d+/controls/\d+/evidence/[A-Za-z0-9._ -]+(/[A-Za-z0-9._ -]+)*$`)

func guardBlobPath(path string) bool {
	return !strings.Contains(path, "..") && validBlobPath.MatchString(path)
}

// maxFileUploadBytes caps a single proxied file upload; the GRC Backend already
// validates size/type before forwarding, this is a defensive backstop.
const maxFileUploadBytes = 25 << 20 // 25 MiB

// FileHandler serves the Azure Blob byte operations (upload/download/list/delete).
// It is the entity's file-storage surface: the GRC Backend proxies evidence and
// risk file bytes through these endpoints — the browser never contacts Azure.
type FileHandler struct {
	storage *storage.Service
}

// NewFileHandler constructs a FileHandler over the given storage service.
func NewFileHandler(s *storage.Service) *FileHandler { return &FileHandler{storage: s} }

// UploadFile handles POST /files — multipart form (blobName + file). Writes the
// bytes to Azure Blob at the given blob name.
func (h *FileHandler) UploadFile(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFileUploadBytes)
	if err := r.ParseMultipartForm(maxFileUploadBytes); err != nil { // #nosec G120 -- body already bounded by MaxBytesReader above
		apierror.WriteJSON(w, http.StatusRequestEntityTooLarge, "file too large or malformed upload")
		return
	}
	blobName := strings.TrimSpace(r.FormValue("blobName"))
	if blobName == "" {
		apierror.WriteJSON(w, http.StatusBadRequest, "blobName is required")
		return
	}
	if !guardBlobPath(blobName) {
		apierror.WriteJSON(w, http.StatusBadRequest, "blobName must be within the audits/{id}/controls/{id}/evidence/ path")
		return
	}
	f, header, err := r.FormFile("file")
	if err != nil {
		apierror.WriteJSON(w, http.StatusBadRequest, "file is required")
		return
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		apierror.WriteJSON(w, http.StatusBadRequest, "could not read uploaded file")
		return
	}
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}

	if err := h.storage.UploadBlob(r.Context(), blobName, contentType, data); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(domain.UploadFileResponse{BlobName: blobName, Size: len(data)})
}

// DownloadFile handles GET /files?path=<blobName> — streams the blob bytes back
// (fully proxied). Risky types are served as attachments.
func (h *FileHandler) DownloadFile(w http.ResponseWriter, r *http.Request) {
	blobName := r.URL.Query().Get("path")
	if blobName == "" {
		apierror.WriteJSON(w, http.StatusBadRequest, "path is required")
		return
	}
	if !guardBlobPath(blobName) {
		apierror.WriteJSON(w, http.StatusBadRequest, "path must be within the audits/{id}/controls/{id}/evidence/ path")
		return
	}
	data, contentType, err := h.storage.ReadBlob(r.Context(), blobName)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filepath.Base(blobName)+"\"")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data) // #nosec G705 -- blob served with nosniff + attachment disposition, browser won't execute it inline
}

// ListFiles handles GET /files/list?prefix=<folderPath> — lists blobs in a folder.
func (h *FileHandler) ListFiles(w http.ResponseWriter, r *http.Request) {
	prefix := r.URL.Query().Get("prefix")
	if prefix == "" {
		apierror.WriteJSON(w, http.StatusBadRequest, "prefix is required")
		return
	}
	if !guardBlobPath(prefix) {
		apierror.WriteJSON(w, http.StatusBadRequest, "prefix must be within the audits/{id}/controls/{id}/evidence/ path")
		return
	}
	items, err := h.storage.ListBlobs(r.Context(), prefix)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	out := make([]domain.BlobFileItem, 0, len(items))
	for _, it := range items {
		out = append(out, domain.BlobFileItem{
			Name:        it.Name,
			FileName:    it.FileName(),
			ContentType: it.ContentType,
			Size:        it.Size,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(domain.ListFilesResponse{Files: out})
}

// DeleteFile handles DELETE /files?path=<blobName>.
func (h *FileHandler) DeleteFile(w http.ResponseWriter, r *http.Request) {
	blobName := r.URL.Query().Get("path")
	if blobName == "" {
		apierror.WriteJSON(w, http.StatusBadRequest, "path is required")
		return
	}
	if !guardBlobPath(blobName) {
		apierror.WriteJSON(w, http.StatusBadRequest, "path must be within the audits/{id}/controls/{id}/evidence/ path")
		return
	}
	if err := h.storage.Delete(r.Context(), blobName); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
