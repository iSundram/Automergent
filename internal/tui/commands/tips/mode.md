tip: switch approval mode — manual, accept-edits, auto, plan
personalized: current mode is {mode}; shift+tab cycles without typing
---
# /mode

Switches the approval mode that governs what the agent may do unasked:

- **manual** — confirm every write and network-reaching action.
- **accept-edits** — file edits apply automatically; shell/web/git still ask.
- **auto** — act freely except destructive operations.
- **plan** — read-only: edits are refused, output is a plan.

## Usage
- `/mode` — show the current mode and options.
- `/mode <name>` — switch.

## Notes
- The mode chip in the status bar always shows the active mode.
- `shift+tab` cycles modes from the prompt — no typing required.
- The agent can also enter plan mode itself via the enter_plan_mode tool.

## Related
- `/permissions` — the always-allow list that outlives mode switches.
- `/plan` — prompt-driven plan mode entry.
