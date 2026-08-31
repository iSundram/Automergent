tip: security-focused review — secrets, injection, unsafe patterns
---
# /security-review

A security-focused pass over the session's changes (or the workspace):
hard-coded secrets, injection risks, unsafe deserialization, path handling
and permission escalations.

## Usage
- `/security-review` — review the current changes.

## Notes
- Backed by the secrets_scan and dependency_audit tools plus model review.
- Findings are ranked by severity with file:line references.

## Related
- `/review` — the general-purpose review.
- `/doctor` — environment-level security posture.
