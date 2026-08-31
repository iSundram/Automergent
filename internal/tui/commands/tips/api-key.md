tip: set the API key for the active provider
---
# /api-key

Sets the API key for the active provider. For other providers' keys use
`/provider api-key`, which names its target explicitly.

## Usage
- `/api-key <key>` — set the key for the active provider.
- `/api-key` — show where the current key resolves from (config, env or
  secret store — never the key itself).

## Notes
- Keys are stored in the user config and never echoed back.
- Environment variables override config per the documented resolution
  order.

## Related
- `/provider` — the provider studio, including per-provider keys.
- `/doctor` — validates key resolution end to end.
