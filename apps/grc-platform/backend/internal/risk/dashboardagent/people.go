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

package dashboardagent

import (
	"fmt"
	"strings"
)

// MaxPeople is the most people one person filter may name.
const MaxPeople = 5

// Person is someone in the company directory.
type Person struct {
	UUID  string
	Name  string
	Email string
}

// PeopleDirectory looks people up by name or email.
type PeopleDirectory interface {
	// SearchPeople returns everyone whose name or email contains query.
	SearchPeople(query string) []Person
}

// meWords are what a user types to mean themselves.
var meWords = map[string]bool{"me": true, "myself": true, "i": true, "my": true, "mine": true}

// ResolvePeople turns the names in a spec's person filters into identities, and
// returns the spec the query should run with. A name never reaches the query:
// only the identity found for it does.
//
// "me" means the caller. A name that matches nobody, or more than one person, is
// not guessed at: the second return value is then a short message for the user
// ("I couldn't find anyone called ..."), and the spec must not be run. The
// message names only people in the company directory, which every signed-in risk
// user can already search.
//
// A spec with no person filter comes back unchanged.
func ResolvePeople(spec Spec, callerUUID string, dir PeopleDirectory) (Spec, string) {
	ds, ok := lookupDataset(spec.Dataset)
	if !ok {
		return spec, ""
	}
	out := spec
	out.Filters = make([]Filter, len(spec.Filters))
	copy(out.Filters, spec.Filters)

	for i, flt := range out.Filters {
		f, found := ds.field(flt.Field)
		if !found || f.Kind != KindPerson {
			continue
		}
		var uuids []string
		for _, raw := range flt.Values {
			name := strings.TrimSpace(raw)
			ids, problem := resolveOnePerson(name, callerUUID, dir)
			if problem != "" {
				return spec, problem
			}
			for _, id := range ids {
				if !contains(uuids, id) {
					uuids = append(uuids, id)
				}
			}
		}
		if len(uuids) > MaxPeople {
			return spec, "That names too many people. Try asking about a few at a time."
		}
		out.Filters[i] = Filter{Field: flt.Field, Op: flt.Op, Values: uuids}
	}
	return out, ""
}

func resolveOnePerson(name, callerUUID string, dir PeopleDirectory) ([]string, string) {
	if meWords[strings.ToLower(name)] {
		if callerUUID == "" {
			return nil, "I can't tell who you are, so I can't show your risks."
		}
		return []string{callerUUID}, ""
	}
	if dir == nil {
		return nil, "I can't look up people right now."
	}
	matches := dir.SearchPeople(name)
	switch len(matches) {
	case 0:
		return nil, fmt.Sprintf("I couldn't find anyone called “%s”. Try their full name or their email.", name)
	case 1:
		return []string{matches[0].UUID}, ""
	}

	// Several people match. A full name or an exact email settles it; anything
	// else is a question, never a guess.
	for _, p := range matches {
		if strings.EqualFold(p.Name, name) || strings.EqualFold(p.Email, name) {
			return []string{p.UUID}, ""
		}
	}
	if len(matches) > MaxPeople {
		return nil, fmt.Sprintf("Several people match “%s”. Try their full name or their email.", name)
	}
	names := make([]string, 0, len(matches))
	for _, p := range matches {
		names = append(names, p.Name)
	}
	return nil, fmt.Sprintf("More than one person matches “%s”: %s. Which one do you mean?", name, strings.Join(names, ", "))
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
