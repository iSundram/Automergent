tip: run the project's test suite
---
# /test

Runs the project's test suite through the agent (detected per project:
`go test`, `npm test`, ...) and reports results in the conversation, where
the agent can act on failures.

## Usage
- `/test` — the whole suite.
- `/test <args>` — extra arguments passed through (e.g. a package or
  `-run TestName`).

## Notes
- Test selection is detected from the project manifest; missing tooling
  reports that plainly.
- Failures render with enough context for the agent to fix them in the
  same conversation.

## Related
- `/build` — compile check without tests.
- `/run` — arbitrary commands.
