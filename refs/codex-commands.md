# Codex Commands System Analysis

This document provides a comprehensive analysis of the Codex commands system, covering command registration, execution flow, built-in vs custom commands, aliases, REPL integration, slash commands, permissions, and output formatting.

---

## 1. Command Registration and Discovery

### 1.1 CLI Subcommand Registration (codex-rs/cli/src/main.rs)

The top-level CLI commands are registered using **clap** derive macros in `/tmp/opencode/refs/codex/codex-rs/cli/src/main.rs` (lines 101-222).

```rust
#[derive(Debug, Parser)]
#[clap(
    author,
    version,
    subcommand_negates_reqs = true,
    bin_name = "codex",
    override_usage = "codex [OPTIONS] [PROMPT]\n       codex [OPTIONS] <COMMAND> [ARGS]"
)]
struct MultitoolCli {
    #[clap(flatten)]
    pub config_overrides: CliConfigOverrides,
    #[clap(flatten)]
    pub feature_toggles: FeatureToggles,
    #[clap(flatten)]
    remote: InteractiveRemoteOptions,
    #[clap(flatten)]
    interactive: TuiCli,
    #[clap(subcommand)]
    subcommand: Option<Subcommand>,
}
```

The `Subcommand` enum (lines 130-222) defines all top-level commands:

- **Exec** (`e` alias) - Non-interactive execution
- **Review** - Code review
- **Login** / **Logout** - Authentication management
- **Mcp** - MCP server management
- **Plugin** - Plugin management
- **McpServer** - Run as MCP server
- **AppServer** - App server tooling
- **RemoteControl** - Remote control daemon
- **App** (macOS/Windows) - Desktop app
- **Completion** - Shell completions
- **Update** - Self-update
- **Doctor** - Diagnostics
- **Sandbox** - Host sandbox commands
- **Debug** - Debugging tools
- **Execpolicy** - Execpolicy tooling (hidden)
- **Apply** (`a` alias) - Apply diffs
- **Resume** - Resume sessions
- **Archive** / **Unarchive** / **Delete** - Session management
- **MigrateRollouts** - Legacy migration
- **Fork** - Fork sessions
- **Cloud** (`cloud-tasks` alias) - Cloud tasks
- **ResponsesApiProxy** / **StdioToUds** - Internal
- **ExecServer** - Exec server
- **Features** - Feature flags

### 1.2 Slash Command Registration (codex-rs/tui/src/slash_command.rs)

Slash commands are defined as an enum in `/tmp/opencode/refs/codex/codex-rs/tui/src/slash_command.rs` (lines 8-80):

```rust
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, EnumString, EnumIter, AsRefStr, IntoStaticStr)]
#[strum(serialize_all = "kebab-case")]
pub enum SlashCommand {
    Model,
    Ide,
    Permissions,
    Keymap,
    Vim,
    #[strum(serialize = "setup-default-sandbox")]
    ElevateSandbox,
    #[strum(serialize = "sandbox-add-read-dir")]
    SandboxReadRoot,
    Experimental,
    #[strum(to_string = "approve")]
    AutoReview,
    Memories,
    Skills,
    Import,
    Hooks,
    Review,
    Rename,
    New,
    Archive,
    Delete,
    Resume,
    Fork,
    App,
    Init,
    Compact,
    Plan,
    Goal,
    Agent,
    Side,
    Btw,
    Copy,
    Export,
    Raw,
    Diff,
    Mention,
    Status,
    Usage,
    DebugConfig,
    Title,
    Statusline,
    Theme,
    #[strum(to_string = "pets", serialize = "pet")]
    Pets,
    Mcp,
    Apps,
    Plugins,
    Logout,
    Quit,
    Exit,
    Feedback,
    Rollout,
    Ps,
    #[strum(to_string = "stop", serialize = "clean")]
    Stop,
    Clear,
    Personality,
    TestApproval,
    #[strum(serialize = "subagents")]
    MultiAgents,
    #[strum(serialize = "debug-m-drop")]
    MemoryDrop,
    #[strum(serialize = "debug-m-update")]
    MemoryUpdate,
}
```

Key registration mechanisms:
- **strum macros** - Auto-generates string serialization/parsing
- **serialize_all = "kebab-case"** - Converts enum variants to kebab-case
- **Custom serialize/to_string** - For aliases (e.g., `/approve` → `AutoReview`, `/pet` → `Pets`, `/clean` → `Stop`, `/subagents` → `MultiAgents`)
- **built_in_slash_commands()** (lines 266-271) - Returns all visible commands for popup display

### 1.3 Feature-Gated Command Discovery

Commands are filtered based on feature flags in `/tmp/opencode/refs/codex/codex-rs/tui/src/bottom_pane/slash_commands.rs` (lines 70-82):

```rust
pub(crate) fn builtins_for_input(flags: BuiltinCommandFlags) -> Vec<(&'static str, SlashCommand)> {
    built_in_slash_commands()
        .into_iter()
        .filter(|(_, cmd)| flags.allow_elevate_sandbox || *cmd != SlashCommand::ElevateSandbox)
        .filter(|(_, cmd)| flags.collaboration_modes_enabled || *cmd != SlashCommand::Plan)
        .filter(|(_, cmd)| flags.connectors_enabled || *cmd != SlashCommand::Apps)
        .filter(|(_, cmd)| flags.plugins_command_enabled || *cmd != SlashCommand::Plugins)
        .filter(|(_, cmd)| flags.token_activity_command_enabled || *cmd != SlashCommand::Usage)
        .filter(|(_, cmd)| flags.goal_command_enabled || *cmd != SlashCommand::Goal)
        .filter(|(_, cmd)| flags.personality_command_enabled || *cmd != SlashCommand::Personality)
        .filter(|(_, cmd)| !flags.side_conversation_active || cmd.available_in_side_conversation())
        .collect()
}
```

The `BuiltinCommandFlags` struct (lines 56-67) controls visibility:
- `collaboration_modes_enabled` - Gates `/plan`
- `connectors_enabled` - Gates `/apps`
- `plugins_command_enabled` - Gates `/plugins`
- `token_activity_command_enabled` - Gates `/usage`
- `goal_command_enabled` - Gates `/goal`
- `personality_command_enabled` - Gates `/personality`
- `allow_elevate_sandbox` - Gates `/setup-default-sandbox` (Windows only)
- `side_conversation_active` - Filters to commands available in side conversations

---

## 2. Command Execution Flow

### 2.1 CLI Subcommand Execution (main.rs)

The main execution flow in `cli_main()` (lines 993-1448) dispatches based on `Subcommand`:

```rust
match subcommand {
    None => {
        // Interactive TUI mode
        let exit_info = run_interactive_tui(...).await?;
        handle_app_exit(exit_info)?;
    }
    Some(Subcommand::Exec(mut exec_cli)) => {
        codex_exec::run_main(exec_cli, arg0_paths.clone()).await?;
    }
    Some(Subcommand::Review(...)) => { ... }
    Some(Subcommand::Mcp(mut mcp_cli)) => { ... }
    // ... other subcommands
}
```

### 2.2 Slash Command Execution (chatwidget/slash_dispatch.rs)

Slash commands are dispatched via `dispatch_command()` (lines 147-553) and `dispatch_command_with_args()` (lines 560-941).

**Flow for bare slash commands** (`/command`):
1. `ChatComposer` parses input, detects leading `/`
2. Returns `InputResult::Command(SlashCommand)`
3. `handle_composer_input_result()` calls `handle_slash_command_dispatch()`
4. `dispatch_command()` validates and executes

**Flow for inline slash commands** (`/command args`):
1. `ChatComposer` detects command with args
2. Returns `InputResult::CommandWithArgs(cmd, args, text_elements)`
3. `handle_composer_input_result()` calls `handle_slash_command_with_args_dispatch()`
4. `dispatch_command_with_args()` prepares args and executes

### 2.3 Validation Gates in Dispatch

Before execution, commands pass through validation (lines 148-163):

```rust
if !self.ensure_slash_command_allowed_in_side_conversation(cmd) { return; }
if !self.ensure_side_command_allowed_outside_review(cmd) { return; }
if self.slash_command_blocked_by_active_task(cmd) { 
    // Shows error, returns early
}
```

- `available_in_side_conversation()` - Only certain commands work in side conversations
- `available_during_task()` - Some commands blocked while agent is running
- `supports_inline_args()` - Only some commands accept inline arguments

---

## 3. Built-in Commands vs Custom Commands

### 3.1 Built-in Commands

All slash commands are **built-in** and defined in the `SlashCommand` enum. There is **no user-defined custom slash command system** in the current codebase.

Built-in commands are categorized by functionality:

| Category | Commands |
|----------|----------|
| **Session Management** | `/new`, `/archive`, `/delete`, `/resume`, `/fork`, `/clear`, `/compact` |
| **Model & Config** | `/model`, `/permissions`, `/experimental`, `/keymap`, `/vim`, `/personality`, `/theme` |
| **Collaboration** | `/plan`, `/goal`, `/agent`, `/side`, `/btw`, `/multi-agents` (alias: `/subagents`) |
| **IDE Integration** | `/ide` |
| **Tools & Extensions** | `/mcp`, `/apps`, `/plugins`, `/skills`, `/hooks` |
| **Memory** | `/memories`, `/debug-m-drop`, `/debug-m-update` |
| **Review & Approval** | `/review`, `/approve` (alias for auto-review) |
| **Session I/O** | `/copy`, `/export`, `/diff`, `/raw`, `/rename` |
| **Status & Info** | `/status`, `/usage`, `/ps`, `/rollout`, `/debug-config`, `/title`, `/statusline` |
| **Pets & Fun** | `/pets` (alias: `/pet`) |
| **Control** | `/quit`, `/exit`, `/logout`, `/stop` (alias: `/clean`), `/init`, `/feedback` |

### 3.2 Plugin/Marketplace Commands (Indirect Customization)

While there's no direct custom slash command API, users can extend functionality via:

1. **MCP Servers** (`/mcp`) - External tools exposed as MCP servers
2. **Plugins** (`/plugins`) - NPM packages that can contribute MCP servers
3. **Apps/Connectors** (`/apps`) - Pre-built integrations (Google Drive, GitHub, etc.)
4. **Skills** (`/skills`) - Reusable prompt templates

Configuration in `config.toml`:
```toml
# Enable/disable plugins
[plugins]
enabled = true

# Per-plugin MCP server config
[plugins.my-plugin.mcp_servers.server-name]
enabled = true
enabled_tools = ["tool1", "tool2"]

# Apps config
[apps.google_drive]
enabled = true
```

### 3.3 Shell Commands (`!` prefix)

Users can run shell commands directly in the TUI by prefixing with `!`:
- Handled in `chat_composer.rs` via `QueuedInputAction::RunShell`
- Executed via `WorkspaceCommandRunner`

---

## 4. Command Aliases and Shortcuts

### 4.1 Slash Command Aliases (via strum attributes)

| Alias | Canonical Command | Defined In |
|-------|------------------|------------|
| `/approve` | `/auto-review` | `#[strum(to_string = "approve")]` |
| `/pet` | `/pets` | `#[strum(to_string = "pets", serialize = "pet")]` |
| `/clean` | `/stop` | `#[strum(to_string = "stop", serialize = "clean")]` |
| `/subagents` | `/multi-agents` | `#[strum(serialize = "subagents")]` |
| `/setup-default-sandbox` | `/elevate-sandbox` | `#[strum(serialize = "setup-default-sandbox")]` |
| `/sandbox-add-read-dir` | `/sandbox-read-root` | `#[strum(serialize = "sandbox-add-read-dir")]` |
| `/debug-m-drop` | `/memory-drop` | `#[strum(serialize = "debug-m-drop")]` |
| `/debug-m-update` | `/memory-update` | `#[strum(serialize = "debug-m-update")]` |
| `/gooooooooal` (any o's) | `/goal` | Special parsing in `find_builtin_command()` |

### 4.2 CLI Subcommand Aliases (main.rs)

| Alias | Canonical Command |
|-------|------------------|
| `codex e` | `codex exec` |
| `codex a` | `codex apply` |
| `codex cloud-tasks` | `codex cloud` |

### 4.3 Goal Command Flexible Parsing

The `/goal` command accepts variations with extra 'o's (e.g., `/goooal`) via special logic in `find_builtin_command()` (lines 113-117):

```rust
let cmd = SlashCommand::from_str(name).ok().or_else(|| {
    let repeated_os = name.strip_prefix('g')?.strip_suffix("al")?;
    (!repeated_os.is_empty() && repeated_os.bytes().all(|byte| byte == b'o'))
        .then_some(SlashCommand::Goal)
})?;
```

---

## 5. Integration with REPL/Main Loop

### 5.1 Main Entry Points

**Interactive TUI (REPL):**
- `cli_main()` → `run_interactive_tui()` → `App::run()`
- Main loop in `tui/src/tui.rs` handles events
- `App::handle_tui_event()` processes key events

**Non-interactive (exec):**
- `cli_main()` → `codex_exec::run_main()`
- Runs single prompt and exits

### 5.2 ChatComposer Input Processing

The `ChatComposer` in `bottom_pane/chat_composer.rs` handles all text input:

1. **Key events** → `handle_key_event()`
2. **Slash detection** → `SlashInput` validates and parses
3. **Popup** → `CommandPopup` for completion
4. **Submission** → Returns `InputResult` variants:
   - `Command(SlashCommand)` - Bare slash command
   - `CommandWithArgs(cmd, args, elements)` - Inline args
   - `ServiceTierCommand` - Model tier selection
   - `Submitted { text, elements }` - Regular message
   - `Queued { ... }` - Deferred during task

### 5.3 App Event Loop

The `App` struct in `tui/src/app.rs` runs the main event loop:

```rust
pub(crate) async fn handle_tui_event(
    &mut self,
    tui: &mut tui::Tui,
    app_server: &mut AppServerSession,
    event: TuiEvent,
) -> Result<AppRunControl>
```

Events flow:
1. `TuiEvent::Key` → `handle_key_event()` → `ChatWidget::handle_key_event()`
2. `ChatWidget` → `ChatComposer` → `InputResult`
3. `handle_composer_input_result()` processes result
4. Slash commands → `dispatch_command()` → `AppEvent` sent to app_server
5. Results rendered via `ChatWidget::render()`

---

## 6. Slash Commands vs Regular Commands

### 6.1 Slash Commands (TUI-internal)

- **Trigger**: Leading `/` in composer
- **Parsing**: `parse_slash_name()` extracts command name and args
- **Completion**: `CommandPopup` with fuzzy matching
- **Dispatch**: `ChatWidget::dispatch_command()` / `dispatch_command_with_args()`
- **Context-aware**: Respects side conversations, active tasks, review mode
- **History**: Recorded in local recall (up-arrow history)

### 6.2 Regular CLI Commands (Subcommands)

- **Trigger**: `codex <subcommand> [args]`
- **Parsing**: clap derives on `Subcommand` enum
- **No completion popup** (shell completions via `codex completion`)
- **Direct execution**: No TUI, runs and exits
- **Examples**: `codex exec`, `codex login`, `codex mcp add`

### 6.3 Shell Commands (`!` prefix)

- **Trigger**: Leading `!` in composer
- **Execution**: Via `WorkspaceCommandRunner` (spawns shell)
- **Output**: Captured and displayed in transcript
- **No slash command parsing**

---

## 7. Command Permissions and Validation

### 7.1 Runtime Permission Checks

**Side Conversation Restrictions** (`slash_dispatch.rs:1170-1193`):
```rust
fn ensure_slash_command_allowed_in_side_conversation(&mut self, cmd: SlashCommand) -> bool {
    if !self.active_side_conversation || cmd.available_in_side_conversation() {
        return true;
    }
    self.add_error_message(format!(
        "'/{}' is unavailable in side conversations. Press Ctrl+C to return to the main thread first.",
        cmd.command()
    ));
    false
}
```

Only these commands work in side conversations:
- `/copy`, `/export`, `/raw`, `/diff`, `/mention`, `/status`, `/usage`, `/ide`

**Review Mode Restrictions** (`slash_dispatch.rs:1182-1193`):
- `/side` and `/btw` blocked during code review

**Task Blocking** (`slash_dispatch.rs:134-145`):
```rust
fn slash_command_blocked_by_active_task(&self, cmd: SlashCommand) -> bool {
    (!cmd.available_during_task()
        && (self.turn_lifecycle.agent_turn_running
            || self.review.is_review_mode
            || (self.bottom_pane.is_task_running()
                && (self.mcp_startup_status.is_none()
                    || self.input_queue.user_turn_pending_start))))
    || (cmd == SlashCommand::Resume && (self.input_queue.user_turn_pending_start || self.turn_lifecycle.agent_turn_running))
    || (cmd == SlashCommand::Export && self.input_queue.suppress_queue_autosend)
}
```

### 7.2 Command-Specific Permissions

| Command | Permission Requirement |
|---------|----------------------|
| `/usage` | Requires Codex backend auth (`has_codex_backend_auth`) |
| `/mcp` | Requires `connectors_enabled` feature |
| `/plugins` | Requires `plugins` feature enabled |
| `/plan` | Requires `collaboration_modes_enabled` |
| `/goal` | Requires `Goals` feature |
| `/personality` | Requires `Personality` feature |
| `/setup-default-sandbox` | Windows only, requires degraded sandbox mode |
| `/auto-review` (approve) | Requires auto-review denials to exist |

### 7.3 Approval/Permission Profiles

Commands that modify permissions trigger approval flows:
- `/permissions` → Opens permission profile picker
- `/approve` → Approves auto-review denial
- `/review` → Opens review configuration

---

## 8. Output Formatting for Commands

### 8.1 Popup/Selection Views

Most slash commands open interactive popups via `BottomPane::show_selection_view()`:

```rust
self.bottom_pane.show_selection_view(SelectionViewParams {
    title: Some("Archive this session?".to_string()),
    subtitle: Some("Are you sure? ...".to_string()),
    footer_hint: Some(standard_popup_hint_line()),
    items: vec![
        SelectionItem { name: "No, don't archive", ... },
        SelectionItem { 
            name: "Yes, archive and exit", 
            actions: vec![Box::new(|tx| tx.send(AppEvent::ArchiveCurrentThread))],
            ...
        },
    ],
    ..
});
```

### 8.2 Info/Error Messages

Added to history via `add_info_message()` / `add_error_message()`:
```rust
self.add_info_message(
    GOAL_USAGE.to_string(),
    Some(GOAL_USAGE_HINT.to_string()),
);
self.add_error_message("Usage: /raw [on|off]".to_string());
```

### 8.3 Specialized Output Methods

| Command | Output Method |
|---------|--------------|
| `/diff` | Spawns async task, sends `AppEvent::DiffResult` |
| `/status` | `add_status_output()` with rate limit fetching |
| `/usage` | `open_usage_menu()` / `add_token_activity_output()` |
| `/ps` | `add_ps_output()` - lists background terminals |
| `/mcp` | `add_mcp_output()` with detail level |
| `/hooks` | `add_hooks_output()` |
| `/rollout` | Shows rollout file path |
| `/debug-config` | `add_debug_config_output()` |
| `/memories` | `open_memories_popup()` |
| `/skills` | `open_skills_menu()` |

### 8.4 Transcript Export

`/export` supports multiple destinations:
```rust
AppEvent::ExportTranscript {
    destination: TranscriptExportDestination::File(PathBuf),
}
```

### 8.5 Raw Output Mode

`/raw` toggles raw scrollback mode:
```rust
fn toggle_raw_output_mode_and_notify(&mut self) -> bool {
    let enabled = !self.config.tui.raw_output_mode;
    self.config.tui.raw_output_mode = enabled;
    self.emit_raw_output_mode_changed(enabled);
    enabled
}
```

---

## 9. Key Implementation Files Summary

| File | Purpose |
|------|---------|
| `codex-rs/cli/src/main.rs` | CLI entry point, subcommand registration, dispatch |
| `codex-rs/cli/src/lib.rs` | Shared CLI types, sandbox commands |
| `codex-rs/tui/src/slash_command.rs` | Slash command enum, metadata, aliases |
| `codex-rs/tui/src/chatwidget/slash_dispatch.rs` | Slash command dispatch logic, validation, execution |
| `codex-rs/tui/src/bottom_pane/slash_commands.rs` | Command filtering, discovery, fuzzy matching |
| `codex-rs/tui/src/bottom_pane/prompt_args.rs` | Slash command parsing from input |
| `codex-rs/tui/src/bottom_pane/chat_composer/slash_input.rs` | Input handling, completion popup |
| `codex-rs/tui/src/bottom_pane/command_popup.rs` | Command completion UI |
| `codex-rs/tui/src/app.rs` | Main app loop, event handling |
| `codex-rs/tui/src/app_command.rs` | App-level command types sent to app-server |
| `codex-rs/config/src/types.rs` | Configuration types for plugins, apps, skills |

---

## 10. Architecture Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                        USER INPUT                           │
└──────────────────────────┬──────────────────────────────────┘
                           │
          ┌────────────────┼────────────────┐
          ▼                ▼                ▼
   ┌─────────────┐  ┌─────────────┐  ┌─────────────┐
   │ CLI Args    │  │ TUI Composer│  │ Shell (!)   │
   │ (clap)      │  │ (/)         │  │             │
   └──────┬──────┘  └──────┬──────┘  └──────┬──────┘
          │                │                │
          ▼                ▼                ▼
   ┌─────────────┐  ┌─────────────┐  ┌─────────────┐
   │ Subcommand  │  │ SlashInput  │  │ Workspace   │
   │ Enum        │  │ + Parser    │  │ Command     │
   └──────┬──────┘  └──────┬──────┘  └──────┬──────┘
          │                │                │
          ▼                ▼                ▼
   ┌─────────────┐  ┌─────────────┐  ┌─────────────┐
   │ Direct      │  │ CommandPopup│  │ Shell       │
   │ Execution   │  │ (fuzzy match)             │
   └─────────────┘  └──────┬──────┘  └─────────────┘
                           │
                           ▼
                  ┌─────────────────┐
                  │ ChatWidget      │
                  │ dispatch_       │
                  │ command()       │
                  └────────┬────────┘
                           │
                           ▼
                  ┌─────────────────┐
                  │ Validation      │
                  │ - side conv     │
                  │ - review mode   │
                  │ - active task   │
                  └────────┬────────┘
                           │
                           ▼
                  ┌─────────────────┐
                  │ AppEvent        │
                  │ (to app_server) │
                  └────────┬────────┘
                           │
                           ▼
                  ┌─────────────────┐
                  │ Output:         │
                  │ - Popups        │
                  │ - Messages      │
                  │ - Async tasks   │
                  │ - Config changes│
                  └─────────────────┘
```

---

## 11. Extending the Command System

### Adding a New Slash Command

1. **Add enum variant** in `slash_command.rs`:
```rust
#[strum(serialize = "my-command")]
MyCommand,
```

2. **Add description** in `description()` method

3. **Set capabilities** in:
- `supports_inline_args()`
- `available_in_side_conversation()`
- `available_during_task()`
- `is_visible()` (for platform/debug gating)

4. **Add dispatch case** in `dispatch_command()` and/or `dispatch_command_with_args()`

5. **Add feature flag** if needed in `BuiltinCommandFlags` and `slash_commands.rs`

### Adding a New CLI Subcommand

1. **Add variant** to `Subcommand` enum in `main.rs`
2. **Define args struct** with clap derives
3. **Add match arm** in `cli_main()` dispatch
4. **Implement handler** function

---

*Generated from codex-rs codebase analysis as of 2026-08-28*
