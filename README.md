<p align="center">
  <img src="assets/logo.svg" width="480" alt="Automergent Logo" />
</p>

<p align="center">
  <strong>Next-gen autonomous AI coding engineer built on Gemini &amp; Vertex AI with multi-phase context intelligence, subagent fleet orchestration, tree-sitter AST parsing, and deep root-cause error diagnostics.</strong>
</p>

<p align="center">
  <a href="https://golang.org"><img src="https://img.shields.io/badge/Go-1.25%2B-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go Version" /></a>
  <a href="https://github.com/iSundram/Automergent/actions"><img src="https://img.shields.io/badge/build-passing-4ade80?style=for-the-badge&logo=githubactions&logoColor=white" alt="Build Status" /></a>
  <a href="https://github.com/iSundram/Automergent/actions"><img src="https://img.shields.io/badge/tests-passing-4ade80?style=for-the-badge&logo=github&logoColor=white" alt="Tests Status" /></a>
  <a href="https://automergent.github.io"><img src="https://img.shields.io/badge/docs-live-38bdf8?style=for-the-badge&logo=googledocs&logoColor=white" alt="Documentation" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue?style=for-the-badge" alt="License" /></a>
  <a href="https://github.com/iSundram/Automergent/releases"><img src="https://img.shields.io/badge/release-v0.1.2--dev-c084fc?style=for-the-badge" alt="Release" /></a>
</p>

<p align="center">
  <a href="#overview">Overview</a> •
  <a href="#key-features">Features</a> •
  <a href="#quickstart--installation">Installation</a> •
  <a href="#configuration--providers">Providers &amp; Config</a> •
  <a href="#4-phase-agent-loop">Agent Loop</a> •
  <a href="#tools-ecosystem">Tools (45+)</a> •
  <a href="#slash-commands">Commands (61)</a> •
  <a href="#themes">Themes</a> •
  <a href="https://automergent.github.io">Documentation</a>
</p>

---

## Overview

**Automergent** is a high-performance, terminal-native AI pair-programmer and autonomous coding agent that lives directly in your command line. Engineered in Go with the Charm v2 stack (`charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`) and tree-sitter AST parsing, Automergent gives you the intelligence of a senior engineer with sub-millisecond local TUI responsiveness.

It reads, writes, searches, tests, debugs, and refactors your codebase autonomously or interactively with approval gates at every step.

> [!NOTE]
> Visit the interactive documentation site at **[automergent.github.io](https://automergent.github.io)** for full guides on configuration, architecture, slash commands, tools, themes, and developer workflows.

---

## Key Features

- 🧠 **4-Phase Autonomous Agent Loop**: Structured pipeline (`Init` → `Explore` → `Plan` → `Build`) preventing premature destructive edits and hallucinated paths.
- ⚡ **First-Class Gemini &amp; Vertex AI**: Native Google AI Studio (`gemini-3.6-flash`, `gemini-3.6-pro`) and Google Vertex AI integrations with 1M+ token context windows.
- 🔌 **Universal Provider Support**: Open `custom` provider backend connecting to any OpenAI- or Gemini-compatible endpoint (OpenAI, Anthropic, DeepSeek, Ollama, vLLM, LiteLLM).
- 🛠️ **45+ Built-in Tools**: Comprehensive capabilities spanning filesystem, tree-sitter AST, terminal PTY, background shells, ripgrep, web fetching, LSP diagnostics, secrets scanning, SQL, subagents, and MCP.
- 👥 **Subagent Fleet Orchestration**: Spawn parallel specialist agents (`explore`, `review`, `build`) with independent contexts and real-time bidirectional steering.
- 🛡️ **Zero-Overhead Tool Sandboxing**: Granular approval modes (`manual`, `accept-edits`, `auto`, `plan`), path allowlists/blocklists, and TOCTOU protection.
- 🎨 **11 Truecolor Themes**: Dynamic, runtime hot-swappable themes including `modern`, `catppuccin`, `tokyonight`, `dracula`, `nord`, `gruvbox`, and `onedark`.
- 💾 **Persistent Session Memory**: Automatic SQLite session history, token compaction at 55% budget, branching, and undo/redo stacks.

---

## Quickstart &amp; Installation

### Option 1: One-Line Installer (Recommended)

```bash
curl -fsSL https://automergent.github.io/install.sh | bash
```

### Option 2: Go Install

```bash
# Requires Go 1.25+ with CGO enabled (for tree-sitter AST parsing)
CGO_ENABLED=1 go install github.com/iSundram/Automergent/cmd/automergent@latest
```

### Option 3: Build From Source

```bash
git clone https://github.com/iSundram/Automergent.git
cd Automergent
make build
make install
```

### Launch

```bash
cd your-project/
automergent   # or use the fast alias: amt
```

> [!TIP]
> The installer automatically creates `amt` as a shortcut alias for `automergent`. Resume any past session instantly with `amt -s <session-id>` or continue the latest with `amt -c`.

---

## Configuration &amp; Providers

Automergent utilizes a layered configuration system: **System** (`/etc/automergent/config.yaml`) $\to$ **Global** (`~/.automergent/config.yaml`) $\to$ **Project** (`.automergent/config.yaml`) $\to$ **Local** (`.automergent/config.local.yaml`) $\to$ **Environment Variables** $\to$ **CLI Flags**.

### Example Configuration (`~/.automergent/config.yaml`)

```yaml
# Active provider and model
provider: "google-aistudio" # google-aistudio | google-vertex | custom
model: "gemini-3.6-flash"
temperature: 0.2
mode: "accept-edits"        # manual | accept-edits | auto | plan
theme: "modern"             # modern | catppuccin | tokyonight | dracula | nord | ...

# Provider Credentials
providers:
  google-aistudio:
    apiKey: "${GEMINI_API_KEY}" # or GOOGLE_API_KEY
  
  google-vertex:
    project: "my-gcp-project"
    location: "us-central1"
  
  custom:
    baseUrl: "https://api.openai.com/v1"
    apiKey: "${OPENAI_API_KEY}"
    model: "gpt-4o"

# Sandboxing & Permissions
sandbox: "auto"
allowedWritePaths:
  - "."
  - "/tmp/build"
blockedWritePaths:
  - ".git"
  - "~/.ssh"

# Context & Memory
autoCompressAt: 0.55
```

### Environment Variables

```bash
# Google AI Studio (either variable is accepted)
export GEMINI_API_KEY="your-gemini-api-key"
export GOOGLE_API_KEY="your-google-api-key"

# Google Vertex AI
export GOOGLE_APPLICATION_CREDENTIALS="/path/to/sa-key.json"
export VERTEX_PROJECT_ID="your-project-id"
export VERTEX_LOCATION="us-central1"
```

---

## 4-Phase Agent Loop

Rather than immediately generating untested edits, Automergent follows a disciplined 4-phase cognitive lifecycle:

```
┌─────────────────┐       ┌─────────────────┐       ┌─────────────────┐       ┌─────────────────┐
│     1. INIT     │  ──▶  │   2. EXPLORE    │  ──▶  │    3. PLAN      │  ──▶  │    4. BUILD     │
│ Deconstruct     │       │ Read-only AST   │       │ Architect diff  │       │ Atomic edits,   │
│ requirements &  │       │ discovery, grep │       │ & validation    │       │ tests, compiler │
│ routing gates   │       │ & dependencies  │       │ specifications  │       │ error recovery  │
└─────────────────┘       └─────────────────┘       └─────────────────┘       └─────────────────┘
```

1. **`Init` Phase**: Parses user prompts, extracts task requirements, checks repository readiness, and routes workflows.
2. **`Explore` Phase**: Read-only inspection using tree-sitter AST symbol lookups, ripgrep matching, and file structure mapping without modifying disk.
3. **`Plan` Phase**: Drafts concrete step-by-step implementation specifications and validates architecture before writing changes.
4. **`Build` Phase**: Executes atomic file writes, runs compilers and tests via PTY shells, detects compiler errors, and performs automatic root-cause repair loops.

---

## Tools Ecosystem

Automergent provides **45+ built-in tools** across specialized subsystems:

| Category | Tools Included | Capabilities |
| :--- | :--- | :--- |
| 📁 **Filesystem** | `read_file`, `write_file`, `edit_file`, `notebook_edit`, `artifact`, `glob`, `grep`, `list_directory` | Full codebase reading, chunked line-precise replacements, Jupyter notebook editing, and file tree exploration. |
| 💻 **Terminal PTY** | `async_runner`, `read_shell`, `write_shell`, `stop_shell`, `list_shells` | Interactive PTY execution, long-running persistent background servers, exit code captures, and ANSI parsing. |
| 🌐 **Web & Docs** | `fetch`, `search` | Live URL scraping (markdown converted) and targeted web search for official API references and documentation. |
| 🔍 **Code Intelligence** | `symbols`, `references`, `definitions`, `diagnostics` | Tree-sitter AST symbol indexing, language server protocol (LSP) diagnostics, and cross-file references. |
| 🛡️ **Security & Quality** | `secrets_scan`, `dependency_audit`, `lint` | Diff secrets leak detection (AWS, GitHub, API tokens), dependency vulnerability checks, and automated linting. |
| 🤖 **Subagents** | `task`, `read_agent`, `list_agents`, `kill_agent` | Dynamic specialist subagents with dedicated memory, parallel exploration, and bidirectional steering. |
| 📝 **Planning & Memory** | `plan`, `replan`, `enter_plan_mode`, `exit_plan_mode`, `memory` | Architectural plan tracking, session checkpointing, memory stores, and cross-session knowledge persistence. |
| 🧩 **Extensibility** | `discover_skills`, `skill`, `mcp` | Dynamic custom skills discovery (`.automergent/skills/`) and Model Context Protocol (MCP) tool integration. |

---

## Slash Commands

With **61 built-in slash commands** accessible from the interactive command palette (`/`), you have total control:

| Command | Category | Purpose |
| :--- | :--- | :--- |
| `/provider`, `/model` | Provider | Switch AI provider, switch models, or test API connectivity |
| `/mode`, `/permissions` | Security | Cycle approval modes (`manual`, `accept-edits`, `auto`, `plan`) |
| `/plan`, `/replan` | Planning | Enter architectural plan mode and review staged execution blueprints |
| `/review`, `/security-review` | Analysis | Perform deep AST code review or run automated secret/vulnerability scans |
| `/diff`, `/artifact` | Workspace | Inspect unified diffs or review agent-generated documentation artifacts |
| `/compact`, `/context`, `/cost` | Context | Compress token window (70-80% savings), inspect budget, and track spend |
| `/agents`, `/workflow`, `/goal` | Orchestration| Monitor live subagent fleet, trigger automated workflows, set long-running goals |
| `/sessions`, `/resume`, `/rewind` | Persistence | Browse stored sessions, resume past work, or revert to previous checkpoints |
| `/theme`, `/keybindings` | Customization | Change visual theme or inspect default/Vim/Emacs keyboard maps |
| `/doctor`, `/error`, `/version` | Diagnostics | Run complete system diagnostic check, view last stacktrace, or check version |

Type `/help` in any active session to view the full categorized registry.

---

## Themes

Automergent includes **11 built-in themes** designed for truecolor terminal emulators:

| Theme | Base Palette | Accent Colors | Description |
| :--- | :--- | :--- | :--- |
| `modern` | Obsidian Dark (`#0d1117`) | Cyan (`#58a6ff`) &amp; Green (`#3fb950`) | Clean, default modern engineering palette |
| `catppuccin` | Mocha Dark (`#1e1e2e`) | Mauve (`#cba6f7`) &amp; Sapphire (`#74c7ec`) | Warm pastel theme inspired by Catppuccin Mocha |
| `tokyonight` | Tokyo Night (`#1a1b26`) | Neon Blue (`#7aa2f7`) &amp; Cyan (`#7dcfff`) | Vivid Japanese neon-lit aesthetic |
| `dracula` | Gothic Slate (`#282a36`) | Purple (`#bd93f9`) &amp; Pink (`#ff79c6`) | High-contrast classic Dracula styling |
| `nord` | Arctic Blue (`#2e3440`) | Frost Cyan (`#88c0d0`) &amp; Ice (`#81a1c1`) | Serene, cool Nordic colorway |
| `gruvbox` | Dark Warm (`#282828`) | Warm Orange (`#fe8019`) &amp; Yellow (`#fabd2f`) | Retro groove palette with warm earth tones |
| `onedark` | Atom Dark (`#282c34`) | Sky Blue (`#61afef`) &amp; Magenta (`#c678dd`) | Atom / VSCode One Dark classic theme |
| `solarized-dark` | Deep Solar (`#002b36`) | Yellow (`#b58900`) &amp; Blue (`#268bd2`) | Ethan Schoonover's precision dark palette |
| `solarized-light` | Light Cream (`#fdf6e3`) | Yellow (`#b58900`) &amp; Cyan (`#2aa198`) | Precision light palette with low glare |
| `monokai` | Charcoal (`#272822`) | Magenta (`#f92672`) &amp; Lime (`#a6e22e`) | Sublime Text classic Monokai |
| `high-contrast` | Pure Black (`#000000`) | Pure White (`#ffffff`) &amp; Yellow (`#ffff00`) | Maximum readability for low-vision environments |

Switch themes anytime during a session by typing `/theme <name>`.

---

## Repository Structure

```text
Automergent/
├── assets/                   # Project logos and visual media
│   └── logo.svg              # Official Automergent SVG vector logo
├── cmd/automergent/          # CLI main entrypoint and registry bootstrap
├── docs-site/                # Comprehensive documentation website
│   ├── css/style.css         # Modern design system (Plus Jakarta Sans, Inter, JetBrains Mono)
│   ├── js/                   # Nav, footer, theme toggle, and search controllers
│   └── pages/                # 11 interactive documentation pages
├── internal/
│   ├── agent/                # 4-Phase agent engine, subagent fleet, approval controller
│   ├── ai/                   # Google AI Studio, Vertex AI, and custom OpenAI backends
│   ├── cache/                # High-performance caching layers
│   ├── config/               # Layered Viper configuration and secrets management
│   ├── context/              # Context budget compaction and AST dependency tracker
│   ├── diagnostics/          # Tree-sitter AST parsing and compiler error repair loops
│   ├── editreview/           # Atomic diff proposals and workspace reviews
│   ├── mcp/                  # Model Context Protocol server client and dynamic tools
│   ├── planning/             # Plan, replan, and task state tracking
│   ├── prompt/               # Phased prompt templates (Init → Explore → Plan → Build)
│   ├── sandbox/              # Filesystem and command execution sandbox
│   ├── session/              # SQLite session persistence, undo stack, and checkpoints
│   ├── tools/                # 45+ built-in developer tools
│   └── tui/                  # Bubble Tea v2 TUI layout, renderers, commands, and themes
└── Makefile                  # Build, test, lint, format, and release automation
```

---

## Developer Guide &amp; Contributing

We welcome contributions from the open-source community!

```bash
# Run tests
make test

# Run linter and formatting
make lint
make fmt

# Execute complete local CI validation suite
make ci

# Build release binaries
make build
```

Please review [CONTRIBUTING.md](CONTRIBUTING.md) and the [Developer Guide](https://automergent.github.io/pages/developer-guide.html) before submitting pull requests.

---

## License

Distributed under the [MIT License](LICENSE). Built with ❤️ by [iSundram](https://github.com/iSundram) and the Automergent community.
