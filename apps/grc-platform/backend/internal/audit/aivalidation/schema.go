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

package aivalidation

import "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/llm"

// submitValidationToolName is the one tool the model is asked to call; the
// response is read only from this tool's input, so a successful prompt
// injection can still only move these fields within the schema below.
const submitValidationToolName = "submit_validation_result"

// gap is one entry of gaps_found — locked to the shape useGetAIValidation.ts
// parses on the frontend.
type gap struct {
	RequirementAspect string `json:"requirementAspect"`
	Issue             string `json:"issue"`
	Severity          string `json:"severity"` // HIGH | MEDIUM | LOW
	FileName          string `json:"fileName,omitempty"`
}

// validationResult is submit_validation_result's input, unmarshaled from the
// tool call. Summary is a one-line headline and is empty for a clean PASS;
// GapsFound carries the detail, shown identically to submitter and reviewer.
type validationResult struct {
	Result    string `json:"result"` // PASS | FAIL | UNCERTAIN
	Summary   string `json:"summary"`
	GapsFound []gap  `json:"gaps_found"`
}

// submitValidationTool builds the tool definition. The schema is
// intentionally exactly the shape validationResult unmarshals into.
func submitValidationTool() llm.Tool {
	gapSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"requirementAspect": map[string]any{"type": "string", "description": "2-5 word title, e.g. \"Missing OS clock\"."},
			"issue":             map[string]any{"type": "string", "description": "One short phrase (under ~12 words): what is wrong."},
			"severity":          map[string]any{"type": "string", "enum": []string{"HIGH", "MEDIUM", "LOW"}},
			"fileName":          map[string]any{"type": "string"},
		},
		"required": []string{"requirementAspect", "issue", "severity"},
	}
	return llm.Tool{
		Name: submitValidationToolName,
		Description: "Report the result of validating one evidence or population submission against " +
			"the control's evidence requirement and the Standing Evidence Rules.",
		Properties: map[string]any{
			"result": map[string]any{"type": "string", "enum": []string{"PASS", "FAIL", "UNCERTAIN"}},
			"summary": map[string]any{
				"type":        "string",
				"description": "Headline under ~8 words. Empty string for a clean PASS.",
			},
			"gaps_found": map[string]any{
				"type":        "array",
				"items":       gapSchema,
				"description": "Every problem found, most severe first. Never include rules or checks that passed. Empty for a clean PASS.",
			},
		},
		Required: []string{"result", "summary", "gaps_found"},
	}
}
