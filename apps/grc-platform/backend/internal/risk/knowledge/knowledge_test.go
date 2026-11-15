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
	if !has(all, "Where things are in the Risk Hub") {
		t.Error("the always-included map of the Risk Hub is missing")
	}
}

func TestRelevantFindsHowToAddARisk(t *testing.T) {
	for _, q := range []string{
		"where can I add a risk",
		"How do I add a new risk?",
		"how to raise a risk",
	} {
		got := Relevant(q, 3)
		if !has(got, "How to add a risk") {
			t.Errorf("%q did not find the add-a-risk steps: %v", q, titles(got))
		}
		if !has(got, "Where things are in the Risk Hub") {
			t.Errorf("%q: the map of the Risk Hub should always come with an answer", q)
		}
	}
}

func TestRelevantFindsOtherTopics(t *testing.T) {
	cases := map[string]string{
		"what does pending revision mean":     "How a risk is approved",
		"who approves a high risk":            "How a risk is approved",
		"how do I download a report":          "Dashboard and analytics",
		"how do I find a risk and open it":    "How to find a risk and open its details",
		"why can't I see the escalate button": "What you can do on a risk",
	}
	for q, want := range cases {
		if got := Relevant(q, 3); !has(got, want) {
			t.Errorf("%q: expected %q, got %v", q, want, titles(got))
		}
	}
}

func TestRelevantStaysSmallAndStable(t *testing.T) {
	got := Relevant("risk", 2)
	// the always-included section plus at most 2 more
	if len(got) > 3 {
		t.Fatalf("asked for 2 more, got %v", titles(got))
	}
	again := Relevant("risk", 2)
	for i := range got {
		if got[i].Title != again[i].Title {
			t.Fatal("the same question must give the same sections in the same order")
		}
	}
	// A question with nothing in common brings only the always-included map.
	if only := Relevant("banana smoothie recipe", 3); len(only) != 1 || only[0].Title != "Where things are in the Risk Hub" {
		t.Fatalf("unrelated question returned %v", titles(only))
	}
	if none := Relevant("", 3); len(none) != 1 {
		t.Fatalf("an empty question returned %v", titles(none))
	}
}

func TestParseSplitsAtHeadings(t *testing.T) {
	got := parse("Doc", "intro before any heading\n\n# One\nfirst\nline\n\n## Two\n\n## Three\nthird\n")
	if len(got) != 2 || got[0].Title != "One" || got[0].Text != "first\nline" || got[1].Title != "Three" {
		t.Fatalf("%+v", got)
	}
	long := strings.Repeat("a line of text\n", 400)
	cut := parse("Doc", "# Long\n"+long)
	if len(cut) != 1 || len(cut[0].Text) > maxSectionLen || strings.HasSuffix(cut[0].Text, "a li") {
		t.Fatalf("a long section was not cut at a line: %d bytes", len(cut[0].Text))
	}
}

func TestTokensFoldPluralsAndDropFiller(t *testing.T) {
	got := tokens("How do I add the risks?")
	if !got["add"] || !got["risk"] || got["how"] || got["the"] || got["risks"] {
		t.Fatalf("%v", got)
	}
}

func TestReadmeIsNotLoadedAsHelp(t *testing.T) {
	for _, s := range All() {
		if strings.Contains(s.Text, "compiled into the backend") {
			t.Fatalf("README content leaked into the help text: %q", s.Title)
		}
	}
}
