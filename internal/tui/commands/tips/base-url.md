tip: set a custom API base URL (proxies, gateways, local servers)
---
# /base-url

Overrides the API endpoint for the active provider — useful for proxies,
enterprise gateways and locally hosted servers.

## Usage
- `/base-url <url>` — set the endpoint.
- `/base-url` — show the current value.
- `/base-url reset` — clear it back to the provider default.

## Notes
- Applies to the active provider only; per-provider overrides live in the
  provider studio (`/provider base-url`).
- An invalid URL fails fast with a validation error, not a request error.

## Related
- `/provider` — provider-level settings.
