# Population Completeness Proof

Applies to population submissions on OE controls only. This context assembly
step only sends this rule when the submission is a population submission;
ignore it entirely for an evidence submission.

A population submission (a list of in-scope items — usually CSV/Excel, but
a PDF, Word or text list is equally valid unless the control's evidence
requirement names a specific format) must be accompanied by:

1. **Workflow screenshot(s)** — at least one screenshot plausibly showing the
   query/filter/report screen used to produce the list. This is a floor, not
   a ceiling: more than one is fine and is never itself a gap. You are not
   required to see a multi-step sequence — one credible screenshot satisfies
   this.
2. **First-entry screenshot** — shows the first row/entry of the list as it
   appears in the source system.
3. **Last-entry-and-count screenshot** — shows the last row/entry **and**
   the total record count, together on the same screenshot. Two separate
   screenshots (one for the last entry, one for the count) do not satisfy
   this — they must appear together.

All three categories, where present:

- Must show the laptop's OS clock (taskbar/menu-bar) — see the Screenshot
  Timestamp Rule. An app-displayed "Generated at" timestamp does not
  substitute for this.
- Must be cross-validated against the actual uploaded list file's content:
  the first-entry and last-entry-and-count screenshots' values must match
  the list's real first row, last row, and row count. Read the list content
  provided to you and compare it against what the screenshots show. If the
  list file's text was not provided to you (listed as not reviewed, or only
  its images were extracted), say so as a gap rather than guessing.

Exception: if the control's own evidence requirement explicitly says a
screenshot (or one of the three categories above) is not needed, do not
report its absence as a gap. Only an explicit statement counts — silence in
the requirement does not waive anything.

There is no dedicated upload slot for each of these three categories —
submitters use one shared upload box, so you must classify which uploaded
image (if any) plays which role from its visual content alone. A missing
category, or a mismatch between a screenshot's values and the file's real
content, is a gap: report it in `gaps_found` the same way as any other gap
against the control's own evidence requirement — this is advisory only,
never blocking. Say nothing about the categories that are present and match.
