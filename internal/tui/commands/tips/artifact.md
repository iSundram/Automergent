tip: review agent artifacts — approve plans, comment, open in editor
personalized: {artifacts} artifact(s) in this session; plans need your decision
---
# /artifact

Opens the artifact review browser: the deliverables the agent produced
this session — plans, reviews, designs, summaries — scoped to this session
only.

## Keys
- `↑↓` — navigate · `p`/`enter` — full-page preview
- `y` — approve · `n` — reject (a reason is required)
- `shift+a` — approve every pending plan
- `c` — comment on the artifact
- `ctrl+g` — open in your editor · `esc` — done

## Review semantics
- **Plans** are the only approvable artifacts: `y` puts the agent to work
  implementing the plan, `n` (with your reason) sends it back for revision.
- **Other artifacts** (reviews, designs, summaries) are informational —
  preview, comment, open in an editor; no approve/reject.
- **Comments** are stored on the artifact; while the agent is working they
  are steered into the running turn.

## Preview mode
Full-page with line numbers: `↑↓/pgup/pgdn` scroll, `g`/`shift+g` top and
bottom, `/` search with enter-to-jump, `esc` back to the list.

## Related
- `/plan` — the command that usually produces the plan artifact.
- The status bar shows `N artifacts · /artifact to review` whenever plans
  are pending.
