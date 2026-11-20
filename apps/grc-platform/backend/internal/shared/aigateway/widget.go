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

package aigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// The dashboard agent asks the model to turn a sentence into a widget
// definition. The model is only ever given the catalog of what a widget may use
// and what the user typed: it is never shown a row of data, and it cannot reach
// a database. Whatever it returns is a suggestion that the caller validates
// against the same catalog before anything runs.

// WidgetCatalogField describes one field the model may use.
type WidgetCatalogField struct {
	ID       string
	Label    string
	Kind     string // text, int, date, flag
	Allowed  []string
	Min, Max int
	Group    bool // may be used as a dimension; otherwise filter only
}

// WidgetCatalogMeasure describes one aggregate the model may use.
type WidgetCatalogMeasure struct {
	ID    string
	Label string
}

// WidgetCatalogColumn describes one column of a list dataset.
type WidgetCatalogColumn struct {
	ID    string
	Label string
}

// WidgetCatalogDataset is one thing the model may build a widget over. A list
// dataset (Rows) shows individual risks through Columns, up to MaxRows, and has
// no measures; its Fields are what it can be filtered on.
type WidgetCatalogDataset struct {
	ID       string
	Label    string
	Rows     bool
	MaxRows  int
	Columns  []WidgetCatalogColumn
	Fields   []WidgetCatalogField
	Measures []WidgetCatalogMeasure
}

// KnownView is a ready-made view the user may ask for by name. It is not shown
// to them up front; the model is told about it so that asking for "the risk heat
// map" produces exactly that.
type KnownView struct {
	Title       string
	Description string
	Widget      string // the widget as JSON
}

// KnowledgeSection is one passage of written help about the Risk Hub, given to
// the model so it can answer how-to questions. It is documentation, never data.
type KnowledgeSection struct {
	Source string
	Title  string
	Text   string
}

// ChatTurn is one earlier message in the popup conversation.
type ChatTurn struct {
	Role string // user or assistant
	Text string
}

// InterpretWidgetRequest is everything the model sees.
type InterpretWidgetRequest struct {
	// Module is the hub the assistant is for: "" is the Risk Hub, WidgetModuleAudit
	// the Audit Hub. It picks the instructions the model is given.
	Module  string
	Message string
	History []ChatTurn
	// CurrentWidget is the widget being refined, as JSON, or empty.
	CurrentWidget string
	Catalog       []WidgetCatalogDataset
	KnownViews    []KnownView
	// Knowledge is the help text relevant to the question.
	Knowledge []KnowledgeSection
	// Today is YYYY-MM-DD, so "this quarter" and "overdue" have a meaning.
	Today string
	// RetryFeedback is set on the one retry, after the first answer failed
	// validation, and says why.
	RetryFeedback string
}

// WidgetModuleAudit selects the Audit Hub's assistant instructions.
const WidgetModuleAudit = "audit"

// Interpretation outcomes.
const (
	WidgetActionBuild       = "widget"
	WidgetActionReply       = "reply"
	WidgetActionClarify     = "clarify"
	WidgetActionUnsupported = "unsupported"
	// WidgetActionOpen asks to open one control (the Audit Hub assistant only).
	WidgetActionOpen = "open_control"
	// WidgetActionExplainControl and WidgetActionCompareControls ask for one
	// control to be described, or two put side by side (Audit Hub only).
	WidgetActionExplainControl  = "explain_control"
	WidgetActionCompareControls = "compare_controls"
)

// InterpretWidgetResult is the model's answer, shape-checked but not yet
// validated against the catalog. Spec is raw JSON for the caller to decode.
type InterpretWidgetResult struct {
	// Control and Audit name the control to open, for WidgetActionOpen: the
	// control number, and the audit's name when the user gave one. They are only
	// ever used as bound filter values.
	Control string
	Audit   string
	// Control2 and Audit2 are the second control, for WidgetActionCompareControls;
	// an empty Audit2 means the first control's audit.
	Control2 string
	Audit2   string
	Action   string
	Question string
	Message  string
	Title    string
	Visual   string
	Note     string
	Spec     json.RawMessage
}

type interpretWidgetOutput struct {
	Control  string          `json:"control"`
	Audit    string          `json:"audit"`
	Control2 string          `json:"control2"`
	Audit2   string          `json:"audit2"`
	Action   string          `json:"action"`
	Question string          `json:"question"`
	Message  string          `json:"message"`
	Title    string          `json:"title"`
	Visual   string          `json:"visual"`
	Note     string          `json:"note"`
	Spec     json.RawMessage `json:"spec"`
}

// Bounds on the free text the model returns, which is shown to the user as
// plain text.
const (
	maxWidgetTitleLen = 80
	maxWidgetTextLen  = 300
	// A reply may be a list of steps, so it gets more room than a one-line note.
	maxReplyLen = 1500
	// A control number or an audit's name, as the user wrote it.
	maxControlRefLen = 60
	maxAuditRefLen   = 200
)

// InterpretWidget asks the model for a widget definition, a clarifying
// question, or a statement that the request cannot be built.
func (c *Client) InterpretWidget(ctx context.Context, req InterpretWidgetRequest) (*InterpretWidgetResult, error) {
	prompt := buildWidgetPrompt(req)
	if req.Module == WidgetModuleAudit {
		prompt = buildAuditWidgetPrompt(req)
	}
	text, err := c.complete(ctx, prompt, nil)
	if err != nil {
		return nil, err
	}
	out, err := parseJSONObject[interpretWidgetOutput](text)
	if err != nil {
		return nil, fmt.Errorf("interpret widget: %w", err)
	}

	messageLen := maxWidgetTextLen
	if out.Action == WidgetActionReply {
		messageLen = maxReplyLen
	}
	res := &InterpretWidgetResult{
		Action:   out.Action,
		Question: clip(out.Question, maxWidgetTextLen),
		Message:  clip(out.Message, messageLen),
		Title:    clip(out.Title, maxWidgetTitleLen),
		Visual:   strings.TrimSpace(out.Visual),
		Note:     clip(out.Note, maxWidgetTextLen),
		Spec:     out.Spec,
		Control:  clip(out.Control, maxControlRefLen),
		Audit:    clip(out.Audit, maxAuditRefLen),
		Control2: clip(out.Control2, maxControlRefLen),
		Audit2:   clip(out.Audit2, maxAuditRefLen),
	}
	switch res.Action {
	case WidgetActionBuild:
		if len(res.Spec) == 0 || string(res.Spec) == "null" {
			return nil, fmt.Errorf("interpret widget: widget answer has no spec")
		}
	case WidgetActionReply:
		if res.Message == "" {
			return nil, fmt.Errorf("interpret widget: reply answer has no message")
		}
	case WidgetActionClarify:
		if res.Question == "" {
			return nil, fmt.Errorf("interpret widget: clarify answer has no question")
		}
	case WidgetActionOpen, WidgetActionExplainControl, WidgetActionCompareControls:
		if res.Control == "" || req.Module != WidgetModuleAudit {
			return nil, fmt.Errorf("interpret widget: %s needs a control, and is only for the Audit Hub", res.Action)
		}
		if res.Action == WidgetActionCompareControls && res.Control2 == "" {
			return nil, fmt.Errorf("interpret widget: compare_controls needs two controls")
		}
	case WidgetActionUnsupported:
		if res.Message == "" {
			return nil, fmt.Errorf("interpret widget: unsupported answer has no message")
		}
	default:
		return nil, fmt.Errorf("interpret widget: unknown action %q", out.Action)
	}
	return res, nil
}

func clip(s string, max int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max])
}

// untrustedTags are the wrapper tags below. Any closing tag in text from the
// user is removed, matched without regard to case or spacing, so that text
// cannot close its own block and have the rest read as instructions.
var untrustedTags = regexp.MustCompile(`(?i)<\s*/\s*(user_request|conversation|current_widget)\s*>`)

// untrusted removes closing tags until none is left. A single pass is not
// enough: removing the inner tag of "</conv</conversation>ersation>" joins the
// pieces around it into a new "</conversation>".
func untrusted(s string) string {
	for {
		out := untrustedTags.ReplaceAllString(s, "")
		if out == s {
			return s
		}
		s = out
	}
}

// writeWidgetCatalog describes the datasets, fields and measures the model may
// use. It is the same for every hub; only the datasets in req differ.
func writeWidgetCatalog(b *strings.Builder, req InterpretWidgetRequest) {
	item := "risk"
	if req.Module == WidgetModuleAudit {
		item = "control"
	}
	b.WriteString("CATALOG\n")
	for _, ds := range req.Catalog {
		b.WriteString("Dataset \"" + ds.ID + "\" - " + ds.Label + "\n")
		if ds.Rows {
			fmt.Fprintf(b, "  This is a LIST: one row per %s, at most %d rows. Use \"columns\" (not measures or dimensions) and the table visual.\n  Columns:\n", item, ds.MaxRows)
			for _, c := range ds.Columns {
				b.WriteString("    " + c.ID + " - " + c.Label + "\n")
			}
			b.WriteString("  Fields (filter only):\n")
		} else {
			b.WriteString("  Fields:\n")
		}
		for _, f := range ds.Fields {
			b.WriteString("    " + f.ID + " (" + f.Kind)
			if len(f.Allowed) > 0 {
				b.WriteString("; values: " + strings.Join(f.Allowed, ", "))
			}
			if f.Kind == "person" {
				b.WriteString("; a person's name as the user wrote it, or \"me\" for the user themselves")
			}
			if f.Max > 0 && f.Max < 1<<20 {
				fmt.Fprintf(b, "; %d to %d", f.Min, f.Max)
			}
			if !f.Group {
				b.WriteString("; filter only")
			}
			b.WriteString(") - " + f.Label + "\n")
		}
		if !ds.Rows {
			b.WriteString("  Measures:\n")
			for _, m := range ds.Measures {
				b.WriteString("    " + m.ID + " - " + m.Label + "\n")
			}
		}
	}
}

// writeWidgetTail adds the untrusted conversation, the current widget, the
// request itself and the answer format. listID is the list dataset the format
// example names.
func writeWidgetTail(b *strings.Builder, req InterpretWidgetRequest, listID string) {
	b.WriteString(promptText("widget_untrusted_notice") + "\n\n")

	if len(req.History) > 0 {
		b.WriteString("<conversation>\n")
		for _, t := range req.History {
			role := "User"
			if t.Role == "assistant" {
				role = "Assistant"
			}
			b.WriteString(role + ": " + untrusted(t.Text) + "\n")
		}
		b.WriteString("</conversation>\n")
	}
	if strings.TrimSpace(req.CurrentWidget) != "" {
		b.WriteString("<current_widget>\n" + untrusted(req.CurrentWidget) + "\n</current_widget>\n")
	}
	b.WriteString("<user_request>\n" + untrusted(req.Message) + "\n</user_request>\n")

	if req.RetryFeedback != "" {
		b.WriteString("\nYour previous answer could not be used: " + untrusted(req.RetryFeedback) + " Answer again with a corrected widget, or ask a clarifying question.\n")
	}

	b.WriteString(promptText("widget_answer_formats_head") + "\n")
	b.WriteString(`For a list the spec is: {"dataset": "` + listID + `", "columns": ["..."], "filters": [...], "sort": {"by": "<a selected column>", "desc": true}, "limit": 50}` + "\n")
	b.WriteString(promptText("widget_answer_formats_tail"))
}

func buildWidgetPrompt(req InterpretWidgetRequest) string {
	var b strings.Builder
	b.WriteString(promptText("risk_widget_intro") + "\n\n")
	b.WriteString("Today's date is " + req.Today + ".\n\n")

	writeWidgetCatalog(&b, req)

	if len(req.KnownViews) > 0 {
		b.WriteString("\nVIEWS THE USER MAY ASK FOR BY NAME\n")
		b.WriteString("Do not offer or show these unprompted. When the user asks for one, return it as a widget exactly as given, changing only what they ask to change.\n")
		for _, v := range req.KnownViews {
			b.WriteString("- " + v.Title + ": " + v.Description + "\n  " + v.Widget + "\n")
		}
	}

	if len(req.Knowledge) > 0 {
		b.WriteString("\nKNOWLEDGE ABOUT THE RISK HUB\n")
		b.WriteString("This is documentation, and the only source you may use to answer questions about how to do something, where something is, or what a term means.\n")
		for _, k := range req.Knowledge {
			b.WriteString("--- " + k.Source + ": " + k.Title + "\n" + k.Text + "\n")
		}
	}

	b.WriteString(promptText("risk_widget_rules") + "\n\n")

	writeWidgetTail(&b, req, "risk_list")
	return b.String()
}
