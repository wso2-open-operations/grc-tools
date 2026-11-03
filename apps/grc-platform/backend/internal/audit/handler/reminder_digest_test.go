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
	"strings"
	"testing"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
)

func mustContain(t *testing.T, body string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(body, w) {
			t.Errorf("email missing %q:\n%s", w, body)
		}
	}
}

func onlyEmail(t *testing.T, s *linkTestServer) string {
	t.Helper()
	got := s.sent()
	if len(got) != 1 {
		t.Fatalf("sent %d emails, want 1", len(got))
	}
	return got[0]
}

// The reminder digest carries what the job put on each item: the audit under
// the control number, the tier, and the unassigned note for a stand-in row.
func TestReminderDigestRendersJobItems(t *testing.T) {
	s := newLinkTestServer(t)
	err := s.deps().SendReminderDigestSync(context.Background(), 1, []model.ReminderItem{
		{AuditID: 7, Type: "REMINDER_OVERDUE", Tier: "Overdue", ControlNumber: "C-1", Description: "Access reviews", DueDate: "2026-09-30", RequirementType: "Population Requirement", AuditName: "SOC 2 2026"},
		{AuditID: 8, Type: "REMINDER_DUE_5", Tier: "Due in 5 days", ControlNumber: "C-2", DueDate: "2026-10-11", RequirementType: "Evidence Requirement", AuditName: "ISO 27001 2026", UnassignedNote: "No owner assigned"},
	})
	if err != nil {
		t.Fatalf("SendReminderDigestSync: %v", err)
	}
	body := onlyEmail(t, s)
	mustContain(t, body,
		"waiting on you and are overdue", // the most urgent tier picks the wording
		"C-1", "SOC 2 2026", "Population Requirement", "Overdue",
		"C-2", "ISO 27001 2026", "Due in 5 days", "No owner assigned",
		`href="https://one.example.com/security/audit/dashboard"`,
	)
}

// An external auditor is now a reminder recipient, so their digest must link
// to the host they can reach.
func TestReminderDigestLinksExternalAuditorToTheirHost(t *testing.T) {
	s := newLinkTestServer(t)
	err := s.deps().SendReminderDigestSync(context.Background(), 2, []model.ReminderItem{
		{AuditID: 7, Type: "REMINDER_DUE_10", Tier: "Due in 10 days", ControlNumber: "C-1", DueDate: "2026-10-16", RequirementType: "Evidence Requirement", AuditName: "SOC 2 2026"},
	})
	if err != nil {
		t.Fatalf("SendReminderDigestSync: %v", err)
	}
	body := onlyEmail(t, s)
	mustContain(t, body, "waiting on you and are due in 10 days", `href="https://grc.example.com/audit/dashboard"`)
	if strings.Contains(body, "one.example.com") {
		t.Error("external auditor's digest links to the internal host")
	}
}

// The admin escalation names who each row is waiting on with the control's
// status beneath, and links each control.
func TestOverdueAdminDigestRendersWaitingOn(t *testing.T) {
	s := newLinkTestServer(t)
	err := s.deps().SendOverdueAdminDigestSync(context.Background(), 1, []model.ReminderItem{
		{AuditID: 7, LinkControlID: 3, Type: "REMINDER_OVERDUE", ControlNumber: "C-1", DueDate: "2026-09-30", RequirementType: "Evidence Requirement", AuditName: "SOC 2 2026", WaitingOn: "Jane Doe (jane@x.com)", Status: "Evidence Pending"},
		{AuditID: 7, LinkControlID: 4, Type: "REMINDER_OVERDUE", ControlNumber: "C-2", DueDate: "2026-09-25", RequirementType: "Population Requirement", AuditName: "SOC 2 2026", WaitingOn: "Unassigned", Status: "Population Pending"},
	})
	if err != nil {
		t.Fatalf("SendOverdueAdminDigestSync: %v", err)
	}
	body := onlyEmail(t, s)
	mustContain(t, body,
		"Audit: SOC 2 2026", "Waiting on",
		"Jane Doe (jane@x.com)", "Evidence Pending",
		"Unassigned", "Population Pending",
		`href="https://one.example.com/security/audit/audits/7?control=3"`,
		`href="https://one.example.com/security/audit/audits/7?control=4"`,
	)
}

// A lead has no audit access: the digest names the owner and each row's audit
// but carries no link.
func TestOverdueLeadDigestNamesOwnerWithoutLinks(t *testing.T) {
	s := newLinkTestServer(t)
	err := s.deps().SendOverdueLeadDigestSync(context.Background(), 1, "lead@x.com", []model.ReminderItem{
		{AuditID: 7, Type: "REMINDER_OVERDUE", ControlNumber: "C-1", DueDate: "2026-09-30", RequirementType: "Evidence Requirement", AuditName: "SOC 2 2026"},
	})
	if err != nil {
		t.Fatalf("SendOverdueLeadDigestSync: %v", err)
	}
	body := onlyEmail(t, s)
	mustContain(t, body, "owned by a member of your team", "Test User (insider@wso2.com)", "C-1", "SOC 2 2026")
	if strings.Contains(body, "<a href") {
		t.Error("lead digest must contain no links")
	}
}

// A recipient who is not active gets nothing, and that is not a send failure —
// otherwise the job would release the claim and retry them every day.
func TestReminderDigestSkipsInactiveRecipient(t *testing.T) {
	s := newLinkTestServer(t)
	d := s.deps()
	d.Users = linkTestUsers{byID: map[int]*model.UserRef{1: {ID: 1, UUID: linkTestInternalUUID, UserType: "INTERNAL", Status: "INACTIVE"}}}
	err := d.SendReminderDigestSync(context.Background(), 1, []model.ReminderItem{
		{AuditID: 7, Type: "REMINDER_OVERDUE", Tier: "Overdue", ControlNumber: "C-1", DueDate: "2026-09-30"},
	})
	if err != nil {
		t.Fatalf("SendReminderDigestSync: %v", err)
	}
	if got := s.sent(); len(got) != 0 {
		t.Errorf("sent %d emails to an inactive recipient, want 0", len(got))
	}
}

// An item reminded only because its status reminds every day is not "due in 10
// days", whatever type it is logged under.
func TestReminderDigestWordsDailyItemsAsAwaitingAction(t *testing.T) {
	s := newLinkTestServer(t)
	err := s.deps().SendReminderDigestSync(context.Background(), 1, []model.ReminderItem{
		{AuditID: 7, Type: "REMINDER_DUE_TODAY", Daily: true, Tier: "Awaiting action", ControlNumber: "C-1", DueDate: "2026-12-01", RequirementType: "Evidence Requirement", AuditName: "SOC 2 2026"},
	})
	if err != nil {
		t.Fatalf("SendReminderDigestSync: %v", err)
	}
	body := onlyEmail(t, s)
	mustContain(t, body, "The following item(s) are waiting on you.", "Awaiting action", "C-1")
	if strings.Contains(body, "due today") {
		t.Error("a daily item was worded as due today")
	}
}

// A real tier in the same digest still sets the wording over daily items.
func TestReminderDigestTierBeatsDailyItems(t *testing.T) {
	s := newLinkTestServer(t)
	err := s.deps().SendReminderDigestSync(context.Background(), 1, []model.ReminderItem{
		{AuditID: 7, Type: "REMINDER_DUE_TODAY", Daily: true, Tier: "Awaiting action", ControlNumber: "C-1", DueDate: "2026-12-01", RequirementType: "Evidence Requirement", AuditName: "SOC 2 2026"},
		{AuditID: 7, Type: "REMINDER_DUE_5", Tier: "Due in 5 days", ControlNumber: "C-2", DueDate: "2026-11-08", RequirementType: "Evidence Requirement", AuditName: "SOC 2 2026"},
	})
	if err != nil {
		t.Fatalf("SendReminderDigestSync: %v", err)
	}
	mustContain(t, onlyEmail(t, s), "waiting on you and are due in 5 days")
}
