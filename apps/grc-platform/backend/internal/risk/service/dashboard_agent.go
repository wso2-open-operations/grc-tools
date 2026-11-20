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

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/dashboardagent"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/knowledge"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/aigateway"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
)

// WidgetQueryRepository is what the dashboard agent needs from storage: run one
// validated spec for a caller with the given visibility.
type WidgetQueryRepository interface {
	Run(ctx context.Context, spec dashboardagent.Spec, vis dashboardagent.Visibility) (*dashboardagent.Result, error)
}

// WidgetInterpreter turns a sentence into a widget suggestion. Implemented by
// *aigateway.Client.
type WidgetInterpreter interface {
	InterpretWidget(ctx context.Context, req aigateway.InterpretWidgetRequest) (*aigateway.InterpretWidgetResult, error)
}

// Interpretation outcomes shown to the user.
const (
	// InterpretExplainRisk is internal: the question names one risk by its code,
	// and the handler answers it from that risk's own record. It is never sent
	// to the web app as it is.
	InterpretExplainRisk = "explain_risk"
	// InterpretCompareRisks is internal in the same way: the question names two
	// risks, and the handler compares their records.
	InterpretCompareRisks = "compare_risks"
	InterpretReply        = aigateway.WidgetActionReply
	InterpretWidget       = aigateway.WidgetActionBuild
	InterpretClarify      = aigateway.WidgetActionClarify
	InterpretUnsupported  = aigateway.WidgetActionUnsupported
)

// Bounds on what the popup may send.
const (
	maxInterpretMessageLen = 500
	maxInterpretHistory    = 6

	// maxKnowledgeSections is how many help sections, beyond the always-included
	// map of the Risk Hub, go to the model with a question.
	maxKnowledgeSections = 3
)

// ErrAssistantUnavailable means the AI service could not be reached or failed.
// The web app shows a calm "try again" message; the cause stays in the log.
var ErrAssistantUnavailable = errors.New("the assistant is temporarily unavailable")

// RiskLink points at one risk, so the web app can offer to open it. It carries
// the risk's id, which the register's own address already uses.
type RiskLink struct {
	ID   int    `json:"id"`
	Code string `json:"code"`
}

// TimelineEvent is one dated line in a risk's history.
type TimelineEvent struct {
	Date string `json:"date"`
	Text string `json:"text"`
}

// Timeline is a risk's history as the popup draws it, oldest first. Note says
// when only the latest events are shown.
type Timeline struct {
	Note   string          `json:"note,omitempty"`
	Events []TimelineEvent `json:"events"`
}

// CompareRow is one line of a comparison: a label and each risk's value.
// Differs is true when the risks do not have the same value.
type CompareRow struct {
	Label   string   `json:"label"`
	Values  []string `json:"values"`
	Differs bool     `json:"differs"`
}

// CompareTable puts two risks side by side. Risks is in column order.
type CompareTable struct {
	Risks []RiskLink   `json:"risks"`
	Rows  []CompareRow `json:"rows"`
}

// ChatTurn is one earlier message in the popup conversation.
type ChatTurn struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

// InterpretRequest is what the popup sends for each message.
type InterpretRequest struct {
	Message string                 `json:"message"`
	History []ChatTurn             `json:"history"`
	Current *dashboardagent.Widget `json:"current"`
}

// InterpretResult is what the popup shows. Exactly one of Question, Message or
// Widget is meaningful, as Action says. Widget has already been validated, so
// the popup can run it as it is.
type InterpretResult struct {
	Action   string                 `json:"action"`
	Question string                 `json:"question,omitempty"`
	Message  string                 `json:"message,omitempty"`
	Note     string                 `json:"note,omitempty"`
	Widget   *dashboardagent.Widget `json:"widget,omitempty"`
	// RiskCode is set only with InterpretExplainRisk, for the handler.
	RiskCode string `json:"-"`
	// RiskCodes is set only with InterpretCompareRisks, for the handler.
	RiskCodes []string `json:"-"`
	// RiskLink, Timeline and Compare are set on the replies that explain or
	// compare risks.
	RiskLink *RiskLink     `json:"risk_link,omitempty"`
	Timeline *Timeline     `json:"timeline,omitempty"`
	Compare  *CompareTable `json:"compare,omitempty"`
}

// DashboardAgentService runs dashboard widget specs against the risk register
// and turns requests into widgets.
type DashboardAgentService interface {
	// Run executes spec, which the caller has already validated, counting only
	// the risks vis allows. A visibility that allows nothing never reaches the
	// database: the result is empty, the same as a caller with no matching data.
	Run(ctx context.Context, spec dashboardagent.Spec, vis dashboardagent.Visibility) (*dashboardagent.Result, error)

	// Interpret asks the model for a widget, a clarifying question, or a "can't
	// build that". It sees only the catalog the caller may use and what they
	// typed, never data, and any widget it returns passed validation.
	Interpret(ctx context.Context, req InterpretRequest, canUse func(priv string) bool) (*InterpretResult, error)
}

type dashboardAgentService struct {
	repo        WidgetQueryRepository
	interpreter WidgetInterpreter
	people      dashboardagent.PeopleDirectory
	now         func() time.Time
}

// NewDashboardAgentService constructs a DashboardAgentService.
func NewDashboardAgentService(repo WidgetQueryRepository, interpreter WidgetInterpreter, people dashboardagent.PeopleDirectory) DashboardAgentService {
	return &dashboardAgentService{repo: repo, interpreter: interpreter, people: people, now: time.Now}
}

// callerUUID is who is asking, taken from the request's own authentication.
func callerUUID(ctx context.Context) string {
	if u := auth.FromContext(ctx); u != nil {
		return u.Subject
	}
	return ""
}

func (s *dashboardAgentService) Run(ctx context.Context, spec dashboardagent.Spec, vis dashboardagent.Visibility) (*dashboardagent.Result, error) {
	if vis.None() {
		return &dashboardagent.Result{Columns: []dashboardagent.Column{}, Rows: [][]any{}}, nil
	}
	// Names in person filters become identities here; the query only ever sees
	// the identities. A name that cannot be settled is a message for the user.
	resolved, problem := dashboardagent.ResolvePeople(spec, callerUUID(ctx), s.people)
	if problem != "" {
		return nil, &dashboardagent.ValidationError{Msg: problem}
	}
	res, err := s.repo.Run(ctx, dashboardagent.WithLinkColumn(resolved), vis)
	if err != nil {
		return nil, err
	}
	dashboardagent.LabelColumns(spec, res)
	return res, nil
}

// couldNotBuild is what the user sees when the model twice failed to produce a
// widget that passes validation. The reason stays out of it: it is about the
// model's mistake, not something the user can act on.
const couldNotBuild = "I couldn't build a valid widget for that. Try rephrasing it, or start from one of the ready-made views."

func (s *dashboardAgentService) Interpret(ctx context.Context, req InterpretRequest, canUse func(priv string) bool) (*InterpretResult, error) {
	message := strings.TrimSpace(req.Message)
	if message == "" || utf8.RuneCountInString(message) > maxInterpretMessageLen {
		return nil, &dashboardagent.ValidationError{Msg: fmt.Sprintf("write a request of up to %d characters", maxInterpretMessageLen)}
	}
	if len(req.History) > maxInterpretHistory {
		return nil, &dashboardagent.ValidationError{Msg: "the conversation is too long, start a new one"}
	}
	history := make([]aigateway.ChatTurn, 0, len(req.History))
	for _, t := range req.History {
		text := strings.TrimSpace(t.Text)
		if (t.Role != "user" && t.Role != "assistant") || text == "" || utf8.RuneCountInString(text) > maxInterpretMessageLen {
			return nil, &dashboardagent.ValidationError{Msg: "the conversation is not valid"}
		}
		history = append(history, aigateway.ChatTurn{Role: t.Role, Text: text})
	}

	// The widget being refined came from the client, so it is validated like
	// any other before the model is shown it.
	current := ""
	if req.Current != nil {
		w, err := dashboardagent.ValidateWidget(*req.Current, canUse)
		if err != nil {
			return nil, err
		}
		b, err := json.Marshal(w)
		if err != nil {
			return nil, fmt.Errorf("interpret: marshal current widget: %w", err)
		}
		current = string(b)
	}

	// A question that names one risk by its code is about that risk, so it goes
	// straight to its record; no model is involved and none sees the risk.
	switch codes := riskCodesIn(message); len(codes) {
	case 0:
	case 1:
		return &InterpretResult{Action: InterpretExplainRisk, RiskCode: codes[0]}, nil
	case 2:
		return &InterpretResult{Action: InterpretCompareRisks, RiskCodes: codes}, nil
	default:
		return &InterpretResult{Action: InterpretReply, Message: "I can compare two risks at a time. Name just two risk codes and I'll put them side by side."}, nil
	}

	gwReq := aigateway.InterpretWidgetRequest{
		Message:       message,
		History:       history,
		CurrentWidget: current,
		Catalog:       toGatewayCatalog(dashboardagent.CatalogFor(canUse)),
		KnownViews:    knownViews(canUse),
		Knowledge:     toGatewayKnowledge(knowledge.Relevant(message, maxKnowledgeSections)),
		Today:         s.now().Format("2006-01-02"),
	}

	// One retry, telling the model why its first widget was refused.
	for attempt := 0; attempt < 2; attempt++ {
		res, err := s.interpreter.InterpretWidget(ctx, gwReq)
		if err != nil {
			slog.Error("dashboard agent: the AI service failed", "err", err)
			return nil, ErrAssistantUnavailable
		}
		switch res.Action {
		case aigateway.WidgetActionReply:
			return &InterpretResult{Action: InterpretReply, Message: res.Message}, nil
		case aigateway.WidgetActionClarify:
			return &InterpretResult{Action: InterpretClarify, Question: res.Question}, nil
		case aigateway.WidgetActionUnsupported:
			return &InterpretResult{Action: InterpretUnsupported, Message: res.Message}, nil
		}

		w, verr := decodeAndValidateWidget(res, canUse)
		if verr == nil {
			// Settle the people it names now, so a name that matches nobody (or
			// several people) is answered here instead of drawing an empty chart.
			if _, problem := dashboardagent.ResolvePeople(w.Spec, callerUUID(ctx), s.people); problem != "" {
				return &InterpretResult{Action: InterpretReply, Message: problem}, nil
			}
			return &InterpretResult{Action: InterpretWidget, Widget: &w, Note: res.Note}, nil
		}
		// The user is told only that it could not be built; the reason belongs
		// in the log, where someone can act on it. It names a rule that was
		// broken, never any data.
		slog.Warn("dashboard agent: the model's widget was refused", "attempt", attempt+1, "reason", verr.Error())
		gwReq.RetryFeedback = verr.Error()
	}
	return &InterpretResult{Action: InterpretUnsupported, Message: couldNotBuild}, nil
}

// decodeAndValidateWidget reads the model's spec strictly (an unknown key is an
// error, not something to ignore) and validates the whole widget.
func decodeAndValidateWidget(res *aigateway.InterpretWidgetResult, canUse func(priv string) bool) (dashboardagent.Widget, error) {
	var spec dashboardagent.Spec
	dec := json.NewDecoder(bytes.NewReader(res.Spec))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&spec); err != nil {
		return dashboardagent.Widget{}, fmt.Errorf("the spec is not valid JSON for a widget: %v", err)
	}
	// Harmless slips (a limit over the cap, a visual that does not fit, a sort on
	// a column that is not shown) are corrected. Everything that protects data is
	// still checked by ValidateWidget.
	spec = dashboardagent.Tidy(spec)
	visual := dashboardagent.FitVisual(res.Visual, spec)
	return dashboardagent.ValidateWidget(dashboardagent.Widget{Title: res.Title, Visual: visual, Spec: spec}, canUse)
}

func toGatewayCatalog(in []dashboardagent.DatasetDef) []aigateway.WidgetCatalogDataset {
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

// knownViews describes the ready-made views to the model, so that asking for one
// by name produces it. They are never shown to the user unprompted. A view the
// caller could not run is left out.
func knownViews(canUse func(priv string) bool) []aigateway.KnownView {
	var out []aigateway.KnownView
	for _, st := range dashboardagent.Starters() {
		w, err := dashboardagent.ValidateWidget(st.Widget, canUse)
		if err != nil {
			continue
		}
		b, err := json.Marshal(w)
		if err != nil {
			continue
		}
		out = append(out, aigateway.KnownView{Title: st.Title, Description: st.Description, Widget: string(b)})
	}
	return out
}

// riskCodePattern matches a risk code such as 2026-INT-Q4-0002 or
// 2025-MS-BO-Q3-0001, and the seeded SEED-MS-01 style.
var riskCodePattern = regexp.MustCompile(`(?i)\b(?:\d{4}-[a-z0-9]+(?:-[a-z0-9]+)*-q[1-4]-\d{3,5}|seed-[a-z0-9]+-\d+)\b`)

// riskCodesIn returns the different risk codes a message names, in the order
// they first appear. The same code written twice, or in another case, counts
// once.
func riskCodesIn(message string) []string {
	var found []string
	for _, m := range riskCodePattern.FindAllString(message, -1) {
		dup := false
		for _, f := range found {
			dup = dup || strings.EqualFold(f, m)
		}
		if !dup {
			found = append(found, m)
		}
	}
	return found
}

func toGatewayKnowledge(in []knowledge.Section) []aigateway.KnowledgeSection {
	out := make([]aigateway.KnowledgeSection, 0, len(in))
	for _, s := range in {
		out = append(out, aigateway.KnowledgeSection{Source: s.Source, Title: s.Title, Text: s.Text})
	}
	return out
}
