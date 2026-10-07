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

// Package aigateway is a thin, synchronous client to Claude via the WSO2 AI
// Gateway — a standard Anthropic-compatible endpoint (x-api-key auth,
// /v1/messages), confirmed by direct smoke test against the real gateway
// (2026-10-01/02). Unlike internal/shared/aiagent (the disabled AI Validation
// Agent, a fire-and-forget trigger to an external async job), every call here
// is a plain request/response — the caller is waiting on the form, not a
// background sweep.
package aigateway

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

// model is fixed — the gateway subscription is provisioned for exactly this
// one model, confirmed working by smoke test. There is nothing to choose.
const model = "claude-haiku-4-5-20251001"

const anthropicVersion = "2023-06-01"

// Client calls the AI Gateway's Messages API.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// New constructs a client pointed at the gateway base URL
// (e.g. https://ai-gateway.dp.aef.wso2.com/security-and-compliance/grc-platform-staging).
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		// LLM calls run longer than a typical internal API call; generous but
		// bounded so a stuck request can't hang the handler indefinitely.
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

// CategoryOption is one live entry from risk_category, passed to the model so
// it always classifies against whatever categories actually exist today.
type CategoryOption struct {
	ID          int
	Name        string
	Description string
}

// SuggestCategoryRequest is the risk text + context the model classifies.
type SuggestCategoryRequest struct {
	Title                    string
	Description              string
	ComplianceReferenceNames []string
	Categories               []CategoryOption
}

// SuggestCategoryResult is the model's suggestion, confidence self-reported
// (not a calibrated probability — Claude has no native confidence output).
type SuggestCategoryResult struct {
	CategoryID int
	Reason     string
	Confidence string // "high" | "medium" | "low"
}

// suggestCategoryOutput is the strict JSON shape the prompt asks the model to
// return, parsed out of the response's text content.
type suggestCategoryOutput struct {
	CategoryID int    `json:"category_id"`
	Reason     string `json:"reason"`
	Confidence string `json:"confidence"`
}

// SuggestCategory asks Claude to pick one category for a risk from the live
// category list, with a one-line reason and a self-reported confidence. Pure
// text reasoning — no tools, unlike SuggestLikelihood below.
func (c *Client) SuggestCategory(ctx context.Context, req SuggestCategoryRequest) (*SuggestCategoryResult, error) {
	prompt := buildCategoryPrompt(req)
	text, err := c.complete(ctx, prompt, nil)
	if err != nil {
		return nil, fmt.Errorf("aigateway: suggest category: %w", err)
	}
	out, err := parseJSONObject[suggestCategoryOutput](text)
	if err != nil {
		return nil, fmt.Errorf("aigateway: suggest category: parse response: %w", err)
	}
	confidence, err := normalizeConfidence(out.Confidence)
	if err != nil {
		return nil, fmt.Errorf("aigateway: suggest category: %w", err)
	}
	return &SuggestCategoryResult{
		CategoryID: out.CategoryID,
		Reason:     out.Reason,
		Confidence: confidence,
	}, nil
}

// SuggestLikelihoodRequest is the risk text + context the model reasons over
// to suggest a Gross or Residual Likelihood. Impact is deliberately never
// part of this — see docs/plans/auto-categorisation-plan.md, "Confirmed:
// Likelihood only, not Impact".
type SuggestLikelihoodRequest struct {
	Title                    string
	Description              string
	ImpactDescription        string
	ComplianceReferenceNames []string
	CategoryName             string
	SourceRegisterName       string
}

// SuggestLikelihoodResult is the model's suggestion.
type SuggestLikelihoodResult struct {
	Score      int // 1-3
	Reason     string
	Confidence string // "high" | "medium" | "low"
}

type suggestLikelihoodOutput struct {
	Score      int    `json:"score"`
	Reason     string `json:"reason"`
	Confidence string `json:"confidence"`
}

// webSearchTool and webFetchTool are Anthropic's server-side tools: executed
// by the gateway itself, results returned inline in the same response — no
// client-side tool loop needed. Confirmed working against the real gateway
// by direct smoke test (2026-10-02): a web_search call returned real, current
// CISA KEV news, and a web_fetch call pulled the live content of
// first.org/epss.
// max_uses capped low (not the earlier 5): the gateway's own upstream
// timeout cuts the whole call off at ~19s regardless of our client timeout
// (confirmed by direct reproduction against the real gateway, 2026-10-06) —
// a prompt that lets the model search+fetch across several sources
// exhaustively reliably blows that budget and 504s. Capping tool calls
// forces the model to be economical instead, trading thoroughness for
// actually returning an answer. Revisit once compliance's exact evidence
// URLs arrive (see buildLikelihoodPrompt) — direct fetches without a search
// step first should need fewer round trips than today's discovery-by-search.
var likelihoodTools = []map[string]any{
	{"type": "web_search_20250305", "name": "web_search"},
	{"type": "web_fetch_20250910", "name": "web_fetch", "max_uses": 2},
}

// SuggestLikelihood asks Claude to check live evidence sources (NVD, KEV,
// EPSS, and whatever else it can find or is told to check) and suggest a
// Likelihood score. Used both for the initial Gross suggestion at risk
// creation and the Residual re-check at reassessment — the prompt and model
// behaviour are identical either way; which score the caller does with the
// result is the caller's concern, not this client's.
func (c *Client) SuggestLikelihood(ctx context.Context, req SuggestLikelihoodRequest) (*SuggestLikelihoodResult, error) {
	prompt := buildLikelihoodPrompt(req)
	text, err := c.complete(ctx, prompt, likelihoodTools)
	if err != nil {
		return nil, fmt.Errorf("aigateway: suggest likelihood: %w", err)
	}
	out, err := parseJSONObject[suggestLikelihoodOutput](text)
	if err != nil {
		return nil, fmt.Errorf("aigateway: suggest likelihood: parse response: %w", err)
	}
	confidence, err := normalizeConfidence(out.Confidence)
	if err != nil {
		return nil, fmt.Errorf("aigateway: suggest likelihood: %w", err)
	}
	return &SuggestLikelihoodResult{
		Score:      out.Score,
		Reason:     out.Reason,
		Confidence: confidence,
	}, nil
}

// SuggestActionPlanRequest is the risk text + context the model drafts an
// action plan description from.
type SuggestActionPlanRequest struct {
	Title                    string
	Description              string
	CategoryName             string
	ComplianceReferenceNames []string
	TreatmentStrategy        string
}

// SuggestActionPlanResult is the model's drafted action plan, confidence
// self-reported (not a calibrated probability — Claude has no native
// confidence output).
type SuggestActionPlanResult struct {
	Description string
	Reason      string
	Confidence  string // "high" | "medium" | "low"
}

// suggestActionPlanOutput is the strict JSON shape the prompt asks the model
// to return, parsed out of the response's text content.
type suggestActionPlanOutput struct {
	Description string `json:"description"`
	Reason      string `json:"reason"`
	Confidence  string `json:"confidence"`
}

// SuggestActionPlan asks Claude to draft an action plan description for a
// risk from its own text — reasoning over the risk's title, description,
// category, compliance reference(s), and treatment strategy, not an evidence
// lookup. Pure text reasoning — no tools, same shape as SuggestCategory, not
// SuggestLikelihood's tool-enabled version.
func (c *Client) SuggestActionPlan(ctx context.Context, req SuggestActionPlanRequest) (*SuggestActionPlanResult, error) {
	prompt := buildActionPlanPrompt(req)
	text, err := c.complete(ctx, prompt, nil)
	if err != nil {
		return nil, fmt.Errorf("aigateway: suggest action plan: %w", err)
	}
	out, err := parseJSONObject[suggestActionPlanOutput](text)
	if err != nil {
		return nil, fmt.Errorf("aigateway: suggest action plan: parse response: %w", err)
	}
	confidence, err := normalizeConfidence(out.Confidence)
	if err != nil {
		return nil, fmt.Errorf("aigateway: suggest action plan: %w", err)
	}
	return &SuggestActionPlanResult{
		Description: out.Description,
		Reason:      out.Reason,
		Confidence:  confidence,
	}, nil
}

// normalizeConfidence lowercases and validates the model's self-reported
// confidence against the only three values risk_ai_suggestion.confidence
// (a strict MySQL ENUM) accepts. Every Suggest* method routes its result
// through this before returning it — without it, a model response that omits
// confidence or returns anything other than exactly "high"/"medium"/"low"
// (e.g. "n/a", "moderate") would reach RecordDecision unchecked and fail (or
// corrupt data in non-strict SQL mode) when written to that column.
func normalizeConfidence(raw string) (string, error) {
	c := strings.ToLower(strings.TrimSpace(raw))
	switch c {
	case "high", "medium", "low":
		return c, nil
	default:
		return "", fmt.Errorf("model returned invalid confidence %q", raw)
	}
}

func buildActionPlanPrompt(req SuggestActionPlanRequest) string {
	var b strings.Builder
	b.WriteString("You are drafting a remediation action plan description for a security/compliance risk.\n\n")
	b.WriteString("Risk Title: " + req.Title + "\n")
	b.WriteString("Risk Description: " + req.Description + "\n")
	if req.CategoryName != "" {
		b.WriteString("Risk Category: " + req.CategoryName + "\n")
	}
	if len(req.ComplianceReferenceNames) > 0 {
		b.WriteString("Security Compliance Reference(s): " + strings.Join(req.ComplianceReferenceNames, ", ") + "\n")
	}
	if req.TreatmentStrategy != "" {
		b.WriteString("Treatment Strategy: " + req.TreatmentStrategy + "\n")
	}
	b.WriteString("\nDraft a concise, high-level action plan description describing the overall remediation approach for this risk, consistent with its treatment strategy. ")
	b.WriteString("Write it as a short flowing paragraph (plain prose, 2-4 sentences), NOT a numbered or bulleted list of steps — individual action steps are captured separately elsewhere and are not part of this field. ")
	b.WriteString("Respond with ONLY a JSON object, no markdown fences, no extra text, in this exact shape:\n")
	b.WriteString(`{"description": "<the drafted action plan description>", "reason": "<one sentence explaining why this plan fits the risk>", "confidence": "high"|"medium"|"low"}`)
	return b.String()
}

func buildLikelihoodPrompt(req SuggestLikelihoodRequest) string {
	var b strings.Builder
	b.WriteString("You are scoring the Likelihood (1-3) of a security/compliance risk, using live web search and page fetching to check real evidence. ")
	b.WriteString("You have a strict time budget: make AT MOST one web_search call and one web_fetch call in total, not one pair per source. ")
	b.WriteString("Pick the single most likely source to confirm or rule out first (CISA KEV is usually fastest to check), and stop looking once you have enough to answer — do not try to exhaustively check NVD, KEV, and EPSS all in one go. ")
	b.WriteString("If your first lookup doesn't resolve it quickly, stop searching and estimate from the description and category instead, marking confidence \"low\" rather than spending more tool calls. ")
	b.WriteString("Do not guess without looking at all — always make at least one lookup attempt before answering.\n\n")
	b.WriteString("Risk Title: " + req.Title + "\n")
	b.WriteString("Risk Description: " + req.Description + "\n")
	b.WriteString("Impact Description: " + req.ImpactDescription + "\n")
	if req.CategoryName != "" {
		b.WriteString("Risk Category: " + req.CategoryName + "\n")
	}
	if len(req.ComplianceReferenceNames) > 0 {
		b.WriteString("Security Compliance Reference(s): " + strings.Join(req.ComplianceReferenceNames, ", ") + "\n")
	}
	if req.SourceRegisterName != "" {
		b.WriteString("Source Register (the WSO2 team/product this risk belongs to, for background context only — not itself evidence of likelihood): " + req.SourceRegisterName + "\n")
	}
	b.WriteString("\nScoring rules (first match wins):\n")
	b.WriteString("3 - High: listed in CISA KEV, or EPSS >= 0.5, or a weaponised/working public exploit exists and the asset is internet-facing.\n")
	b.WriteString("2 - Medium: a public proof-of-concept exists, or EPSS is 0.1-0.5, or there's a rising trend in exploitation interest in the last 14 days.\n")
	b.WriteString("1 - Low: no public exploit, EPSS below 0.1, and access requires authentication or an internal network position.\n")
	b.WriteString("If the risk describes a process, contractual, or physical gap with no CVE or technical vulnerability to look up, estimate from the description and category instead, and mark confidence \"low\".\n")
	b.WriteString("\nRespond with ONLY a JSON object, no markdown fences, no extra text, in this exact shape:\n")
	b.WriteString(`{"score": 1|2|3, "reason": "<one or two sentences citing the specific evidence found, e.g. 'CVE-2024-1234 is listed in CISA KEV' or 'EPSS score 0.72'>", "confidence": "high"|"medium"|"low"}`)
	return b.String()
}

func buildCategoryPrompt(req SuggestCategoryRequest) string {
	var b strings.Builder
	b.WriteString("You are classifying a security/compliance risk into exactly one category.\n\n")
	b.WriteString("Risk Title: " + req.Title + "\n")
	b.WriteString("Risk Description: " + req.Description + "\n")
	if len(req.ComplianceReferenceNames) > 0 {
		b.WriteString("Security Compliance Reference(s): " + strings.Join(req.ComplianceReferenceNames, ", ") + "\n")
	}
	b.WriteString("\nAvailable categories:\n")
	for _, c := range req.Categories {
		desc := c.Description
		if desc == "" {
			desc = "(no description provided)"
		}
		b.WriteString(fmt.Sprintf("- id=%d name=%q: %s\n", c.ID, c.Name, desc))
	}
	b.WriteString("\nPick exactly one category id from the list above that best fits this risk. ")
	b.WriteString("Respond with ONLY a JSON object, no markdown fences, no extra text, in this exact shape:\n")
	b.WriteString(`{"category_id": <int>, "reason": "<one sentence, citing what in the risk text matched this category>", "confidence": "high"|"medium"|"low"}`)
	return b.String()
}

// complete sends a single-turn message and returns the model's final text
// content. tools is nil for plain text reasoning (Category); for Likelihood
// it carries web_search/web_fetch, which Anthropic executes server-side —
// results come back inline in this same response, no client-side tool loop
// needed (confirmed by direct smoke test against the real gateway).
func (c *Client) complete(ctx context.Context, prompt string, tools []map[string]any) (string, error) {
	// max_tokens is higher when tools are enabled: a tool-use response
	// interleaves search/fetch result blocks with the model's own text, which
	// costs far more of the budget than a plain single-paragraph answer.
	maxTokens := 512
	if len(tools) > 0 {
		maxTokens = 4096
	}
	body := map[string]any{
		"model":      model,
		"max_tokens": maxTokens,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	if len(tools) > 0 {
		body["tools"] = tools
	}
	b, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/messages", bytes.NewReader(b))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("call gateway: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gateway responded %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var msg struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBody, &msg); err != nil {
		return "", fmt.Errorf("decode message: %w", err)
	}
	// The LAST text block, not the first: with tools enabled the response
	// interleaves server_tool_use/web_search_tool_result/web_fetch_tool_result
	// blocks with the model's own text, and the model may think out loud in
	// an earlier text block before giving its real final answer (asked to be
	// ONLY JSON) in the last one — confirmed by direct smoke test against the
	// real gateway. Category has no tools and always produces exactly one
	// text block, so this is a no-op change for it.
	text := ""
	for _, block := range msg.Content {
		if block.Type == "text" && block.Text != "" {
			text = block.Text
		}
	}
	if text == "" {
		return "", fmt.Errorf("no text content in response")
	}
	return text, nil
}

// parseJSONObject extracts a JSON object from model output that is usually
// clean JSON but, per the prompt's own instruction not to, might still arrive
// wrapped in a markdown code fence — stripped here rather than relied on the
// model to never do it. Falls back to the last balanced {...} substring in
// the text if the whole trimmed string still isn't valid JSON on its own —
// SuggestLikelihood's tool-use responses are more prone to the model adding
// a stray lead-in sentence than Category's plain-text ones ever were.
func parseJSONObject[T any](text string) (T, error) {
	var out T
	trimmed := strings.TrimSpace(text)
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)

	if err := json.Unmarshal([]byte(trimmed), &out); err == nil {
		return out, nil
	}

	if candidate, ok := lastJSONObject(trimmed); ok {
		if err := json.Unmarshal([]byte(candidate), &out); err == nil {
			return out, nil
		}
	}
	return out, fmt.Errorf("unmarshal %q: no valid JSON object found", trimmed)
}

// lastJSONObject returns the last top-level {...} substring in text, using
// brace depth to find its matching close — not just the first '{' to the
// last '}', which would wrongly span multiple separate objects or text
// containing stray braces outside the real one.
func lastJSONObject(text string) (string, bool) {
	end := strings.LastIndexByte(text, '}')
	if end == -1 {
		return "", false
	}
	depth := 0
	for i := end; i >= 0; i-- {
		switch text[i] {
		case '}':
			depth++
		case '{':
			depth--
			if depth == 0 {
				return text[i : end+1], true
			}
		}
	}
	return "", false
}
