tip: export the conversation to Markdown
---
# /export

Writes the current conversation to a readable, deterministic Markdown
transcript: user prompts, assistant replies, tool calls and results.

## Usage
- `/export` — writes `conversation.md` in the workspace.
- `/export <relative/path.md>` — choose the destination.

## Notes
- Paths must be relative to the workspace root.
- The export reflects the persisted session, including resumed history.

## Related
- `/summary` — a model-written session summary instead of the raw transcript.
