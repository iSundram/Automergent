tip: initialize project docs the agent reads every session
---
# /init

Generates or refreshes the project instruction file (AUTOMERGENT.md) by
having the agent analyze the repository: build commands, test commands,
conventions and safety rules. The file is injected into every future
session's context automatically.

## Usage
- `/init` — analyze the repo and write/update the instruction file.
- `/init add <note>` — append a line yourself.

## Notes
- Review the generated file — it is the agent's standing brief for this
  project; wrong facts there mislead every session.
- Keep it short and current; stale instructions are worse than none.

## Related
- `/doctor` — checks that project config resolves sanely.
- `/memory` — agent memory management.
