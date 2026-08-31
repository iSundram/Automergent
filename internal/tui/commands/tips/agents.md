tip: the live subagent roster — what every agent is doing
---
# /agents

Shows the live agent roster: every subagent this session spawned, its type,
current activity ("in grep"), elapsed time, tool calls and terminal
outcome.

## Usage
- `/agents` — the roster page.
- Selecting a row opens the agent's side-channel transcript.

## Notes
- Background workflow agents appear here with their run state.
- Killing a subagent also kills its descendants; logs and artifacts are
  preserved.

## Related
- `/workflow` — the workflow runs that spawn agents.
- `/cancel` — stop the main agent turn.
