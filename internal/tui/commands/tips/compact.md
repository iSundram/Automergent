tip: compact the context window — summarize and reclaim space
personalized: compaction keeps the recent tail and summarizes the rest
---
# /compact

Compacts the conversation: older messages are summarized by the model, the
recent tail is kept verbatim, and the freed window is reported. Runs in the
background; the status bar shows progress.

## When to use
- The context meter passes ~80%.
- Long sessions where early exploration is no longer needed verbatim.

## Notes
- Tool results may be dropped to reclaim space — the agent re-runs a tool
  when it needs the content again.
- The conversation view keeps its history; only the model's context shrinks.
- Automatic compaction also exists for long-running work.

## Related
- `/context` — check usage before deciding.
- `/new` — the heavier reset when compaction is not enough.
