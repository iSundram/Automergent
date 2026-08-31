tip: fork the session into a named branch
---
# /branch

Forks the current conversation into a new named session. The original stays
untouched — useful for exploring an alternative approach without losing the
baseline.

## Usage
- `/branch <name>` — create the fork (name required).

## Notes
- The branch starts as a copy of the current history and appears in
  `/sessions` titled "branch: <name>".
- Works while the agent is idle; cancel the run first otherwise.

## Related
- `/rewind` — truncate in place instead of forking.
- `/new` — start clean instead of forking.
