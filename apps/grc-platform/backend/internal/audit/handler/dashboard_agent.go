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

package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	auditservice "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/dashboardagent"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/aigateway"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
)

const (
	// A spec is a few ids and values; anything bigger is not a spec.
	assistantMaxBody       int64 = 16 << 10
	assistantRatePerMinute       = 30

	// Interpreting costs a model call, so it is limited harder than a query.
	assistantMaxInterpretBody       int64 = 32 << 10
	assistantInterpretRatePerMinute       = 10
)

// Outcomes recorded in the audit log.
const (
	outcomeAllowed      = "allowed"
	outcomeNotAvailable = "not_available"
	outcomeRateLimited  = "rate_limited"
	outcomeInvalid      = "invalid"
	outcomeUnavailable  = "unavailable"
	outcomeError        = "error"
)

// assistantScope works out which controls the caller may count. It is the
// dashboard's own rule (deriveScopes), never anything the request says:
//
//	AUDIT_VIEW_ALL_AUDITS, global   every control
//	the same, on a team             that team's controls, plus the caller's own
//	submits evidence                controls they own
//	validates evidence              controls they audit
//
// A caller with none of these, or who cannot be resolved to a user, is None:
// nothing is ever widened to fill a gap. External auditors are refused at the
// route (see routeguard), so they never get here.
func assistantScope(ctx context.Context) dashboardagent.AuditScope {
	scope, _ := deriveScopes(ctx)
	user := auth.FromContext(ctx)
	switch scope {
	case model.ScopeAll:
		return dashboardagent.AuditScope{Scope: "all"}
	case model.ScopeTeam, model.ScopeOwned, model.ScopeAssigned:
		if user == nil || user.UserID <= 0 {
			return dashboardagent.AuditScope{}
		}
		out := dashboardagent.AuditScope{Scope: string(scope), UserID: user.UserID}
		if scope == model.ScopeTeam {
			out.TeamIDs = managedTeamIDs(auth.Grants(ctx))
		}
		return out
	}
	return dashboardagent.AuditScope{}
}

// auditAssistant records one request to the Audit assistant: who asked, which
// endpoint, how it ended and what kind of access they had, plus the extras
// given. It names the fields a request used and how big the answer was. It never
// holds the user's message, a filter value or a row of the result.
func auditAssistant(endpoint, user, outcome string, scope dashboardagent.AuditScope, extra ...any) {
	args := append([]any{"endpoint", endpoint, "user", user, "outcome", outcome, "access", scope.Kind(), "hub", "audit"}, extra...)
	slog.Info("audit assistant request", args...)
}

func assistantFilterFields(spec dashboardagent.Spec) []string {
	out := make([]string, 0, len(spec.Filters))
	for _, f := range spec.Filters {
		out = append(out, f.Field)
	}
	return out
}

// assistantAccess is the gate every audit assistant endpoint passes through: the
// feature must be on, the caller must be authenticated, and they must have some
// audit visibility. Anything else gets the same 404, so the endpoints reveal
// nothing to someone who cannot use them.
func (d *Deps) assistantAccess(w http.ResponseWriter, r *http.Request, endpoint string) (string, dashboardagent.AuditScope, bool) {
	if !d.DashboardAgentEnabled || d.DashboardAgent == nil {
		response.WriteError(w, http.StatusNotFound, response.ErrMsgNotFound)
		return "", dashboardagent.AuditScope{}, false
	}
	user := auth.FromContext(r.Context())
	if user == nil {
		response.WriteError(w, http.StatusUnauthorized, response.ErrMsgUnauthorized)
		return "", dashboardagent.AuditScope{}, false
	}
	scope := assistantScope(r.Context())
	if scope.None() {
		if endpoint != "starters" {
			auditAssistant(endpoint, user.Subject, outcomeNotAvailable, scope)
		}
		response.WriteError(w, http.StatusNotFound, response.ErrMsgNotFound)
		return "", dashboardagent.AuditScope{}, false
	}
	return user.Subject, scope, true
}

func assistantTooManyRequests(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "60")
	response.WriteError(w, http.StatusTooManyRequests, "too many requests, please try again shortly")
}

// assistantUnavailableMessage is what the user is told when the AI service could
// not be reached. The cause is in the log; none of it reaches the web app.
const assistantUnavailableMessage = "The assistant is temporarily unavailable. Please try again in a moment."

// handleAssistantStarters serves GET /api/v1/audits/dashboard-agent/starters.
// The Audit assistant offers no ready-made views, so the list is empty; the call
// is the check the web app makes to decide whether to show the assistant at all,
// and it answers 404 whenever the assistant is not available to this caller.
func (d *Deps) handleAssistantStarters(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := d.assistantAccess(w, r, "starters"); !ok {
		return
	}
	response.WriteJSONValue(w, http.StatusOK, map[string]any{"starters": []any{}})
}

// handleAssistantInterpret serves POST /api/v1/audits/dashboard-agent/interpret:
// it turns a sentence into a widget, a clarifying question, or a "can't build
// that". The model sees only the catalog and the sentence, and any widget it
// returns has been validated; running it is a separate call to /query, which
// applies the caller's scope.
func (d *Deps) handleAssistantInterpret(w http.ResponseWriter, r *http.Request) {
	callerUUID, scope, ok := d.assistantAccess(w, r, "interpret")
	if !ok {
		return
	}
	if !d.assistantInterpretLimiter.Allow(callerUUID) {
		auditAssistant("interpret", callerUUID, outcomeRateLimited, scope)
		assistantTooManyRequests(w)
		return
	}

	var req auditservice.AssistantRequest
	if err := response.DecodeJSONLimit(w, r, &req, assistantMaxInterpretBody); err != nil {
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
			auditAssistant("interpret", callerUUID, outcomeInvalid, scope)
			response.WriteError(w, http.StatusBadRequest, verr.Msg)
		case errors.Is(err, auditservice.ErrAssistantUnavailable):
			auditAssistant("interpret", callerUUID, outcomeUnavailable, scope, "model", aigateway.ModelName, "ms", time.Since(started).Milliseconds())
			response.WriteError(w, http.StatusServiceUnavailable, assistantUnavailableMessage)
		default:
			auditAssistant("interpret", callerUUID, outcomeError, scope)
			response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		}
		return
	}
	// A request to open a control is answered here, by code: the control is looked
	// up with the caller's own scope, and only then is the user pointed at it.
	opened := false
	switch res.Action {
	case auditservice.AssistantOpenControl, auditservice.AssistantExplainControl, auditservice.AssistantCompareControls:
		asked := res
		switch asked.Action {
		case auditservice.AssistantOpenControl:
			res, err = d.openControl(r.Context(), scope, asked.ControlNumber, asked.AuditName)
		case auditservice.AssistantExplainControl:
			res, err = d.explainControl(r.Context(), scope, asked.ControlNumber, asked.AuditName)
		default:
			res, err = d.compareControls(r.Context(), scope, asked.ControlNumber, asked.AuditName, asked.ControlNumber2, asked.AuditName2)
		}
		if err != nil {
			auditAssistant("interpret", callerUUID, outcomeError, scope, "controlRequest", asked.Action)
			response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
			return
		}
		opened = true
	}
	auditAssistant("interpret", callerUUID, outcomeAllowed, scope,
		"action", res.Action,
		"openedControl", opened,
		"found", res.Navigate != nil,
		"messageChars", len([]rune(req.Message)),
		"followUp", req.Current != nil,
		"model", aigateway.ModelName,
		"ms", time.Since(started).Milliseconds(),
	)
	response.WriteJSONValue(w, http.StatusOK, res)
}

// handleAssistantQuery serves POST /api/v1/audits/dashboard-agent/query: it runs
// one widget spec and returns the aggregate.
//
// The spec is checked against the Audit Hub's catalog and the caller's scope is
// worked out here, on the server; the request has no field for either the data
// scope or the privileges.
func (d *Deps) handleAssistantQuery(w http.ResponseWriter, r *http.Request) {
	callerUUID, scope, ok := d.assistantAccess(w, r, "query")
	if !ok {
		return
	}
	if !d.assistantLimiter.Allow(callerUUID) {
		auditAssistant("query", callerUUID, outcomeRateLimited, scope)
		assistantTooManyRequests(w)
		return
	}

	var req dashboardagent.Spec
	if err := response.DecodeJSONLimit(w, r, &req, assistantMaxBody); err != nil {
		return
	}
	spec, err := dashboardagent.ValidateIn(dashboardagent.ModuleAudit, req, func(priv string) bool {
		return auth.HasPrivilege(r.Context(), priv)
	})
	if err != nil {
		auditAssistant("query", callerUUID, outcomeInvalid, scope)
		response.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	started := time.Now()
	result, err := d.DashboardAgent.Run(r.Context(), spec, scope)
	if err != nil {
		var verr *dashboardagent.ValidationError
		if errors.As(err, &verr) {
			auditAssistant("query", callerUUID, outcomeInvalid, scope, "dataset", spec.Dataset, "filterFields", assistantFilterFields(spec))
			response.WriteError(w, http.StatusBadRequest, verr.Msg)
			return
		}
		auditAssistant("query", callerUUID, outcomeError, scope, "dataset", spec.Dataset)
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	d.namePeopleColumns(r.Context(), result)
	// The sentence is written after the names are in, from the numbers here: no
	// identity can end up in it and no model is involved.
	result.Summary = dashboardagent.Summarize(spec, result)

	auditAssistant("query", callerUUID, outcomeAllowed, scope,
		"dataset", spec.Dataset,
		"measures", spec.Measures,
		"dimensions", spec.Dimensions,
		"columns", spec.Columns,
		"filterFields", assistantFilterFields(spec),
		"rows", len(result.Rows),
		"ms", time.Since(started).Milliseconds(),
	)
	response.WriteJSONValue(w, http.StatusOK, result)
}

// Labels shown for a control owner or auditor that has no name to show.
const (
	personUnassigned = "Unassigned"
	personUnknown    = "Unknown"
)

// namePeopleColumns swaps the identities in the owner and auditor columns for
// display names. The query returns the user's uuid because the platform stores
// no names; the uuid never leaves the server.
func (d *Deps) namePeopleColumns(ctx context.Context, res *dashboardagent.Result) {
	labelPeopleColumns(res, []string{"owner", "auditor"}, func(uuids []string) map[string]string {
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

// labelPeopleColumns replaces each uuid in the named columns with the name lookup
// gives for it. A missing value becomes "Unassigned", a uuid with no name becomes
// "Unknown": never the raw identity.
func labelPeopleColumns(res *dashboardagent.Result, ids []string, lookup func(uuids []string) map[string]string) {
	var cols []int
	for i, c := range res.Columns {
		for _, id := range ids {
			if c.ID == id {
				cols = append(cols, i)
			}
		}
	}
	if len(cols) == 0 {
		return
	}
	var uuids []string
	for _, row := range res.Rows {
		for _, col := range cols {
			if s, ok := row[col].(string); ok && s != "" {
				uuids = append(uuids, s)
			}
		}
	}
	names := lookup(uuids)
	for _, row := range res.Rows {
		for _, col := range cols {
			s, _ := row[col].(string)
			switch {
			case s == "":
				row[col] = personUnassigned
			case names[s] != "":
				row[col] = names[s]
			default:
				row[col] = personUnknown
			}
		}
	}
}
