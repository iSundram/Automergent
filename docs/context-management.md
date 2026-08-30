# Context Window Management (Agent Loop)

This describes the context engine that runs inside the agent's phase loop
(`internal/agent/autocompact.go`). It is a port of the Claude Code
auto-compact design, adapted to Automergent's multi-phase arc
(init → explore → plan → build). The design analysis it follows lives in
`refs/claude-code-context-management.md`.

## Where it runs

Every iteration of `runPhaseLoop` (all phases, main agent and subagents
alike) passes through `manageContextWindow` BEFORE the provider request is
built, and the error path handles overflow with `reactiveCompact`. A manual
`/compact` in the TUI runs the same `CompactSessionMessages`.

## The ladder

In order, per iteration:

1. **Ghost oversized tool outputs** — tool results above
   `MaxToolOutputChars` (default 32k chars) are replaced by a head preview
   plus a hint to re-read. Cheap and idempotent.
2. **Micro-compact** — once usage crosses ~55% of the effective window, old
   tool-result messages (all but the 6 most recent) have their content
   replaced with a short marker + preview. Tool calls and call/result
   pairing are never touched, so strict providers keep accepting the
   sequence. Below the fraction, history is left alone to preserve prompt
   caches.
3. **Auto-compact** — a full compaction (`CompactSessionMessages`) that
   summarizes the middle of the history with the LLM, preserves
   high-signal messages, keeps the recent suffix verbatim, and re-attaches
   recently read files. Fires when usage crosses the auto-compact
   threshold.
4. **Warning / blocking states** — a status event warns the user when
   headroom drops below 20k tokens; the loop refuses to call the provider
   at the blocking limit (3k headroom), which reserves room for a manual
   compaction.

On top of the ladder:

- **Predictive trigger** — before the request, if one more full turn
  (reply + worst-case tool growth) would overflow the window, compact now
  instead of discovering it from a failed call.
- **Reactive compaction** — if the provider rejects the prompt as too long
  anyway (estimation drifted from its real counting), compact and retry
  once. Detection matches the typed `CONTEXT_TOO_LONG` code plus message
  heuristics; the Google provider now classifies 400s with context-length
  wording as `CodeContextTooLong`.
- **Circuit breaker** — after 3 consecutive compactions that failed to
  shrink the estimate, auto-compact stops firing (ghosting still runs), so
  a broken summarizer degrades instead of looping.

## Thresholds

All thresholds are expressed against the **effective window**:
`context limit − 20k` (the summary's own output reserve).

| Threshold | Value |
|---|---|
| Summary reserve | 20,000 tokens |
| Auto-compact buffer | 13k (30k ≥400k window, 50k ≥800k window) |
| Warning buffer | 20,000 tokens |
| Blocking (manual-compact) buffer | 3,000 tokens |
| Predictive turn growth | max output + 15,000 tokens |
| Micro-compact trigger | 55% of effective window |
| Post-compact restore | ≤5 files, ≤5k tokens each, ≤50k total |

A configured `autoCompressAt` percentage can only make auto-compact fire
EARLIER, never later than the buffered threshold.

## Token counting

`tokenCountWithEstimation` anchors on the last provider-reported usage
(input + output + cache hits) for the message prefix it covered, plus a
rough chars/4 estimate for everything appended since — the same
anchor-plus-estimate scheme Claude Code uses. The anchor is invalidated
whenever messages are rewritten in place (compaction, ghosting, manual
compact via `InvalidateUsageAnchor`).

## Compaction boundaries

The summary message produced by `CompactSessionMessages` carries a
`compact_boundary` metadata marker. The summarization prompt also embeds
the live phase-arc state (open todos, active goal), and the split index is
adjusted so a tool result is never separated from its generating assistant
message — an API invariant for strict providers.

## Observability

`Agent.ContextWindowStats` exposes the live thresholds, usage estimate,
and compaction health; the TUI's context report renders it. Compactions
emit `EventCompacted` with before/after token counts.

## Subagents

- Each subagent runs the same ladder in its own phase loop, bounded by the
  phase step budget (wrap-up nudge at `MaxSteps`, hard stop at 2×).
- Subagents get a cloned tool registry with their own task-state store, so
  their `todo_write` does not mutate the parent's plan.
- Background subagent completions reach the model as
  `<task-notification>` user messages (via the steering channel) instead of
  requiring `read_agent` polls; sync results carry a usage footer.
