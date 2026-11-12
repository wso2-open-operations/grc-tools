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
	"strings"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
)

// handlerLabels names the groups the way the rest of the module does: the
// control's owner, the compliance team and the auditor.
var handlerLabels = map[string]string{
	"OWNER":      "Owner",
	"COMPLIANCE": "Compliance",
	"AUDITOR":    "Auditor",
}

// canSeeDelayLog reports whether the caller may see a control's delay log. The
// log names which group was late, so only org-wide readers (GRC) get it;
// Owners and auditors see their own current deadline and nothing about the
// other groups' lateness.
func canSeeDelayLog(ctx context.Context) bool {
	scope, _ := deriveScopes(ctx)
	return scope == model.ScopeAll
}

// redactDelayLog clears the delay log when the caller may not see it.
func redactDelayLog(controls []*model.AuditControl, allowed bool) {
	if allowed {
		return
	}
	for _, c := range controls {
		c.DelayLog = nil
	}
}

// delayLogText renders a delay log as "Owner - 11/08; Compliance - 11/21".
func delayLogText(entries []model.DelayLogEntry) string {
	parts := make([]string, 0, len(entries))
	for _, e := range entries {
		name := handlerLabels[e.HandledBy]
		if name == "" {
			name = e.HandledBy
		}
		parts = append(parts, name+" - "+monthDay(e.Date))
	}
	return strings.Join(parts, "; ")
}

// monthDay turns YYYY-MM-DD into MM/DD without a timezone round trip.
func monthDay(date string) string {
	if len(date) >= 10 && date[4] == '-' && date[7] == '-' {
		return date[5:7] + "/" + date[8:10]
	}
	return date
}
