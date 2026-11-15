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
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/directory"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/dashboardagent"
)

// directoryPeople lets the dashboard agent look people up in the company
// directory. It searches the snapshot the platform already keeps warm, so a
// lookup makes no outside call.
type directoryPeople struct{ dir *directory.Service }

// NewDirectoryPeople adapts the directory for the dashboard agent. A nil
// directory finds nobody.
func NewDirectoryPeople(dir *directory.Service) dashboardagent.PeopleDirectory {
	return directoryPeople{dir: dir}
}

func (d directoryPeople) SearchPeople(query string) []dashboardagent.Person {
	if d.dir == nil {
		return nil
	}
	found := d.dir.SearchDomain(query)
	out := make([]dashboardagent.Person, 0, len(found))
	for _, p := range found {
		out = append(out, dashboardagent.Person{UUID: p.UUID, Name: p.DisplayName, Email: p.Email})
	}
	return out
}
