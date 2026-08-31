tip: the API error log — retries, failures, request ids (alias /errors)
---
# /error

Shows the recorded provider API failures for this session as a full page:
retried attempts (warnings) and terminal failures (failures), newest first,
with error codes, retry timing, request ids for provider-side log
correlation and remediation hints.

## Usage
- `/error` or `/errors` — the error page.
- `/error <n>` — expand the nth entry in place.
- `/error clear` — wipe the log.

## Notes
- Retried attempts are marked distinctly from final failures; a request
  that eventually succeeded still shows its retries.
- Credentials are sanitized out of recorded messages.
- Session-scoped: `/new` starts a clean log.

## Related
- `/doctor` — connectivity check.
- `/provider fallback` — the chain used on failure.
