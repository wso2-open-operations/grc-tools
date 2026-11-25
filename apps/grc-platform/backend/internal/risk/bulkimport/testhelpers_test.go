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

package bulkimport

import (
	"context"
	"strings"
	"testing"
)

// DirectoryUser is a test stand-in for one directory account: the fixtures list
// the people the fake directory knows.
type DirectoryUser struct {
	Email string
	UUID  string
}

// snapshotLookup turns a list of accounts into an EmailLookup. An email listed
// with two different uuids is ambiguous and never resolves, like the real
// lookup.
func snapshotLookup(users []DirectoryUser) EmailLookup {
	uuids := map[string]string{}
	ambiguous := map[string]bool{}
	for _, u := range users {
		key := strings.ToLower(strings.TrimSpace(u.Email))
		if key == "" || u.UUID == "" {
			continue
		}
		if existing, seen := uuids[key]; seen && existing != u.UUID {
			ambiguous[key] = true
			continue
		}
		uuids[key] = u.UUID
	}
	return func(_ context.Context, email string) (string, bool, error) {
		key := strings.ToLower(strings.TrimSpace(email))
		if ambiguous[key] {
			return "", false, nil
		}
		uuid, ok := uuids[key]
		return uuid, ok, nil
	}
}

// expectedHeaders is the header row the Managed Services fixtures are written
// against: every required column plus the optional Source Register one, which
// the fixtures fill in.
var expectedHeaders = append([]string{"Source Register"}, requiredHeaders(templateManagedServices)...)

// msTarget is the Managed Services register of rd, as a run would target it.
func msTarget(t *testing.T, rd RefData) Target {
	t.Helper()
	return targetOf(t, rd, templateManagedServices)
}

// targetOf is the first register of rd on template, as a run would target it.
func targetOf(t *testing.T, rd RefData, template string) Target {
	t.Helper()
	best := 0
	for id, tpl := range rd.TeamTemplateByID {
		if tpl == template && rd.TeamCodeByID[id] != "" && (best == 0 || id < best) {
			best = id
		}
	}
	if best == 0 {
		t.Fatalf("no register with a code on template %s in the fixture", template)
	}
	return Target{RegisterID: best, Name: rd.TeamNameByID[best], Template: template}
}
