tip: rename the current session
---
# /rename

Sets the title of the current session. Titles appear in `/sessions`, the
exit banner and resume matching; unnamed sessions fall back to their
auto-generated title or first message.

## Usage
- `/rename <title>` — rename (empty titles are refused).

## Notes
- Renaming persists immediately to storage.
- `/resume <title-substring>` matches the new name right away.
- In the session picker, `ctrl+r` renames any listed session inline.

## Related
- `/sessions` — where titles are shown and searched.
