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

package service

import (
	"context"
	"strings"
	"testing"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/repository"
)

// fakeSuggestionRepo records what RecordDecision writes. The embedded
// interface covers the methods these tests never reach.
type fakeSuggestionRepo struct {
	repository.SuggestionRepository
	created []model.CreateSuggestionRequest
	decided []model.DecideSuggestionRequest
}

func (f *fakeSuggestionRepo) Create(_ context.Context, req model.CreateSuggestionRequest) (int, error) {
	f.created = append(f.created, req)
	return len(f.created), nil
}

func (f *fakeSuggestionRepo) Decide(_ context.Context, _ int, req model.DecideSuggestionRequest, _ string) error {
	f.decided = append(f.decided, req)
	return nil
}

func (f *fakeSuggestionRepo) ListByRisk(context.Context, int, string, string) ([]model.Suggestion, error) {
	return nil, nil
}

func TestNormaliseSuggestionConfidence(t *testing.T) {
	for in, want := range map[string]string{"high": "HIGH", " Medium ": "MEDIUM", "LOW": "LOW"} {
		got, err := normaliseSuggestionConfidence(in)
		if err != nil || got != want {
			t.Errorf("normaliseSuggestionConfidence(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "certain", "HIGH; DROP TABLE"} {
		if _, err := normaliseSuggestionConfidence(in); err == nil {
			t.Errorf("normaliseSuggestionConfidence(%q): expected error", in)
		}
	}
}

func TestCategoryRecordDecision(t *testing.T) {
	good := func() *model.AICategorySuggestion {
		return &model.AICategorySuggestion{CategoryID: 5, Reason: "fits", Confidence: "high"}
	}
	cases := []struct {
		name       string
		suggestion *model.AICategorySuggestion
		final      []int
		wantErr    bool
		wantStatus model.SuggestionStatus
	}{
		{"accepted when final includes it", good(), []int{2, 5}, false, model.SuggestionStatusAccepted},
		{"overridden when final differs", good(), []int{2}, false, model.SuggestionStatusOverridden},
		{"nil suggestion is a no-op", nil, []int{5}, false, ""},
		{"invalid category id", &model.AICategorySuggestion{CategoryID: 0, Confidence: "high"}, []int{5}, true, ""},
		{"invalid confidence", &model.AICategorySuggestion{CategoryID: 5, Confidence: "sure"}, []int{5}, true, ""},
		{"reason too long", &model.AICategorySuggestion{CategoryID: 5, Confidence: "low", Reason: strings.Repeat("x", maxSuggestionReasonLen+1)}, []int{5}, true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeSuggestionRepo{}
			s := &categorySuggestionService{suggestionRepo: repo}
			err := s.RecordDecision(context.Background(), 1, tc.suggestion, tc.final, "user-1")
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantStatus == "" {
				if len(repo.created) != 0 {
					t.Fatalf("expected no row written, got %d", len(repo.created))
				}
				return
			}
			if len(repo.created) != 1 || repo.created[0].SuggestedValue != "5" || repo.created[0].Confidence != "HIGH" {
				t.Fatalf("unexpected created rows: %+v", repo.created)
			}
			if len(repo.decided) != 1 || repo.decided[0].Status != tc.wantStatus {
				t.Fatalf("decided = %+v, want status %s", repo.decided, tc.wantStatus)
			}
		})
	}
}

func TestLikelihoodRecordDecision(t *testing.T) {
	cases := []struct {
		name       string
		suggestion *model.AILikelihoodSuggestion
		final      int
		wantErr    bool
		wantStatus model.SuggestionStatus
	}{
		{"accepted when scores match", &model.AILikelihoodSuggestion{Score: 3, Confidence: "medium"}, 3, false, model.SuggestionStatusAccepted},
		{"overridden when scores differ", &model.AILikelihoodSuggestion{Score: 3, Confidence: "medium"}, 2, false, model.SuggestionStatusOverridden},
		{"score out of range", &model.AILikelihoodSuggestion{Score: 7, Confidence: "medium"}, 2, true, ""},
		{"invalid confidence", &model.AILikelihoodSuggestion{Score: 2, Confidence: ""}, 2, true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeSuggestionRepo{}
			s := &likelihoodSuggestionService{suggestionRepo: repo}
			err := s.RecordDecision(context.Background(), 1, tc.suggestion, tc.final, "user-1")
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				if len(repo.created) != 0 {
					t.Fatalf("expected no row written, got %d", len(repo.created))
				}
				return
			}
			if len(repo.decided) != 1 || repo.decided[0].Status != tc.wantStatus {
				t.Fatalf("decided = %+v, want status %s", repo.decided, tc.wantStatus)
			}
		})
	}
}

func TestActionPlanRecordDecision(t *testing.T) {
	cases := []struct {
		name       string
		suggestion *model.AIActionPlanSuggestion
		wantErr    bool
		wantStatus model.SuggestionStatus
	}{
		{"used is accepted", &model.AIActionPlanSuggestion{Description: "Patch it.", Confidence: "high", Used: true}, false, model.SuggestionStatusAccepted},
		{"not used is overridden", &model.AIActionPlanSuggestion{Description: "Patch it.", Confidence: "high"}, false, model.SuggestionStatusOverridden},
		{"empty description", &model.AIActionPlanSuggestion{Description: "  ", Confidence: "high"}, true, ""},
		{"description too long", &model.AIActionPlanSuggestion{Description: strings.Repeat("x", maxActionPlanSuggestionLen+1), Confidence: "high"}, true, ""},
		{"invalid confidence", &model.AIActionPlanSuggestion{Description: "Patch it.", Confidence: "x"}, true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeSuggestionRepo{}
			s := &actionPlanSuggestionService{suggestionRepo: repo}
			err := s.RecordDecision(context.Background(), 1, tc.suggestion, "user-1")
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				if len(repo.created) != 0 {
					t.Fatalf("expected no row written, got %d", len(repo.created))
				}
				return
			}
			if len(repo.decided) != 1 || repo.decided[0].Status != tc.wantStatus {
				t.Fatalf("decided = %+v, want status %s", repo.decided, tc.wantStatus)
			}
		})
	}
}
