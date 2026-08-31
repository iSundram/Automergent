tip: open the diff pane — every file changed this session
personalized: diffs accumulate per session; empty until the agent edits something
---
# /diff

Opens the fullscreen diff overlay: one tab per file the agent touched this
session, including writes that never asked for confirmation (accept-edits
mode, always-allow grants, brand-new files).

## Usage
- `/diff` — open (or report nothing to review when no changes exist).
- `ctrl+w` — the keybinding equivalent.

## Keys
- `tab` — cycle file tabs.
- `↑↓ / pgup / pgdown` — scroll.
- `esc` — close.

## Notes
- Newly created files diff as pure additions.
- The status bar counts pending modified files while edits accrue.

## Related
- `/files` — the plain list of touched files.
- `/review` — a model-driven code review of the changes.
