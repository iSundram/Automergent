tip: copy the last reply (or a range) to the clipboard
---
# /copy

Copies conversation content to the system clipboard: the last assistant
reply by default, or a chosen slice.

## Usage
- `/copy` — last assistant message.
- `/copy last` — same, explicit.
- `/copy <n>` — the nth assistant message (1-based from the start).

## Notes
- Works through the system clipboard tooling; falls back with an error
  message when no clipboard is available (e.g. headless SSH).

## Related
- `/export` — the whole conversation to a file instead.
