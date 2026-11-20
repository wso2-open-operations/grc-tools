# Prompts

The fixed wording of each AI prompt is kept here as plain text, so it can be read,
reviewed and changed without reading Go code.

- Files are compiled into the binary (`go:embed`), so a change ships with the next
  deploy. It is not a runtime setting.
- Only wording lives here. The parts built from a request (the risk text, the
  catalog, the conversation) and the tags that fence untrusted text
  (`<risk_data>`, `<user_request>`, `<conversation>`, `<current_widget>`) stay in the
  Go code, so editing a file here cannot remove a safety boundary.
- Trailing newlines are ignored; the code adds the ones each prompt needs.
- Every change goes through a pull request. After changing a file, run
  `go test ./internal/shared/aigateway`, and for the dashboard assistants run the
  question-set check described in `internal/risk/service/assistant_eval_test.go`.

| File | Used by |
|---|---|
| `category_*.txt` | Category suggestion |
| `action_plan_*.txt` | Action plan suggestion |
| `likelihood_*.txt` | Likelihood suggestion |
| `untrusted_risk_notice.txt` | Category and Action Plan (the notice about the fenced risk text) |
| `risk_widget_*.txt` | Risk Hub assistant |
| `audit_widget_*.txt` | Audit Hub assistant |
| `widget_*.txt` | Both assistants (untrusted-text notice and answer formats) |
