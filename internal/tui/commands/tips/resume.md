tip: resume a session by id prefix, #N (newest first) or title match
personalized: #1 is your newest session; title matches work too
---
# /resume

Resumes a saved session without opening the picker. The reference resolves
in this order:

1. **Exact session id** — the full UUID.
2. **Unique id prefix** — the first characters of the id. Ambiguous
   prefixes name their match count instead of guessing.
3. **`#N`** — the Nth most recently updated session (1 = newest).
4. **Title substring** — case-insensitive; must match exactly one session.

With no argument it opens the `/sessions` picker instead.

## Notes
- Refuses to run while the agent is mid-turn (`/cancel` first).
- Crash-recovery points are preferred over the last clean save when they
  hold a richer history.
- Restores provider/model, replays the transcript and this session's
  artifacts.

## Related
- `/sessions` — the full picker with search, rename and delete.
