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

package repository

import "strings"

// likeEscape escapes backslash, percent, and underscore so that s can be safely
// embedded in a MySQL LIKE pattern without matching unintended rows. MySQL's
// default ESCAPE character is '\', so no ESCAPE clause is needed in the query.
var likeReplacer = strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`)

func likeEscape(s string) string { return likeReplacer.Replace(s) }
