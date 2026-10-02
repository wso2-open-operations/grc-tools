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

// Package llm is a thin wrapper over the Anthropic SDK. It holds
// ANTHROPIC_API_KEY and exposes exactly one operation: system prompt + user
// content blocks + one tool -> the tool's raw input. Callers (the
// aivalidation package) depend on the Caller interface, not *Client, so tests
// can inject a fake instead of calling the real API.
package llm

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// DefaultModel is the value every request sends as Model. There is no
// override: an AI gateway (ANTHROPIC_BASE_URL) manages model selection
// itself, and the public Anthropic API otherwise — this is just a fixed
// value the Anthropic SDK's request shape requires one of.
const DefaultModel = "claude-sonnet-5"

// defaultMaxTokens is used when a Request leaves MaxTokens unset.
const defaultMaxTokens = 2000

// Block is one piece of user-turn content: text, an image, or a PDF
// document. The concrete Anthropic SDK type stays unexported so this
// package's SDK dependency doesn't leak into aivalidation.
type Block struct {
	block  anthropic.ContentBlockParamUnion
	digest [sha256.Size]byte
}

// Digest identifies the block's content (kind, media type and raw bytes), so
// callers can tell whether two requests would send the model the same input.
func (b Block) Digest() [sha256.Size]byte { return b.digest }

func digestOf(kind, mediaType string, data []byte) [sha256.Size]byte {
	h := sha256.New()
	h.Write([]byte(kind))
	h.Write([]byte{0})
	h.Write([]byte(mediaType))
	h.Write([]byte{0})
	h.Write(data)
	return [sha256.Size]byte(h.Sum(nil))
}

// NewTextBlock wraps plain text (a CSV/TXT file's content, or an instruction).
func NewTextBlock(text string) Block {
	return Block{block: anthropic.NewTextBlock(text), digest: digestOf("text", "", []byte(text))}
}

// NewImageBlock wraps an image. mediaType is a full MIME type such as
// "image/png" — the caller is responsible for sniffing/validating it.
func NewImageBlock(mediaType string, data []byte) Block {
	return Block{
		block:  anthropic.NewImageBlockBase64(mediaType, base64.StdEncoding.EncodeToString(data)),
		digest: digestOf("image", mediaType, data),
	}
}

// NewPDFBlock wraps a PDF as a native document block — embedded screenshots
// on a page are visible to the model this way, no separate extraction step.
func NewPDFBlock(data []byte) Block {
	return Block{
		block: anthropic.NewDocumentBlock(anthropic.Base64PDFSourceParam{
			Data: base64.StdEncoding.EncodeToString(data),
		}),
		digest: digestOf("pdf", "application/pdf", data),
	}
}

// Tool describes the single tool the model is asked to call. Properties is
// the JSON Schema "properties" object for the tool's input.
type Tool struct {
	Name        string
	Description string
	Properties  map[string]any
	Required    []string
}

// Request is one validation call. The system prompt is split so the large,
// call-invariant portion (SystemStatic — e.g. the Standing Evidence Rules) is
// cache_control-tagged and stays first; the small per-call portion
// (SystemDynamic — e.g. one control's evidence_requirement) is appended
// uncached, after the cache breakpoint, so it never invalidates the cache.
type Request struct {
	SystemStatic  string
	SystemDynamic string
	Content       []Block
	Tool          Tool
	// MaxTokens defaults to defaultMaxTokens when zero.
	MaxTokens int64
}

// Usage carries token accounting for the caller's own logging, including
// prompt-cache hit/miss accounting for SystemStatic (see Request).
type Usage struct {
	InputTokens              int64
	OutputTokens             int64
	CacheReadInputTokens     int64
	CacheCreationInputTokens int64
}

// Result is the tool call's input, still raw — the caller (whose tool
// schema this is) unmarshals it into its own typed struct.
type Result struct {
	ToolInput json.RawMessage
	Usage     Usage
}

// ErrNoToolCall means the response held no matching tool_use block. The model
// is only asked (not forced) to call the tool, so a text-only reply is
// possible; a refusal or a max_tokens cutoff before the tool call completes
// can also produce one.
var ErrNoToolCall = errors.New("llm: model did not return the tool call")

// Caller is the interface aivalidation depends on, so its tests can inject a
// fake instead of calling the real Anthropic API.
type Caller interface {
	Call(ctx context.Context, req Request) (Result, error)
}

// Client is the real Caller, backed by the Anthropic API.
type Client struct {
	api     anthropic.Client
	timeout time.Duration
}

var _ Caller = (*Client)(nil)

// New constructs a Client, which always sends DefaultModel as Model (see its
// doc comment). baseURL, when non-empty, replaces the Anthropic API root (an
// AI gateway). timeout bounds every call (the caller passes
// aivalidation.JobTimeout).
func New(apiKey, baseURL string, timeout time.Duration) *Client {
	opts := []option.RequestOption{option.WithAPIKey(apiKey)}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	return &Client{
		api:     anthropic.NewClient(opts...),
		timeout: timeout,
	}
}

// Call issues one request: system prompt (cached static + uncached dynamic
// portions), the user-turn content blocks, and a call to req.Tool.
// Extended thinking runs at low effort — enough for the model to reason
// about cross-checking screenshot values against file content without
// spending heavily on every submission.
func (c *Client) Call(ctx context.Context, req Request) (Result, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	content := make([]anthropic.ContentBlockParamUnion, len(req.Content))
	for i, b := range req.Content {
		content[i] = b.block
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(DefaultModel),
		MaxTokens: maxTokens,
		System: []anthropic.TextBlockParam{
			{Text: req.SystemStatic, CacheControl: anthropic.NewCacheControlEphemeralParam()},
			{Text: req.SystemDynamic},
		},
		Messages: []anthropic.MessageParam{anthropic.NewUserMessage(content...)},
		Tools: []anthropic.ToolUnionParam{{OfTool: &anthropic.ToolParam{
			Name:        req.Tool.Name,
			Description: anthropic.String(req.Tool.Description),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: req.Tool.Properties,
				Required:   req.Tool.Required,
			},
		}}},
		// Thinking can't be combined with a forced tool_choice (only auto/none),
		// so the tool is the sole one offered and a miss falls to ErrNoToolCall.
		ToolChoice:   anthropic.ToolChoiceUnionParam{OfAuto: &anthropic.ToolChoiceAutoParam{}},
		Thinking:     anthropic.ThinkingConfigParamUnion{OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{}},
		OutputConfig: anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffortLow},
	}

	resp, err := c.api.Messages.New(ctx, params)
	if err != nil {
		return Result{}, fmt.Errorf("llm: call failed: %w", err)
	}
	if resp.StopReason == anthropic.StopReasonRefusal {
		return Result{}, fmt.Errorf("%w: refused (%s)", ErrNoToolCall, resp.StopDetails.Category)
	}

	usage := Usage{
		InputTokens:              resp.Usage.InputTokens,
		OutputTokens:             resp.Usage.OutputTokens,
		CacheReadInputTokens:     resp.Usage.CacheReadInputTokens,
		CacheCreationInputTokens: resp.Usage.CacheCreationInputTokens,
	}
	for _, block := range resp.Content {
		if tu, ok := block.AsAny().(anthropic.ToolUseBlock); ok && tu.Name == req.Tool.Name {
			return Result{ToolInput: tu.Input, Usage: usage}, nil
		}
	}
	return Result{}, ErrNoToolCall
}
