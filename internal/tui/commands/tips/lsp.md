tip: language-server diagnostics for the workspace
---
# /lsp

Shows language-server diagnostics for the current workspace and the files
the session touched: errors and warnings the LSP sees, the same source the
agent's lsp_diagnostics tool uses.

## Usage
- `/lsp` — diagnostics for session-touched files.
- `/lsp all` — whole-workspace diagnostics.

## Notes
- Requires a language server for the file type; unsupported types are
  skipped.
- Problems the agent introduced are also counted in the status bar.

## Related
- `/doctor` — whether LSP integration is healthy at all.
