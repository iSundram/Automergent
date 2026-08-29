# Claude Code TUI Analysis

## Overview

Claude Code is a sophisticated terminal-based UI (TUI) built on **React + Ink** (a React renderer for terminal UIs). The application uses a component-based architecture with custom hooks, context providers, and a custom keybinding system.

---

## 1. Main TUI Framework

### Entry Point: `src/main.tsx`

The main entry point (`src/main.tsx:1-5640`) orchestrates the entire application lifecycle:

- **Startup profiling** (lines 9-22): Tracks initialization checkpoints using `profileCheckpoint()`
- **Parallel prefetching** (lines 14-22): Starts MDM reads and keychain prefetches in parallel with imports
- **Command registration** (lines 1066-4557): Uses `@commander-js/extra-typings` for CLI parsing
- **Mode detection** (lines 958-974): Determines interactive vs headless (`-p/--print`) mode
- **Ink Root creation** (lines 2750-2759): Creates `@anthropic/ink` Root only for interactive sessions
- **REPL launch** (lines 3710-3727, 3764-3780, 3842-3858, 3964-3984, 4135-4155, 4371-4388, 4449-4458): Multiple entry points all call `launchRepl()`

### App Wrapper: `src/components/App.tsx` (lines 1-36)

```tsx
<FpsMetricsProvider getFpsMetrics={getFpsMetrics}>
  <StatsProvider store={stats}>
    <AppStateProvider initialState={initialState} onChangeAppState={onChangeAppState}>
      <ThemeProvider initialState={getGlobalConfig().theme} onThemeSave={...}>
        {children}
      </ThemeProvider>
    </AppStateProvider>
  </StatsProvider>
</FpsMetricsProvider>
```

**Provider hierarchy:**
1. `FpsMetricsProvider` - Tracks render performance
2. `StatsProvider` - Metrics collection store
3. `AppStateProvider` - Global application state (Zustand-like)
4. `ThemeProvider` - Ink's built-in theming system

---

## 2. Rendering System (React/Ink)

### Ink Integration

Claude Code uses **@anthropic/ink** (a fork of Ink) as the rendering engine:

- **Root creation**: `createRoot()` from `@anthropic/ink` (main.tsx:2758)
- **Render options**: `getBaseRenderOptions()` configures stdout/stdin handling (utils/renderOptions.ts)
- **Frame timing**: `onFrame` callback in render options tracks FPS and flicker (interactiveHelpers.tsx:328-365)

### Virtualized Message List

`src/components/VirtualMessageList.tsx` (43,602 lines) implements virtualized rendering:

- **Window-based rendering**: Only renders visible messages + overscan
- **Height caching**: Caches message heights to avoid remeasurement
- **Sticky scroll**: Auto-scrolls to bottom on new content (`stickyScroll` prop)
- **Selection tracking**: Integrates with `useSelection()` for text selection

### Fullscreen Layout

`src/components/FullscreenLayout.tsx` (549 lines) provides the main layout:

```tsx
<Box flexDirection="column" flexGrow={1}>
  {/* Sticky prompt header (when scrolled up) */}
  {headerPrompt && <StickyPromptHeader />}
  
  <ScrollBox ref={scrollRef} flexGrow={1} stickyScroll>
    <ScrollChromeContext>{scrollable}</ScrollChromeContext>
    {overlay}  {/* Permission dialogs, etc. */}
  </ScrollBox>
  
  {/* "N new messages" pill */}
  {!hidePill && pillVisible && <NewMessagesPill />}
  
  {/* Bottom content (prompt, spinner, footer) */}
  <Box flexShrink={0} maxHeight="50%">
    <SuggestionsOverlay />
    <DialogOverlay />
    {bottom}
  </Box>
</Box>
```

**Two modes:**
- **Fullscreen** (alt buffer, mouse tracking): `isFullscreenEnvEnabled()` (lines 347-449)
- **Main-screen** (sequential render): Lines 452-459

### Key Rendering Components

| Component | Purpose | File |
|-----------|---------|------|
| `VirtualMessageList` | Virtualized message rendering | VirtualMessageList.tsx |
| `Messages` | Message container with grouping | Messages.tsx |
| `MessageRow` | Individual message rendering | MessageRow.tsx |
| `FullscreenLayout` | Main layout with sticky/pill | FullscreenLayout.tsx |
| `ScrollKeybindingHandler` | Keyboard/mouse scroll handling | ScrollKeybindingHandler.tsx |

---

## 3. Keybindings and Input Handling

### Architecture

The keybinding system is built on `@anthropic/ink`'s keybinding infrastructure with app-specific extensions:

```
@anthropic/ink KeybindingProvider
    │
    ├── Default bindings (defaultBindings.ts)
    ├── User bindings (loadUserBindings.ts)
    ├── Validation (validate.ts)
    └── Hot-reload (chokidar watcher)
```

### Default Bindings: `src/keybindings/defaultBindings.ts`

Organized by **context** (lines 32-368):

```typescript
DEFAULT_BINDINGS: KeybindingBlock[] = [
  { context: 'Global', bindings: { 'ctrl+c': 'app:interrupt', 'ctrl+d': 'app:exit', ... }},
  { context: 'Chat', bindings: { escape: 'chat:cancel', enter: 'chat:submit', ... }},
  { context: 'Confirmation', bindings: { enter: 'confirm:yes', escape: 'confirm:no', ... }},
  { context: 'Select', bindings: { up: 'select:previous', down: 'select:next', ... }},
  { context: 'Scroll', bindings: { pageup: 'scroll:pageUp', wheeldown: 'scroll:lineDown', ... }},
  // ... 14 more contexts
]
```

**Platform-specific bindings** (lines 12-30):
- Image paste: `alt+v` on Windows, `ctrl+v` elsewhere
- Mode cycle: `shift+tab` (VT mode), `meta+m` (Windows without VT)

### User Configuration

- **File**: `~/.claude/keybindings.json`
- **Format**: `{ "bindings": [ { "context": "Chat", "bindings": { "ctrl+k": "chat:killAgents" } } ] }`
- **Hot-reload**: Chokidar watches for changes (loadUserBindings.ts:386-400)
- **Gated**: Only available for Anthropic employees (`USER_TYPE === 'ant'`) (loadUserBindings.ts:41-46)

### Keybinding Hooks

```typescript
// From @anthropic/ink (re-exported at keybindings/useKeybinding.ts:2)
useKeybinding(action, handler, options)
useKeybindings(bindingsMap, options)

// App-specific
useShortcutDisplay(action, context, fallback) // keybindings/useShortcutDisplay.ts
```

**Context-aware**: `useKeybindings` accepts `{ context: 'Chat', isActive }` to enable/disable based on focus.

### Input Handling: `src/components/TextInput.tsx`

- Wraps `BaseTextInput` with voice recording waveform cursor
- Uses `useTextInput` hook for core editing logic
- Integrates with voice mode for audio level visualization

---

## 4. Screen Layouts

### Main Screen: `src/screens/REPL.tsx` (6,684 lines)

The REPL is the primary interactive screen, composing:

```tsx
<App getFpsMetrics={getFpsMetrics} stats={stats} initialState={initialState}>
  <FullscreenLayout
    scrollable={<Messages />}
    bottom={<PromptInput />}
    overlay={permissionDialogs}
    modal={slashCommandDialogs}
    scrollRef={scrollRef}
    dividerYRef={dividerYRef}
  />
  <ScrollKeybindingHandler scrollRef={scrollRef} isActive={true} />
  <GlobalKeybindingHandlers />
  <CommandKeybindingHandlers />
  <KeybindingSetup>  // Provides keybinding context
</App>
```

### Sub-screens

| Screen | File | Purpose |
|--------|------|---------|
| `REPL` | screens/REPL.tsx | Main interactive session |
| `Doctor` | screens/Doctor.tsx | Health check UI |
| `ResumeConversation` | screens/ResumeConversation.tsx | Session resume picker |

### Layout Composition (FullscreenLayout)

```
┌─────────────────────────────────────┐
│ StickyPromptHeader (when scrolled)  │  ← ScrollChromeContext
├─────────────────────────────────────┤
│                                     │
│     ScrollBox (messages)            │  ← VirtualMessageList
│      ┌─────────────────┐            │
│      │   Overlay       │            │  ← PermissionRequest, etc.
│      └─────────────────┘            │
│                                     │
├─────────────────────────────────────┤
│ "N new messages" pill (absolute)    │  ← pillVisible via useSyncExternalStore
├─────────────────────────────────────┤
│ SuggestionsOverlay (portal)         │  ← PromptOverlayContext
├─────────────────────────────────────┤
│ DialogOverlay (portal)              │  ← PromptOverlayContext
├─────────────────────────────────────┤
│ Bottom slot (PromptInput + Spinner) │  ← maxHeight="50%"
└─────────────────────────────────────┘
Modal (bottom-anchored, absolute)     │  ← ModalContext, grows upward
```

---

## 5. Interactive Elements

### Dialogs (all use `showSetupDialog` / `showDialog` pattern)

**Pattern**: `showDialog(root, renderer)` returns `Promise<T>` (interactiveHelpers.tsx:57-62)

```typescript
export function showDialog<T>(root: Root, renderer: (done: (result: T) => void) => React.ReactNode): Promise<T>
```

### Key Dialog Components

| Dialog | File | Purpose |
|--------|------|---------|
| `TrustDialog` | components/TrustDialog/TrustDialog.tsx | Workspace trust acceptance |
| `Onboarding` | components/Onboarding.tsx | First-run setup |
| `PermissionRequest` | components/permissions/PermissionRequest.tsx | Tool permission prompts |
| `ModelPicker` | components/ModelPicker.tsx | Model selection |
| `GlobalSearchDialog` | components/GlobalSearchDialog.tsx | Project-wide search |
| `HistorySearchDialog` | components/HistorySearchDialog.tsx | Command history search |
| `QuickOpenDialog` | components/QuickOpenDialog.tsx | File quick-open |
| `MessageSelector` | components/MessageSelector.tsx | Rewind to message |
| `DiffDialog` | components/StructuredDiff.tsx | File diffs |

### Select Components

**Core**: `src/components/CustomSelect/select.tsx` (807 lines)

Features:
- Three layouts: `compact`, `expanded`, `compact-vertical`
- Input-type options (inline editing)
- Two-column label+description layout
- Keyboard navigation (arrows, j/k, ctrl+p/n)
- Highlight text search
- Image paste support

**TreeSelect**: `src/components/ui/TreeSelect.tsx` (341 lines)
- Hierarchical tree with expand/collapse
- Flattens tree for Select component
- Left/right arrows for expand/collapse

### Prompt Input: `src/components/PromptInput/PromptInput.tsx` (2,650 lines)

Central input component with:
- **Modes**: `insert`, `vim`, `shell`, `command`, `search`
- **Slash command autocomplete**: `useTypeahead` hook
- **History navigation**: Arrow keys, ctrl+r search
- **Attachments**: Image paste, file references
- **Footer**: Mode indicator, shortcuts, queued commands
- **Vim support**: `VimTextInput` component

---

## 6. Status Bars, Progress Indicators

### Spinner: `src/components/Spinner.tsx` (555 lines)

**Modes**: `thinking`, `streaming`, `compact`, `idle`

Features:
- **Animation**: `useAnimationFrame` at 50ms (SpinnerAnimationRow)
- **Shimmer effect**: Color cycling across text
- **Teammate tree**: `TeammateSpinnerTree` shows multi-agent status
- **Token budget**: Progress bar for token limits (ant-only)
- **Tips**: Contextual hints after time thresholds
- **Brief mode variant**: Minimal single-line spinner

### Status Line: `src/components/StatusLine.tsx` (587 lines)

Two independent status lines:
1. **Built-in** (`BuiltinStatusLine`): Model | Context% | 5h limit | 7d limit | Cost
2. **Custom shell command**: User-defined `/statusline` command output

**Cache Pill** (lines 53-158): Shows cache hit rate + 1-hour TTL countdown

### Progress Indicators

| Component | Purpose |
|-----------|---------|
| `SpinnerWithVerb` | Main thinking/working indicator |
| `BriefSpinner` | Minimal mode spinner |
| `TeleportProgress` | SSH/remote connection progress |
| `AutoUpdater` | Background update progress |

---

## 7. Theme/Styling System

### Theme Definition: `src/utils/theme.ts` (639 lines)

**6 built-in themes:**
- `dark` (default)
- `light`
- `dark-daltonized` (color-blind friendly)
- `light-daltonized`
- `dark-ansi` (16-color)
- `light-ansi`

**Theme structure** (lines 4-89): 88 semantic color tokens including:
- Brand colors: `claude`, `claudeShimmer`, `permission`, `permissionShimmer`
- Semantic: `success`, `error`, `warning`, `merged`
- Diff: `diffAdded`, `diffRemoved`, `diffAddedWord`, `diffRemovedWord`
- Agent colors: 8 distinct colors for sub-agents
- TUI v2: `clawd_body`, `userMessageBackground`, `selectionBg`, etc.
- Rainbow: 7 colors + shimmer variants for ultrathink highlighting

**Color formats**: RGB (`rgb(215,119,87)`) or ANSI (`ansi:redBright`)

### Theme Provider

Uses `@anthropic/ink`'s `ThemeProvider` (App.tsx:26-31, interactiveHelpers.tsx:109-116):

```tsx
<ThemeProvider
  initialState={getGlobalConfig().theme}
  onThemeSave={setting => saveGlobalConfig(current => ({ ...current, theme: setting }))}
/>
```

### Styling Patterns

1. **Semantic color props**: `<Text color="success">`, `<Box borderColor="warning">`
2. **Dim color**: `dimColor` prop for muted text
3. **Background colors**: `backgroundColor="userMessageBackground"`
4. **Shimmer variants**: `*Shimmer` tokens for animated highlights

---

## 8. Component Composition Patterns

### Context-Based State Management

**AppState** (`src/state/AppState.ts`): Zustand-like store with:
- `useAppState(selector)` - Subscribe to state slices
- `useSetAppState()` - Get setter function
- `useAppStateStore()` - Access raw store

**Specialized contexts:**
- `ModalContext` - Modal dimensions + scroll ref
- `PromptOverlayContext` - Slash command suggestions
- `ScrollChromeContext` - Sticky prompt header
- `NotificationsContext` - Toast notifications

### Hook Composition

REPL.tsx imports 60+ hooks, organized by concern:

```typescript
// State
useAppState, useSetAppState, useAppStateStore
useSettings, useMainLoopModel

// UI
useTerminalSize, useFpsMetrics, useAfterFirstRender
useDeferredHookMessages, useSearchHighlight

// Features
useReplBridge, useRemoteSession, useDirectConnect, useSSHSession
useVoiceIntegration, useScheduledTasks, useGoalContinuation
useSwarmInitialization, useTeammateViewAutoExit

// Input
useKeybinding, useKeybindings, useShortcutDisplay
useInputBuffer, useArrowKeyHistory, useHistorySearch
useTypeahead, usePromptSuggestion

// Background
useQueueProcessor, useMailboxBridge, useInboxPoller
useBackgroundTaskNavigation, useTaskListWatcher
```

### Component Reusability

**Compound components** (e.g., `Select` + `SelectOption` + `SelectInputOption`)

**Render props / children as function**: `showDialog(renderer)` pattern

**Portal pattern**: `SuggestionsOverlay`, `DialogOverlay` in FullscreenLayout render at `bottom="100%"` (absolute positioning)

---

## 9. TUI ↔ Backend Integration

### MCP (Model Context Protocol)

**Connection management**: `useManageMCPConnections` / `prefetchAllMcpResources` (main.tsx:2956-2974)

- **Two-phase**: Prefetch (warm connections) → lazy connect on first use
- **Per-server state**: `pending` → `connected` / `failed`
- **Deduplication**: Signature-based dedup between plugin + claude.ai configs

### Remote Sessions

**Direct Connect** (`feature('DIRECT_CONNECT')`):
- `createDirectConnectSession()` → WebSocket to remote server
- Local TUI, remote execution

**SSH Remote** (`feature('SSH_REMOTE')`):
- `createSSHSession()` deploys binary, tunnels auth via unix socket
- Tools execute remotely, UI renders locally

**CCR (Claude Code Remote)**:
- `--remote` / `--teleport` flags
- `RemoteSessionManager` handles session lifecycle
- `useRemoteSession` hook manages connection

### Agent/Task System

**In-process teammates** (KAIROS/PROACTIVE):
- `InProcessTeammateTask` - Spawns child React components
- `leaderPermissionBridge` - Coordinates tool permissions

**Background agents** (tmux):
- `LocalAgentTask` / `RemoteAgentTask`
- `BackgroundAgentSelector` for UI

### Hooks System

**Session hooks** (`processSessionStartHooks`, `processSetupHooks`):
- Run in parallel with MCP connections (main.tsx:2976-2999)
- Injected into message stream before first API call

**Status line hooks**: Custom shell commands for status bar (StatusLine.tsx:376-426)

### API Integration

**Query engine** (`src/QueryEngine.ts`): Handles API requests with:
- Token budget management
- Compact/summarization
- Tool use orchestration
- Streaming response handling

---

## Key Files Summary

| Category | File | Lines | Description |
|----------|------|-------|-------------|
| **Entry** | `main.tsx` | 5,640 | Main entry, CLI, REPL launch |
| **Screen** | `REPL.tsx` | 6,684 | Primary interactive screen |
| **Layout** | `FullscreenLayout.tsx` | 549 | Main layout with sticky/pill |
| **Messages** | `VirtualMessageList.tsx` | 43,602 | Virtualized message rendering |
| **Messages** | `Messages.tsx` | 1,113 | Message container & grouping |
| **Input** | `PromptInput.tsx` | 2,650 | Main input component |
| **Input** | `TextInput.tsx` | 129 | Base text input with voice |
| **Select** | `select.tsx` | 807 | Core select component |
| **Select** | `TreeSelect.tsx` | 341 | Hierarchical select |
| **Keybindings** | `defaultBindings.ts` | 368 | Default key mappings |
| **Keybindings** | `loadUserBindings.ts` | 456 | User config + hot reload |
| **Keybindings** | `ScrollKeybindingHandler.tsx` | 1,046 | Scroll + selection handling |
| **Theme** | `theme.ts` | 639 | Theme definitions |
| **Status** | `StatusLine.tsx` | 587 | Status bar + cache pill |
| **Status** | `Spinner.tsx` | 555 | Progress indicators |
| **State** | `AppState.tsx` | - | Global state store |
| **Helpers** | `interactiveHelpers.tsx` | 368 | Dialog/render utilities |

---

## Architecture Summary

```
┌─────────────────────────────────────────────────────────────────┐
│                        main.tsx                                  │
│  ┌─────────────┐ ┌──────────────┐ ┌─────────────────────────┐  │
│  │ CLI Parsing │ │ Startup Seq  │ │ Mode Dispatch           │  │
│  └─────────────┘ └──────────────┘ └─────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                      launchRepl()                               │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ createRoot() → Ink Root                                 │   │
│  └─────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                        App.tsx                                  │
│  FpsMetricsProvider → StatsProvider → AppStateProvider          │
│                              → ThemeProvider                    │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    FullscreenLayout                             │
│  ┌──────────────┐ ┌────────────┐ ┌──────────┐ ┌────────────┐   │
│  │Sticky Header │ │ ScrollBox  │ │   Pill   │ │  Bottom    │   │
│  │              │ │(Messages)  │ │ (N new)  │ │ (PromptIn) │   │
│  └──────────────┘ └────────────┘ └──────────┘ └────────────┘   │
│         │                │                │            │         │
│         ▼                ▼                ▼            ▼         │
│  ScrollChromeCtx  VirtualMsgList   useSyncExtStore  PromptInput │
└─────────────────────────────────────────────────────────────────┘
                              │
              ┌───────────────┼───────────────┐
              ▼               ▼               ▼
      ┌─────────────┐ ┌─────────────┐ ┌─────────────┐
      │ Keybindings │ │  MCP/Remote │ │   Hooks     │
      │  Contexts   │ │  Services   │ │  System     │
      └─────────────┘ └─────────────┘ └─────────────┘
```

The TUI is a sophisticated React/Ink application with:
- **Virtualized rendering** for large message histories
- **Context-aware keybindings** with hot-reload support
- **Multi-mode layouts** (fullscreen/main-screen)
- **Portal-based overlays** for dialogs/suggestions
- **Deep backend integration** (MCP, remote sessions, agents)
- **Comprehensive theming** with accessibility variants
