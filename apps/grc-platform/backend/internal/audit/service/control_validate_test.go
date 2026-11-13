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
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
)

func addReq(requirementType string, dueDate *string) model.AddControlRequest {
	ev := "evidence"
	req := model.AddControlRequest{
		ControlNumber: "CA-1", Description: "d", EvidenceRequirement: &ev, RequirementType: requirementType,
		ControlType: "CONFIG", Scope: "COMMON", DueDate: dueDate,
	}
	if requirementType == "OE" {
		pop := "2026-12-01"
		req.Population = &model.PopulationDetails{DueDate: &pop}
	}
	return req
}

// A single Design control may omit its due date, so the entity can default it
// from the audit's Owner initial days. OE controls always need one.
func TestValidateAddRequestDueDate(t *testing.T) {
	date := "2026-12-31"
	cases := map[string]struct {
		req     model.AddControlRequest
		wantErr bool
	}{
		"design without a date": {addReq("DESIGN", nil), false},
		"design with a date":    {addReq("DESIGN", &date), false},
		"oe without a date":     {addReq("OE", nil), true},
		"oe with a date":        {addReq("OE", &date), false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := validateAddRequest(tc.req)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

// Every row of a bulk add carries its own date, Design or not.
func TestBulkAddStillRequiresADueDate(t *testing.T) {
	svc := &controlService{}
	_, err := svc.BulkAdd(context.Background(), 7, []model.AddControlRequest{addReq("DESIGN", nil)}, "admin")
	var apiErr *apierror.Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected a 422, got %v", err)
	}
}
