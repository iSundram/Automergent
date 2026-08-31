tip: work on a GitHub issue — fetch and solve it
---
# /issue

Fetches a GitHub issue (title, body, comments) and hands it to the agent as
a work order: understand, plan, implement, verify.

## Usage
- `/issue <number>` — the issue in the default repository.
- `/issue <owner/repo#number>` — another repository.

## Notes
- Requires `gh` authentication for private repositories.
- The issue body enters the conversation verbatim so the agent quotes
  requirements accurately.

## Related
- `/pr-comments` — same loop for pull-request feedback.
