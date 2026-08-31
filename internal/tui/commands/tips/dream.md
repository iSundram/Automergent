tip: consolidate session memory — reflect and store durable facts
---
# /dream

Runs a memory-consolidation pass over the conversation: durable facts,
decisions and preferences are distilled into agent memory while the working
transcript stays untouched.

## Usage
- `/dream` — consolidate now.

## Notes
- The pass is model-driven; it reports what it stored.
- Consolidated memory is recalled automatically in later sessions.
- Runs in the background; results arrive as a system message.

## Related
- `/memory` — inspect and edit stored memory directly.
- `/summary` — a summary for humans instead of memory.
