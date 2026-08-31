tip: manage agent memory — durable facts across sessions
---
# /memory

Manages agent memory: the durable facts, decisions and preferences the
agent recalls automatically in later sessions. Memory is project-scoped and
separate from the conversation.

## Usage
- `/memory` — list stored memories.
- `/memory add <text>` — store a fact.
- `/memory remove <id>` — forget one.
- `/memory clear` — wipe all.

## Notes
- `/dream` consolidates the conversation into memory automatically.
- Memory is injected as context, not commands — the agent treats it as
  standing knowledge.

## Related
- `/dream` — the consolidation pass.
- `/init` — project instructions (a different, file-based channel).
