# Risk assistant knowledge

The Risk assistant answers "how do I..." and "where is..." questions from written
help. Nothing here is data about risks, people or customers: the relevant parts
of the help are sent to the AI model with each question.

`basics.md` describes the Risk Hub's pages, buttons and fields as they are in the
web app. It is compiled into the backend, so a change needs a new build. Keep it
in step with the web app when a label or a page changes.

Headings (`#`, `##`, `###`) become the sections the assistant looks up, so keep
one topic under each heading. Any other `.md` file added here is picked up the
same way.
