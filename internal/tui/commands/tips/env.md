tip: environment report — versions, terminal, capabilities
---
# /env

Prints the runtime environment: application version, Go version, terminal
capabilities (truecolor, sync updates, keyboard protocol) and the
resolution of key environment variables.

## Usage
- `/env` — the full report.

## Notes
- The capability lines explain why some visual features degrade on certain
  terminals (tmux, screen).
- API keys are shown as resolution sources, never values.

## Related
- `/doctor` — health checks on top of the environment.
- `/version` — just the version.
