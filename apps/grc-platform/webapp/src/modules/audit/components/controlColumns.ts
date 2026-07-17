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

// Static column catalogue for the controls table show/hide picker. Kept in a
// standalone module (not in ControlsTable.tsx) so the component file only exports
// components — required by the react-refresh/only-export-components lint rule.
// Keep ids in sync with the `columns` array built inside ControlsTable.

export interface ControlColumnMeta {
  id: string;
  label: string;
  alwaysVisible?: boolean;
  defaultHidden?: boolean;
}

export const CONTROL_COLUMNS: ControlColumnMeta[] = [
  { id: "controlNumber", label: "Control No.", alwaysVisible: true },
  { id: "description", label: "Description" },
  { id: "requirementType", label: "Req. Type" },
  { id: "controlType", label: "Control Type" },
  { id: "status", label: "Status" },
  { id: "auditorName", label: "Auditor POC" },
  { id: "ownerName", label: "Process Owner" },
  { id: "teamName", label: "Team" },
  { id: "scope", label: "Scope" },
  { id: "dueDate", label: "Due Date" },
  { id: "populationDueDate", label: "Population Due Date", defaultHidden: true },
  { id: "populationOwnerName", label: "Population Owner", defaultHidden: true },
  { id: "populationTeamName", label: "Population Team", defaultHidden: true },
];

export const DEFAULT_VISIBLE_CONTROL_COLUMNS = CONTROL_COLUMNS.filter((c) => !c.defaultHidden).map((c) => c.id);
export const CONTROL_COLUMNS_STORAGE_KEY = "audit.controlsTable.visibleColumns";
