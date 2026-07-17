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

package middleware

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
)

// bucket is a single caller's token bucket. tokens is refilled continuously at
// RateLimiter.rate up to RateLimiter.burst.
type bucket struct {
	tokens float64
	last   time.Time // last refill time
	seen   time.Time // last access time, for idle eviction
}

// RateLimiter is a per-principal token-bucket limiter for the evidence-app route
// group. It provides per-caller fairness behind the Choreo gateway's perimeter
// throttling (design §4D). Per-replica in-memory state is acceptable: it is a
// fairness control, not the only DoS defense. Keyed by authenticated email, with
// client IP as a fallback for unauthenticated edge cases.
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rate    float64 // tokens per second
	burst   float64 // bucket capacity
}

// NewRateLimiter builds a limiter allowing ratePerSec sustained requests with the
// given burst capacity per caller, and starts a background idle-eviction sweep.
func NewRateLimiter(ratePerSec, burst float64) *RateLimiter {
	rl := &RateLimiter{
		buckets: make(map[string]*bucket),
		rate:    ratePerSec,
		burst:   burst,
	}
	go rl.evictLoop()
	return rl
}

// allow consumes one token for key. It returns whether the request is allowed and,
// when denied, the suggested Retry-After duration until a token is available.
func (rl *RateLimiter) allow(key string, now time.Time) (bool, time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, ok := rl.buckets[key]
	if !ok {
		b = &bucket{tokens: rl.burst, last: now}
		rl.buckets[key] = b
	}
	// Refill based on elapsed time, capped at burst.
	b.tokens += now.Sub(b.last).Seconds() * rl.rate
	if b.tokens > rl.burst {
		b.tokens = rl.burst
	}
	b.last = now
	b.seen = now

	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	// Seconds until the bucket accrues one whole token.
	retry := time.Duration((1-b.tokens)/rl.rate*float64(time.Second)) + time.Second
	return false, retry
}

// evictLoop removes buckets that have been idle for over 10 minutes so the map
// does not grow unbounded across many distinct callers.
func (rl *RateLimiter) evictLoop() {
	t := time.NewTicker(10 * time.Minute)
	defer t.Stop()
	for range t.C {
		cutoff := time.Now().Add(-10 * time.Minute)
		rl.mu.Lock()
		for k, b := range rl.buckets {
			if b.seen.Before(cutoff) {
				delete(rl.buckets, k)
			}
		}
		rl.mu.Unlock()
	}
}

// Wrap applies the limiter to a single handler. Exceeding the limit returns 429
// with a Retry-After header. Use it per evidence-app route (see routes.go).
func (rl *RateLimiter) Wrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		allowed, retry := rl.allow(limitKey(r), time.Now())
		if !allowed {
			secs := int(retry.Seconds())
			if secs < 1 {
				secs = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(secs))
			response.WriteError(w, http.StatusTooManyRequests, "Rate limit exceeded. Please retry later.")
			return
		}
		next.ServeHTTP(w, r)
	}
}

// limitKey identifies the caller: the authenticated email when present, otherwise
// the client IP (first X-Forwarded-For hop, else RemoteAddr).
func limitKey(r *http.Request) string {
	if info := UserInfoFromContext(r.Context()); info != nil && info.Email != "" {
		return "email:" + info.Email
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if first := strings.TrimSpace(strings.Split(xff, ",")[0]); first != "" {
			return "ip:" + first
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return "ip:" + host
	}
	return "ip:" + r.RemoteAddr
}
