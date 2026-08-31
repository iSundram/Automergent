tip: session cost and token totals
personalized: totals accrue across the whole session, {model} pricing
---
# /cost

Reports the session's accumulated cost and token totals (input/output),
computed from the provider's live telemetry.

## Notes
- Totals are per session — `/new` resets the counter.
- Cost comes from the provider's pricing table; models without published
  pricing report tokens only.

## Related
- `/stats` — the fuller dashboard (tokens, cost, tool counts).
- `/context` — window usage right now.
