tip: expand every tool card, thinking block and shell output at once
---
# /expand

Opens every collapsible conversation block in one move: tool cards show
their full arguments and results, thinking blocks their complete reasoning,
shell output its whole transcript.

## Notes
- The status line names the inverse (`/collapse`) after it runs.
- Individual blocks still expand and collapse on their own; this is the
  global switch.
- Clipped blocks always hint at the command that flips the current state.

## Related
- `/collapse` — the inverse, one line per block.
- `/review-mode` — richer rendering for edit proposals.
