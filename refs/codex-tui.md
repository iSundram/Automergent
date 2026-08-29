# Codex TUI Architecture Analysis

## Overview

The Codex TUI (Terminal User Interface) is a sophisticated Rust-based terminal application built on **ratatui** (a Rust TUI framework using crossterm for terminal backend). It provides an interactive chat interface for AI-assisted coding with rich markdown rendering, streaming output, diff visualization, and multi-agent support.

---

## 1. Main TUI Framework

### Framework Stack
- **Primary Framework**: **ratatui** v0.28+ (Rust TUI library)
- **Terminal Backend**: **crossterm** (cross-platform terminal manipulation)
- **Async Runtime**: **tokio** (async event loop and task management)
- **Event Processing**: Custom event broker with broadcast channels
- **Terminal Detection**: `codex-terminal-detection` crate for terminal capabilities

### Entry Point
The TUI is initialized through `codex-rs/tui/src/tui.rs`:
- `Tui::new()` creates the terminal with raw mode, alternate screen, bracketed paste, and keyboard enhancement
- `set_modes()` configures terminal: raw mode, bracketed paste, focus change events, keyboard enhancement flags
- `init()` performs startup probes (cursor position, default colors, keyboard enhancement support)

### Architecture Pattern
The TUI follows a **Model-View-Controller-like** separation:
- **App** (`app.rs:516`): Top-level application state machine
- **ChatWidget** (`chatwidget.rs`): Main chat surface with history and active cell
- **BottomPane** (`bottom_pane/mod.rs`): Footer with composer and popup stack
- **Tui** (`tui.rs:578`): Terminal abstraction, frame scheduling, event stream

---

## 2. Rendering System

### Core Rendering Abstractions (`render/renderable.rs`)

The rendering system uses a **trait-based composable widget pattern**:

```rust
pub trait Renderable {
    fn render(&self, area: Rect, buf: &mut Buffer);
    fn desired_height(&self, width: u16) -> u16;
    fn cursor_pos(&self, _area: Rect) -> Option<(u16, u16)>;
    fn cursor_style(&self, _area: Rect) -> SetCursorStyle;
}
```

### Layout Primitives
- **ColumnRenderable**: Vertical stack with auto-height children
- **FlexRenderable**: Flexbox-like column layout with proportional sizing (Flutter-inspired)
- **RowRenderable**: Horizontal row with fixed-width children
- **InsetRenderable**: Wrapper adding padding/margins

### Dual Draw Paths (`tui.rs`)

1. **Legacy Draw Path** (`Tui::draw`): Uses `pending_viewport_area()` heuristic with cursor position queries
2. **Resize-Reflow Path** (`Tui::draw_with_resize_reflow`): Feature-gated path that rebuilds scrollback from transcript source on resize (`resize_reflow` module)

### Synchronized Updates
All terminal writes use `stdout().sync_update(|_| { ... })` (crossterm's `SynchronizedUpdate`) to prevent tearing during resize/suspend.

### Viewport Management
- Inline viewport (normal mode): History stays in scrollback, composer fixed at bottom
- Alternate screen (overlay mode): Full-screen transcript pager, diff views
- `enter_alt_screen()` / `leave_alt_screen()` manage transitions with viewport save/restore

### History Insertion (`insert_history.rs`)
- `InsertHistoryMode::Standard`: Normal scroll region insertion
- `InsertHistoryMode::ZellijRaw`: Special handling for Zellij multiplexer
- Batched pending lines flushed on each draw

---

## 3. Keybindings and Input Handling

### Keymap Architecture (`keymap.rs`, `keymap_setup.rs`)

#### Resolution Precedence
1. Context-specific binding (`tui.keymap.<context>`)
2. Global fallback (`tui.keymap.global`)
3. Built-in defaults

#### Keymap Contexts (`keymap.rs:48-73`)
- `AppKeymap`: Global app actions (transcript, editor, vim mode, fast mode, raw output)
- `ChatKeymap`: Chat-level (interrupt, reasoning effort, edit queued message)
- `ComposerKeymap`: Input composer (submit, queue, shortcuts, history search)
- `EditorKeymap`: Text editing (movement, deletion, kill/yank)
- `VimNormalKeymap` / `VimOperatorKeymap` / `VimTextObjectKeymap`: Modal editing
- `PagerKeymap`: Transcript overlay navigation
- `ListKeymap`: Selection list navigation
- `ApprovalKeymap`: Approval dialog actions

#### KeyChord Support (`chords.rs`)
- Two-stroke key chords with `KEY_CHORD_TIMEOUT` (default 300ms)
- `KeyChordMatcher` tracks pending first key
- Configurable via `tui.keymap.chords`

#### Conflict Validation
- Prevents duplicate keys within same context
- App + Composer uniqueness validated (app handlers execute first)
- Global fallback only for actions marked `supports_global_fallback`

### Input Event Flow (`app.rs:752-846`)

```
TuiEvent::Key -> route_key_chord_event() -> handle_key_event()
                    |
                    v
            TuiEvent::Paste -> chat_widget.handle_paste()
                    |
                    v
            TuiEvent::Draw/Resize/Resume -> render_chat_widget_frame()
```

### Paste Burst Handling (`bottom_pane/paste_burst.rs`)
- Detects rapid character bursts (non-bracketed paste on Windows)
- `PasteBurst` state machine buffers and flushes as explicit paste
- Differentiates ASCII (hold first char) vs non-ASCII (IME-friendly)

### Vim Mode (`bottom_pane/textarea.rs`)
- Full modal editing: Normal, Insert, Visual, Operator-pending
- Keybindings configurable per mode
- Operator + motion composition (dw, yy, ci", etc.)

---

## 4. Screen Layouts

### Main Layout Structure
```
┌─────────────────────────────────────┐
│           Transcript Area           │  ← ChatWidget renders HistoryCells
│   (committed + active streaming)    │
├─────────────────────────────────────┤
│         Status Line (optional)      │  ← Footer with model, git, tokens
├─────────────────────────────────────┤
│         Chat Composer               │  ← BottomPane with textarea
│   [attachments] [input] [hints]     │
└─────────────────────────────────────┘
```

### Layout Regions (`chatwidget.rs`, `bottom_pane/footer.rs`)
- **Transcript**: Variable height, fills available space
- **Status Line**: Configurable via `/statusline`, rendered in footer
- **Composer**: Fixed at bottom, expands for multi-line input
- **Footer Hints**: Contextual (quit reminder, shortcuts, queue hint, mode indicator)

### Overlay Layouts (`pager_overlay.rs`)
- **TranscriptOverlay**: Full-screen pager with committed + live tail
- **StaticOverlay**: Generic pager for diffs, help, debug views
- Side-by-side preview when width permits (e.g., theme picker)

### Responsive Behavior (`bottom_pane/footer.rs:21-39`)
Single-line footer collapse rules:
1. Full left hint + right context
2. Queue hint prioritized over right context
3. "? for shortcuts" dropped before "(shift+tab to cycle)"
4. Mode-only fallback
5. Empty footer if nothing fits

---

## 5. Interactive Elements

### Popup/Modal System (`bottom_pane/`)

#### Selection Views (`list_selection_view.rs`)
- **ListSelectionView**: Generic selector with columns, descriptions, tabs
- Side-by-side or stacked layout based on width
- Keyboard navigation (arrows, vim keys, page up/down, home/end)
- Multi-select support with toggle column

#### Approval Overlays (`approval_overlay.rs`)
- **ApplyPatchApprovalRequest**: File diff with accept/reject
- **ExecApprovalRequest**: Command execution with network policy options
- **PermissionsApprovalRequest**: Permission profile changes
- **McpElicitationApprovalRequest**: MCP server form elicitation

#### Specialized Pickers
- **AgentPicker** (`agent_picker.rs`): Switch between sub-agent threads
- **ThemePicker** (`theme_picker.rs`): Live syntax theme preview
- **Resume/Fork Pickers** (`resume_picker.rs`): Session selection with transcript preview
- **Keymap Editor** (`keymap_setup.rs`): Guided key remapping with capture
- **Status Line Builder** (`status_line_setup.rs`): Drag-drop segment configuration
- **Hooks Browser** (`hooks_browser_view.rs`): Hook management
- **Skills Toggle** (`skills_toggle_view.rs`): Skill enable/disable

#### Custom Prompt (`custom_prompt_view.rs`)
- Arbitrary prompt with validation callback
- Used for confirmations, text input, etc.

### Transcript Overlay (`pager_overlay.rs:54-57`)
- `Overlay::Transcript(TranscriptOverlay)`: Full history with live tail sync
- `Overlay::Static(StaticOverlay)`: Renderable-based content (diffs, help)
- Cached live tail with `ActiveCellTranscriptKey` invalidation

### External Editor Integration (`external_editor.rs`)
- Launches `$EDITOR` with current draft
- `with_restored()` temporarily restores terminal for external process
- Resumes TUI modes after editor exits

---

## 6. Status Bars, Progress Indicators

### Status Line (`bottom_pane/footer.rs`, `status/`)
Configurable via `/statusline` with segments:
- **Model**: Current model name with reasoning effort indicator
- **Directory**: Working directory (truncated)
- **Git Branch**: Current branch with status
- **Context Usage**: Token percentage with progress bar
- **Rate Limits**: Per-model limits with reset countdown
- **Account**: Plan type, usage link
- **Collaboration Mode**: Plan mode indicator
- **Agents**: Sub-agent activity summary

### Progress Indicators

#### Spinner Animation (`frames.rs`)
- 12 built-in spinner variants (default, codex, openai, blocks, dots, hash, hbars, vbars, shapes, slug)
- 36 frames each, 80ms tick interval
- `ascii_animation.rs` drives frame cycling

#### Task Running State (`chatwidget.rs:21-23`)
- Derived from `agent_turn_running` + `mcp_startup_status`
- Drives spinner in footer and interrupt hint
- `update_task_running_state()` synchronizes

#### Streaming Animation (`chatwidget.rs`)
- `CommitTick` events at `TARGET_FRAME_INTERVAL` (16ms/60fps)
- Smooth streaming: one line per tick
- Backlog draining: multiple lines per tick

#### Shimmer Effect (`shimmer.rs`)
- Animated gradient for loading states
- Applied to skeleton placeholders

#### Ambient Pet (`pets.rs`)
- Animated character in transcript background
- Multiple animation variants
- Rendered via direct terminal writes in `sync_update`

### Rate Limit Display (`status/rate_limits.rs`)
- Progress bars for token windows
- Reset credit indicators
- "Refreshing..." state during fetch

---

## 7. Theme/Styling System

### Terminal Palette (`terminal_palette.rs`)
- **Color Level Detection**: TrueColor, Ansi256, Ansi16, Unknown
- **Startup Probe**: Queries OSC 10/11 for default fg/bg
- **Windows Terminal Detection**: Forces TrueColor via `WT_SESSION`
- **Best Color Matching**: Perceptual distance (CIE76) for Ansi256 fallback

### Theme Configuration
- Syntax themes: Bundled + custom `.tmTheme` from `{CODEX_HOME}/themes/`
- Theme picker with live preview (`theme_picker.rs`)
- Persisted via `config.toml` `[tui] theme = "..."`

### Style Helpers (`style.rs`)
```rust
fn accent_style() -> Style           // Cyan bold (dark), Dark cyan bold (light)
fn user_message_style() -> Style     // Subtle bg tint based on terminal bg
fn proposed_plan_style() -> Style    // Same as user message
fn table_separator_style() -> Style  // Blended fg/bg at 20% alpha
```

### Light/Dark Adaptation (`color.rs:1-5`)
```rust
fn is_light(bg: (u8,u8,u8)) -> bool {
    let y = 0.299*r + 0.587*g + 0.114*b;
    y > 128.0
}
```

### Markdown Styles (`markdown_render.rs:90-126`)
- Headings (H1-H6): bold, italic, underline combinations
- Code: cyan
- Links: cyan underline
- Blockquote: green
- List markers: light blue (ordered), default (unordered)

### Diff Rendering (`diff_render.rs`)
- Line types: Context, Insert, Delete, Hunk header
- Syntax-highlighted diff content
- Line numbers with configurable width

---

## 8. Component Composition Patterns

### Renderable Trait Composition (`render/renderable.rs`)
- **Trait Objects**: `Box<dyn Renderable>` for heterogeneous collections
- **Arc<Renderable>**: Shared renderable references
- **ColumnRenderable/FlexRenderable/RowRenderable**: Layout combinators
- **InsetRenderable**: Decorator pattern for padding

### Widget State Machine Pattern
Components own their state and expose:
- `render(area, buf)` - pure rendering
- `handle_event(event) -> Result<()>` - input handling
- `desired_height(width)` - layout negotiation
- `cursor_pos(area)` / `cursor_style(area)` - cursor management

### Event Bus (`app_event.rs`, `app_event_sender.rs`)
- `AppEvent` enum: 80+ variants for all UI actions
- `AppEventSender`: `mpsc::UnboundedSender<AppEvent>` cloneable handle
- Widgets emit events; `App` handles in central loop
- Decouples UI components from business logic

### ChatWidget Architecture (`chatwidget.rs`)
- **Committed Cells**: `Vec<Arc<dyn HistoryCell>>` - immutable history
- **Active Cell**: Single mutable cell for in-flight streaming
- **Transcript Overlay Sync**: `ActiveCellTranscriptKey` cache invalidation
- **Cell Types**: User, Assistant, ToolCall, ToolResult, Patch, Exec, System, etc.

### HistoryCell Trait (`history_cell.rs`)
```rust
pub trait HistoryCell: Send + Sync {
    fn render(&self, area: Rect, buf: &mut Buffer, ...);
    fn desired_height(&self, width: u16) -> u16;
    fn as_any(&self) -> &dyn Any;
}
```
Implementations: `PlainHistoryCell`, `CompositeHistoryCell`, `SessionInfoCell`, `UserHistoryCell`, etc.

### BottomPane View Stack (`bottom_pane/mod.rs`)
- Base: `ChatComposer` (always present)
- Overlay: `Vec<BottomPaneView>` - transient popups
- Input routing: View first, then composer, then app-level
- View lifecycle: `ViewCompletion::Done` / `ViewCompletion::Exit`

---

## 9. Backend Integration

### App Server Protocol (`app_server_session.rs`, `codex-app-server-protocol`)
- **gRPC/JSON-RPC** over Unix sockets or WebSocket
- **In-Process**: Embedded app server (`InProcessAppServerClient`)
- **Local Daemon**: Unix socket connection
- **Remote**: WebSocket with optional auth token

### Session Management (`app_server_session.rs`)
```rust
struct AppServerSession {
    client: AppServerClient,
    thread_params_mode: ThreadParamsMode,
    startup_config: Config,
    remote_cwd_override: Option<PathBuf>,
}
```

### Event Flow
```
Backend -> ServerNotification -> App::handle_server_notification()
                    |
                    v
            ThreadBufferedEvent -> per-thread mpsc channel
                    |
                    v
            App::poll_thread_events() -> ChatWidget::apply_notification()
```

### Key Backend Interactions

#### Thread Lifecycle
- `thread/start` - new conversation
- `thread/read` - resume existing
- `thread/list` - session picker
- `turn/start`, `turn/interrupt`, `turn/steer` - turn control

#### Approvals
- `ServerRequest::ExecApproval` -> `ApprovalOverlay`
- `ServerRequest::ApplyPatchApproval` -> diff review
- `ServerRequest::McpServerElicitation` -> form input

#### Configuration
- `config/read`, `config/write` - live config updates
- `config/batch_write` - atomic multi-key updates

#### File Operations
- `exec_command` - shell command execution
- `file_search` - ripgrep-backed search
- `apply_patch` - unified diff application

### Configuration System (`config/`, `legacy_core/config.rs`)
- Layered: System → User → Project → CLI overrides
- `ConfigLayerStack` with precedence
- Hot reload via `config/write` notifications
- Validation with detailed error reporting

### Telemetry (`codex-otel`)
- OpenTelemetry integration
- Session metrics, token usage, rate limits
- Exported via OTLP

### Authentication (`codex-login`)
- ChatGPT, Codex API Key, Workload Identity
- Token refresh handled by app server
- Login flow in onboarding screens (`onboarding/auth.rs`)

---

## Summary

The Codex TUI is a **production-grade Rust terminal application** demonstrating advanced patterns:

| Aspect | Implementation |
|--------|----------------|
| Framework | ratatui + crossterm + tokio |
| Rendering | Composable `Renderable` trait, dual draw paths, synchronized updates |
| Input | Multi-context keymap with chords, vim mode, paste burst handling |
| Layout | Responsive footer collapse, overlay alternates, flexbox columns |
| Interactivity | View stack, selection lists, approval dialogs, live previews |
| Status | Configurable segments, spinners, shimmer, rate limit bars |
| Theming | Terminal palette detection, perceptual color matching, syntax themes |
| Composition | Trait objects, event bus, history cell polymorphism, view stack |
| Backend | gRPC/JSON-RPC app server, in-process/daemon/remote, hot config reload |

The codebase shows careful attention to:
- **Cross-platform terminal quirks** (Windows, Zellij, various terminal emulators)
- **Performance** (cached wrapping, incremental render, frame rate limiting)
- **Accessibility** (keyboard-only, high contrast, configurable keybindings)
- **Extensibility** (plugin system, MCP servers, custom themes, keymap editor)
