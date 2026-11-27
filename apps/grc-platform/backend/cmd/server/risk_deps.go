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
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/config"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/directory"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/hrentity"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/bulkimport"
	riskhandler "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/handler"
	riskentity "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/repository/entity"
	riskservice "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/scim"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/adminactivity"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/aigateway"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/applink"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/emailer"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/file"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/grant"
	userentity "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/user/entity"
)

// buildRiskDeps wires the full Risk Hub dependency graph:
// repositories → services → handler Deps struct.
//
// Every repository is served by the Compliance Entity; the Risk Hub holds no
// database handle. fileSvc is the shared Azure Blob service used by evidence
// uploads, and hrClient talks to the HR entity's GraphQL service for employee
// lookups — neither is backed by the GRC platform's own database either.
// emailCfg wires the risk-owner notification email sent on risk creation.
// grantRepo powers the Risk Owner / Management Approver pickers (see
// riskhandler.Deps.Grants) — nil in local dev, when no privilege store is
// configured.
//
// Dashboard and analytics take their payload already assembled: the entity runs
// the aggregate queries and the pivots, so these services pass it through,
// mirroring the audit module.
func buildRiskDeps(
	ec *entityclient.Client,
	fileSvc *file.Service,
	hrClient *hrentity.Client,
	grantRepo grant.Repository,
	dirSvc *directory.Service,
	scimClient *scim.Client,
	emailCfg config.EmailConfig,
	leadEscalationEmails bool,
	activityLog *adminactivity.Client,
	aiGatewayCfg config.AIGatewayConfig,
) riskhandler.Deps {
	userRepo := userentity.NewRepository(ec)
	actionPlanRepo := riskentity.NewActionPlanRepository(ec)
	riskRepo := riskentity.NewRiskRepository(ec)
	riskCategoryRepo := riskentity.NewRiskCategoryRepository(ec)
	riskTeamRepo := riskentity.NewTeamRepository(ec)
	complianceRepo := riskentity.NewComplianceReferenceRepository(ec)
	suggestionRepo := riskentity.NewSuggestionRepository(ec)
	gatewayClient := aigateway.New(aiGatewayCfg.BaseURL, aiGatewayCfg.APIKey)
	categorySuggestionSvc := riskservice.NewCategorySuggestionService(
		gatewayClient,
		riskCategoryRepo,
		complianceRepo,
		suggestionRepo,
	)
	likelihoodSuggestionSvc := riskservice.NewLikelihoodSuggestionService(
		gatewayClient,
		riskCategoryRepo,
		riskTeamRepo,
		complianceRepo,
		suggestionRepo,
	)
	actionPlanSuggestionSvc := riskservice.NewActionPlanSuggestionService(
		gatewayClient,
		riskCategoryRepo,
		complianceRepo,
		suggestionRepo,
	)
	return riskhandler.Deps{
		BulkImport:                  newBulkImporter(ec, scimClient),
		Risk:                        riskservice.NewRiskService(riskRepo, actionPlanRepo),
		Assessment:                  riskservice.NewRiskAssessmentService(riskentity.NewAssessmentRepository(ec)),
		Team:                        riskservice.NewTeamService(riskTeamRepo),
		Score:                       riskservice.NewRiskScoreService(riskentity.NewRiskScoreRepository(ec)),
		Category:                    riskservice.NewRiskCategoryService(riskCategoryRepo),
		CategorySuggestion:          categorySuggestionSvc,
		CategorySuggestionEnabled:   aiGatewayCfg.CategorizationEnabled,
		LikelihoodSuggestion:        likelihoodSuggestionSvc,
		LikelihoodSuggestionEnabled: aiGatewayCfg.LikelihoodEnabled,
		ActionPlanSuggestion:        actionPlanSuggestionSvc,
		ActionPlanSuggestionEnabled: aiGatewayCfg.ActionPlanEnabled,
		Platforms:                   riskservice.NewLookupService(riskentity.NewLookupRepository(ec, "/risk/platforms")),
		Customers:                   riskservice.NewLookupService(riskentity.NewLookupRepository(ec, "/risk/customers")),
		Products:                    riskservice.NewLookupService(riskentity.NewLookupRepository(ec, "/risk/products")),
		DeploymentTypes:             riskservice.NewLookupService(riskentity.NewLookupRepository(ec, "/risk/deployment-types")),
		ActionPlan:                  riskservice.NewActionPlanService(actionPlanRepo, userRepo),
		Evidence:                    riskservice.NewEvidenceService(riskentity.NewRiskEvidenceRepository(ec), riskRepo, actionPlanRepo, fileSvc),
		History:                     riskservice.NewHistoryService(riskentity.NewHistoryRepository(ec)),
		Escalation:                  riskservice.NewEscalationService(riskentity.NewEscalationRepository(ec), riskRepo, actionPlanRepo, userRepo, hrClient, scimClient, dirSvc),
		Compliance:                  riskservice.NewComplianceReferenceService(complianceRepo),
		Analytics:                   riskservice.NewAssembledAnalyticsService(riskentity.NewAnalyticsRepository(ec)),
		Dashboard:                   riskservice.NewAssembledDashboardService(riskentity.NewDashboardRepository(ec)),
		Employee:                    riskservice.NewEmployeeSearchService(hrClient),
		Users:                       userRepo,
		HREntity:                    hrClient,
		SCIM:                        scimClient,
		Grants:                      grantRepo,
		Directory:                   dirSvc,
		Email:                       emailer.New(emailCfg.ServiceURL, emailCfg.FromAddress, emailCfg.TokenURL, emailCfg.ClientID, emailCfg.ClientSecret, emailCfg.Enabled),
		FrontendBaseURL:             emailCfg.OneWSO2WebappURL + applink.OneWSO2SecurityPath,
		LeadEscalationEmails:        leadEscalationEmails,
		ActivityLog:                 activityLog,
	}
}

// bulkImportHTTPTimeout is the per-request timeout of the import's own entity
// client. Longer than the shared client's: the import writes a risk with its
// children in one request, and the entity can be slow under a large run.
const bulkImportHTTPTimeout = 90 * time.Second

// newBulkImporter wires the Compliance Admin's bulk risk import. It returns nil —
// which the handlers answer with 503 — when no directory is configured, because
// the import resolves every person in the sheet through it and, without one,
// would reject every row.
//
// The import keeps its own client on the entity's URL rather than sharing ec: it
// needs each response's raw status and body to tell a row-level refusal from an
// outage, which ec folds into a message.
func newBulkImporter(ec *entityclient.Client, scimClient *scim.Client) riskhandler.BulkImporter {
	if scimClient == nil {
		return nil
	}
	lookup := func(ctx context.Context, email string) (string, bool, error) {
		u, err := scimClient.LookupByEmail(ctx, email)
		if err != nil {
			return "", false, err
		}
		// LookupByEmail answers (nil, nil) for no match and for an ambiguous one.
		// A disabled account is no account: nobody who can no longer sign in may be
		// named as a risk's owner.
		if u == nil || !strings.EqualFold(strings.TrimSpace(u.Email), strings.TrimSpace(email)) || u.State == scim.AccountDisabled {
			return "", false, nil
		}
		return u.UUID, true, nil
	}
	return bulkimport.NewService(bulkimport.NewEntityClient(ec.BaseURL(), bulkImportHTTPTimeout), lookup, slog.Default())
}
