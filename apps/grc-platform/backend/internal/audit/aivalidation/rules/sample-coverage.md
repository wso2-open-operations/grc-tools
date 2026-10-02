# Sample Coverage

Applies only to an EVIDENCE submission, and only when a "## Sample selected
by the external auditor" section appears below in the submission data. If
that section is absent, skip this rule entirely — do not report anything
about sampling, and do not treat its absence itself as a gap.

When present, that section is the external auditor's own selection: the
items or records they chose to test, given as a free-text note, uploaded
files, or both. Check whether this evidence submission actually corresponds
to that selection — for example, whether the entity, record, or item names
the evidence refers to match what the auditor's note names or what the
sample files show.

This is not a line-item reconciliation — the sample is often informal (a
short note, a marked-up screenshot, a partial list). Use judgment: if the
evidence plausibly covers what was sampled, say nothing about it. If the
evidence looks unrelated to the sample, covers only part of it, or you
cannot tell because the evidence doesn't identify what it's for, report it
in `gaps_found` with `severity: "HIGH"` — a human reviewer must not approve
evidence that doesn't match the auditor's own sample.
