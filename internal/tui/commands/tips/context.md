tip: context-window usage breakdown — what is consuming the window
personalized: live estimate for {model}; /compact when it climbs past ~80%
---
# /context

Shows the context-window breakdown: how much of the window the system
prompt, tool definitions, conversation and loaded context files consume,
plus the live adaptive token estimate.

## Usage
- `/context` — overview.
- `/context detail` — the full itemized breakdown.

## Notes
- The header's usage meter and this command read the same numbers.
- The model has its own view of this via the ctx_inspect tool.
- When usage is high, prefer narrow reads and let compaction reclaim space
  rather than re-reading whole files.

## Related
- `/compact` — compact the context now.
- `/stats` — session-wide token and cost totals.
