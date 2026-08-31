tip: set the API key for any provider (not just active)
---
# /provider api-key

Sets the API key for a named provider without switching to it — configure
now, switch later.

## Usage
- `/provider api-key <provider> <key>` — set the key.
- `/provider api-key <provider>` — show where its key resolves from.

## Notes
- Never echoes keys back; shows the resolution source only.
- Stored in user config, so it survives restarts.

## Related
- `/api-key` — the active provider's key.
- `/provider` — the full provider studio.
