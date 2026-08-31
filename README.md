# AUTOMERGENT

<p align="center">
  <strong>Next-gen autonomous coding engineer built on Gemini & Vertex AI with multi-phase context intelligence, subagent orchestration, and deep root-cause error diagnostics.</strong>
</p>

<p align="center">
  <a href="https://golang.org"><img src="https://img.shields.io/badge/Go-1.25%2B-00ADD8?style=flat-square&logo=go" alt="Go Version" /></a>
  <a href="https://github.com/iSundram/Automergent/actions"><img src="https://img.shields.io/badge/build-passing-4ade80?style=flat-square&logo=githubactions" alt="Build Status" /></a>
  <a href="https://github.com/iSundram/Automergent/actions"><img src="https://img.shields.io/badge/tests-passing-4ade80?style=flat-square&logo=github" alt="Tests Status" /></a>
  <a href="https://goreportcard.com/report/github.com/iSundram/Automergent"><img src="https://img.shields.io/badge/go%20report-A%2B-38bdf8?style=flat-square" alt="Go Report Card" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue?style=flat-square" alt="License" /></a>
  <a href="https://github.com/iSundram/Automergent/releases"><img src="https://img.shields.io/badge/release-v0.1.0-c084fc?style=flat-square" alt="Release" /></a>
</p>

---

## What is Automergent?

**Automergent** is a high-performance, terminal-native AI pair-programmer that lives in your command line. It reads, writes, searches, tests, and refactors your codebase autonomously or interactively with approval at every step.

> [!NOTE]
> Powered by the Charm v2 stack (`charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`) and tree-sitter AST parsing, Automergent gives you the power of modern IDE coding agents directly inside any terminal or remote SSH session with sub-millisecond responsiveness.

---

## Live TUI Experience

Automergent's terminal interface features real-time streaming, structured tool execution slabs, automated diagnostic recovery, and live telemetry:

```text
╭─ ✦ AUTOMERGENT ────────── [EXECUTE] ────────── 󰊭 gemini-2.5-pro ────────── $0.0042 │ 12.3k/200k │ 󰊢 main ─╮
│                                                                                                          │
│                                                                          ╭── You ──────────────────────╮ │
│                                                                          │ run go test, fix the failing│ │
│                                                                          │ auth test, and verify build │ │
│                                                                          ╰─────────────────────────────╯ │
│                                                                                                          │
│  ✦  Running the test suite to inspect the failure.                                                       │
│                                                                                                          │
│  ● Bash ─────────────────────────────────────────────────────────────────── ✗ exit 1    0.84s            │
│    $ go test ./internal/middleware/...                                                                   │
│    --- FAIL: TestValidateToken_Expired (0.01s)                                                           │
│        auth_test.go:52: expected error "token expired", got nil                                          │
│    FAIL                                                                                                  │
│    ⎿  ✗ exit 1: test assertion failed in auth_test.go                                                    │
│                                                                                                          │
│  ┌──────────────────────────────────────────────────────────────────────────────────────────────────────┐ │
│  │ ✦ ERROR: test failure detected in internal/middleware/auth_test.go:52                                │ │
│  └──────────────────────────────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                                          │
│  ● ReadFile  "internal/middleware/auth.go"  ·  86 lines                                    0.08s         │
│    ⎿  file read successfully                                                                             │
│                                                                                                          │
│  ● EditFile  "internal/middleware/auth.go"  ·  1 hunk applied                              0.12s         │
│    ⎿  ✓ hunk 1: added claims.ExpiresAt timestamp validation check                                        │
│                                                                                                          │
│  ● Bash ─────────────────────────────────────────────────────────────────── ✓ exit 0    0.42s            │
│    $ go test -v ./internal/middleware/...                                                                │
│    === RUN   TestValidateToken_Expired                                                                   │
│    --- PASS: TestValidateToken_Expired (0.00s)                                                           │
│    PASS  ok  github.com/iSundram/Automergent/internal/middleware  0.024s                                     │
│    ⎿  ✓ PASS: all 14 tests passing  ·  build verified                                                    │
│                                                                                                          │
│  ✦  Fixed the token expiration check in auth.go. Re-ran test suite — all 14 tests now pass cleanly.      │
│                                                                                                          │
│  └─ shift+tab browse · ctrl+p palette · ctrl+e expand                                                    │
╰─ [ ACCEPT EDITS ]  ⚙ Ready  ────────────────────────────────────────────  ctx 18% ##──────  ·  󰊢 main+1 ─╯
```

---

## Quickstart & Installation

### Option 1: One-Line Install

```bash
curl -fsSL https://automergent.github.io/install.sh | bash
```

### Option 2: Go Install

```bash
# Requires Go 1.25+ with CGO enabled
CGO_ENABLED=1 go install github.com/iSundram/Automergent/cmd/automergent@latest
```

### Option 3: Build From Source

```bash
git clone https://github.com/iSundram/Automergent.git
cd Automergent
make build
make install
```

### Option 4: Launch

```bash
cd your-project/
automergent
```

> [!TIP]
> Automergent automatically detects git context, repository root, configuration files, and project languages on startup.

---

## Configuration & AI Providers

Configure via environment variables or `~/.automergent/config.yaml`:

```yaml
provider: "gemini" # gemini | openai | anthropic | deepseek | ollama
model: "gemini-2.5-pro"
temperature: 0.2
mode: "accept-edits"  # manual | accept-edits | auto | plan
theme: "modern"       # modern | catppuccin | tokyonight | dracula | nord | gruvbox | onedark
providers:
  gemini:
    api_key: "${GEMINI_API_KEY}"
  openai:
    api_key: "${OPENAI_API_KEY}"
  anthropic:
    api_key: "${ANTHROPIC_API_KEY}"
```

### Environment Variables

```bash
export GEMINI_API_KEY="your-gemini-key"
export OPENAI_API_KEY="your-openai-key"
export ANTHROPIC_API_KEY="your-anthropic-key"
```

---

## Capabilities

| Capability | Description |
| :--- | :--- |
| **Filesystem** | Read, write, edit, glob, grep, tree — full codebase access |
| **Terminal PTY** | Synchronous commands, persistent background shells, interactive sessions |
| **Code Intelligence** | Tree-sitter AST parsing, symbol search, AST-aware diffs |
| **Search & Web** | Ripgrep pattern matching, live web fetch, documentation scraping |
| **Diagnostics** | Compiler error parsing, root-cause analysis, automated recovery |
| **Multi-Agent** | Sub-agent orchestration, concurrent background tasks, parallel execution |
| **Session Memory** | Persistent transcripts, undo/redo, project-scoped sessions |
| **Artifact System** | Plan documents, approval workflows, structured deliverables |

For slash commands, keyboard shortcuts, and detailed feature documentation, visit **[automergent.github.io](https://automergent.github.io)**.

---

## Themes

11 built-in color themes optimized for truecolor and standard terminal palettes. Switch anytime with `/theme <name>`. Visit **[automergent.github.io](https://automergent.github.io)** for the full theme gallery.

---

## Architecture

```text
Automergent/
├── cmd/automergent/          # CLI entrypoint
├── internal/
│   ├── agent/                # Agent loop, orchestration, approval policies
│   ├── ai/                   # Provider abstraction (Gemini, OpenAI, Anthropic)
│   ├── config/               # Viper configuration, validation
│   ├── context/              # Budget management, dependency tracking
│   ├── diagnostics/          # Compiler error parsing, root-cause analysis
│   ├── errors/               # Structured errors, retry policies
│   ├── prompt/               # Phased prompt system (init→explore→plan→build)
│   ├── sandbox/              # OS-level sandboxing
│   ├── session/              # Persistent transcripts, history, undo stack
│   ├── tools/                # 43+ tools (FS, Terminal, AST, Web, Diagnostics)
│   ├── tui/                  # Bubble Tea v2 TUI
│   │   ├── app/              # Core event loop, layout, view composition
│   │   ├── components/       # Modular UI widgets
│   │   ├── themes/           # 11 color palettes
│   │   └── render/           # ANSI parser, diff engine, Markdown renderer
│   └── workflow/             # Artifact system, plan management
└── Makefile                  # Build, test, lint, release
```

---

## Development

```bash
make test    # Run all tests
make lint    # Linter checks
make fmt     # Format code
make ci      # Full CI checks locally
```

---

## License

Distributed under the [MIT License](LICENSE). Built by [iSundram](https://github.com/iSundram) and contributors.
