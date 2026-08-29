# OpenCode TUI Architecture Analysis

## Overview

OpenCode's Terminal User Interface (TUI) is built on a custom framework called **OpenTUI** (`@opentui/core`, `@opentui/keymap`, `@opentui/solid`) which provides a reactive, component-based architecture using **SolidJS** as the rendering engine. The TUI is contained primarily in `packages/tui/` with backend integration via `packages/opencode/`.

---

## 1. Main TUI Framework

### Core Technologies
- **Rendering Engine**: `@opentui/solid` - SolidJS-based reactive rendering on top of `@opentui/core`
- **Keybinding System**: `@opentui/keymap` - Declarative keymap with leader key, mode stacks, and command dispatch
- **Terminal Backend**: `@opentui/core` - Low-level terminal rendering with `CliRenderer`
- **State Management**: SolidJS signals, stores, and context system
- **Effects**: Effect-TS for backend service orchestration

### Entry Point: `packages/tui/src/app.tsx`
The main application entry point (`run` function, line 186) sets up:
1. **Renderer creation** (lines 191-213): `createCliRenderer` with 60fps target, kitty keyboard protocol, mouse support
2. **Keymap registration** (lines 215-219): `createDefaultOpenTuiKeymap` + `registerOpencodeKeymap`
3. **Provider hierarchy** (lines 245-349): Deeply nested context providers:
   - `ExitProvider`, `EpilogueProvider`, `ErrorBoundary`
   - `TuiPathsProvider`, `TuiTerminalEnvironmentProvider`, `TuiStartupProvider`
   - `ClipboardProvider`, `OpencodeKeymapProvider`, `ArgsProvider`
   - `KVProvider`, `ToastProvider`, `RouteProvider`, `TuiConfigProvider`
   - `PluginRuntimeProvider`, `SDKProvider`, `PermissionProvider`
   - `ProjectProvider`, `SyncProvider`, `DataProvider`, `ThemeProvider`
   - `LocalProvider`, `PromptStashProvider`, `DialogProvider`
   - `FrecencyProvider`, `PromptHistoryProvider`, `PromptRefProvider`
   - `EditorContextProvider`, `LocationProvider`
4. **Route rendering** (lines 1110-1128): Switch between `Home` and `Session` routes with plugin slots

### Key Dependencies (package.json)
```json
"@opentui/core": "catalog:",
"@opentui/keymap": "catalog:",
"@opentui/solid": "catalog:",
"solid-js": "catalog:",
"effect": "catalog:",
"opentui-spinner": "catalog:",
"remeda": "catalog:",
"fuzzysort": "catalog:"
```

---

## 2. Rendering System

### OpenTUI Core (`@opentui/core`)
The rendering system uses a **box-based flexbox layout model** with these primitives:
- `box` - Flex container with flexDirection, gap, padding, margins
- `text` - Styled text with fg/bg colors, attributes (bold, italic, underline)
- `textarea` - Editable text input with syntax highlighting, extmarks, virtual text
- `scrollbox` - Scrollable container with virtual scrolling, sticky positioning
- `input` - Single-line input field

### SolidJS Integration (`@opentui/solid`)
- Reactive rendering via SolidJS signals/memos
- Custom JSX elements map to OpenTUI renderables
- `useRenderer()`, `useTerminalDimensions()` hooks for renderer access
- `render()` function mounts the component tree to the terminal

### Layout System (app.tsx:1088-1133)
```tsx
<box flexDirection="column" backgroundColor={theme.background}>
  <Show when={ready()}>
    <box flexGrow={1} flexDirection="column">
      <Switch>
        <Match when={route.data.type === "home"}><Home /></Match>
        <Match when={route.data.type === "session"}><Session /></Match>
      </Switch>
      {plugin()}
    </box>
    <box flexShrink={0}><pluginRuntime.Slot name="app_bottom" /></box>
    <pluginRuntime.Slot name="app" />
  </Show>
  <StartupLoading />
</box>
```

### Session Layout (session/index.tsx:1178-1361)
- **Main content area**: Flex-grow scrollbox with messages
- **Sidebar**: Conditional 42-char wide panel (auto-hide on narrow terminals)
- **Prompt area**: Fixed bottom section with textarea
- **Permissions/Questions**: Overlay prompts when needed

---

## 3. Keybindings and Input Handling

### Keymap Architecture (`packages/tui/src/keymap.tsx`)
- **Leader key system**: Configurable leader (default `ctrl+x`) with timed sequences
- **Mode stacks**: Push/pop modes (base, modal, etc.) via `createOpencodeModeStack`
- **Key aliases**: `enter`→`return`, `esc`→`escape`, `pgdown`→`pagedown`, etc.
- **Managed textarea layer**: Automatic binding activation when textarea focused
- **Command dispatch**: `keymap.dispatchCommand(name)` for programmatic triggering

### Keybinding Configuration (`packages/tui/src/config/keybind.ts`)
Comprehensive keybinding definitions (240+ bindings) organized by category:
- **App-level**: `app_exit` (ctrl+c, ctrl+d, leader+q), `app_debug`, `app_console`
- **Session**: `session_list` (leader+l), `session_timeline` (leader+g), `session_compact` (leader+c)
- **Navigation**: `messages_page_up/down`, `messages_line_up/down`, `session_first/last`
- **Prompt**: `prompt_submit` (return), `prompt_editor` (leader+e), `prompt_stash` commands
- **Input editing**: Full emacs-style bindings (ctrl+a/e, alt+f/b, ctrl+k/u, etc.)
- **Dialog**: `dialog.select.prev/next/submit`, `dialog.prompt.submit`
- **Which-key**: Toggle, layout switch, scroll, group navigation

### Binding Resolution (keymap.tsx:214-244)
```typescript
registerOpencodeKeymap(keymap, renderer, config) {
  // 1. Mode stack for modal dialogs
  // 2. Comma-separated bindings (e.g., "a,b,c")
  // 3. Key alias expansion
  // 4. Base layout fallback (vim-like hjkl)
  // 5. Timed leader key (config.leader_timeout)
  // 6. Escape clears pending sequence
  // 7. Backspace pops pending sequence
  // 8. Managed textarea layer for input fields
}
```

### Usage in Components (session/index.tsx:1098-1121)
```tsx
useBindings(() => ({ commands: sessionCommands() }))
useBindings(() => ({ bindings: tuiConfig.keybinds.gather("session.global", sessionGlobalBindingCommands) }))
useBindings(() => ({ 
  enabled: () => renderer.currentFocusedEditor === null,
  bindings: tuiConfig.keybinds.gather("session.global.unfocused", sessionGlobalUnfocusedBindingCommands) 
}))
useBindings(() => ({ mode: OPENCODE_BASE_MODE, bindings: tuiConfig.keybinds.gather("session", sessionBindingCommands) }))
```

---

## 4. Screen Layouts

### Home Screen (`packages/tui/src/routes/home.tsx`)
- Centered logo with plugin slot override
- Prompt input with placeholder rotation
- Footer slot for plugins
- Full-screen flex layout with vertical centering

### Session Screen (`packages/tui/src/routes/session/index.tsx:1178-1361`)
```
┌─────────────────────────────────────────────────────────────┐
│ Messages Scrollbox (flex-grow)                              │
│ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐             │
│ │ User Msg    │ │ Assistant   │ │ Tool Parts  │             │
│ │ (left bar)  │ │ Msg         │ │ (diff, etc) │             │
│ └─────────────┘ └─────────────┘ └─────────────┘             │
├─────────────────────────────────────────────────────────────┤
│ Permissions / Questions (conditional)                       │
├─────────────────────────────────────────────────────────────┤
│ Prompt Input (textarea with extmarks, autocomplete)         │
│ [Agent/Model info] [Spinner/Status] [Shortcuts]             │
└─────────────────────────────────────────────────────────────┘
│ Sidebar (42 cols, auto-hide <120 cols, overlay on narrow)  │
│ ┌─────────────┐                                             │
│ │ Session     │                                             │
│ │ Title       │                                             │
│ │ Workspace   │                                             │
│ ├─────────────┤                                             │
│ │ Plugin slots│                                             │
│ │ (todo, lsp, │                                             │
│ │  mcp, files)│                                             │
│ ├─────────────┤                                             │
│ │ Version info│                                             │
│ └─────────────┘                                             │
```

### Responsive Behavior
- Sidebar auto-shows when terminal width > 120 cols (`wide()` memo)
- Overlay mode on narrow terminals with dimmed backdrop
- Prompt max-width scales with terminal (70% up to configured max)

---

## 5. Interactive Elements

### Dialog System (`packages/tui/src/ui/dialog.tsx`)
**Stack-based modal dialogs** with:
- `DialogProvider` - Root provider managing dialog stack
- `Dialog` - Base component with backdrop, sizing (medium/large/xlarge)
- `useDialog()` hook for imperative control: `replace()`, `clear()`, `setSize()`
- **Keyboard handling**: Escape/Ctrl+C closes top dialog, respects text selection
- **Focus management**: Restores focus to previous element on close
- **Mode stack integration**: Pushes "modal" mode while dialogs open

### Dialog Variants
| Component | Purpose |
|-----------|---------|
| `DialogSelect` | Fuzzy-searchable list with categories, actions, footer hints |
| `DialogConfirm` | Yes/no confirmation |
| `DialogAlert` | Information alert |
| `DialogPrompt` | Text input dialog |
| `DialogHelp` | Keybinding reference |
| `DialogModel` | Model selection with provider grouping |
| `DialogAgent` | Agent switching |
| `DialogMcp` | MCP server toggles |
| `DialogThemeList` | Theme picker with preview |
| `DialogSessionList` | Session switcher with preview |
| `DialogWorkspaceList` | Workspace management |
| `DialogProviderList` | Provider connection flow |
| `DialogTimeline` | Message timeline navigator |
| `DialogForkFromTimeline` | Fork session from message |
| `DialogSessionRename` | Rename current session |
| `DialogExportOptions` | Export format options |
| `DialogRetryAction` | Retry/upgrade prompts |
| `DialogStatus` | System status display |
| `DialogDebug` | Debug information |
| `DialogVariant` | Model variant selector |
| `DialogConsoleOrg` | Organization switcher |

### Prompt Input (`packages/tui/src/component/prompt/index.tsx`)
**Advanced textarea** with:
- **Extmarks**: Virtual text placeholders for files, agents, pasted content
- **Autocomplete**: Slash commands, @-mentions, file paths
- **Modes**: Normal + Shell mode (toggle with `!`)
- **History**: Up/down navigation with cursor-position awareness
- **Stash**: Push/pop/list prompt drafts
- **Editor integration**: External editor via `prompt.editor` (leader+e)
- **Paste handling**: Image/file detection, multi-line summary, URL handling
- **Editor context**: Automatic file/selection inclusion with dismiss option

### DialogSelect (`packages/tui/src/ui/dialog-select.tsx:80-791`)
- **Fuzzy filtering**: fuzzysort with title/category weighting
- **Categories**: Group headers with optional custom views
- **Keyboard**: Up/down, page up/down, home/end, enter to select
- **Actions**: Footer action bar with keybinding hints (Tab to cycle)
- **Mouse**: Click to select, hover to highlight
- **Dynamic sizing**: Max 50% terminal height
- **Preserve selection**: Maintains selection across filter changes

### Command Palette (`packages/tui/src/component/command-palette.tsx`)
- Searchable command list with categories
- Slash command aliases (`/models`, `/agents`, `/themes`, etc.)
- Keybinding hints for each command
- Plugin-extensible command registration

---

## 6. Status Bars, Progress Indicators

### Toast System (`packages/tui/src/ui/toast.tsx`)
- **Position**: Top-right, 2px margin
- **Variants**: info, success, warning, error (colored left/right borders)
- **Auto-dismiss**: Configurable duration (default 5s)
- **Stacking**: Single toast at a time, replaces previous
- **API**: `toast.show()`, `toast.error()`, `toast.currentToast`

### Spinner System (`packages/tui/src/component/spinner.tsx`)
- **Registration**: `registerOpencodeSpinner()` registers "opencode" spinner
- **Styles**: "blocks" style with configurable color, frames, alpha
- **Usage**: Session status, prompt submission, background tasks
- **Colors**: Agent-specific colors via `local.agent.color()`

### Session Status Bar (Prompt footer, prompt/index.tsx:1516-1580)
- **Spinner**: Animated when session busy
- **Status text**: "Working...", "Thinking...", "Waiting for permission...", etc.
- **Token usage**: Context percentage, cost display
- **Retry actions**: Inline buttons for rate limit/quota errors
- **Shortcuts**: Context-aware keybinding hints

### Attention System (`packages/tui/src/attention.ts`)
- **Desktop notifications**: `renderer.triggerNotification()`
- **Sounds**: 6 built-in sounds (default, question, permission, error, done, subagent_done)
- **Sound packs**: Customizable, persistable, with fallback chain
- **Focus-aware**: Configurable `when` (always/focused/blurred)
- **Volume control**: Global + per-notification override

### Which-Key Panel (`packages/tui/src/feature-plugins/system/which-key.tsx`)
- **Two layouts**: Dock (bottom panel) / Overlay (floating)
- **Grouped bindings**: By category with tab navigation
- **Pending key preview**: Shows completions for partial sequences
- **Scrollable**: Page up/down, home/end, group prev/next
- **Persistent state**: Layout, pinned, pending preview saved to KV

---

## 7. Theme/Styling System

### Theme Architecture (`packages/tui/src/theme/index.ts`)
**ThemeJson structure** with:
- **Defs**: Color aliases for reuse
- **Theme colors**: 60+ semantic color roles (primary, background, diff, markdown, syntax)
- **Mode support**: Dark/light variants per color
- **Optional fields**: `selectedListItemText`, `backgroundMenu`, `thinkingOpacity`

### Built-in Themes (30 themes in `packages/tui/src/theme/assets/`)
- opencode (default), catppuccin (3 variants), tokyonight, nord, dracula, gruvbox, rosepine, kanagawa, everforest, onedark, ayu, nightowl, material, github, solarized, zenburn, monokai, cursor, vercel, vesper, carbonfox, aura, palenight, mercury, matrix, lucent-orng, orng, osaka-jade, cobalt2, synthwave84, flexoki

### Theme Resolution (`theme/index.ts:241-299`)
```typescript
resolveTheme(theme: ThemeJson, mode: "dark" | "light"): Theme
```
- Resolves color references (defs → theme → variant)
- Circular reference detection
- ANSI color fallback (0-255)
- Generates `selectedListItemText` fallback from background
- System theme generation from terminal palette

### Syntax Highlighting (`theme/index.ts:556-1089`)
- **Full TextMate-style scopes**: 120+ rules covering comments, strings, keywords, types, functions, variables, operators, punctuation, markdown, diff
- **Subtle syntax**: Dimmed version for thinking blocks (uses `thinkingOpacity`)
- **Markdown-specific**: Headings, links, code, blockquotes, lists, images
- **Diff highlighting**: Added/removed/context lines with backgrounds

### Theme Context (`packages/tui/src/context/theme.tsx`)
- **Auto-discovery**: Scans `~/.opencode/themes/`, `.opencode/themes/`, project `.opencode/themes/`
- **System theme**: Detects terminal background, generates matching theme
- **Mode detection**: OSC 11/997 sequences, manual toggle, lock
- **Custom themes**: Hot-reload on SIGUSR2, file watching
- **Syntax style memoization**: Retains styles during idle for smooth transitions

### Color Utilities
- `tint(base, overlay, alpha)` - Alpha blending
- `selectedForeground(theme, bg)` - Contrast calculation
- `terminalMode(colors)` - Light/dark detection from terminal
- `generateSystem(colors, mode)` - Dynamic theme from terminal palette

---

## 8. Component Composition Patterns

### Context Providers (Helper: `createSimpleContext`)
```typescript
export const { use: useTheme, provider: ThemeProvider } = createSimpleContext({
  name: "Theme",
  init: (props) => { /* returns theme API object */ }
})
```
Used for: Theme, SDK, Route, Clipboard, Exit, Epilogue, Permission, Prompt, Local, KV, Project, Sync, Data, Editor, Location, Runtime, Args, TuiConfig, PluginRuntime

### Plugin System (`packages/tui/src/plugin/`)
- **Slots**: Named insertion points (`home_logo`, `home_prompt`, `session_prompt`, `sidebar_content`, `app_bottom`, `app`)
- **Modes**: `replace`, `single_winner`, `append`
- **Runtime**: `PluginRuntime` manages command registration, slot rendering, route registration
- **API**: `TuiPluginApi` provides keymap, theme, dialog, toast, KV, SDK, slots, config, event bus

### Feature Plugins (`packages/tui/src/feature-plugins/`)
| Plugin | Slots | Purpose |
|--------|-------|---------|
| `sidebar/todo` | `sidebar_content` | Todo list from session |
| `sidebar/lsp` | `sidebar_content` | LSP diagnostics |
| `sidebar/mcp` | `sidebar_content` | MCP server status |
| `sidebar/files` | `sidebar_content` | File tree |
| `sidebar/footer` | `sidebar_footer` | Version info |
| `system/which-key` | `home_bottom`, `app`, `app_bottom` | Keybinding help |
| `system/notifications` | - | Desktop notifications |
| `system/diff-viewer` | - | Git diff UI |
| `home/tips` | `home_bottom` | Startup tips |

### Component Patterns
1. **Render props**: `<Prompt ref={bind} right={<Slot />} />`
2. **Slot injection**: `<pluginRuntime.Slot name="session_prompt" mode="replace" ref={bind}><Prompt /></pluginRuntime.Slot>`
3. **Context consumption**: `const { theme } = useTheme()`, `const sync = useSync()`
4. **Memoized derivatives**: `createMemo(() => computedValue)` for reactive derived state
5. **Event bus**: `useEvent().on("event.name", handler)` for cross-component communication

---

## 9. Backend Integration

### SDK Client (`packages/tui/src/context/sdk.tsx`)
- **HTTP client**: `createOpencodeClient` from `@opencode-ai/sdk/v2`
- **SSE event stream**: Long-polling with exponential backoff (1s → 30s)
- **Event batching**: 16ms batch window for render coalescing
- **Workspace sync**: Automatic `sdk.sync.start()` on connect

### Session Management (`packages/opencode/src/session/session.ts`)
- **CRUD**: create, get, list, fork, remove, touch
- **Messages**: Paginated loading (50/page), full history retrieval
- **Parts**: Granular part updates (delta, replace, remove)
- **Revert/Redo**: Snapshot-based message reverting
- **Compaction**: Summarization via LLM
- **Sharing**: Session sharing with URL generation
- **Workspace isolation**: Per-workspace session lists

### ACP Protocol (`packages/opencode/src/acp/service.ts`)
**Agent Client Protocol** implementation for editor integrations:
- Session lifecycle: initialize, newSession, loadSession, resumeSession, closeSession, forkSession
- Configuration: setSessionConfigOption (model, effort, mode), setSessionModel, setSessionMode
- Prompt handling: Regular prompts, slash commands, compact command
- MCP server registration per session
- Usage tracking: Context tokens, cost reporting
- OAuth/API key auth flows

### Sync System (`packages/tui/src/context/sync.tsx`)
- **Bootstrap**: Loads providers, agents, commands, skills, config
- **Session sync**: Real-time message/part updates via SSE
- **Status tracking**: Session status (idle, running, retry, compacting)
- **Permissions/Questions**: Queued approval prompts
- **Capability detection**: Experimental features, background subagents

### Event System (`packages/tui/src/context/event.ts`)
- **Typed events**: `GlobalEvent` from SDK with workspace filtering
- **Local events**: `tui.command.execute`, `tui.toast.show`, `tui.session.select`
- **Cross-workspace isolation**: Events tagged with workspace ID

### Data Flow
```
User Input → Prompt Component → SDK Client → HTTP/SSE → Backend (Effect-TS Services)
                                                              ↓
                                                    Database (SQLite via Drizzle)
                                                              ↓
                                                    Event Bridge → SSE → SDK → TUI Contexts
                                                              ↓
                                                    Reactive UI Updates (SolidJS)
```

---

## Summary

OpenCode's TUI is a sophisticated, production-grade terminal application featuring:

1. **Modern reactive architecture** - SolidJS + custom OpenTUI primitives
2. **Comprehensive keybinding system** - Leader keys, modes, contextual bindings
3. **Rich component library** - Dialogs, selectors, prompts, autocomplete
4. **Advanced theming** - 30+ themes, system detection, syntax highlighting
5. **Plugin extensibility** - Slot-based composition, custom routes/commands
6. **Robust backend integration** - HTTP/SSE, ACP protocol, real-time sync
7. **Accessibility features** - Focus management, screen reader considerations
8. **Cross-platform support** - Windows (ConPTY), macOS, Linux with terminal detection

The codebase demonstrates excellent separation of concerns, reactive patterns, and thoughtful UX design for a developer-focused terminal application.
