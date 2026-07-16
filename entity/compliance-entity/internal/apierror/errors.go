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

// Package apierror provides shared HTTP error types and response helpers.
package apierror

import (
	"encoding/json"
	"net/http"
)

// ErrorResponse is the JSON body returned for all error responses.
type ErrorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ValidationError signals a caller-side input problem (HTTP 400).
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

// NotFoundError signals the requested resource does not exist (HTTP 404).
type NotFoundError struct{ Msg string }

func (e *NotFoundError) Error() string { return e.Msg }

// ServiceUnavailableError signals a downstream dependency is temporarily unavailable (HTTP 503).
type ServiceUnavailableError struct{ Msg string }

func (e *ServiceUnavailableError) Error() string { return e.Msg }

// ConflictError signals a concurrent modification prevented the operation (HTTP 409).
type ConflictError struct{ Msg string }

func (e *ConflictError) Error() string { return e.Msg }

// WriteJSON writes an ErrorResponse JSON body with the given HTTP status code.
func WriteJSON(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{Code: status, Message: msg})
}
