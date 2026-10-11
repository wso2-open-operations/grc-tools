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
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSignVerifySuggestion_RoundTrip(t *testing.T) {
	c := &Client{apiKey: "test-key"}

	token, err := c.SignSuggestion("CATEGORY", "5", "matches the compliance ref", "high", 42, "user-1")
	if err != nil {
		t.Fatalf("SignSuggestion: %v", err)
	}

	got, err := c.VerifySuggestion(token, "CATEGORY", 42, "user-1")
	if err != nil {
		t.Fatalf("VerifySuggestion: %v", err)
	}
	if got.Value != "5" || got.Reason != "matches the compliance ref" || got.Confidence != "high" {
		t.Errorf("got %+v", got)
	}
}

func TestVerifySuggestion_WrongRiskFails(t *testing.T) {
	c := &Client{apiKey: "test-key"}

	token, err := c.SignSuggestion("CATEGORY", "5", "r", "high", 42, "user-1")
	if err != nil {
		t.Fatalf("SignSuggestion: %v", err)
	}
	if _, err := c.VerifySuggestion(token, "CATEGORY", 99, "user-1"); err == nil {
		t.Fatal("expected risk-mismatch error, got nil")
	}
}

func TestVerifySuggestion_UnknownRiskAtSignTimeIsNotChecked(t *testing.T) {
	c := &Client{apiKey: "test-key"}

	// riskID 0 at sign time means the risk didn't exist yet (Add Risk);
	// RecordDecision always has a real id by then, and it must still pass.
	token, err := c.SignSuggestion("CATEGORY", "5", "r", "high", 0, "user-1")
	if err != nil {
		t.Fatalf("SignSuggestion: %v", err)
	}
	if _, err := c.VerifySuggestion(token, "CATEGORY", 42, "user-1"); err != nil {
		t.Fatalf("VerifySuggestion: %v", err)
	}
}

func TestVerifySuggestion_WrongRequesterFails(t *testing.T) {
	c := &Client{apiKey: "test-key"}

	token, err := c.SignSuggestion("CATEGORY", "5", "r", "high", 42, "user-1")
	if err != nil {
		t.Fatalf("SignSuggestion: %v", err)
	}
	if _, err := c.VerifySuggestion(token, "CATEGORY", 42, "user-2"); err == nil {
		t.Fatal("expected requester-mismatch error, got nil")
	}
}

func TestVerifySuggestion_WrongKeyFails(t *testing.T) {
	signer := &Client{apiKey: "signer-key"}
	verifier := &Client{apiKey: "different-key"}

	token, err := signer.SignSuggestion("CATEGORY", "5", "r", "high", 42, "user-1")
	if err != nil {
		t.Fatalf("SignSuggestion: %v", err)
	}
	if _, err := verifier.VerifySuggestion(token, "CATEGORY", 42, "user-1"); err == nil {
		t.Fatal("expected signature mismatch error, got nil")
	}
}

func TestVerifySuggestion_TamperedPayloadFails(t *testing.T) {
	c := &Client{apiKey: "test-key"}

	token, err := c.SignSuggestion("CATEGORY", "5", "r", "high", 42, "user-1")
	if err != nil {
		t.Fatalf("SignSuggestion: %v", err)
	}
	rawPart, sigPart, _ := strings.Cut(token, ".")
	raw, _ := base64.RawURLEncoding.DecodeString(rawPart)

	var payload SuggestionTokenPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	payload.Value = "999" // attacker tries to claim a different suggested category
	tampered, _ := json.Marshal(payload)
	forged := base64.RawURLEncoding.EncodeToString(tampered) + "." + sigPart

	if _, err := c.VerifySuggestion(forged, "CATEGORY", 42, "user-1"); err == nil {
		t.Fatal("expected signature mismatch error for tampered payload, got nil")
	}
}

func TestVerifySuggestion_WrongFeatureFails(t *testing.T) {
	c := &Client{apiKey: "test-key"}

	token, err := c.SignSuggestion("CATEGORY", "5", "r", "high", 42, "user-1")
	if err != nil {
		t.Fatalf("SignSuggestion: %v", err)
	}
	if _, err := c.VerifySuggestion(token, "LIKELIHOOD", 42, "user-1"); err == nil {
		t.Fatal("expected feature-mismatch error, got nil")
	}
}

func TestVerifySuggestion_ExpiredFails(t *testing.T) {
	c := &Client{apiKey: "test-key"}

	payload := SuggestionTokenPayload{
		Feature:    "CATEGORY",
		Value:      "5",
		Reason:     "r",
		Confidence: "high",
		IssuedAt:   time.Now().Add(-suggestionTokenTTL - time.Minute).Unix(),
	}
	raw, _ := json.Marshal(payload)
	sig := c.suggestionMAC(raw)
	token := base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(sig)

	if _, err := c.VerifySuggestion(token, "CATEGORY", 42, "user-1"); err == nil {
		t.Fatal("expected expired-token error, got nil")
	}
}

func TestVerifySuggestion_MalformedTokenFails(t *testing.T) {
	c := &Client{apiKey: "test-key"}
	if _, err := c.VerifySuggestion("not-a-valid-token", "CATEGORY", 42, "user-1"); err == nil {
		t.Fatal("expected malformed-token error, got nil")
	}
}
