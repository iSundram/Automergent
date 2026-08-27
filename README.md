# ✦ AUTOMERGENT

<p align="center">
  <strong>Terminal-native autonomous AI coding agent built with Charm (Bubble Tea, Lip Gloss & Glamour) and Tree-sitter.</strong>
</p>

<p align="center">
  <a href="https://golang.org"><img src="https://img.shields.io/badge/Go-1.24%2B-00ADD8?style=flat-square&logo=go" alt="Go Version" /></a>
  <a href="https://github.com/iSundram/Automergent/actions"><img src="https://img.shields.io/badge/build-passing-4ade80?style=flat-square&logo=githubactions" alt="Build Status" /></a>
  <a href="https://github.com/iSundram/Automergent/actions"><img src="https://img.shields.io/badge/tests-passing-4ade80?style=flat-square&logo=github" alt="Tests Status" /></a>
  <a href="https://goreportcard.com/report/github.com/iSundram/Automergent"><img src="https://img.shields.io/badge/go%20report-A%2B-38bdf8?style=flat-square" alt="Go Report Card" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue?style=flat-square" alt="License" /></a>
  <a href="https://github.com/iSundram/Automergent/releases"><img src="https://img.shields.io/badge/release-v0.1.0-c084fc?style=flat-square" alt="Release" /></a>
</p>

---

## ⚡ What is Automergent?

**Automergent** is a high-performance, terminal-native AI pair-programmer that lives in your command line. It reads, writes, searches, tests, and refactors your codebase autonomously or interactively with approval at every step.

> [!NOTE]
> Powered by the Charm v2 stack (`charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`) and tree-sitter AST parsing, Automergent gives you the power of modern IDE coding agents directly inside any terminal or remote SSH session with sub-millisecond responsiveness.

---

## 🖥️ Live TUI Experience

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

## 🚀 Quickstart & Installation

### Option 1: Go Install (Recommended)

```bash
# Requires Go 1.24+ with CGO enabled
CGO_ENABLED=1 go install github.com/iSundram/Automergent/cmd/automergent@latest
```

### Option 2: Build From Source

```bash
git clone https://github.com/iSundram/Automergent.git
cd Automergent

# Build binary into ./bin/automergent
make build

# Install globally to $GOPATH/bin
make install
```

### Option 3: Launch

```bash
cd your-project/
automergent
```

> [!TIP]
> Automergent automatically detects git context, repository root, configuration files, and project languages on startup.

---

## 🧩 Key Capabilities

| Capability | Category | Supported Operations |
| :--- | :--- | :--- |
| **📁 Filesystem** | `read`, `write`, `edit` | `read_file`, `write_file`, `edit_file`, `list_directory`, `glob`, `grep`, `tree` |
| **💻 Terminal PTY** | `shell`, `exec` | Synchronous commands, persistent background shells, interactive PTY sessions, exit status tracking |
| **🧠 Code Intelligence** | `AST`, `tree-sitter` | Tree-sitter syntax parsing, symbol search, AST-aware diff application |
| **🔍 Search & Web** | `grep`, `fetch` | Ripgrep pattern matching, live web fetch, documentation scraping, web search |
| **🩺 Diagnostics & LSP** | `lsp`, `compiler` | Real-time `gopls` / compiler error parsing, semantic error detection, automated recovery |
| **🤖 Multi-Agent Orchestration**| `agents`, `tasks` | Autonomous sub-agents, concurrent background docks, hierarchical task planning |
| **🔌 MCP Protocol** | `mcp`, `tools` | Model Context Protocol server client, custom tool extensions |
| **💾 Session Memory** | `persistence` | JSONL conversation transcripts, undo/redo state stack, project memory |

---

## ⚙️ Configuration & AI Providers

Automergent supports multiple AI backends. Configure your preferred provider via environment variables or `~/.automergent/config.yaml`:

```yaml
# ~/.automergent/config.yaml
provider: "gemini" # gemini | openai | anthropic | deepseek | ollama
model: "gemini-2.5-pro"
temperature: 0.2

# Approval Autonomy Mode: manual | accept-edits | auto | plan
mode: "accept-edits"

# UI Theme
theme: "modern" # modern | catppuccin | tokyonight | dracula | nord | gruvbox | onedark

# Key API Tokens (or set via environment variables)
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

## 🎨 Themes

Automergent includes 10 built-in color themes optimized for truecolor and standard terminal palettes. Switch anytime with `/theme <name>`:

| Theme Name | Style / Palette Description |
| :--- | :--- |
| `modern` *(default)* | Clean obsidian-slate surface with Catppuccin semantic accents |
| `catppuccin` | Soothing Catppuccin Mocha palette with mauve highlights |
| `tokyonight` | Tokyo Night deep navy blue and vibrant neon accents |
| `dracula` | Famous Dracula dark theme with purple and pink accents |
| `nord` | Arctic, north-bluish clean pastel aesthetic |
| `gruvbox` | Retro groove warm brown/amber color scheme |
| `onedark` | Atom One Dark iconic balanced colors |
| `solarized` | Solarized Dark precision contrast palette |
| `monokai` | Classic Monokai high-contrast green/yellow highlights |
| `high-contrast` | OLED true black with maximum readability neon tokens |

---

## ⌨️ Shortcuts & Slash Commands

### Slash Commands

| Command | Action |
| :--- | :--- |
| `/help` | Show interactive command help overlay |
| `/theme <name>` | Switch active color palette in real-time |
| `/model <name>` | Switch active LLM model and provider |
| `/mode <mode>` | Change approval mode (`manual`, `accept-edits`, `auto`, `plan`) |
| `/clear` | Clear active conversation history |
| `/diff` | Open full-screen interactive diff review pane |
| `/tasks` | Open background taskboard and sub-agent dock |

### Keyboard Shortcuts

| Shortcut | Description |
| :--- | :--- |
| `Ctrl + P` | Open fuzzy Command Palette & omni-search |
| `Ctrl + E` | Expand / collapse full tool output & slabs |
| `Ctrl + D` | Toggle interactive Diff reviewer |
| `Ctrl + B` | Toggle background Agent & Process Dock |
| `Shift + Tab` | Switch focus to conversation history browser |
| `Esc` | Cancel running operation / dismiss modals |
| `Ctrl + C` | Send interrupt signal to active shell or agent |

---

## 🏗️ Architecture

```text
Automergent/
├── cmd/
│   ├── automergent/      # Primary CLI entrypoint & command bootstrapping
│   └── installer/        # Self-updating standalone installer
├── internal/
│   ├── agent/            # AI agent loop, orchestration, planning & approval policies
│   ├── tui/              # Bubble Tea v2 TUI, Lip Gloss styles & Glamour rendering
│   │   ├── app/          # Core application event loop, layout & view composition
│   │   ├── components/   # Modular UI widgets (Header, StatusBar, Tools, Diff, Palette)
│   │   ├── themes/       # 10 built-in color palettes & syntax styling engine
│   │   └── render/       # ANSI streaming parser, diff engine & Markdown renderer
│   ├── tools/            # 48 extensible tools (FS, Terminal, AST, Web, Diagnostics)
│   ├── diagnostics/      # Compiler & LSP diagnostic parsers and error recovery
│   ├── mcp/              # Model Context Protocol client & server transport
│   ├── config/           # Viper configuration loader & validation
│   └── session/          # Persistent transcripts, history & undo stack
└── Makefile              # Build, test, lint, and release recipes
```

---

## 🧪 Development & Testing

```bash
# Run all unit and integration tests
make test

# Run linter checks
make lint

# Format code and imports
make fmt

# Run continuous integration checks locally
make ci
```

---

## 📄 License

Distributed under the [MIT License](LICENSE). Built with ❤️ by [iSundram](https://github.com/iSundram) and contributors.
