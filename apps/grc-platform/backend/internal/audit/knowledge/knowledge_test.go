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

package knowledge

import (
	"strings"
	"testing"
)

func titles(ss []Section) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = s.Title
	}
	return out
}

func has(ss []Section, title string) bool {
	for _, s := range ss {
		if s.Title == title {
			return true
		}
	}
	return false
}

func TestEverySectionIsUsable(t *testing.T) {
	all := All()
	if len(all) < 5 {
		t.Fatalf("only %d sections loaded", len(all))
	}
	for _, s := range all {
		if s.Title == "" || strings.TrimSpace(s.Text) == "" || s.Source == "" {
			t.Errorf("incomplete section %+v", s)
		}
		if len(s.Text) > maxSectionLen {
			t.Errorf("%q is %d bytes, over the %d cap", s.Title, len(s.Text), maxSectionLen)
		}
	}
	if !has(all, "Where things are in the Audit Hub") {
		t.Error("the always-included map of the Audit Hub is missing")
	}
}

func TestRelevantFindsEvidenceSteps(t *testing.T) {
	for _, q := range []string{"how do I submit evidence for a control", "where do I upload evidence"} {
		got := Relevant(q, 3)
		if !has(got, "Where things are in the Audit Hub") || !has(got, "How to submit evidence") {
			t.Errorf("%q: got %v", q, titles(got))
		}
	}
}

func TestRelevantFindsHowToCreateAnAudit(t *testing.T) {
	if got := Relevant("how do I create a new audit", 3); !has(got, "How to create an audit") {
		t.Errorf("got %v", titles(got))
	}
}

func TestUnrelatedQuestionsGetOnlyTheMap(t *testing.T) {
	if got := Relevant("what is the weather", 3); len(got) != 1 {
		t.Errorf("got %v", titles(got))
	}
}
