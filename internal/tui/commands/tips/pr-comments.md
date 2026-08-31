tip: pull PR review comments and address them
---
# /pr-comments

Fetches the review comments on a pull request and has the agent address
them: each comment is quoted, resolved in code and the reasoning recorded.

## Usage
- `/pr-comments` — the current branch's PR.
- `/pr-comments <number>` — a specific pull request.

## Notes
- Requires `gh` authentication.
- The agent works comment by comment; unresolved threads are surfaced at
  the end rather than silently dropped.

## Related
- `/issue` — the issue-driven loop.
- `/commit` — commit the fixes afterwards.
