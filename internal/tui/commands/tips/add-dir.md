tip: add an extra read-only search root
---
# /add-dir

Adds a directory outside the workspace as a read-only search root: the
agent's read/search tools can look there, but writes still require the
normal permission flow.

## Usage
- `/add-dir <path>` — absolute path to add.

## Notes
- Roots persist for the session and show up in `/directory`.
- Adding a root does not grant write access — only read/search reach.

## Related
- `/directory` — list the workspace plus extra roots.
