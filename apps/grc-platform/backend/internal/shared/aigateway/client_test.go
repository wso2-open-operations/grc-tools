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

package aigateway

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseJSONObject_Clean(t *testing.T) {
	out, err := parseJSONObject[suggestLikelihoodOutput](`{"score": 2, "reason": "EPSS 0.3", "confidence": "medium"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Score != 2 || out.Reason != "EPSS 0.3" || out.Confidence != "medium" {
		t.Errorf("got %+v", out)
	}
}

func TestParseJSONObject_MarkdownFence(t *testing.T) {
	out, err := parseJSONObject[suggestCategoryOutput]("```json\n{\"category_id\": 5, \"reason\": \"x\", \"confidence\": \"high\"}\n```")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.CategoryID != 5 {
		t.Errorf("got %+v", out)
	}
}

func TestParseJSONObject_LeadingChatterFromToolUse(t *testing.T) {
	// The shape SuggestLikelihood's tool-enabled responses can arrive in: the
	// model explains itself before giving the final JSON, despite being told
	// not to — this is the case parseJSONObject's fallback exists for.
	text := "Based on my search, I found that CVE-2024-1234 is listed in CISA KEV.\n\n" +
		`{"score": 3, "reason": "Listed in CISA KEV", "confidence": "high"}`
	out, err := parseJSONObject[suggestLikelihoodOutput](text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Score != 3 || out.Confidence != "high" {
		t.Errorf("got %+v", out)
	}
}

func TestParseJSONObject_TrailingChatter(t *testing.T) {
	text := `{"score": 1, "reason": "No public exploit", "confidence": "low"}` +
		"\n\nFor a complete list, see https://www.cisa.gov/known-exploited-vulnerabilities."
	out, err := parseJSONObject[suggestLikelihoodOutput](text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Score != 1 {
		t.Errorf("got %+v", out)
	}
}

func TestParseJSONObject_NoJSON(t *testing.T) {
	_, err := parseJSONObject[suggestLikelihoodOutput]("I couldn't find any evidence for this risk.")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}

func TestLastJSONObject_MultipleObjectsPicksLast(t *testing.T) {
	text := `ignore {"not": "this one"} and use {"score": 2, "reason": "r", "confidence": "medium"}`
	got, ok := lastJSONObject(text)
	if !ok {
		t.Fatal("expected to find a JSON object")
	}
	want := `{"score": 2, "reason": "r", "confidence": "medium"}`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLastJSONObject_NestedBraces(t *testing.T) {
	text := `prefix {"outer": {"inner": "value"}, "score": 2} suffix`
	got, ok := lastJSONObject(text)
	if !ok {
		t.Fatal("expected to find a JSON object")
	}
	want := `{"outer": {"inner": "value"}, "score": 2}`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLikelihoodPromptNamesOnlyVerifiedSources(t *testing.T) {
	p := buildLikelihoodPrompt(SuggestLikelihoodRequest{Title: "t", Description: "d"})
	for _, s := range evidenceSources {
		if !strings.Contains(p, s.FeedURL) {
			t.Errorf("prompt missing feed URL %s", s.FeedURL)
		}
	}
	for _, banned := range []string{"CISA KEV, NVD, EPSS, vendor advisories", "EPSS >= 0.5"} {
		if strings.Contains(p, banned) {
			t.Errorf("prompt still references unverified source text %q", banned)
		}
	}
}

func TestLikelihoodToolsRestrictedToEvidenceDomains(t *testing.T) {
	want := evidenceDomains()
	for _, tool := range likelihoodTools {
		got, ok := tool["allowed_domains"].([]string)
		if !ok || !reflect.DeepEqual(got, want) {
			t.Errorf("%v: allowed_domains = %v, want %v", tool["name"], tool["allowed_domains"], want)
		}
	}
	seen := map[string]bool{}
	for _, d := range want {
		if seen[d] {
			t.Errorf("duplicate domain %s", d)
		}
		seen[d] = true
	}
}
