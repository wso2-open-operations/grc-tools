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

package server

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// TestNewRouterRegistersEveryRoute builds the full router. The standard mux
// panics at startup when two route patterns can match the same path and
// neither is more specific, which no other test would notice until the
// service is started.
func TestNewRouterRegistersEveryRoute(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("the router could not be built: %v", r)
		}
	}()
	if NewRouter(db, nil) == nil {
		t.Fatal("NewRouter returned nil")
	}
}
