tip: manage the always-allow tool permission list
---
# /permissions

Opens the permissions manager: the scopes the agent may use without asking,
recorded from your "always allow" answers and persisted with the session so
they survive restarts and resumes.

## Usage
- `/permissions` — open the picker.
- `/permissions add <scope>` / `/permissions remove <scope>` — edit directly.
- `/permissions clear` — drop everything.

## Notes
- Scopes are per tool and per path pattern where applicable.
- Mode switches (`/mode`) do not clear the list; they stack on top of it.
- Removing a scope takes effect on the next tool call.

## Related
- `/mode` — the coarser approval-mode switch.
- `/doctor` — includes a permissions sanity check.
