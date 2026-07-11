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

// Package handler contains HTTP handlers for the compliance entity service.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
)

const maxRequestBodySize = 1 << 20 // 1 MiB

// decodeRequest decodes a JSON request body into dst, enforcing unknown-field
// rejection, a 1 MiB body cap, and no trailing data after the JSON object.
// Returns false and writes the error response if decoding fails.
func decodeRequest[T any](w http.ResponseWriter, r *http.Request, dst *T) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		apierror.WriteJSON(w, http.StatusBadRequest, decodeErrMsg(err))
		return false
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		apierror.WriteJSON(w, http.StatusBadRequest, "request body must contain a single JSON object")
		return false
	}
	return true
}

func decodeErrMsg(err error) string {
	var maxBytes *http.MaxBytesError
	if errors.As(err, &maxBytes) {
		return "request body too large"
	}
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return fmt.Sprintf("request body contains malformed JSON at position %d", syntaxErr.Offset)
	}
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		return fmt.Sprintf("invalid value for field %q: expected %s", typeErr.Field, typeErr.Type)
	}
	msg := err.Error()
	if strings.HasPrefix(msg, "json: unknown field ") {
		field := strings.TrimPrefix(msg, "json: unknown field ")
		return fmt.Sprintf("unknown field %s in request body", field)
	}
	return "invalid request body"
}

// parsePagination parses the "limit" and "offset" query parameters.
// If either param is present but not a valid integer, it writes a 400 response
// and returns ok=false. Missing params default to 0 (the service layer applies
// the actual default page size via normalizePagination).
func parsePagination(w http.ResponseWriter, r *http.Request) (limit, offset int, ok bool) {
	if s := r.URL.Query().Get("limit"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil {
			writeServiceError(w, r, &apierror.ValidationError{Msg: "limit must be an integer"})
			return 0, 0, false
		}
		limit = v
	}
	if s := r.URL.Query().Get("offset"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil {
			writeServiceError(w, r, &apierror.ValidationError{Msg: "offset must be an integer"})
			return 0, 0, false
		}
		offset = v
	}
	return limit, offset, true
}

// writeServiceError maps a service-layer error to the appropriate HTTP response.
func writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var (
		ve  *apierror.ValidationError
		nfe *apierror.NotFoundError
		sue *apierror.ServiceUnavailableError
		ce  *apierror.ConflictError
	)
	switch {
	case errors.As(err, &ve):
		log.Printf("Bad request: %s %s: %s", r.Method, r.URL.Path, ve.Msg) // #nosec G706
		apierror.WriteJSON(w, http.StatusBadRequest, ve.Msg)
	case errors.As(err, &nfe):
		log.Printf("Not found: %s %s: %s", r.Method, r.URL.Path, nfe.Msg) // #nosec G706
		apierror.WriteJSON(w, http.StatusNotFound, nfe.Msg)
	case errors.As(err, &ce):
		log.Printf("Conflict: %s %s: %s", r.Method, r.URL.Path, ce.Msg) // #nosec G706
		apierror.WriteJSON(w, http.StatusConflict, ce.Msg)
	case errors.As(err, &sue):
		log.Printf("Service unavailable: %s %s: %s", r.Method, r.URL.Path, sue.Msg) // #nosec G706
		apierror.WriteJSON(w, http.StatusServiceUnavailable, "service temporarily unavailable, please try again later")
	case errors.Is(err, context.DeadlineExceeded):
		log.Printf("Request timeout: %s %s", r.Method, r.URL.Path) // #nosec G706
		apierror.WriteJSON(w, http.StatusRequestTimeout, "request timed out")
	case errors.Is(err, context.Canceled):
		log.Printf("Request canceled: %s %s", r.Method, r.URL.Path) // #nosec G706
	default:
		log.Printf("Internal error: %s %s: %v", r.Method, r.URL.Path, err) // #nosec G706
		apierror.WriteJSON(w, http.StatusInternalServerError, "internal server error")
	}
}
