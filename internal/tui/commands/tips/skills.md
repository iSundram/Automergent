tip: browse skills — runnable commands and project playbooks
---
# /skills

Opens the skills inventory: custom markdown commands (runnable skills) and
the project/user skill playbooks the agent can load, with descriptions and
sources.

## Usage
- `/skills` — the inventory page.

## Notes
- Skills live in `.automergent/skills/` (project) and
  `~/.automergent/skills/` (user); project skills override user skills by
  name.
- The agent discovers and loads skills itself via the discover_skills and
  skill tools; this page is your view of the same inventory.

## Related
- `/commands` — the command listing (runnable skills appear there too).
- `/memory` — durable knowledge rather than playbooks.
