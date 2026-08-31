tip: the files touched this session — reads, writes, edits
---
# /context-files

Shows every file the agent touched this session — reads, writes and edits —
as a full-page list (capped at 50 entries with the remainder counted).

## Usage
- `/context-files` — the touched-files page.

## Notes
- Session-scoped: resuming restores that session's list from its history.
- Pair with `/diff` to see what the writes actually changed.

## Related
- `/diff` — the content view of the writes.
- `/files` → renamed: this command was previously `/files`.
