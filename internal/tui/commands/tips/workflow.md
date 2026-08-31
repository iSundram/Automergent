tip: run saved workflows — chained prompts over the codebase
---
# /workflow

Runs saved workflows: named chains of prompts (from `.automergent/workflows/`)
executed in sequence by the agent, with per-step results and run history.

## Usage
- `/workflow` — list available workflows.
- `/workflow <name>` — run one.
- `/workflow status` — the current or last run's progress.

## Notes
- Workflow definitions are plain Markdown files: one prompt per section.
- Runs appear in the background dock; `/agents` shows their agents.

## Related
- `/agents` — the live agent roster.
- `/run` — one-off shell commands.
