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

// Package middleware provides composable HTTP middleware for the compliance entity service.
package middleware

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
)

const correlationIDHeader = "X-CSM-Correlation-ID"

type correlationIDKey struct{}

// CorrelationID reads the X-CSM-Correlation-ID request header (forwarded by the
// portal BFF) or generates a UUID v4 if absent. The ID is stored in the request
// context and echoed in the response header for end-to-end tracing.
func CorrelationID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(correlationIDHeader)
		if id == "" {
			id = newCorrelationID()
		}
		w.Header().Set(correlationIDHeader, id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), correlationIDKey{}, id)))
	})
}

// CorrelationIDFromContext returns the correlation ID stored in ctx, or "".
func CorrelationIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(correlationIDKey{}).(string)
	return v
}

func newCorrelationID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("correlationid: failed to read random bytes: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
