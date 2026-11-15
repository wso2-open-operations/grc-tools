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
	"fmt"
	"sort"
	"strings"

	auditservice "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/dashboardagent"
)

// controlNotFound is the answer when the control cannot be found, and also when
// it exists but the caller may not see it: the two are never told apart.
const controlNotFound = "I couldn't find that control with the access you have. Check the control number and the audit name, or open Audits and search for it."

// maxControlMatches is how many matching controls are looked at.
const maxControlMatches = 10

// resolveControl finds the one control a user means. The search is an ordinary
// scoped query, so it can only ever find controls the caller may see; the control
// number and audit name are bound filter values, and nothing said back is more
// than the caller could already read.
//
//	one control found          returned (reply is nil)
//	several (no audit given)   a reply asking which audit, naming only audits the caller sees
//	none, or not visible       the same "couldn't find" reply either way
func (d *Deps) resolveControl(ctx context.Context, scope dashboardagent.AuditScope, number, auditName string) (*controlHit, *auditservice.AssistantResult, error) {
	number, auditName = strings.TrimSpace(number), strings.TrimSpace(auditName)
	reply := func(msg string) (*controlHit, *auditservice.AssistantResult, error) {
		return nil, &auditservice.AssistantResult{Action: auditservice.AssistantReply, Message: msg}, nil
	}
	if number == "" {
		return reply(controlNotFound)
	}

	find := func(audit string) ([]controlHit, error) {
		filters := []dashboardagent.Filter{{Field: "control_number", Op: "in", Values: []string{number}}}
		if audit != "" {
			filters = append(filters, dashboardagent.Filter{Field: "audit", Op: "in", Values: []string{audit}})
		}
		spec := dashboardagent.Spec{
			Dataset: "control_list",
			Columns: []string{"control_number", "audit", "audit_id", "control_id"},
			Filters: filters,
			Limit:   maxControlMatches,
		}
		res, err := d.DashboardAgent.Run(ctx, spec, scope)
		if err != nil {
			return nil, err
		}
		return controlHits(res), nil
	}

	hits, err := find(auditName)
	if err != nil {
		return nil, nil, err
	}
	// An audit name that matches nothing may just be worded differently: look for
	// the control on its own and let the user pick.
	named := auditName != ""
	if len(hits) == 0 && named {
		if hits, err = find(""); err != nil {
			return nil, nil, err
		}
	}

	switch {
	case len(hits) == 0:
		return reply(controlNotFound)
	case len(hits) == 1 && (!named || strings.EqualFold(hits[0].audit, auditName)):
		return &hits[0], nil, nil
	}

	names := make([]string, 0, len(hits))
	for _, h := range hits {
		names = append(names, h.audit)
	}
	sort.Strings(names)
	prefix := fmt.Sprintf("%s is in more than one audit you can see", number)
	if named {
		prefix = fmt.Sprintf("I couldn't find an audit called “%s” with %s, but it is in these", auditName, number)
	}
	return reply(fmt.Sprintf("%s: %s. Which one do you mean?", prefix, strings.Join(names, ", ")))
}

// openControl answers a request to be taken to a control.
func (d *Deps) openControl(ctx context.Context, scope dashboardagent.AuditScope, number, auditName string) (*auditservice.AssistantResult, error) {
	h, reply, err := d.resolveControl(ctx, scope, number, auditName)
	if err != nil || reply != nil {
		return reply, err
	}
	return &auditservice.AssistantResult{
		Action:   auditservice.AssistantReply,
		Message:  fmt.Sprintf("Taking you to %s in %s.", h.number, h.audit),
		Navigate: &auditservice.ControlTarget{AuditID: h.auditID, ControlID: h.controlID},
	}, nil
}

type controlHit struct {
	number, audit      string
	auditID, controlID int
}

// controlHits reads the rows of the lookup above, skipping any that are not
// shaped as expected.
func controlHits(res *dashboardagent.Result) []controlHit {
	idx := map[string]int{}
	for i, c := range res.Columns {
		idx[c.ID] = i
	}
	num := func(v any) int {
		switch n := v.(type) {
		case int64:
			return int(n)
		case int:
			return n
		case float64:
			return int(n)
		}
		return 0
	}
	var out []controlHit
	for _, row := range res.Rows {
		var h controlHit
		h.number, _ = row[idx["control_number"]].(string)
		h.audit, _ = row[idx["audit"]].(string)
		h.auditID, h.controlID = num(row[idx["audit_id"]]), num(row[idx["control_id"]])
		if h.number != "" && h.audit != "" && h.auditID > 0 && h.controlID > 0 {
			out = append(out, h)
		}
	}
	return out
}
