tip: model-driven code review of the session's changes
---
# /review

Asks the agent to review the session's diff — correctness, tests, style,
missing error handling — and report findings in the conversation. Distinct
from the diff pane: it is an analysis, not a view.

## Usage
- `/review` — review the current changes.
- `/review <focus>` — steer what to look for (e.g. "concurrency").

## Notes
- Findings reference file:line so you can jump to them.
- `/review-mode` makes every edit proposal render with review grammar
  (a accepts, r rejects) instead of plain diffs.

## Related
- `/security-review` — the security-focused pass.
- `/diff` — the raw changes.
