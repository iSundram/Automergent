# Claude Code Agent Architecture Analysis

## Overview

This document provides a comprehensive analysis of the Claude Code agent architecture, covering the main agent loop, subagent spawning and management, background shell execution, process management, agent-to-agent communication, task delegation, shell session persistence, background job handling, shell enabled detection, and input/output routing between agents.

---

## 1. Main Agent Loop (src/main.tsx, src/assistant/)

### Entry Point: `src/main.tsx`

The main entry point orchestrates the entire application lifecycle:

**Key Responsibilities:**
- **Pre-initialization**: Early profiling, MDM raw reads, keychain prefetching (lines 9-22)
- **Feature Flags**: Uses `bun:bundle` feature flags for dead code elimination (COORDINATOR_MODE, KAIROS, etc.) (lines 118-128)
- **CLI Parsing**: Commander.js-based argument parsing with 100+ options (lines 1066-1434)
- **Session Management**: Handles resume (`-c`, `-r`), fork (`--fork-session`), and teleport modes
- **Settings Loading**: Eager loading of `--settings` and `--setting-sources` flags (lines 654-668)
- **Trust Dialog**: Non-interactive mode (`-p/--print`) skips trust dialog (line 965)

**Main Loop Flow:**
1. **Bootstrap** (`initializeEntrypoint`, line 977): Sets `CLAUDE_CODE_ENTRYPOINT` env var
2. **Client Type Detection** (lines 980-997): Determines client type (cli, sdk, github-action, etc.)
3. **Settings & Migrations** (lines 1022, 494): Runs migrations, loads settings
4. **Run Function** (line 1026): Starts the CLI command handler
5. **Pre-Action Hook** (lines 1087-1148): Runs `init()`, sets up sinks, loads remote settings
6. **Command Execution**: Routes to appropriate handler (main REPL, print, mcp, etc.)

### Assistant Mode: `src/assistant/`

The assistant mode (KAIROS feature) provides a daemon-based agent experience:

- **Gate Check** (`src/assistant/gate.ts`): GrowthBook gate `tengu_kairos` controls availability
- **Session Chooser** (`src/assistant/AssistantSessionChooser.tsx`): UI for selecting/resuming sessions
- **Team Initialization** (`src/assistant/index.ts`): Creates in-process team context for subagent spawning
- **Force Flag** (`markAssistantForced`): Bypasses gate for daemon-spawned teammates

---

## 2. Subagent Spawning and Management (src/tasks/, src/agents/)

### Task Types (`src/Task.ts`)

```typescript
export type TaskType =
  | 'local_bash'           // Background shell commands
  | 'local_agent'          // Background agents (Agent tool)
  | 'remote_agent'         // CCR remote agents
  | 'in_process_teammate'  // Swarm teammates
  | 'local_workflow'       // Workflow tasks
  | 'monitor_mcp'          // MCP monitoring
  | 'dream'                // Dream tasks
```

### Local Agent Task (`src/tasks/LocalAgentTask/LocalAgentTask.tsx`)

**State Structure** (`LocalAgentTaskState`, lines 146-178):
```typescript
{
  type: 'local_agent',
  agentId: string,
  prompt: string,
  selectedAgent: AgentDefinition,
  agentType: string,
  model: string,
  abortController: AbortController,
  progress: AgentProgress,
  isBackgrounded: boolean,
  pendingMessages: string[],
  retain: boolean,
  diskLoaded: boolean,
  evictAfter?: number,
}
```

**Spawn Modes:**
1. **Async (Background)**: `registerAsyncAgent()` (lines 523-576) - immediate background
2. **Sync (Foreground)**: `registerAgentForeground()` (lines 587-677) - runs in foreground, can be backgrounded
3. **Fork Subagent**: `FORK_AGENT` path (lines 418-431) - cache-identical system prompt

**Key Functions:**
- `registerAsyncAgent()`: Creates background task with abort controller (line 523)
- `registerAgentForeground()`: Registers foreground task with auto-background timer (line 587)
- `backgroundAgentTask()`: Transitions foreground to background (line 683)
- `runAsyncAgentLifecycle()`: Drives agent from spawn to completion (line 520, `agentToolUtils.ts`)

### Remote Agent Task (`src/tasks/RemoteAgentTask/RemoteAgentTask.tsx`)

- Spawns agents in CCR (Claude Code Remote) environments
- Uses `teleportToRemote()` for session creation
- Registers via `registerRemoteAgentTask()` with `remoteTaskType: 'remote-agent'`

### In-Process Teammate Task (`src/tasks/InProcessTeammateTask/InProcessTeammateTask.tsx`)

- Runs teammates in same process using AsyncLocalStorage isolation
- Supports plan mode approval flow
- Message injection via `injectUserMessageToTeammate()` (line 70)

### Task Framework (`src/utils/task/framework.ts`)

**Core Functions:**
- `registerTask()`: Registers task, emits `task_started` SDK event (line 77)
- `updateTaskState()`: Type-safe state updates (line 48)
- `generateTaskAttachments()`: Polls for output deltas, handles eviction (line 158)
- `applyTaskOffsetsAndEvictions()`: Applies patches against fresh state (line 213)
- `pollTasks()`: Main polling loop called by framework (line 255)

---

## 3. Background Shell Execution (src/shell/, src/daemon/, src/services/)

### Shell Command Execution (`src/utils/Shell.ts`, `src/utils/ShellCommand.ts`)

**ShellCommand Interface** (`src/utils/ShellCommand.ts`, lines 32-47):
```typescript
{
  background: (taskId: string) => boolean,
  result: Promise<ExecResult>,
  kill: () => void,
  status: 'running' | 'backgrounded' | 'completed' | 'killed',
  cleanup: () => void,
  taskOutput: TaskOutput
}
```

**Execution Modes:**
1. **File Mode** (bash commands): stdout/stderr → file fd (no JS involvement)
2. **Pipe Mode** (hooks): stdout/stderr → StreamWrapper → TaskOutput in-memory

**Key Implementation** (`ShellCommandImpl`, lines 114-382):
- Uses `tree-kill` for process tree termination
- Size watchdog (5s interval, 5GB cap) prevents disk exhaustion
- Auto-background support via `shouldAutoBackground` flag
- CWD tracking via temp file with `pwd -P`

### Daemon Supervisor (`src/daemon/main.ts`)

**Worker Management:**
- Spawns worker processes with exponential backoff (2s → 120s cap)
- Rapid failure detection: parks worker after 5 crashes within 10s
- Permanent error code 78 (EX_CONFIG) prevents retry

**Worker Types:**
- `remoteControl`: Runs headless bridge loop for remote sessions

### Daemon Worker (`src/daemon/workerRegistry.ts`)

- Entry point: `claude --daemon-worker=<kind>`
- Environment variables configure behavior:
  - `DAEMON_WORKER_DIR`, `DAEMON_WORKER_SPAWN_MODE`, `DAEMON_WORKER_CAPACITY`
  - `DAEMON_WORKER_PERMISSION`, `DAEMON_WORKER_SANDBOX`
- Runs `runBridgeHeadless()` for remote control sessions

### Background Sessions (`src/cli/bg.ts`, `src/cli/bg/engines/`)

**Engines:**
1. **TmuxEngine** (`tmux.ts`): Creates tmux session, supports interactive input
2. **DetachedEngine** (`detached.ts`): Uses `detached: true` spawn, log file only

**Session Persistence:**
- PID files in `~/.claude/sessions/<pid>.json`
- Logs in `~/.claude/sessions/logs/<name>.log`
- `claude daemon attach` reconnects via tmux attach or log tail

---

## 4. Process Management and PTY Handling

### Process Spawning (`src/utils/Shell.ts`)

```typescript
const childProcess = spawn(spawnBinary, shellArgs, {
  env: { ...subprocessEnv(), SHELL: binShell, GIT_EDITOR: 'true', CLAUDECODE: '1' },
  cwd,
  stdio: usePipeMode ? ['pipe', 'pipe', 'pipe'] : ['pipe', outputHandle?.fd, outputHandle?.fd],
  detached: provider.detached,
  windowsHide: true,
})
```

**PTY Considerations:**
- No raw PTY allocation - uses shell with file descriptor redirection
- File mode: Both stdout/stderr → same file fd with `O_APPEND` for atomic interleaving
- Windows: Uses `'w'` mode instead of `O_APPEND` for FILE_GENERIC_WRITE
- Sandbox: Uses `bwrap` via `SandboxManager.wrapWithSandbox()`

### Process Termination

- **Graceful**: `SIGTERM` → 30s grace → `SIGKILL` (daemon supervisor, lines 313-318)
- **Immediate**: `tree-kill` with `SIGKILL` for shell commands (ShellCommandImpl.#doKill, line 340)
- **AbortController**: Linked to parent for cascading aborts

---

## 5. Agent-to-Agent Communication

### SendMessage Tool (`packages/builtin-tools/src/tools/SendMessageTool/SendMessageTool.ts`)

**Routing Targets:**
1. **Teammate Name**: In-process teammate via mailbox (`writeToMailbox`)
2. **`*` (Broadcast)**: All teammates in team
3. **`bridge:<session-id>`**: Cross-machine via bridge (CCR)
4. **`uds:<socket-path>`**: Local peer via Unix domain socket
5. **`tcp:<host>:<port>`**: LAN peer via TCP (feature-gated)
6. **Agent ID/Name**: Local subagent (AgentTool) - queues message for next tool round

**Message Types:**
- Plain text with summary
- Structured: `shutdown_request`, `shutdown_response`, `plan_approval_response`

**Delivery Mechanisms:**
- **In-process**: `queuePendingMessage()` → drained at tool boundaries
- **Mailbox**: File-based (`teammateMailbox.ts`) for teammate coordination
- **Bridge**: `postInterClaudeMessage()` via WebSocket
- **UDS/TCP**: Direct socket communication

### Agent Context (`src/utils/agentContext.ts`)

Uses `AsyncLocalStorage` for context isolation:

```typescript
SubagentContext = {
  agentId, parentSessionId, agentType: 'subagent',
  subagentName, isBuiltIn, invokingRequestId, invocationKind, invocationEmitted
}

TeammateAgentContext = {
  agentId, agentName, teamName, agentColor,
  planModeRequired, parentSessionId, isTeamLead,
  agentType: 'teammate', invokingRequestId, invocationKind, invocationEmitted
}
```

**Why AsyncLocalStorage**: Multiple background agents run concurrently in same process; AppState would be overwritten causing attribution conflicts (lines 16-21).

---

## 6. Task Delegation and Coordination

### Coordinator Mode (`src/coordinator/coordinatorMode.ts`)

**Enabled via**: `CLAUDE_CODE_COORDINATOR_MODE=1` env var

**Coordinator Capabilities:**
- **Tools**: `Agent`, `SendMessage`, `TaskStop`, `subscribe_pr_activity`
- **Worker Tools**: Bash, Read, Edit, MCP tools (no Agent, SendMessage, TaskStop)
- **Workflow**: Research → Synthesis → Implementation → Verification

**System Prompt** (lines 111-369): Detailed instructions for parallel work delegation

### Task Delegation Flow

1. **Coordinator calls AgentTool** with `subagent_type: 'worker'`
2. **AgentTool.call()** (AgentTool.tsx:322):
   - Resolves agent definition
   - Checks MCP requirements
   - Determines async vs sync execution
   - For async: `registerAsyncAgent()` + `runAsyncAgentLifecycle()`
3. **Background agent runs** via `runAgent()` (runAgent.ts:257)
4. **Completion**: `completeAsyncAgent()` → `enqueueAgentNotification()` → `<task-notification>` XML
5. **Coordinator receives** notification as user message with task-notification XML

### Task Stop Tool

- `TaskStopTool` stops running agents by `task_id`
- Calls `killAsyncAgent()` which aborts controller and marks task killed
- Stopped agents can be resumed via `SendMessage`

### Worktree Isolation

- `isolation: 'worktree'` creates git worktree for agent (`createAgentWorktree()`)
- Automatic cleanup on completion if no changes (`hasWorktreeChanges()` → `removeAgentWorktree()`)
- Hook-based worktrees preserved (can't detect VCS changes)

---

## 7. Shell Session Persistence

### Task Output (`src/utils/task/diskOutput.ts`, `src/utils/task/TaskOutput.ts`)

**DiskTaskOutput** (lines 97-231):
- Async write queue with single drain loop
- 5GB cap with truncation notice
- `O_NOFOLLOW` for symlink attack prevention
- Per-task output file: `~/.claude/projects/<project>/<session>/tasks/<taskId>.output`

**Symlink Pattern** (`initTaskOutputAsSymlink`, line 427):
- Agent transcripts: symlink to `~/.claude/projects/<project>/<session>/subagents/agent-<id>.jsonl`
- Survives `/clear` via symlink re-link in `clearConversation`

### Session Storage (`src/utils/sessionStorage.ts`)

**Project Class** (lines 537-1421):
- Buffered writes with 100ms flush interval (10ms for CCR)
- Write queues per file with 1000-entry limit
- Metadata re-append: custom-title, tag, agent-name, mode, worktree-state, pr-link
- Remote persistence: Session Ingress API or CCR v2 internal events

**Transcript Structure**:
- Main session: `<projectDir>/<sessionId>.jsonl`
- Subagents: `<projectDir>/<sessionId>/subagents/agent-<id>.jsonl`
- Remote agents: `<projectDir>/<sessionId>/remote-agents/remote-agent-<taskId>.meta.json`

---

## 8. Background Job Handling

### Background Task Lifecycle

1. **Spawn**: `registerAsyncAgent()` or `spawnShellTask()`
2. **Registration**: Added to `AppState.tasks` with `isBackgrounded: true`
3. **Execution**: Runs independently via `void` fire-and-forget
4. **Progress**: Periodic `updateAgentProgress()` / `updateTaskOutputDelta()`
5. **Completion**: `completeAsyncAgent()` / `completeMainSessionTask()` → notification
6. **Eviction**: `evictTerminalTask()` after notification + grace period (30s)

### Stall Detection (`src/tasks/LocalShellTask/LocalShellTask.tsx`)

```typescript
const STALL_CHECK_INTERVAL_MS = 5_000;
const STALL_THRESHOLD_MS = 45_000;
const PROMPT_PATTERNS = [/(y\/n)/i, /\[y\/n\]/i, /Press (any key|Enter)/i, ...];
```

- Watches output file size every 5s
- If no growth for 45s AND tail matches prompt pattern → notification
- Prevents false positives on slow commands (git log, builds)

### Summarization (`src/services/AgentSummary/agentSummary.js`)

- Periodic 1-2 sentence progress summaries for coordinator/SDK
- `startAgentSummarization()` returns `stop()` function
- Enabled for coordinator, fork subagents, or SDK progress summaries

---

## 9. "Shell Enabled" Detection

### Shell Availability (`src/utils/Shell.ts`)

**Detection Logic** (`findSuitableShell()`, lines 74-174):
1. **Explicit Override**: `CLAUDE_CODE_SHELL` env var (validated as bash/zsh)
2. **User Preference**: `$SHELL` env var (if bash/zsh)
3. **Discovery**: `which('bash')`, `which('zsh')` + common paths
4. **Validation**: `isExecutable()` checks `X_OK` or `--version` execution

**Windows Special Handling**:
- Warns if WSL bash without Git for Windows bash (lines 79-109)
- PowerShell provider as alternative (`getPsProvider`, line 185)

### Sandbox Detection (`src/utils/sandbox/sandbox-adapter.ts`)

- `SandboxManager.isSandboxingEnabled()`: Checks for bwrap availability
- `SandboxManager.areUnsandboxedCommandsAllowed()`: Policy check
- Used in `--bare` mode and enterprise environments

---

## 10. Input/Output Routing Between Agents

### Message Flow Architecture

```
User Input → processUserInput() → QueryEngine.submitMessage()
    ↓
Tool Execution (Bash, Agent, etc.) → ToolUseContext
    ↓
AgentTool.call() → spawns subagent via registerAsyncAgent()
    ↓
Subagent Context (AsyncLocalStorage) → runAgent()
    ↓
Subagent messages → TaskOutput (disk) + AppState (memory)
    ↓
Completion → enqueueAgentNotification() → <task-notification> XML
    ↓
MessageQueueManager.enqueue() → queued as 'task-notification'
    ↓
Next main loop turn → delivered as user message to coordinator
```

### TaskOutput Data Flow

1. **Shell Commands** (`LocalShellTask`):
   - Child process writes directly to file fd
   - `TaskOutput` polls file tail for progress display
   - `getTaskOutputDelta()` reads incremental updates

2. **Agents** (`LocalAgentTask`):
   - Messages appended to `task.messages` in AppState
   - Sidechain transcript written via `recordSidechainTranscript()`
   - UI can `retain` task for live transcript view

3. **Bridge Sessions** (`bridgeMain.ts`):
   - WebSocket/SSE transport for remote sessions
   - Session ingress JWT for authentication
   - Heartbeat polling for long-running sessions

### Notification Format

All background tasks use XML notification format:
```xml
<task-notification>
  <task-id>agentId</task-id>
  <tool-use-id>toolUseId</tool-use-id>
  <output-file>/path/to/output</output-file>
  <status>completed|failed|killed</status>
  <summary>Human readable summary</summary>
  <result>Final agent response</result>
  <usage>
    <total_tokens>N</total_tokens>
    <tool_uses>N</tool_uses>
    <duration_ms>N</duration_ms>
  </usage>
  <worktree>
    <worktree-path>...</worktree-path>
    <worktree-branch>...</worktree-branch>
  </worktree>
</task-notification>
```

### SDK Event Emission

- `task_started` on registration (`framework.ts:104`)
- `task_progress` on tool use (`emitTaskProgress`, `agentToolUtils.ts:378`)
- `task_terminated` on completion (`emitTaskTerminatedSdk`)

---

## Summary: Key Architectural Patterns

| Pattern | Implementation |
|---------|---------------|
| **Task Registry** | `AppState.tasks` map with typed state per task type |
| **Async Execution** | `void` fire-and-forget + `AbortController` for cancellation |
| **State Isolation** | `AsyncLocalStorage` for agent context, per-task output files |
| **Progress Tracking** | Polling-based (`pollTasks` 1s) + file tail deltas |
| **Notification** | XML via `messageQueueManager` → delivered as user messages |
| **Persistence** | JSONL transcripts + sidecar metadata + symlink for agents |
| **Process Mgmt** | `tree-kill` + size watchdog + exponential backoff restart |
| **Inter-Agent Comm** | Mailbox (file), Bridge (WebSocket), UDS/TCP, in-process queue |
| **Worktree Isolation** | Git worktrees per agent with auto-cleanup |

---

## File Reference Index

| Component | Key Files |
|-----------|-----------|
| Main Entry | `src/main.tsx` |
| Main Loop | `src/QueryEngine.ts`, `src/query.ts` |
| Assistant Mode | `src/assistant/` |
| Task Framework | `src/Task.ts`, `src/utils/task/framework.ts` |
| Local Agent | `src/tasks/LocalAgentTask/LocalAgentTask.tsx` |
| Shell Tasks | `src/tasks/LocalShellTask/LocalShellTask.tsx` |
| Shell Exec | `src/utils/Shell.ts`, `src/utils/ShellCommand.ts` |
| Daemon | `src/daemon/main.ts`, `src/daemon/workerRegistry.ts` |
| Bridge | `src/bridge/bridgeMain.ts`, `src/bridge/bridgeUI.ts` |
| Agent Tool | `packages/builtin-tools/src/tools/AgentTool/AgentTool.tsx` |
| SendMessage | `packages/builtin-tools/src/tools/SendMessageTool/SendMessageTool.ts` |
| Agent Context | `src/utils/agentContext.ts` |
| Session Storage | `src/utils/sessionStorage.ts` |
| Disk Output | `src/utils/task/diskOutput.ts`, `src/utils/task/TaskOutput.ts` |
| Background Sessions | `src/cli/bg.ts`, `src/cli/bg/engines/` |
| Coordinator | `src/coordinator/coordinatorMode.ts` |
| Remote Agent | `src/tasks/RemoteAgentTask/` |
| In-Process Teammate | `src/tasks/InProcessTeammateTask/` |

