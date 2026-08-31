tip: compile the project and surface build errors
---
# /build

Builds the project (detected per project: `go build`, `npm run build`,
...) and surfaces errors and warnings in the conversation.

## Usage
- `/build` — the default build.
- `/build <args>` — pass extra arguments through.

## Notes
- Build failures arrive as structured output the agent can iterate on.
- Faster feedback loop than `/test` when only compilation matters.

## Related
- `/test` — the full suite.
- `/lsp` — editor-grade diagnostics without a build.
