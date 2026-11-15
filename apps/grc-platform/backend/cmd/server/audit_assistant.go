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

package main

import (
	audithandler "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/handler"
	auditentity "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository/entity"
	auditservice "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/config"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/directory"
	riskservice "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/aigateway"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
)

// wireAuditAssistant adds the Audit Hub assistant to the audit dependencies. It
// stays off (its endpoints answer 404) unless AI_AUDIT_DASHBOARD_AGENT_ENABLED is
// true. The assistant reads through the Compliance Entity like the rest of the
// module, and finds people in the same company directory the Risk Hub uses.
func wireAuditAssistant(deps *audithandler.Deps, ec *entityclient.Client, dirSvc *directory.Service, cfg config.AIGatewayConfig) {
	deps.DashboardAgent = auditservice.NewDashboardAgentService(
		auditentity.NewWidgetQueryRepository(ec),
		aigateway.New(cfg.BaseURL, cfg.APIKey),
		riskservice.NewDirectoryPeople(dirSvc),
	)
	deps.DashboardAgentEnabled = cfg.AuditDashboardAgentEnabled
}
