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

package mysql

import "database/sql"

// nullStringPtr converts a sql.NullString to a *string (nil when not valid).
func nullStringPtr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	return &ns.String
}

// nullIntPtr converts a sql.NullInt64 to a *int (nil when not valid).
func nullIntPtr(ni sql.NullInt64) *int {
	if !ni.Valid {
		return nil
	}
	v := int(ni.Int64)
	return &v
}

// stringPtrVal returns the string value or empty string when ptr is nil.
// Used for nullable INSERT/UPDATE args so the driver stores NULL correctly
// — pass the *string directly; the driver maps nil pointer to NULL.
func stringPtrVal(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

// intPtrVal returns the int value or nil when ptr is nil.
func intPtrVal(i *int) any {
	if i == nil {
		return nil
	}
	return *i
}

// nullInt64Ptr converts a sql.NullInt64 to a *int64 (nil when not valid).
func nullInt64Ptr(ni sql.NullInt64) *int64 {
	if !ni.Valid {
		return nil
	}
	return &ni.Int64
}
