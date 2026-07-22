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

package entity

// Unimplemented repositories, kept so the risk module can stop depending on a
// *sql.DB before these features exist.
//
// Escalation, action plans, risk evidence and notifications are all scaffolding
// today: every method below returns errNotImplemented, none of them is routed,
// and their tables are empty. Their MySQL counterparts were identical — stubs
// holding a database handle they never used. Holding an entity client they
// never use instead changes nothing at runtime and removes the last reason for
// these four to keep the backend tied to MySQL.
//
// Each is scheduled to be built properly as its own change. When that happens,
// the implementation belongs here, against the Compliance Entity, not against
// MySQL. Notes for whoever picks them up:
//
//   - Escalation: the entity already implements this in full — repository,
//     service, handler and four routes under /risks/{riskId}/escalations.
//   - Action plans: the entity has action-plan and step endpoints, but the
//     working code today lives inside the risk create/update transactions;
//     reconcile the two before adding standalone endpoints, or a plan will be
//     editable through two paths with different rules.
//   - Evidence: the entity's delete is `DELETE FROM risk_evidence WHERE id = ?`
//     with no risk_id scoping, so any file id can be deleted regardless of which
//     risk owns it. The MySQL TODO called for `AND risk_id = ?`. Fix that first.
//   - Notifications: nothing exists on the entity side at all; this one is a
//     feature to design, not a migration.

import (
	"context"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
)

// ── Escalation ───────────────────────────────────────────────────────────────

type escalationRepository struct{ c *entityclient.Client }

// NewEscalationRepository returns an unimplemented escalation repository.
func NewEscalationRepository(c *entityclient.Client) repository.EscalationRepository {
	return &escalationRepository{c: c}
}

func (r *escalationRepository) List(ctx context.Context, riskID int) ([]*model.Escalation, error) {
	return nil, errNotImplemented
}

// ── Action plans ─────────────────────────────────────────────────────────────

type actionPlanRepository struct{ c *entityclient.Client }

// NewActionPlanRepository returns an unimplemented action plan repository.
func NewActionPlanRepository(c *entityclient.Client) repository.ActionPlanRepository {
	return &actionPlanRepository{c: c}
}

func (r *actionPlanRepository) List(ctx context.Context, riskID int) ([]*model.ActionPlan, error) {
	return nil, errNotImplemented
}

func (r *actionPlanRepository) GetByID(ctx context.Context, planID int) (*model.ActionPlan, error) {
	return nil, errNotImplemented
}

func (r *actionPlanRepository) Create(ctx context.Context, riskID int, req model.CreateActionPlanRequest, createdBy string) (*model.ActionPlan, error) {
	return nil, errNotImplemented
}

func (r *actionPlanRepository) Update(ctx context.Context, planID int, req model.UpdateActionPlanRequest, updatedBy string) error {
	return errNotImplemented
}

func (r *actionPlanRepository) ListSteps(ctx context.Context, planID int) ([]*model.ActionPlanStep, error) {
	return nil, errNotImplemented
}

func (r *actionPlanRepository) AddStep(ctx context.Context, planID, stepNo int, req model.AddActionPlanStepRequest, createdBy string) (*model.ActionPlanStep, error) {
	return nil, errNotImplemented
}

func (r *actionPlanRepository) UpdateStep(ctx context.Context, stepID int, req model.UpdateActionPlanStepRequest, updatedBy string) error {
	return errNotImplemented
}

// ── Risk evidence ────────────────────────────────────────────────────────────

type riskEvidenceRepository struct{ c *entityclient.Client }

// NewRiskEvidenceRepository returns an unimplemented risk evidence repository.
func NewRiskEvidenceRepository(c *entityclient.Client) repository.RiskEvidenceRepository {
	return &riskEvidenceRepository{c: c}
}

func (r *riskEvidenceRepository) List(ctx context.Context, riskID int) ([]*model.RiskEvidence, error) {
	return nil, errNotImplemented
}

func (r *riskEvidenceRepository) Create(ctx context.Context, riskID int, fileName, filePath, note, evidenceType, createdBy string) (*model.RiskEvidence, error) {
	return nil, errNotImplemented
}

func (r *riskEvidenceRepository) Delete(ctx context.Context, riskID, evidenceID int, byUserID string) error {
	return errNotImplemented
}

// ── Notifications ────────────────────────────────────────────────────────────

type notificationRepository struct{ c *entityclient.Client }

// NewNotificationRepository returns an unimplemented notification repository.
func NewNotificationRepository(c *entityclient.Client) repository.NotificationRepository {
	return &notificationRepository{c: c}
}

func (r *notificationRepository) List(ctx context.Context, recipientID int) ([]*model.Notification, error) {
	return nil, errNotImplemented
}

func (r *notificationRepository) MarkRead(ctx context.Context, id, recipientID int) error {
	return errNotImplemented
}
