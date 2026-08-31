tip: search the workspace — files by name or content
---
# /search

Searches the workspace and shows matches in the conversation. By default it
behaves like a filename search; quoted arguments switch to content search.

## Usage
- `/search <term>` — find files whose name matches.
- `/search "text"` — grep the workspace contents for text.

## Notes
- Honors extra search roots added with `/add-dir`.
- For anything the agent should act on, just ask it in the prompt — it has
  its own search tools.

## Related
- `/files` — what the agent already touched.
- `/tree` — browse instead of search.
