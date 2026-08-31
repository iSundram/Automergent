tip: run a shell command through the agent's shell tool
---
# /run

Runs a shell command via the agent's shell subsystem (the same engine as
the bash tool): persistent working directory, output capture, background
support — and the output lands in the conversation where the agent can see
and reason about it.

## Usage
- `/run <command>` — execute and show output.
- The `!` prefix in the prompt is the quick equivalent.

## Notes
- Output caps and stall watchdogs apply exactly as for the agent's own
  shell calls.
- For fire-and-forget commands prefer the agent's background shells.

## Related
- `/test`, `/build` — canned wrappers for the common cases.
