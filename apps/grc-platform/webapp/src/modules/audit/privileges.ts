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

// Audit Hub privilege name constants.
// Values must match privilege_name in the privilege table and the constants in
// backend/internal/shared/privilege/privilege.go exactly.
export const AuditPrivilege = {
  ViewAudits:           "VIEW_AUDITS",
  CreateAudit:          "CREATE_AUDIT",
  UpdateAudit:          "UPDATE_AUDIT",
  MoveAuditToFieldwork: "MOVE_AUDIT_TO_FIELDWORK",
  SubmitAuditForReview: "SUBMIT_AUDIT_FOR_REVIEW",
  CompleteAudit:        "COMPLETE_AUDIT",
  ManageControls:       "MANAGE_CONTROLS",
  SubmitEvidence:       "SUBMIT_EVIDENCE",
  ReviewEvidence:       "REVIEW_EVIDENCE",
  ManagePopulation:     "MANAGE_POPULATION",
  AddComment:           "ADD_COMMENT",
  ManageAssignments:    "MANAGE_ASSIGNMENTS",
  ViewTrail:            "VIEW_TRAIL",
  ManageFrameworks:     "MANAGE_FRAMEWORKS",
  ManageUsers:          "MANAGE_USERS",
  ExportReport:         "EXPORT_REPORT",
} as const;
