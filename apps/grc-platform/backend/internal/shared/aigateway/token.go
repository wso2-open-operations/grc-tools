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
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// suggestionTokenTTL bounds how long a Suggest response stays redeemable at
// save time — long enough for a realistic Add/Edit Risk editing session,
// short enough that a captured token can't be replayed indefinitely.
const suggestionTokenTTL = 24 * time.Hour

// SuggestionTokenPayload is the signed content behind one suggestion token:
// exactly what Suggest returned, plus which feature it's for and when it was
// minted.
type SuggestionTokenPayload struct {
	Feature    string `json:"feature"`
	Value      string `json:"value"`
	Reason     string `json:"reason"`
	Confidence string `json:"confidence"`
	IssuedAt   int64  `json:"issued_at"`
}

// SignSuggestion mints an opaque, tamper-evident token for one Suggest
// result, returned to the caller alongside the human-readable response. The
// frontend echoes it back unmodified at save time (AICategorySuggestion.Token
// and its Likelihood/ActionPlan equivalents); VerifySuggestion is what
// RecordDecision uses to recover the actual suggested content instead of
// trusting whatever value/reason/confidence a save request claims — a save
// request that instead resent those fields directly could otherwise record
// an "AI suggested this" audit row the model never produced.
//
// HMAC-signed with the AI Gateway API key as the key, not a separate secret:
// that key is already required for this client to do anything, already a
// secret, and already identical across every instance serving the same
// gateway, so this adds no new config, env var, or rotation surface. The
// signature only proves this process (or one sharing its API key) minted the
// token — it isn't meant to keep the payload confidential, so it's base64,
// not encrypted.
func (c *Client) SignSuggestion(feature, value, reason, confidence string) (string, error) {
	payload := SuggestionTokenPayload{
		Feature:    feature,
		Value:      value,
		Reason:     reason,
		Confidence: confidence,
		IssuedAt:   time.Now().Unix(),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("sign suggestion: marshal payload: %w", err)
	}
	sig := c.suggestionMAC(raw)
	return base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// VerifySuggestion checks a token minted by SignSuggestion: well-formed,
// correctly signed, for the expected feature, and not expired. The returned
// payload is the only source RecordDecision should ever treat as "what the
// AI actually suggested."
func (c *Client) VerifySuggestion(token, wantFeature string) (*SuggestionTokenPayload, error) {
	rawPart, sigPart, ok := strings.Cut(token, ".")
	if !ok {
		return nil, fmt.Errorf("verify suggestion: malformed token")
	}
	raw, err := base64.RawURLEncoding.DecodeString(rawPart)
	if err != nil {
		return nil, fmt.Errorf("verify suggestion: decode payload: %w", err)
	}
	sig, err := base64.RawURLEncoding.DecodeString(sigPart)
	if err != nil {
		return nil, fmt.Errorf("verify suggestion: decode signature: %w", err)
	}
	if !hmac.Equal(sig, c.suggestionMAC(raw)) {
		return nil, fmt.Errorf("verify suggestion: signature mismatch")
	}

	var payload SuggestionTokenPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("verify suggestion: unmarshal payload: %w", err)
	}
	if payload.Feature != wantFeature {
		return nil, fmt.Errorf("verify suggestion: token is for feature %q, not %q", payload.Feature, wantFeature)
	}
	if time.Since(time.Unix(payload.IssuedAt, 0)) > suggestionTokenTTL {
		return nil, fmt.Errorf("verify suggestion: token expired")
	}
	return &payload, nil
}

func (c *Client) suggestionMAC(data []byte) []byte {
	mac := hmac.New(sha256.New, []byte(c.apiKey))
	mac.Write(data)
	return mac.Sum(nil)
}
