tip: starts a fresh session; the current one is saved and resumable
personalized: your current session is auto-titled — /resume #1 returns to it anytime
---
# /new

Starts a fresh session. The current conversation is saved to disk first
(when it has messages), the view clears, and a new session begins with the
same provider, model and working directory.

## When to use
- Starting an unrelated task and you want a clean context.
- The current conversation has drifted and token usage is climbing.

## Behavior
- The previous session is saved automatically — nothing is lost.
- Session-scoped state resets: rewind checkpoints, API error history,
  artifacts and usage stats.
- Refuses to run while the agent is mid-turn: /cancel first, then /new.

## Related
- `/sessions` — browse and resume saved sessions (search, rename, delete).
- `/resume <id|prefix|#N|title>` — jump straight back.
- `/compact` — alternative when you want to keep the session but shrink it.
