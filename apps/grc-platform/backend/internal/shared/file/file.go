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

// Package file provides Azure Blob Storage operations used by evidence attachments
// in both the Risk and Audit modules.
//
// Authentication uses Azure Shared Key (HMAC-SHA256) for server-side API calls
// (ListBlobs, Delete) and Service SAS tokens for agent-side direct uploads.
// No Azure SDK is required — all operations use the Azure Blob Storage REST API
// via stdlib net/http.
package file

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

// StorageConfig holds Azure Blob Storage credentials from environment variables.
type StorageConfig struct {
	AccountName   string
	AccountKey    string
	ContainerName string
}

// BlobItem describes a single blob returned by ListBlobs.
type BlobItem struct {
	// Name is the full blob path within the container, e.g. "audits/19/controls/5/evidence/1622/report.pdf".
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
	cfg StorageConfig
}

// NewService creates a new file Service.
func NewService(cfg StorageConfig) *Service {
	return &Service{cfg: cfg}
}

// ContainerURL returns the base HTTPS URL of the container (no trailing slash, no SAS).
func (s *Service) ContainerURL() string {
	return fmt.Sprintf("https://%s.blob.core.windows.net/%s", s.cfg.AccountName, s.cfg.ContainerName)
}

// BlobURL returns the full HTTPS URL for a blob by its full name within the container.
func (s *Service) BlobURL(blobName string) string {
	return s.ContainerURL() + "/" + blobName
}

// blobSAS signs a blob-scoped Service SAS with the given permissions ("cw" for
// create+write uploads, "r" for read-only views) and returns the full signed URL.
// respContentType, when non-empty, overrides the Content-Type Azure returns on a
// read (the rsct response header), so browsers display the blob inline instead of
// downloading an application/octet-stream. Ignored for uploads.
func (s *Service) blobSAS(blobName, perms, respContentType string, ttl time.Duration) (signedURL string, expiresAt time.Time, err error) {
	keyBytes, err := base64.StdEncoding.DecodeString(s.cfg.AccountKey)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("file: decode account key: %w", err)
	}

	now := time.Now().UTC()
	start := now.Add(-5 * time.Minute) // small clock-skew allowance
	expiry := now.Add(ttl)

	const sv = "2020-02-10"
	const sr = "b" // blob resource (scoped to one blob)
	const spr = "https"

	const azureFmt = "2006-01-02T15:04:05Z"
	st := start.Format(azureFmt)
	se := expiry.Format(azureFmt)

	// Blob-level canonical resource includes the full blob name path.
	canonicalizedResource := "/blob/" + s.cfg.AccountName + "/" + s.cfg.ContainerName + "/" + blobName

	// Service SAS string-to-sign (sv 2020-02-10, 15 fields). rsct is the last field.
	stringToSign := strings.Join([]string{
		perms,                 // signedPermissions
		st,                    // signedStart
		se,                    // signedExpiry
		canonicalizedResource, // canonicalizedResource
		"",                    // signedIdentifier
		"",                    // signedIP
		spr,                   // signedProtocol
		sv,                    // signedVersion
		sr,                    // signedResource
		"",                    // signedSnapshotTime
		"",                    // rscc
		"",                    // rscd
		"",                    // rsce
		"",                    // rscl
		respContentType,       // rsct (response Content-Type override)
	}, "\n")

	mac := hmac.New(sha256.New, keyBytes)
	mac.Write([]byte(stringToSign))
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	q := url.Values{}
	q.Set("sv", sv)
	q.Set("sr", sr)
	q.Set("sp", perms)
	q.Set("st", st)
	q.Set("se", se)
	q.Set("spr", spr)
	if respContentType != "" {
		q.Set("rsct", respContentType)
	}
	q.Set("sig", sig)

	return s.BlobURL(blobName) + "?" + q.Encode(), expiry, nil
}

// GenerateBlobSASURL creates a blob-scoped create+write SAS URL for uploading
// exactly one blob. The caller cannot touch any other blob with this token.
func (s *Service) GenerateBlobSASURL(blobName string, ttl time.Duration) (uploadURL string, expiresAt time.Time, err error) {
	return s.blobSAS(blobName, "cw", "", ttl)
}

// GenerateReadSASURL creates a blob-scoped read-only SAS URL for viewing or
// downloading exactly one blob. contentType (optional) is returned as the
// Content-Type so the browser can display the blob inline.
func (s *Service) GenerateReadSASURL(blobName, contentType string, ttl time.Duration) (readURL string, expiresAt time.Time, err error) {
	return s.blobSAS(blobName, "r", contentType, ttl)
}

// BlobName extracts the blob path from a full blob URL produced by BlobURL.
// Returns the input unchanged if it is not a URL under this container.
func (s *Service) BlobName(fullURL string) string {
	return strings.TrimPrefix(fullURL, s.ContainerURL()+"/")
}

// UploadBlob writes data to the named blob using Shared Key auth (PUT Block Blob).
// Used for server-side proxied uploads: the file bytes travel client -> backend ->
// Azure, so the account key never leaves the backend and no SAS is handed to the
// client. contentType sets the stored blob's Content-Type.
func (s *Service) UploadBlob(ctx context.Context, blobName, contentType string, data []byte) error {
	keyBytes, err := base64.StdEncoding.DecodeString(s.cfg.AccountKey)
	if err != nil {
		return fmt.Errorf("file: decode account key: %w", err)
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	const version = "2020-02-10"
	date := time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")
	blobURL := s.BlobURL(blobName)
	contentLength := fmt.Sprintf("%d", len(data))

	// Canonicalized headers must be sorted alphabetically by header name.
	canonHeaders := "x-ms-blob-type:BlockBlob\n" +
		"x-ms-date:" + date + "\n" +
		"x-ms-version:" + version + "\n"

	canonResource := fmt.Sprintf("/%s/%s/%s", s.cfg.AccountName, s.cfg.ContainerName, blobName)

	stringToSign := "PUT\n" +
		"\n" + // Content-Encoding
		"\n" + // Content-Language
		contentLength + "\n" + // Content-Length
		"\n" + // Content-MD5
		contentType + "\n" + // Content-Type
		"\n" + // Date (empty — x-ms-date used instead)
		"\n" + // If-Modified-Since
		"\n" + // If-Match
		"\n" + // If-None-Match
		"\n" + // If-Unmodified-Since
		"\n" + // Range
		canonHeaders +
		canonResource

	mac := hmac.New(sha256.New, keyBytes)
	mac.Write([]byte(stringToSign))
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, blobURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("file: upload blob: build request: %w", err)
	}
	req.ContentLength = int64(len(data))
	req.Header.Set("x-ms-date", date)
	req.Header.Set("x-ms-version", version)
	req.Header.Set("x-ms-blob-type", "BlockBlob")
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", fmt.Sprintf("SharedKey %s:%s", s.cfg.AccountName, sig))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("file: upload blob: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("file: upload blob: azure returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// ListBlobs returns all blobs whose names start with prefix, using Shared Key auth.
// prefix should end with "/" to list a logical folder (e.g. "audits/19/controls/5/evidence/1622000000/").
func (s *Service) ListBlobs(ctx context.Context, prefix string) ([]BlobItem, error) {
	keyBytes, err := base64.StdEncoding.DecodeString(s.cfg.AccountKey)
	if err != nil {
		return nil, fmt.Errorf("file: decode account key: %w", err)
	}

	const version = "2020-02-10"
	date := time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")

	// Build the request URL — query params must be sorted alphabetically for canonical resource.
	q := url.Values{}
	q.Set("comp", "list")
	q.Set("prefix", prefix)
	q.Set("restype", "container")
	endpoint := s.ContainerURL() + "?" + q.Encode()

	// Canonicalized headers must be sorted alphabetically by header name.
	canonHeaders := "x-ms-date:" + date + "\n" +
		"x-ms-version:" + version + "\n"

	// Canonicalized resource: /{account}/{container}\n{sorted-params}
	// Params appear in the canonical resource in their decoded (not percent-encoded) form.
	canonResource := fmt.Sprintf("/%s/%s\ncomp:list\nprefix:%s\nrestype:container",
		s.cfg.AccountName, s.cfg.ContainerName, prefix)

	stringToSign := "GET\n" + // VERB
		"\n" + // Content-Encoding
		"\n" + // Content-Language
		"\n" + // Content-Length (empty for GET)
		"\n" + // Content-MD5
		"\n" + // Content-Type
		"\n" + // Date (empty — x-ms-date used instead)
		"\n" + // If-Modified-Since
		"\n" + // If-Match
		"\n" + // If-None-Match
		"\n" + // If-Unmodified-Since
		"\n" + // Range
		canonHeaders +
		canonResource

	mac := hmac.New(sha256.New, keyBytes)
	mac.Write([]byte(stringToSign))
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("file: list blobs: build request: %w", err)
	}
	req.Header.Set("x-ms-date", date)
	req.Header.Set("x-ms-version", version)
	req.Header.Set("Authorization", fmt.Sprintf("SharedKey %s:%s", s.cfg.AccountName, sig))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("file: list blobs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("file: list blobs: azure returned %d: %s", resp.StatusCode, string(body))
	}

	var listResp blobListResponse
	if err := xml.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, fmt.Errorf("file: list blobs: decode response: %w", err)
	}

	items := make([]BlobItem, 0, len(listResp.Blobs.Blob))
	for _, b := range listResp.Blobs.Blob {
		items = append(items, BlobItem{
			Name:        b.Name,
			ContentType: b.Properties.ContentType,
			Size:        b.Properties.ContentLength,
		})
	}
	return items, nil
}

// Delete removes the blob with the given full name from the container.
func (s *Service) Delete(ctx context.Context, blobName string) error {
	keyBytes, err := base64.StdEncoding.DecodeString(s.cfg.AccountKey)
	if err != nil {
		return fmt.Errorf("file: decode account key: %w", err)
	}

	const version = "2020-02-10"
	date := time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")
	blobURL := s.BlobURL(blobName)

	canonHeaders := "x-ms-date:" + date + "\n" +
		"x-ms-version:" + version + "\n"

	canonResource := fmt.Sprintf("/%s/%s/%s", s.cfg.AccountName, s.cfg.ContainerName, blobName)

	stringToSign := "DELETE\n" +
		"\n" + // Content-Encoding
		"\n" + // Content-Language
		"\n" + // Content-Length
		"\n" + // Content-MD5
		"\n" + // Content-Type
		"\n" + // Date
		"\n" + // If-Modified-Since
		"\n" + // If-Match
		"\n" + // If-None-Match
		"\n" + // If-Unmodified-Since
		"\n" + // Range
		canonHeaders +
		canonResource

	mac := hmac.New(sha256.New, keyBytes)
	mac.Write([]byte(stringToSign))
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, blobURL, nil)
	if err != nil {
		return fmt.Errorf("file: delete blob: build request: %w", err)
	}
	req.Header.Set("x-ms-date", date)
	req.Header.Set("x-ms-version", version)
	req.Header.Set("Authorization", fmt.Sprintf("SharedKey %s:%s", s.cfg.AccountName, sig))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("file: delete blob: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("file: delete blob: azure returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// blobListResponse is the XML envelope returned by the Azure List Blobs REST API.
type blobListResponse struct {
	XMLName xml.Name `xml:"EnumerationResults"`
	Blobs   struct {
		Blob []struct {
			Name       string `xml:"Name"`
			Properties struct {
				ContentType   string `xml:"Content-Type"`
				ContentLength int64  `xml:"Content-Length"`
			} `xml:"Properties"`
		} `xml:"Blob"`
	} `xml:"Blobs"`
}
