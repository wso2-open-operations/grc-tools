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

// Package storage provides Azure Blob Storage byte operations for evidence and
// risk file attachments. The Compliance Entity is the only component that holds
// the Azure account key and talks to Azure — the GRC Backend proxies file uploads
// and downloads through this service.
//
// Authentication uses Azure Shared Key (HMAC-SHA256) for all operations (upload,
// read, list, delete). No SAS is issued; reads are streamed back through the
// entity (fully proxied). No Azure SDK is required — all operations use the Azure
// Blob Storage REST API via stdlib net/http.
package storage

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const azureAPIVersion = "2020-02-10"

// Config holds Azure Blob Storage credentials.
type Config struct {
	AccountName   string
	AccountKey    string
	ContainerName string
}

// BlobItem describes a single blob returned by ListBlobs.
type BlobItem struct {
	// Name is the full blob path within the container,
	// e.g. "audits/19/controls/5/evidence/1622/report.pdf".
	Name        string
	ContentType string
	Size        int64
}

// FileName returns just the file name portion of the blob path.
func (b BlobItem) FileName() string {
	parts := strings.Split(b.Name, "/")
	return parts[len(parts)-1]
}

// Service wraps Azure Blob Storage operations.
type Service struct {
	cfg    Config
	client *http.Client
}

// NewService creates a new storage Service.
func NewService(cfg Config) *Service {
	return &Service{cfg: cfg, client: &http.Client{Timeout: 60 * time.Second}}
}

// ContainerURL returns the base HTTPS URL of the container (no trailing slash).
func (s *Service) ContainerURL() string {
	return fmt.Sprintf("https://%s.blob.core.windows.net/%s", s.cfg.AccountName, s.cfg.ContainerName)
}

// BlobURL returns the full HTTPS URL for a blob by its full name within the container.
// Each path segment is percent-encoded so filenames with #, ?, or % do not truncate
// or corrupt the URL. The canonical resource for Shared Key auth uses the raw blobName
// (pre-encoding) which is what Azure expects per the Shared Key specification.
func (s *Service) BlobURL(blobName string) string {
	segments := strings.Split(blobName, "/")
	for i, seg := range segments {
		segments[i] = url.PathEscape(seg)
	}
	return s.ContainerURL() + "/" + strings.Join(segments, "/")
}

// BlobName extracts the blob path from a full blob URL produced by BlobURL.
// Returns the input unchanged if it is not a URL under this container.
func (s *Service) BlobName(fullURL string) string {
	return strings.TrimPrefix(fullURL, s.ContainerURL()+"/")
}

// sign computes the Shared Key Authorization header value for a request.
// stringToSign must already be assembled per the Azure Shared Key scheme.
func (s *Service) sign(stringToSign string) (string, error) {
	keyBytes, err := base64.StdEncoding.DecodeString(s.cfg.AccountKey)
	if err != nil {
		return "", fmt.Errorf("storage: decode account key: %w", err)
	}
	mac := hmac.New(sha256.New, keyBytes)
	mac.Write([]byte(stringToSign))
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("SharedKey %s:%s", s.cfg.AccountName, sig), nil
}

// UploadBlob writes data to the named blob (PUT Block Blob) using Shared Key auth.
func (s *Service) UploadBlob(ctx context.Context, blobName, contentType string, data []byte) error {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	date := time.Now().UTC().Format(http.TimeFormat)
	contentLength := fmt.Sprintf("%d", len(data))

	canonHeaders := "x-ms-blob-type:BlockBlob\n" +
		"x-ms-date:" + date + "\n" +
		"x-ms-version:" + azureAPIVersion + "\n"
	canonResource := fmt.Sprintf("/%s/%s/%s", s.cfg.AccountName, s.cfg.ContainerName, blobName)

	stringToSign := "PUT\n\n\n" + contentLength + "\n\n" + contentType + "\n\n\n\n\n\n\n" +
		canonHeaders + canonResource

	auth, err := s.sign(stringToSign)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, s.BlobURL(blobName), bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("storage: upload build request: %w", err)
	}
	req.ContentLength = int64(len(data))
	req.Header.Set("x-ms-date", date)
	req.Header.Set("x-ms-version", azureAPIVersion)
	req.Header.Set("x-ms-blob-type", "BlockBlob")
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", auth)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("storage: upload blob: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("storage: upload blob: azure returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// ReadBlob fetches the blob's bytes and its stored content type (GET Blob) using
// Shared Key auth. Used to stream a file back through the entity (fully proxied).
func (s *Service) ReadBlob(ctx context.Context, blobName string) (data []byte, contentType string, err error) {
	date := time.Now().UTC().Format(http.TimeFormat)
	canonHeaders := "x-ms-date:" + date + "\n" + "x-ms-version:" + azureAPIVersion + "\n"
	canonResource := fmt.Sprintf("/%s/%s/%s", s.cfg.AccountName, s.cfg.ContainerName, blobName)

	stringToSign := "GET\n\n\n\n\n\n\n\n\n\n\n\n" + canonHeaders + canonResource

	auth, err := s.sign(stringToSign)
	if err != nil {
		return nil, "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.BlobURL(blobName), nil)
	if err != nil {
		return nil, "", fmt.Errorf("storage: read build request: %w", err)
	}
	req.Header.Set("x-ms-date", date)
	req.Header.Set("x-ms-version", azureAPIVersion)
	req.Header.Set("Authorization", auth)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("storage: read blob: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("storage: read blob: azure returned %d: %s", resp.StatusCode, string(body))
	}
	data, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("storage: read blob body: %w", err)
	}
	return data, resp.Header.Get("Content-Type"), nil
}

// ListBlobs returns all blobs whose names start with prefix, using Shared Key auth.
// prefix should end with "/" to list a logical folder. It pages through all Azure
// results (max 5,000 per call) so the list is never silently truncated.
func (s *Service) ListBlobs(ctx context.Context, prefix string) ([]BlobItem, error) {
	var items []BlobItem
	marker := ""
	for {
		page, next, err := s.listBlobsPage(ctx, prefix, marker)
		if err != nil {
			return nil, err
		}
		items = append(items, page...)
		if next == "" {
			break
		}
		marker = next
	}
	return items, nil
}

// listBlobsPage fetches one page of blob results. marker is "" on the first call.
// Returns the blobs, the NextMarker for the following page (empty string = done), and any error.
func (s *Service) listBlobsPage(ctx context.Context, prefix, marker string) ([]BlobItem, string, error) {
	date := time.Now().UTC().Format(http.TimeFormat)

	q := url.Values{}
	q.Set("comp", "list")
	if marker != "" {
		q.Set("marker", marker)
	}
	q.Set("prefix", prefix)
	q.Set("restype", "container")
	endpoint := s.ContainerURL() + "?" + q.Encode()

	// Canonical resource: query parameters must appear alphabetically (comp, marker, prefix, restype).
	// marker is only included when present — omitting it keeps the signature correct on the first page.
	canonResource := fmt.Sprintf("/%s/%s\ncomp:list", s.cfg.AccountName, s.cfg.ContainerName)
	if marker != "" {
		canonResource += "\nmarker:" + marker
	}
	canonResource += "\nprefix:" + prefix + "\nrestype:container"

	canonHeaders := "x-ms-date:" + date + "\n" + "x-ms-version:" + azureAPIVersion + "\n"
	stringToSign := "GET\n\n\n\n\n\n\n\n\n\n\n\n" + canonHeaders + canonResource

	auth, err := s.sign(stringToSign)
	if err != nil {
		return nil, "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, "", fmt.Errorf("storage: list build request: %w", err)
	}
	req.Header.Set("x-ms-date", date)
	req.Header.Set("x-ms-version", azureAPIVersion)
	req.Header.Set("Authorization", auth)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("storage: list blobs: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("storage: list blobs: azure returned %d: %s", resp.StatusCode, string(body))
	}

	var listResp blobListResponse
	if err := xml.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, "", fmt.Errorf("storage: list blobs decode: %w", err)
	}
	items := make([]BlobItem, 0, len(listResp.Blobs.Blob))
	for _, b := range listResp.Blobs.Blob {
		items = append(items, BlobItem{
			Name:        b.Name,
			ContentType: b.Properties.ContentType,
			Size:        b.Properties.ContentLength,
		})
	}
	return items, listResp.NextMarker, nil
}

// Delete removes the blob with the given full name from the container.
func (s *Service) Delete(ctx context.Context, blobName string) error {
	date := time.Now().UTC().Format(http.TimeFormat)
	canonHeaders := "x-ms-date:" + date + "\n" + "x-ms-version:" + azureAPIVersion + "\n"
	canonResource := fmt.Sprintf("/%s/%s/%s", s.cfg.AccountName, s.cfg.ContainerName, blobName)

	stringToSign := "DELETE\n\n\n\n\n\n\n\n\n\n\n\n" + canonHeaders + canonResource

	auth, err := s.sign(stringToSign)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, s.BlobURL(blobName), nil)
	if err != nil {
		return fmt.Errorf("storage: delete build request: %w", err)
	}
	req.Header.Set("x-ms-date", date)
	req.Header.Set("x-ms-version", azureAPIVersion)
	req.Header.Set("Authorization", auth)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("storage: delete blob: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("storage: delete blob: azure returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// blobListResponse is the XML envelope returned by the Azure List Blobs REST API.
type blobListResponse struct {
	XMLName    xml.Name `xml:"EnumerationResults"`
	NextMarker string   `xml:"NextMarker"`
	Blobs      struct {
		Blob []struct {
			Name       string `xml:"Name"`
			Properties struct {
				ContentType   string `xml:"Content-Type"`
				ContentLength int64  `xml:"Content-Length"`
			} `xml:"Properties"`
		} `xml:"Blob"`
	} `xml:"Blobs"`
}
