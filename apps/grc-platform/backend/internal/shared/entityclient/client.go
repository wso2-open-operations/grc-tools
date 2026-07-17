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

// Package entityclient is a small typed HTTP client to the Compliance Entity.
// Audit repositories use it to fetch/store data through the entity instead of
// querying MySQL directly. Non-2xx responses are mapped to apierror.Error so the
// service/handler layers behave exactly as they did with direct DB access.
package entityclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/apierror"
)

// Client is an HTTP client to the Compliance Entity base URL.
type Client struct {
	baseURL string
	http    *http.Client
}

// New creates a client pointed at the Compliance Entity (e.g. http://entity:8080).
func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// entityError mirrors the entity's error body: {"code":..,"message":".."}.
type entityError struct {
	Message string `json:"message"`
}

// Get performs GET path and decodes a 2xx JSON body into out (out may be nil).
func (c *Client) Get(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodGet, path, nil, out)
}

// Post performs POST path with a JSON body, decoding the 2xx response into out.
func (c *Client) Post(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPost, path, body, out)
}

// Patch performs PATCH path with a JSON body, decoding the 2xx response into out.
func (c *Client) Patch(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPatch, path, body, out)
}

// Delete performs DELETE path (no response body expected).
func (c *Client) Delete(ctx context.Context, path string) error {
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("entityclient: marshal body: %w", err)
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("entityclient: build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		slog.ErrorContext(ctx, "entity request failed", "method", method, "path", path, "err", err)
		return &apierror.Error{StatusCode: http.StatusServiceUnavailable, Body: "data service unavailable"}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := "data service error"
		var e entityError
		if raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10)); len(raw) > 0 {
			if json.Unmarshal(raw, &e) == nil && e.Message != "" {
				msg = e.Message
			}
		}
		return &apierror.Error{StatusCode: resp.StatusCode, Body: msg}
	}

	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("entityclient: decode response: %w", err)
	}
	return nil
}
