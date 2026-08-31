tip: configure providers — switch, test, fallback chain, API keys
personalized: active provider serves {model}; /provider test checks connectivity
---
# /provider

Opens the provider studio: switch providers, set per-provider API keys and
base URLs, test connectivity live, and manage the fallback chain used when
a provider fails.

## Subcommands
- `/provider test <name>` — live connectivity check (async result).
- `/provider fallback` — show the fallback chain.
- `/provider fallback <a,b,c>` — replace it.
- `/provider api-key <name> <key>` — set a key without leaving the prompt.

## Notes
- Keys are never echoed back; `/provider` shows where a key resolves from
  (config, env or secret store), not the key itself.
- The fallback chain is tried in order on request failure.

## Related
- `/model` — model selection within the active provider.
- `/doctor` — full environment health check.
