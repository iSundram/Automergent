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
provider: "google-aistudio" # google-aistudio | google-vertex | custom
model: "gemini-3.6-flash"
temperature: 0.2
mode: "accept-edits"  # manual | accept-edits | auto | plan
theme: "modern"       # modern | catppuccin | tokyonight | dracula | nord | gruvbox | onedark | solarized-dark | solarized-light | high-contrast | monokai
providers:
  google-aistudio:
    apiKey: "${GEMINI_API_KEY}"
```

> [!NOTE]
> The provider catalog is deliberately small: the two Google backends as
> first-class providers plus one open `custom` provider for any OpenAI- or
> Gemini-compatible endpoint. Legacy names (`google`, `openai`, `anthropic`,
> `deepseek`, `ollama`) keep working in old configs by resolving to their
> nearest supported equivalent — they are aliases, not supported providers.

Config is layered (later layers override earlier): system policy
(`/etc/automergent/config.yaml`), global (`~/.automergent/config.yaml`),
project (`.automergent/config.yaml`), and local
(`.automergent/config.local.yaml`).

### Environment Variables

```bash
export GOOGLE_API_KEY="your-key"   # or GEMINI_API_KEY — both are accepted
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
| **Session Memory** | Persistent sessions, undo/redo, project-scoped history |
| **Artifact System** | Plan documents, approval workflows, structured deliverables |

For slash commands, keyboard shortcuts, and detailed feature documentation, visit **[automergent.github.io](https://automergent.github.io)**.

---

## Themes

11 built-in color themes optimized for truecolor and standard terminal palettes. Switch anytime with `/theme <name>`. Visit **[automergent.github.io](https://automergent.github.io)** for the full theme gallery.

---

## Architecture

```text
Automergent/
├── cmd/automergent/          # CLI entrypoint, tool registry bootstrap
├── internal/
│   ├── agent/                # Agent loop, orchestration, approval policies, subagent fleet
│   ├── ai/                   # Provider abstraction (Gemini API, Vertex AI, OpenAI-compatible)
│   ├── cache/                # Shared caching primitives
│   ├── config/               # Layered configuration (Viper), provider catalog, secrets
│   ├── context/              # Budget management, dependency tracking, adaptive token estimation
│   ├── debug/                # Debug diagnostics
│   ├── diagnostics/          # Compiler error parsing, root-cause analysis
│   ├── editreview/           # Atomic edit proposals and review store
│   ├── errors/               # Structured errors, retry policies
│   ├── git/                  # Blame, branch, conflict handling
│   ├── mcp/                  # MCP server orchestration
│   ├── planning/             # Planning/replanning tools
│   ├── prompt/               # Phased prompt system (init→explore→plan→build)
│   ├── recovery/             # Automated error recovery
│   ├── sandbox/              # OS-level sandboxing
│   ├── session/              # Persistent sessions, history, undo stack
│   ├── shared/               # Cross-package types
│   ├── taskstate/            # Task/todo state
│   ├── tools/                # 45+ tools (FS, Terminal, AST, Web, Diagnostics, MCP, Skills)
│   ├── tui/                  # Bubble Tea v2 TUI
│   │   ├── app/              # Core event loop, layout, view composition
│   │   ├── commands/         # Slash-command registry and dispatch
│   │   ├── components/       # Modular UI widgets
│   │   ├── themes/           # 11 color palettes
│   │   └── render/           # ANSI parser, diff engine, Markdown renderer
│   ├── version/              # Build version info
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
