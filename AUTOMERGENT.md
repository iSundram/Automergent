# AUTOMERGENT.md

Guidance for Automergent agents working in this repository.
Keep it short, factual and current.

## Overview

Automergent is a terminal-native autonomous coding agent (this very tool),
written in Go on the Charm v2 stack (Bubble Tea v2, Lip Gloss, Glamour) with
tree-sitter AST parsing. The agent loop lives in `internal/agent/`, the
phased prompt system (init → explore → plan → build) in `internal/prompt/`,
and the TUI in `internal/tui/`.

## Build & Test

- Build: `make build` (CGO required; writes `bin/automergent`)
- Test: `make test` (all packages) or `go test ./internal/<pkg>/...` for one area
- Lint: `make lint` (golangci-lint) · Format: `make fmt` (gofmt + goimports)
- Full local CI: `make ci`

## Conventions

- Go 1.25+; match the surrounding code's comment density and naming.
- TUI glyph discipline: non-ASCII glyphs must be admitted by the charter in
  `internal/tui/render/glyphs.go` (single-width, no emoji, no nerd-font PUA).
  Add new glyphs there first, deliberately.
- One source of truth: slash commands register in `internal/tui/commands/`
  (registry is palette + help + dispatch); tool self-descriptions live in
  `internal/tools/meta.go`; provider specs in `internal/config/providers.go`.
- Status bar vs header vs spinner: run timing lives in the spinner slot, the
  cwd in the status bar's left edge, context usage only in the header bar —
  do not duplicate a readout across chrome slots.
- Tests ride next to the code (`*_test.go`); keep them passing or say why not.

## Safety

- Never commit secrets or credentials.
- Ask before destructive operations (deletes, force-pushes, migrations).
- The write sandbox and per-directory grants are security features — do not
  bypass them to "fix" a failing path; fix the path instead.
