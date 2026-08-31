tip: health check — provider, storage, LSP, permissions, paths
---
# /doctor

Runs a full health check: provider connectivity, session storage health,
LSP availability, permission-path sanity, config validity and toolchain
presence. Each check reports ✓ / ! / ✗ with details.

## Usage
- `/doctor` — run every check.

## Notes
- The exit banner's tips aside, this is the first stop when something
  behaves oddly — it names the failing subsystem precisely.
- Storage checks cover session files and crash-recovery state.

## Related
- `/env` — the environment without the checks.
- `/errors` — the API failure log.
