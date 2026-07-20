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

// Package aiagent is a thin trigger client to the AI Validation Agent
// (project-internal Choreo component). The backend calls it fire-and-forget
// after an evidence submission; a failure here never affects the submission.
package aiagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client posts validation triggers to the agent, authenticated with the
// agent's inbound bearer key.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// New constructs a client pointed at the agent base URL.
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// Scope is the evidence chain a validation runs against.
type Scope struct {
	AuditID    int `json:"auditId"`
	ControlID  int `json:"controlId"`
	EvidenceID int `json:"evidenceId"`
}

// TriggerRequest is the body of POST /api/v1/validations.
type TriggerRequest struct {
	Task        string `json:"task"`
	Scope       Scope  `json:"scope"`
	RequestedBy string `json:"requestedBy"`
}

// Trigger fires a validation job. It returns when the agent has accepted the
// job (202) — the job itself runs asynchronously inside the agent.
func (c *Client) Trigger(ctx context.Context, req TriggerRequest) error {
	b, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("aiagent: marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/validations", bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("aiagent: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("aiagent: trigger: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("aiagent: agent responded %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return nil
}
