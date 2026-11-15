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
	"sync"
	"time"
)

// Limiter caps how many requests one user may make in a sliding window. It is
// per process, which is enough to stop one account from running up queries or
// model cost; the gateway's own limits sit in front of it.
type Limiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	hits   map[string][]time.Time
	now    func() time.Time
}

// NewLimiter allows max requests per key in each window.
func NewLimiter(max int, window time.Duration) *Limiter {
	return &Limiter{max: max, window: window, hits: map[string][]time.Time{}, now: time.Now}
}

// Allow records a request for key and reports whether it is within the limit.
// A refused request is not recorded, so it does not extend the wait.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	cutoff := now.Add(-l.window)
	recent := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	if len(recent) >= l.max {
		l.hits[key] = recent
		return false
	}
	l.hits[key] = append(recent, now)

	// Keep the map from growing with every user who ever asked.
	if len(l.hits) > 10000 {
		for k, ts := range l.hits {
			if len(ts) == 0 || !ts[len(ts)-1].After(cutoff) {
				delete(l.hits, k)
			}
		}
	}
	return true
}
