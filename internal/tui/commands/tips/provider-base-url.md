tip: set a custom base URL for any provider
---
# /provider base-url

Overrides the API endpoint for a named provider without switching to it.

## Usage
- `/provider base-url <provider> <url>` — set the endpoint.
- `/provider base-url <provider> reset` — back to the provider default.

## Notes
- Useful for gateways and local servers on secondary providers.
- Validated on save; bad URLs fail immediately.

## Related
- `/base-url` — the active provider's endpoint.
