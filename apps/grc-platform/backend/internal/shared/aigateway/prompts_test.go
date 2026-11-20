package aigateway

import (
	"strings"
	"testing"
)

// Every prompt file must be present and non-empty: an empty file would silently
// send the model a prompt with a missing section.
func TestPromptFilesAreNotEmpty(t *testing.T) {
	entries, err := promptFiles.ReadDir("prompts")
	if err != nil {
		t.Fatal(err)
	}
	var n int
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".txt") {
			continue
		}
		n++
		if strings.TrimSpace(promptText(strings.TrimSuffix(name, ".txt"))) == "" {
			t.Errorf("prompt file %s is empty", name)
		}
	}
	if n == 0 {
		t.Fatal("no prompt files found")
	}
}

// The tags that fence untrusted text are written by the code, not read from the
// prompt files, so a file edit cannot remove a safety boundary.
func TestPromptFilesDoNotCarryTheFencingTags(t *testing.T) {
	entries, _ := promptFiles.ReadDir("prompts")
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".txt") {
			continue
		}
		data, _ := promptFiles.ReadFile("prompts/" + e.Name())
		text := string(data)
		for _, tag := range []string{"<risk_data>\n", "</risk_data>\n"} {
			if strings.Contains(text, tag) {
				t.Errorf("%s contains the line %q, which the code must write", e.Name(), tag)
			}
		}
	}
}
