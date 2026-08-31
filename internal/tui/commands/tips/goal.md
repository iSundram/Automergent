tip: set an autonomy objective the agent works toward
---
# /goal

Installs an objective with an optional token budget that the agent keeps
working toward across turns — the continuation loop drives itself until the
goal is met, the budget runs out, or you pause it.

## Usage
- `/goal <objective>` — set a goal (optionally end with `budget <n>` tokens).
- `/goal` — snapshot of the current goal and progress.
- `/goal pause|resume|continue|clear` — control the loop.

## Notes
- Progress is reported in the header/status while active.
- Clearing the goal stops the loop immediately.
- The agent never uses goal mode to skip permission prompts.

## Related
- `/mode` — approval autonomy is separate from goal autonomy.
- `/cancel` — stop the current turn without clearing the goal.
