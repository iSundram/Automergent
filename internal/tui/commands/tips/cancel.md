tip: cancel the active agent turn (alias /stop)
---
# /cancel

Cancels the agent's in-flight turn: the running provider request and any
executing tool are stopped, partial output stays in the transcript, and the
prompt returns to you.

## Usage
- `/cancel` — stop the current run.
- `esc` — the keybinding equivalent; `ctrl+c` twice force-quits instead.

## Notes
- Only meaningful while a run is active; the palette disables it otherwise.
- Interrupted runs report how many tools completed before the stop.
- Queued messages stay queued; they deliver on the next run.

## Related
- `/rewind` — also undo the partial turn.
- `/goal` — clear a goal to stop the continuation loop.
