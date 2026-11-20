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
	"embed"
	"strings"
)

// The fixed wording of every prompt lives in prompts/*.txt, not in Go source,
// so it can be read, reviewed and edited as plain text. The files are compiled
// into the binary, so a change to one still ships with a deploy.
//
// Only the wording is here. What is built from a request (the risk text, the
// catalog, the conversation) and the tags that fence untrusted text stay in
// the code, so editing a prompt file cannot remove a safety boundary.
//
//go:embed prompts/*.txt
var promptFiles embed.FS

// promptText returns the text of prompts/<name>.txt without its trailing
// newlines. The code adds back the newlines the prompt needs, so an editor
// that adds or drops a final newline changes nothing.
func promptText(name string) string {
	data, err := promptFiles.ReadFile("prompts/" + name + ".txt")
	if err != nil {
		panic("aigateway: missing prompt file " + name + ": " + err.Error())
	}
	return strings.TrimRight(string(data), "\n")
}
