tip: browse previous sessions; type to search, ctrl+r renames, ctrl+d deletes
personalized: type to search titles — #1 is always the newest session
---
# /sessions

Opens the session picker: a searchable list of this workspace's sessions,
newest first, filtered to the current project directory.

## Keys
- `↑↓ / ctrl+p / ctrl+n` — navigate
- `type` — search titles and first messages
- `enter` — resume the highlighted session
- `ctrl+r` — rename in place (ctrl+u clears the draft)
- `ctrl+d` twice — delete (never the active session)
- `pgup / pgdown` — page through the list
- `esc` — clear the search, then close

## Rows
Each row shows the auto-generated title (or first user message), relative
time, message count, disk size, provider/model and token totals. `✓ Current`
marks the active session.

## Related
- `/resume <id-prefix|#N|title-substring>` — resume without opening the picker.
- `/rename` — rename the current session from the prompt.
