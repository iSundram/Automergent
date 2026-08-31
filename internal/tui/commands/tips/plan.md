tip: enter plan mode — read-only analysis, output is a plan
---
# /plan

Sends the agent into plan mode: read-only exploration, then a written
implementation plan (as an artifact) for your approval before anything is
edited.

## Usage
- `/plan` — plan mode with no particular focus.
- `/plan <focus>` — steer what the plan should cover.
- `/plan copy` — copy the current plan to the clipboard.

## Notes
- The plan lands in `.automergent/artifacts/plan.md` and appears in
  `/artifact` for approve/reject.
- Approving a plan puts the agent straight to work implementing it;
  rejecting requires a reason, which the agent uses to revise.
- If the task does not warrant a plan, the agent continues normally
  instead — plan mode is a tool, not a toll.

## Related
- `/artifact` — the review browser for plans and deliverables.
- `/mode` — plan mode is also one of the approval modes.
