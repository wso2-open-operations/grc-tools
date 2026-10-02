# Screenshot Timestamp Rule

Applies to screenshots only, on any control type (DESIGN or OE): every
screenshot submitted as evidence must show the laptop's OS clock
(taskbar/menu-bar) at the moment of capture.

A screenshot is an image captured from a computer screen — an application
window, console, admin page or terminal. It is still a screenshot when it
was pasted into a PDF, Word or PowerPoint file rather than uploaded on its
own.

This rule does **not** apply to:

- documents themselves: policies, procedures, contracts, meeting minutes,
  signed or scanned forms;
- reports exported or printed from a system (PDF, Excel or CSV exports,
  "Save as PDF" printouts);
- diagrams, charts, photos and logos.

If you cannot tell whether an image is a screenshot, do not report a clock
gap for it. If the control's evidence requirement explicitly says a
timestamp is not needed, do not report it either — only an explicit
statement counts.

An app-displayed "Generated at" or "Report time" timestamp inside the
screenshotted application does **not** satisfy this rule — it only proves
when the underlying report was generated, not when the screenshot itself
was taken. Look specifically for the operating system's own clock: the
Windows taskbar clock (bottom-right) or the macOS menu-bar clock (top-right).

A screenshot missing a visible OS clock is a gap — report it in `gaps_found`
with `severity: "MEDIUM"` unless a more specific rule (such as the
Population Completeness Proof rule) requires it at a higher severity for that
screenshot's role. Report nothing when the clock is present.
