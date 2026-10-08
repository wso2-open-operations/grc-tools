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
	"log/slog"
	"net/http"
	"sync/atomic"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// likelihoodRecheckJobHandler exposes a manual trigger for the quarterly
// Likelihood re-check sweep (internal/risk/job.LikelihoodRecheckJob), same
// shape as reminderJobHandler — mainly for QA/ops to exercise the window
// logic without waiting for an actual quarter-end to roll around.
type likelihoodRecheckJobHandler struct {
	// trigger runs the sweep's full pass. A plain function, not a
	// job.LikelihoodRecheckJob field, for the same import-cycle reason as
	// reminderJobHandler.trigger. Nil (job wiring not configured) answers 503.
	trigger func(ctx context.Context) error
	running atomic.Bool
}

// run handles POST /api/v1/risks/likelihood/recheck/run.
//
// Detached in a goroutine, same reasoning as reminderJobHandler.run: the
// server's WriteTimeout is 30s and a sweep may take up to 30 minutes.
//
// Re-running safely no-ops outside the quarter-end window (the job's own
// enabled/window check runs first), and within the window only re-checks
// risks it hasn't already checked this quarter.
func (h *likelihoodRecheckJobHandler) run(w http.ResponseWriter, r *http.Request) {
	// ManageRiskHub, not a register-scoped privilege: this sweep checks risks
	// across EVERY register, same gating as the due-date reminder trigger.
	if !auth.RequirePrivilege(r.Context(), w, privilege.ManageRiskHub) {
		return
	}
	if h.trigger == nil {
		response.WriteError(w, http.StatusServiceUnavailable, "likelihood recheck job is not configured")
		return
	}
	if !h.running.CompareAndSwap(false, true) {
		response.WriteError(w, http.StatusConflict, "likelihood recheck job is already running")
		return
	}
	go func() { // #nosec G118 -- deliberately detached from r.Context(): it would cancel this sweep the instant the handler returns 202, well before the up-to-30min run finishes
		defer h.running.Store(false)
		defer func() {
			if p := recover(); p != nil {
				slog.Error("likelihood recheck job: manual trigger panic", "panic", p)
			}
		}()
		if err := h.trigger(context.Background()); err != nil {
			slog.Error("likelihood recheck job: manual trigger failed", "err", err)
		}
	}()
	w.WriteHeader(http.StatusAccepted)
}
