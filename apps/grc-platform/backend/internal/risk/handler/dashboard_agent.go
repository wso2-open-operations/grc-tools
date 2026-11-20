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
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/dashboardagent"
	riskservice "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/aigateway"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
)

const (
	// A spec is a few ids and values; anything bigger is not a spec.
	dashboardAgentMaxBody       int64 = 16 << 10
	dashboardAgentRatePerMinute       = 30

	// Interpreting costs a model call, so it is limited harder than a query.
	dashboardAgentMaxInterpretBody       int64 = 32 << 10
	dashboardAgentInterpretRatePerMinute       = 10
)

// widgetVisibility works out which risks the caller may count in a dashboard
// widget. It follows the risk list's own rule (see handleListRisks), never
// anything the request says:
//
//	GLOBAL ViewRisks            every risk
//	any other grant             risks raised in, or routed to, a team they hold a grant on
//	no grant at all             risks whose action plan names them
//
// A caller whose grants give them no risk visibility (a platform admin holding
// only MANAGE_USERS, or someone whose grants are all audit-team scoped) comes
// back as None, as does a caller who cannot be resolved to a user: nothing is
// ever widened to fill a gap.
func (d *Deps) widgetVisibility(ctx context.Context) (dashboardagent.Visibility, error) {
	if seesEveryRisk(ctx) {
		return dashboardagent.Visibility{All: true}, nil
	}
	if isTeamScopedOnly(ctx) {
		return dashboardagent.Visibility{TeamIDs: callerGrants(ctx).RiskScopeIDs()}, nil
	}
	id, err := d.callerUserID(ctx)
	if err != nil || id == nil {
		return dashboardagent.Visibility{}, err
	}
	return dashboardagent.Visibility{ActionOwnerUserID: id}, nil
}

// Outcomes recorded in the audit log.
const (
	outcomeAllowed      = "allowed"
	outcomeNotAvailable = "not_available"
	outcomeRateLimited  = "rate_limited"
	outcomeInvalid      = "invalid"
	outcomeUnavailable  = "unavailable"
	outcomeError        = "error"
)

// auditAssistant records one request to the Risk assistant: who asked, which
// endpoint, how it ended and what kind of access they had, plus the extras given.
// It names the fields a request used and how big the answer was. It never holds
// the user's message, a filter value or a row of the result.
func auditAssistant(endpoint, user, outcome string, vis dashboardagent.Visibility, extra ...any) {
	args := append([]any{"endpoint", endpoint, "user", user, "outcome", outcome, "access", vis.Kind()}, extra...)
	slog.Info("dashboard agent request", args...)
}

// filterFields lists the fields a spec filters on, not what it filters for.
func filterFields(spec dashboardagent.Spec) []string {
	out := make([]string, 0, len(spec.Filters))
	for _, f := range spec.Filters {
		out = append(out, f.Field)
	}
	return out
}

// dashboardAgentAccess is the gate every dashboard agent endpoint passes
// through: the feature must be on, the caller must be authenticated, and they
// must have some risk visibility. Anything else gets the same 404, so the
// endpoints reveal nothing to someone who cannot use them. On success it
// returns who is asking and what they may see.
//
// A refusal is logged, except for the "starters" check, which the web app makes
// on every Risk Hub page and which would only add noise.
func (d *Deps) dashboardAgentAccess(w http.ResponseWriter, r *http.Request, endpoint string) (string, dashboardagent.Visibility, bool) {
	if !d.DashboardAgentEnabled {
		response.WriteError(w, http.StatusNotFound, response.ErrMsgNotFound)
		return "", dashboardagent.Visibility{}, false
	}
	callerUUID, ok := requireCallerUUID(w, r)
	if !ok {
		return "", dashboardagent.Visibility{}, false
	}
	vis, err := d.widgetVisibility(r.Context())
	if err != nil {
		auditAssistant(endpoint, callerUUID, outcomeError, dashboardagent.Visibility{})
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return "", dashboardagent.Visibility{}, false
	}
	if vis.None() {
		if endpoint != "starters" {
			auditAssistant(endpoint, callerUUID, outcomeNotAvailable, vis)
		}
		response.WriteError(w, http.StatusNotFound, response.ErrMsgNotFound)
		return "", dashboardagent.Visibility{}, false
	}
	return callerUUID, vis, true
}

func tooManyRequests(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "60")
	response.WriteError(w, http.StatusTooManyRequests, "too many requests, please try again shortly")
}

// unavailableMessage is what the user is told when the AI service could not be
// reached. The cause is in the log; none of it reaches the web app.
const unavailableMessage = "The assistant is temporarily unavailable. Please try again in a moment."

// handleDashboardAgentStarters serves GET /api/v1/risks/dashboard-agent/starters:
// the ready-made views. It doubles as the check the web app makes to decide
// whether to show the agent at all — it answers 404 whenever the agent is not
// available to this caller.
func (d *Deps) handleDashboardAgentStarters(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := d.dashboardAgentAccess(w, r, "starters"); !ok {
		return
	}
	response.WriteJSONValue(w, http.StatusOK, map[string]any{"starters": dashboardagent.Starters()})
}

// handleDashboardAgentInterpret serves POST /api/v1/risks/dashboard-agent/interpret:
// it turns a sentence into a widget, a clarifying question, or a "can't build
// that". The model sees only the catalog and the sentence, and any widget it
// returns has been validated; running it is a separate call to /query, which
// applies the caller's visibility.
func (d *Deps) handleDashboardAgentInterpret(w http.ResponseWriter, r *http.Request) {
	callerUUID, vis, ok := d.dashboardAgentAccess(w, r, "interpret")
	if !ok {
		return
	}
	if !d.dashboardAgentInterpretLimiter.Allow(callerUUID) {
		auditAssistant("interpret", callerUUID, outcomeRateLimited, vis)
		tooManyRequests(w)
		return
	}

	var req riskservice.InterpretRequest
	if err := response.DecodeJSONLimit(w, r, &req, dashboardAgentMaxInterpretBody); err != nil {
		return
	}
	started := time.Now()
	res, err := d.DashboardAgent.Interpret(r.Context(), req, func(priv string) bool {
		return auth.HasPrivilege(r.Context(), priv)
	})
	if err != nil {
		var verr *dashboardagent.ValidationError
		switch {
		case errors.As(err, &verr):
			auditAssistant("interpret", callerUUID, outcomeInvalid, vis)
			response.WriteError(w, http.StatusBadRequest, verr.Msg)
		case errors.Is(err, riskservice.ErrAssistantUnavailable):
			auditAssistant("interpret", callerUUID, outcomeUnavailable, vis, "model", aigateway.ModelName, "ms", time.Since(started).Milliseconds())
			response.WriteError(w, http.StatusServiceUnavailable, unavailableMessage)
		default:
			auditAssistant("interpret", callerUUID, outcomeError, vis)
			response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		}
		return
	}

	// A question that names one risk is answered from that risk's own record, by
	// code here, after checking the caller may see it. The web app only ever
	// receives a reply (with a link to the risk when it was found).
	explained := false
	switch res.Action {
	case riskservice.InterpretExplainRisk:
		text, link, timeline, err := d.explainRisk(r.Context(), res.RiskCode)
		if err != nil {
			auditAssistant("interpret", callerUUID, outcomeError, vis, "explainedRisk", true)
			response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
			return
		}
		res = &riskservice.InterpretResult{Action: riskservice.InterpretReply, Message: text, RiskLink: link, Timeline: timeline}
		explained = true
	case riskservice.InterpretCompareRisks:
		text, table, err := d.compareRisks(r.Context(), res.RiskCodes)
		if err != nil {
			auditAssistant("interpret", callerUUID, outcomeError, vis, "comparedRisks", true)
			response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
			return
		}
		res = &riskservice.InterpretResult{Action: riskservice.InterpretReply, Message: text, Compare: table}
		explained = true
	}

	extra := []any{
		"action", res.Action,
		"explainedRisk", explained,
		"messageChars", len([]rune(req.Message)),
		"followUp", req.Current != nil,
		"ms", time.Since(started).Milliseconds(),
	}
	if !explained {
		extra = append(extra, "model", aigateway.ModelName) // the AI was asked
	}
	auditAssistant("interpret", callerUUID, outcomeAllowed, vis, extra...)
	response.WriteJSONValue(w, http.StatusOK, res)
}

// handleDashboardAgentQuery serves POST /api/v1/risks/dashboard-agent/query: it
// runs one dashboard widget spec and returns the aggregate.
//
// The spec is checked against the catalog and the caller's visibility is worked
// out here, on the server; the request has no field for either the data scope
// or the privileges.
func (d *Deps) handleDashboardAgentQuery(w http.ResponseWriter, r *http.Request) {
	callerUUID, vis, ok := d.dashboardAgentAccess(w, r, "query")
	if !ok {
		return
	}
	if !d.dashboardAgentLimiter.Allow(callerUUID) {
		auditAssistant("query", callerUUID, outcomeRateLimited, vis)
		tooManyRequests(w)
		return
	}

	var req dashboardagent.Spec
	if err := response.DecodeJSONLimit(w, r, &req, dashboardAgentMaxBody); err != nil {
		return
	}
	spec, err := dashboardagent.Validate(req, func(priv string) bool {
		return auth.HasPrivilege(r.Context(), priv)
	})
	if err != nil {
		auditAssistant("query", callerUUID, outcomeInvalid, vis)
		response.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	started := time.Now()
	result, err := d.DashboardAgent.Run(r.Context(), spec, vis)
	if err != nil {
		var verr *dashboardagent.ValidationError
		if errors.As(err, &verr) {
			auditAssistant("query", callerUUID, outcomeInvalid, vis, "dataset", spec.Dataset, "filterFields", filterFields(spec))
			response.WriteError(w, http.StatusBadRequest, verr.Msg)
			return
		}
		auditAssistant("query", callerUUID, outcomeError, vis, "dataset", spec.Dataset)
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	d.nameActionOwners(r.Context(), result)
	// The sentence is written after the names are in, from the numbers here: no
	// identity can end up in it and no model is involved.
	result.Summary = dashboardagent.Summarize(spec, result)

	auditAssistant("query", callerUUID, outcomeAllowed, vis,
		"dataset", spec.Dataset,
		"measures", spec.Measures,
		"dimensions", spec.Dimensions,
		"columns", spec.Columns,
		"filterFields", filterFields(spec),
		"rows", len(result.Rows),
		"ms", time.Since(started).Milliseconds(),
	)
	response.WriteJSONValue(w, http.StatusOK, result)
}

// Labels shown for an Action Owner that has no name to show.
const (
	actionOwnerUnassigned = "Unassigned"
	actionOwnerUnknown    = "Unknown"
)

// nameActionOwners swaps the identities in the action_owner column for display
// names. The query returns the user's uuid because the platform stores no
// names; the uuid never leaves the server.
func (d *Deps) nameActionOwners(ctx context.Context, res *dashboardagent.Result) {
	labelActionOwners(res, func(uuids []string) map[string]string {
		if d.Directory == nil {
			return nil
		}
		out := map[string]string{}
		for uuid, p := range d.Directory.LookupAll(ctx, uuids) {
			if p.DisplayName != "" {
				out[uuid] = p.DisplayName
			}
		}
		return out
	})
}

// labelActionOwners replaces each uuid in the action_owner column with the name
// lookup gives for it. A missing value becomes "Unassigned", a uuid with no name
// becomes "Unknown": never the raw identity.
func labelActionOwners(res *dashboardagent.Result, lookup func(uuids []string) map[string]string) {
	col := -1
	for i, c := range res.Columns {
		if c.ID == "action_owner" {
			col = i
		}
	}
	if col < 0 {
		return
	}
	var uuids []string
	for _, row := range res.Rows {
		if s, ok := row[col].(string); ok && s != "" {
			uuids = append(uuids, s)
		}
	}
	names := lookup(uuids)
	for _, row := range res.Rows {
		s, _ := row[col].(string)
		switch {
		case s == "":
			row[col] = actionOwnerUnassigned
		case names[s] != "":
			row[col] = names[s]
		default:
			row[col] = actionOwnerUnknown
		}
	}
}
