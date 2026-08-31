tip: list every registered command with metadata
---
# /commands

Lists the live command registry: every command with its category, aliases,
argument hints and source (built-in or custom markdown command).

## Usage
- `/commands` — the full listing.
- `/commands <filter>` — narrow by name or category.

## Notes
- The listing is the registry itself — the same source the palette, help
  and dispatch use, so it can never drift from reality.
- Custom commands from `.automergent/commands/` appear with their source
  path.

## Related
- `/help` — the same commands with usage guidance.
- `/skills` — the skill inventory.
