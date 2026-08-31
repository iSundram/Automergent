tip: exit the application (alias /quit)
---
# /quit

Exits the application. The session is saved first; the exit banner shows
the session id, duration and the resume command.

## Usage
- `/quit` or `/exit` — leave.
- `ctrl+c` twice when idle — the keybinding equivalent.

## Notes
- The banner's resume line (`automergent -s <id>`) restores exactly this
  session, artifacts included.
- A running agent is stopped on exit.

## Related
- `/sessions` — resume within the app instead.
