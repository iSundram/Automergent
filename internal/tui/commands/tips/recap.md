tip: instant deterministic digest of this conversation
---
# /recap

Prints a deterministic digest of the current conversation: turn counts,
tools used, the last user message and timestamps. No model call — instant
and free.

## When to use
- Quick orientation after resuming a session.
- Checking how much tool activity a session accumulated.

## Notes
- Computed from session internals, so it never hallucinates.
- For a narrative summary use `/summary` (model-written, costs a call).

## Related
- `/summary` — the model-written narrative version.
- `/stats` — token/cost totals rather than shape.
