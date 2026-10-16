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

package service

import (
	"testing"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
)

func TestCancellable(t *testing.T) {
	approved, empty := "2026-09-01T10:00:00Z", ""
	cases := []struct {
		name     string
		status   string
		approved *string
		want     bool
	}{
		{"awaiting first owner approval", model.StatusPendingOwnerApproval, nil, true},
		{"rejected before any approval", model.StatusPendingRevision, nil, true},
		{"rejected before any approval, empty stamp", model.StatusPendingRevision, &empty, true},
		{"rejected after an owner approval", model.StatusPendingRevision, &approved, false},
		{"in remediation", model.StatusApproved, &approved, false},
		{"pending compliance review", model.StatusPendingComplianceReview, &approved, false},
		{"closed", model.StatusClosed, &approved, false},
	}
	for _, tc := range cases {
		if got := cancellable(tc.status, tc.approved); got != tc.want {
			t.Errorf("%s: cancellable(%s) = %v, want %v", tc.name, tc.status, got, tc.want)
		}
	}
}
