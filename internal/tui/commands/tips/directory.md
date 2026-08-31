tip: show the working directory and search roots
---
# /directory

Prints the active working directory and any extra search roots added with
`/add-dir`, plus the write-boundary policy (which paths are blocked or
allowed by configuration).

## Notes
- The working directory is fixed at launch; extra roots are session state.
- Writes outside allowed paths always ask first — the boundary is enforced
  by the security layer, not by the model's goodwill.

## Related
- `/add-dir` — add a root.
- `/doctor` — includes path-permission diagnostics.
