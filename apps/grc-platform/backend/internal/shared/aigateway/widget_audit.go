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

package aigateway

import "strings"

// buildAuditWidgetPrompt is the Audit Hub assistant's instructions. It differs
// from the Risk Hub's in what it is about and in its rules; the catalog, the
// treatment of untrusted text and the answer format are shared.
func buildAuditWidgetPrompt(req InterpretWidgetRequest) string {
	var b strings.Builder
	b.WriteString(promptText("audit_widget_intro") + "\n\n")
	b.WriteString("Today's date is " + req.Today + ".\n\n")

	writeWidgetCatalog(&b, req)

	if len(req.Knowledge) > 0 {
		b.WriteString("\nKNOWLEDGE ABOUT THE AUDIT HUB\n")
		b.WriteString("This is documentation, and the only source you may use to answer questions about how to do something, where something is, or what a status or term means.\n")
		for _, k := range req.Knowledge {
			b.WriteString("--- " + k.Source + ": " + k.Title + "\n" + k.Text + "\n")
		}
	}

	b.WriteString(promptText("audit_widget_rules") + "\n\n")

	writeWidgetTail(&b, req, "control_list")
	b.WriteString(promptText("audit_widget_control_actions"))
	return b.String()
}
