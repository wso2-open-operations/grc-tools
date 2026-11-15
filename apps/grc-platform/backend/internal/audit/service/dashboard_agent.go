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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/knowledge"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/dashboardagent"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/aigateway"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
)

// The Audit Hub assistant. It is the Risk Hub assistant's twin over audits,
// controls and evidence: the model only turns a request into a validated widget
// spec and never sees data; the Compliance Entity runs the spec limited to the
// controls the caller may see; sentences about the numbers are written here from
// fixed templates.

// AssistantWidgetQueryRepository runs one validated spec for one caller.
type AssistantWidgetQueryRepository interface {
	Run(ctx context.Context, spec dashboardagent.Spec, scope dashboardagent.AuditScope) (*dashboardagent.Result, error)
}

// AssistantInterpreter asks the model for a widget. It is satisfied by
// *aigateway.Client.
type AssistantInterpreter interface {
	InterpretWidget(ctx context.Context, req aigateway.InterpretWidgetRequest) (*aigateway.InterpretWidgetResult, error)
}

// Interpretation outcomes shown to the user.
const (
	AssistantReply       = aigateway.WidgetActionReply
	AssistantWidget      = aigateway.WidgetActionBuild
	AssistantClarify     = aigateway.WidgetActionClarify
	AssistantUnsupported = aigateway.WidgetActionUnsupported
	// AssistantOpenControl is internal: the handler turns it into a reply that
	// takes the user to the control. It is never sent to the web app as it is.
	AssistantOpenControl = aigateway.WidgetActionOpen
	// AssistantExplainControl and AssistantCompareControls are internal in the
	// same way: the handler answers them from the controls' own records.
	AssistantExplainControl  = aigateway.WidgetActionExplainControl
	AssistantCompareControls = aigateway.WidgetActionCompareControls
)

const (
	maxAssistantMessageLen = 500
	maxAssistantHistory    = 6
	// maxAssistantKnowledge is how many help sections, beyond the always-included
	// map of the Audit Hub, go to the model with a question.
	maxAssistantKnowledge = 3
)

// ErrAssistantUnavailable means the AI service could not be reached or failed.
// The web app shows a calm "try again" message; the cause stays in the log.
var ErrAssistantUnavailable = errors.New("the assistant is temporarily unavailable")

// ChatTurn is one earlier message in the popup conversation.
type ChatTurn struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

// AssistantRequest is what the popup sends to be interpreted.
type AssistantRequest struct {
	Message string                 `json:"message"`
	History []ChatTurn             `json:"history"`
	Current *dashboardagent.Widget `json:"current"`
}

// AssistantResult is what the popup shows. The shape matches the Risk Hub
// assistant's, so the web app draws both the same way.
type AssistantResult struct {
	Action   string                 `json:"action"`
	Question string                 `json:"question,omitempty"`
	Message  string                 `json:"message,omitempty"`
	Note     string                 `json:"note,omitempty"`
	Widget   *dashboardagent.Widget `json:"widget,omitempty"`
	// ControlNumber and AuditName say which control to open, for the handler
	// only. They are not part of the response.
	ControlNumber  string `json:"-"`
	AuditName      string `json:"-"`
	ControlNumber2 string `json:"-"`
	AuditName2     string `json:"-"`
	// ControlLink, Timeline and Compare are filled in by the handler when it
	// answers from a control's record. Their shape matches the Risk Hub's.
	ControlLink *ControlLink  `json:"control_link,omitempty"`
	Timeline    *Timeline     `json:"timeline,omitempty"`
	Compare     *CompareTable `json:"compare,omitempty"`
	// Navigate, set by the handler, is where the web app should take the user.
	Navigate *ControlTarget `json:"navigate,omitempty"`
}

// ControlLink points at one control so the web app can offer to open it.
type ControlLink struct {
	AuditID   int    `json:"audit_id"`
	ControlID int    `json:"control_id"`
	Code      string `json:"code"`
}

// TimelineEvent, Timeline, CompareRow and CompareTable have the same shape as
// the Risk Hub's, so the web app draws them the same way. A compare heading is a
// control: ID is the control's id and AuditID the audit it is in.
type TimelineEvent struct {
	Date string `json:"date"`
	Text string `json:"text"`
}

type Timeline struct {
	Note   string          `json:"note,omitempty"`
	Events []TimelineEvent `json:"events"`
}

type CompareRow struct {
	Label   string   `json:"label"`
	Values  []string `json:"values"`
	Differs bool     `json:"differs"`
}

type CompareHead struct {
	ID      int    `json:"id"`
	Code    string `json:"code"`
	AuditID int    `json:"audit_id"`
}

type CompareTable struct {
	Risks []CompareHead `json:"risks"`
	Rows  []CompareRow  `json:"rows"`
}

// ControlTarget is one control the user asked to be taken to: the ids the Audit
// Hub's own address uses. The Audit Hub decides what the user may open.
type ControlTarget struct {
	AuditID   int `json:"audit_id"`
	ControlID int `json:"control_id"`
}

// DashboardAgentService is the Audit Hub assistant.
type DashboardAgentService interface {
	// Run runs one validated spec, limited to the controls scope allows. A scope
	// that sees nothing never reaches the database.
	Run(ctx context.Context, spec dashboardagent.Spec, scope dashboardagent.AuditScope) (*dashboardagent.Result, error)
	// Interpret asks the model for a widget, a clarifying question, or a "can't
	// build that". It sees only the catalog the caller may use and what they
	// typed, never data, and any widget it returns passed validation.
	Interpret(ctx context.Context, req AssistantRequest, canUse func(priv string) bool) (*AssistantResult, error)
}

type dashboardAgentService struct {
	repo        AssistantWidgetQueryRepository
	interpreter AssistantInterpreter
	people      dashboardagent.PeopleDirectory
	now         func() time.Time
}

// NewDashboardAgentService constructs the Audit Hub assistant.
func NewDashboardAgentService(repo AssistantWidgetQueryRepository, interpreter AssistantInterpreter, people dashboardagent.PeopleDirectory) DashboardAgentService {
	return &dashboardAgentService{repo: repo, interpreter: interpreter, people: people, now: time.Now}
}

func (s *dashboardAgentService) Run(ctx context.Context, spec dashboardagent.Spec, scope dashboardagent.AuditScope) (*dashboardagent.Result, error) {
	if scope.None() {
		return &dashboardagent.Result{Columns: []dashboardagent.Column{}, Rows: [][]any{}}, nil
	}
	callerUUID := ""
	if u := auth.FromContext(ctx); u != nil {
		callerUUID = u.Subject
	}
	// Names in person filters become identities here; the query only ever sees
	// the identities. A name that cannot be settled is a message for the user.
	resolved, problem := dashboardagent.ResolvePeople(spec, callerUUID, s.people)
	if problem != "" {
		return nil, &dashboardagent.ValidationError{Msg: problem}
	}
	res, err := s.repo.Run(ctx, dashboardagent.WithControlLinkColumns(resolved), scope)
	if err != nil {
		return nil, err
	}
	dashboardagent.LabelColumns(spec, res)
	return res, nil
}

// couldNotBuild is what the user sees when the model twice failed to produce a
// widget that passes validation. The reason stays in the log.
const couldNotBuild = "I couldn't build a valid widget for that. Try rephrasing it."

func (s *dashboardAgentService) Interpret(ctx context.Context, req AssistantRequest, canUse func(priv string) bool) (*AssistantResult, error) {
	message := strings.TrimSpace(req.Message)
	if message == "" || utf8.RuneCountInString(message) > maxAssistantMessageLen {
		return nil, &dashboardagent.ValidationError{Msg: fmt.Sprintf("write a request of up to %d characters", maxAssistantMessageLen)}
	}
	if len(req.History) > maxAssistantHistory {
		return nil, &dashboardagent.ValidationError{Msg: "the conversation is too long, start a new one"}
	}
	history := make([]aigateway.ChatTurn, 0, len(req.History))
	for _, t := range req.History {
		text := strings.TrimSpace(t.Text)
		if (t.Role != "user" && t.Role != "assistant") || text == "" || utf8.RuneCountInString(text) > maxAssistantMessageLen {
			return nil, &dashboardagent.ValidationError{Msg: "the conversation is not valid"}
		}
		history = append(history, aigateway.ChatTurn{Role: t.Role, Text: text})
	}

	// The widget being refined came from the client, so it is validated like any
	// other before the model is shown it.
	current := ""
	if req.Current != nil {
		w, err := dashboardagent.ValidateWidgetIn(dashboardagent.ModuleAudit, *req.Current, canUse)
		if err != nil {
			return nil, err
		}
		b, err := json.Marshal(w)
		if err != nil {
			return nil, fmt.Errorf("interpret: marshal current widget: %w", err)
		}
		current = string(b)
	}

	callerUUID := ""
	if u := auth.FromContext(ctx); u != nil {
		callerUUID = u.Subject
	}
	gwReq := aigateway.InterpretWidgetRequest{
		Module:        aigateway.WidgetModuleAudit,
		Message:       message,
		History:       history,
		CurrentWidget: current,
		Knowledge:     toAssistantKnowledge(knowledge.Relevant(message, maxAssistantKnowledge)),
		Catalog:       toAssistantCatalog(dashboardagent.CatalogForModule(dashboardagent.ModuleAudit, canUse)),
		Today:         s.now().Format("2006-01-02"),
	}

	// One retry, telling the model why its first widget was refused.
	for attempt := 0; attempt < 2; attempt++ {
		res, err := s.interpreter.InterpretWidget(ctx, gwReq)
		if err != nil {
			slog.Error("audit assistant: the AI service failed", "err", err)
			return nil, ErrAssistantUnavailable
		}
		switch res.Action {
		case aigateway.WidgetActionReply:
			return &AssistantResult{Action: AssistantReply, Message: res.Message}, nil
		case aigateway.WidgetActionClarify:
			return &AssistantResult{Action: AssistantClarify, Question: res.Question}, nil
		case aigateway.WidgetActionUnsupported:
			return &AssistantResult{Action: AssistantUnsupported, Message: res.Message}, nil
		case aigateway.WidgetActionOpen, aigateway.WidgetActionExplainControl, aigateway.WidgetActionCompareControls:
			return &AssistantResult{
				Action: res.Action, ControlNumber: res.Control, AuditName: res.Audit,
				ControlNumber2: res.Control2, AuditName2: res.Audit2,
			}, nil
		}

		w, verr := decodeAssistantWidget(res, canUse)
		if verr == nil {
			// Settle the people it names now, so a name that matches nobody (or
			// several people) is answered here instead of drawing an empty chart.
			if _, problem := dashboardagent.ResolvePeople(w.Spec, callerUUID, s.people); problem != "" {
				return &AssistantResult{Action: AssistantReply, Message: problem}, nil
			}
			return &AssistantResult{Action: AssistantWidget, Widget: &w, Note: res.Note}, nil
		}
		slog.Warn("audit assistant: the model's widget was refused", "attempt", attempt+1, "reason", verr.Error())
		gwReq.RetryFeedback = verr.Error()
	}
	return &AssistantResult{Action: AssistantUnsupported, Message: couldNotBuild}, nil
}

// decodeAssistantWidget reads the model's spec strictly (an unknown key is an
// error) and validates the whole widget against the Audit Hub's catalog.
func decodeAssistantWidget(res *aigateway.InterpretWidgetResult, canUse func(priv string) bool) (dashboardagent.Widget, error) {
	var spec dashboardagent.Spec
	dec := json.NewDecoder(bytes.NewReader(res.Spec))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&spec); err != nil {
		return dashboardagent.Widget{}, fmt.Errorf("the spec is not valid JSON for a widget: %v", err)
	}
	// Harmless slips are corrected; everything that protects data is still
	// checked by ValidateWidgetIn.
	spec = dashboardagent.Tidy(spec)
	visual := dashboardagent.FitVisual(res.Visual, spec)
	return dashboardagent.ValidateWidgetIn(dashboardagent.ModuleAudit, dashboardagent.Widget{Title: res.Title, Visual: visual, Spec: spec}, canUse)
}

func toAssistantCatalog(in []dashboardagent.DatasetDef) []aigateway.WidgetCatalogDataset {
	out := make([]aigateway.WidgetCatalogDataset, 0, len(in))
	for _, ds := range in {
		g := aigateway.WidgetCatalogDataset{ID: ds.ID, Label: ds.Label, Rows: ds.Rows, MaxRows: ds.MaxRows}
		for _, c := range ds.Columns {
			g.Columns = append(g.Columns, aigateway.WidgetCatalogColumn{ID: c.ID, Label: c.Label})
		}
		for _, f := range ds.Fields {
			g.Fields = append(g.Fields, aigateway.WidgetCatalogField{
				ID: f.ID, Label: f.Label, Kind: f.Kind, Allowed: f.Allowed, Min: f.Min, Max: f.Max, Group: f.Group,
			})
		}
		for _, m := range ds.Measures {
			g.Measures = append(g.Measures, aigateway.WidgetCatalogMeasure{ID: m.ID, Label: m.Label})
		}
		out = append(out, g)
	}
	return out
}

func toAssistantKnowledge(in []knowledge.Section) []aigateway.KnowledgeSection {
	out := make([]aigateway.KnowledgeSection, 0, len(in))
	for _, k := range in {
		out = append(out, aigateway.KnowledgeSection{Source: k.Source, Title: k.Title, Text: k.Text})
	}
	return out
}
