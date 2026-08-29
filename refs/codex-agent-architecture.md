# Codex Agent Architecture Analysis

This document provides a comprehensive analysis of the Codex agent architecture based on the codebase at `/tmp/opencode/refs/codex`. The architecture consists of a main agent, subagents, background shell execution, and various coordination mechanisms.

---

## 1. Main Agent Loop

The main agent loop is implemented in the **Session** structure (`codex-rs/core/src/session/session.rs`), which runs the core conversation loop processing user input and model responses.

### Key Components

**Session Structure** (`session.rs:35-66`):
```rust
pub(crate) struct Session {
    pub(crate) thread_id: ThreadId,
    pub(crate) installation_id: String,
    pub(super) tx_event: Sender<Event>,
    pub(super) agent_status: watch::Sender<AgentStatus>,
    pub(super) state: Mutex<SessionState>,
    pub(super) managed_network_proxy_refresh_lock: Semaphore,
    pub(super) features: ManagedFeatures,
    pub(crate) conversation: Arc<RealtimeConversationManager>,
    pub(crate) active_turn: Mutex<Option<ActiveTurn>>,
    pub(crate) async_hook_results: async_channel::Receiver<HookCompletedEvent>,
    pub(crate) input_queue: InputQueue,
    pub(crate) guardian_review_session: GuardianReviewSessionManager,
    pub(crate) services: SessionServices,
    pub(super) git_enrichment_policy: GitEnrichmentPolicy,
    pub(super) fork_persistence: ForkPersistence,
    pub(super) next_internal_sub_id: AtomicU64,
}
```

### Turn Processing Loop

The main agent loop processes turns via the **InputQueue** (`input_queue.rs`) and **Session** turn handling:

1. **Input Reception**: User input arrives via `TurnInputRequest` through `CodexThread::start_or_steer_turn()` (`codex_thread.rs:283-288`)
2. **Capacity Check**: `AgentControl::ensure_execution_capacity_for_turn_start()` ensures agent execution limits (`agent/control.rs:221-222`)
3. **Turn Submission**: Input submitted via `SessionIo::submit_turn_input()` which queues to `InputQueue`
4. **Model Execution**: The turn runner processes the input, calls the model, executes tools, and streams events
5. **Event Streaming**: Events flow through `tx_event` channel to connected clients

### Turn Lifecycle (`session/turn.rs`)

The turn processing follows these phases:
- **Turn Start**: `TurnInputMode::StartOrSteer` or `StartIfIdle`
- **Model Response**: Streaming response items (reasoning, function calls, messages)
- **Tool Execution**: Sandboxed command execution via `exec_command` tool
- **Turn Completion**: Final response, token usage, status updates

---

## 2. Subagent Spawning and Management

Subagents are managed through the **AgentControl** system (`codex-rs/core/src/agent/control.rs`) and **AgentRunner** (`ext/agent/src/lib.rs`).

### AgentControl Architecture

**AgentControl** (`control.rs:106-121`):
```rust
pub(crate) struct AgentControl {
    session_id: SessionId,           // Shared across entire agent tree
    manager: Weak<ThreadManagerState>,
    thread_id_generator: ThreadIdGenerator,
    state: Arc<AgentRegistry>,       // Tracks live agent metadata
    v2_residency: Arc<V2Residency>,  // V2 multi-agent residency slots
    agent_execution_limiter: Arc<AgentExecutionLimiter>,
    rollout_budget: Arc<RolloutBudget>,
}
```

### Subagent Types

1. **ThreadSpawn Subagents** (`control.rs:650-658`):
   - Created via `SessionSource::SubAgent(SubAgentSource::ThreadSpawn)`
   - Have parent-child relationships with depth tracking
   - Inherit environments and exec policies from parent

2. **Review Subagents** (`turn_processor.rs:1283-1397`):
   - Inline reviews (same thread)
   - Detached reviews (separate thread with review skill)

3. **V2 Multi-Agent Subagents** (`control/spawn.rs`):
   - Use `MultiAgentVersion::V2` with residency slots
   - Support forking with `SpawnAgentForkMode::FullHistory` or `LastNTurns`

### Spawning Flow (`control/spawn.rs:389-593`)

```rust
async fn spawn_agent_internal(
    &self,
    config: Config,
    initial_input: SpawnInitialInput,  // UserInput or InterAgentCommunication
    session_source: Option<SessionSource>,
    options: SpawnAgentOptions,
) -> CodexResult<LiveAgent>
```

Key steps:
1. **Capacity Check**: `ensure_execution_capacity()` for V2 agents
2. **Residency Slot**: Reserve V2 residency if applicable (`reserve_v2_residency_slot`)
3. **Registry Reservation**: `state.reserve_spawn_slot()` for agent metadata
4. **Thread Creation**: 
   - `spawn_new_thread_with_source()` for new agents
   - `fork_thread_with_source()` for forked agents
5. **Initial Input**: Submit prompt via `send_input()` or communication
6. **Completion Watcher**: Start watcher for non-V2 agents (`maybe_start_completion_watcher`)

### Agent Metadata & Registry (`agent/registry.rs`)

Tracks:
- `agent_id`: ThreadId
- `agent_path`: Hierarchical path (e.g., "root/child/grandchild")
- `agent_nickname`: Human-readable name
- `agent_role`: Configured role

---

## 3. Background Shell Execution

Background shell execution is handled through the **UnifiedExecProcessManager** (`unified_exec/process_manager.rs`) and **UnifiedExecProcess** (`unified_exec/process.rs`).

### Process Management

**UnifiedExecProcessManager** (`process_manager.rs:421-447`):
- Process ID allocation (random in production, deterministic in tests)
- Process store with LRU eviction (`MAX_UNIFIED_EXEC_PROCESSES = 50`)
- Background terminal tracking via `BackgroundTerminalInfo`

**Process Lifecycle** (`process_manager.rs:459-747`):
1. **exec_command**: Starts process via `open_session_with_sandbox()`
2. **Streaming Output**: `start_streaming_output()` captures stdout/stderr
3. **Yield Collection**: Collects output until deadline (`collect_output_until_deadline`)
4. **Background Persistence**: Long-running processes stored in `ProcessStore`
5. **write_stdin**: Resume interaction with background process
6. **Exit Watcher**: `spawn_exit_watcher()` handles process termination

### PTY Handling (`unified_exec/process.rs`)

**UnifiedExecProcess** wraps either:
- Local PTY process via `codex_sandboxing::spawn_process()`
- Remote exec-server process via gRPC

Key features:
- `OutputHandles` with `HeadTailBuffer` for output buffering
- `CancellationToken` for process termination
- `interaction_lock` for serialized stdin/write operations
- Support for both TTY and non-TTY modes

---

## 4. Process Management and PTY Handling

### Local Process Spawning (`codex-rs/core/src/spawn.rs`)

```rust
pub async fn spawn_child_async(request: SpawnChildRequest) -> Result<Child>
```

Uses `tokio::process::Command` with:
- `StdioPolicy::RedirectForShellTool` for piped stdout/stderr
- Process group management for proper signal propagation
- Environment preparation with sandbox variables

### Sandbox Integration (`codex-rs/core/src/exec.rs`)

**ExecParams** (`exec.rs:95-108`):
```rust
pub struct ExecParams {
    pub command: Vec<String>,
    pub cwd: AbsolutePathBuf,
    pub expiration: ExecExpiration,
    pub capture_policy: ExecCapturePolicy,
    pub env: HashMap<String, String>,
    pub network: Option<NetworkProxy>,
    pub sandbox_permissions: SandboxPermissions,
    pub windows_sandbox_level: WindowsSandboxLevel,
    pub justification: Option<String>,
    pub arg0: Option<String>,
}
```

### Windows Sandbox (`exec.rs:575-747`)

Special handling for `SandboxType::WindowsRestrictedToken`:
- Runs via `codex_windows_sandbox::run_windows_sandbox_capture_*`
- Supports elevated and legacy sandbox levels
- Filesystem overrides for read/write roots

### Output Management

- **Streaming**: Real-time `ExecCommandOutputDeltaEvent` events
- **Buffering**: `HeadTailBuffer` retains head/tail with omission markers
- **Token Estimation**: `approx_tokens_from_byte_count()` for output sizing
- **Drain Timeout**: `IO_DRAIN_TIMEOUT_MS = 2000` for pipe cleanup

---

## 5. Agent-to-Agent Communication

Communication between agents uses **InterAgentCommunication** protocol (`codex-rs/core/src/agent_communication.rs`).

### Communication Types

```rust
pub enum InterAgentCommunication {
    Result { ... },           // Completion notification
    UserMessage { ... },      // User message forwarding
    ToolCall { ... },         // Tool call delegation
    ToolResult { ... },       // Tool result return
    Custom { ... },           // Custom payload
}
```

### Communication Flow (`control.rs:210-293`)

```rust
pub async fn send_inter_agent_communication(
    &self,
    agent_id: ThreadId,
    communication: InterAgentCommunication,
    context: AgentCommunicationContext,
    parent_turn_id: Option<String>,
    root_turn_id: Option<String>,
) -> CodexResult<String>
```

1. **Capacity Check**: For `trigger_turn` communications
2. **Submission**: Via `Op::InterAgentCommunication` to target thread
3. **Logging**: Structured logging via `emit_agent_communication_send()`
4. **V2 Completion**: Automatic result propagation for V2 subagents (`maybe_start_completion_watcher`)

### V2 Multi-Agent Communication (`control.rs:513-602`)

For `MultiAgentVersion::V2` with `ThreadSpawn` source:
- Automatic completion watcher subscribes to child status
- On completion, formats `InterAgentCommunication::Result` to parent
- Uses `AgentPath` for addressing (e.g., "root/child")

---

## 6. Task Delegation and Coordination

### Thread Manager (`codex-rs/core/src/thread_manager.rs`)

**ThreadManager** coordinates all threads:
- `spawn_new_thread()` / `spawn_new_thread_with_source()`
- `spawn_subagent()` for subagent creation
- `fork_thread_with_source()` for forked threads
- `resume_thread_with_history_with_source()` for persistence restore
- Thread lifecycle: `shutdown_all_threads_bounded()`, `remove_thread()`

### Execution Capacity (`agent/control.rs:155-159`)

```rust
pub(crate) fn with_session_id(mut self, session_id: SessionId, max_threads: usize) -> Self {
    self.session_id = session_id;
    self.agent_execution_limiter.initialize(max_threads);
    self
}
```

**AgentExecutionLimiter** (`agent/control/execution.rs`):
- Semaphore-based concurrency control
- Per-session limits (`effective_agent_max_threads`)
- Separate limits for V1 vs V2 agents

### Turn Coordination (`session/turn_input.rs`)

**TurnInputMode** variants:
- `StartOrSteer`: Start new or steer existing turn
- `StartIfIdle`: Only start if no active turn
- `Steer`: Only steer specific turn ID
- `Recover`: Resume interrupted turn

**Submission Results**:
- `TurnInputSubmission::Started { turn_id }`
- `TurnInputSubmission::Steered { turn_id }`
- `TurnInputSubmission::NotSubmitted { reason }`

---

## 7. Shell Session Persistence

Shell sessions persist via **ThreadStore** and **Rollout** system.

### Persistence Layers

1. **ThreadStore** (`codex-thread-store`): SQLite-backed thread metadata
2. **Rollout** (`codex-rollout`): JSONL event log per thread
3. **StateDb** (`codex-state`): SQLite for analytics/logs

### Session Creation (`session.rs:742-822`)

```rust
let thread_persistence_fut = async {
    if config.ephemeral { Ok(None) } else {
        match &initial_history {
            InitialHistory::New | Cleared | Forked(_) => {
                LiveThread::create(thread_store, params).await
            }
            InitialHistory::Resumed(resumed) => {
                LiveThread::resume(thread_store, params).await
            }
        }
    }
};
```

### Fork Persistence (`session.rs:659-662`, `control/spawn.rs:658-661`)

```rust
if let InitialHistory::Forked(items) = &mut initial_history {
    Self::assign_missing_rollout_response_item_ids(items);
}
```

Forked threads inherit parent rollout items, with optional truncation (`LastNTurns`).

### Rollout Flushing (`codex_thread.rs:261-264`)

```rust
pub async fn flush_rollout(&self) -> std::io::Result<()> {
    self.session.flush_rollout().await
}
```

---

## 8. Background Job Handling

Background jobs are managed through the **UnifiedExecProcessManager** process store.

### Background Terminal Tracking (`process_manager.rs:1536-1553`)

```rust
pub async fn list_processes(&self) -> Vec<BackgroundTerminalInfo> {
    let store = self.process_store.lock().await;
    store.processes.values()
        .filter(|entry| !entry.process.has_exited())
        .map(|entry| BackgroundTerminalInfo { ... })
        .collect()
}
```

### Process Pruning (`process_manager.rs:1445-1487`)

LRU eviction with protections:
- Protects 8 most recently used processes
- Prefers evicting exited processes
- Respects interaction locks (write_stdin, terminal events)
- Soft limit: `MAX_UNIFIED_EXEC_PROCESSES = 50`

### Termination (`process_manager.rs:1518-1534`, `1555-1587`)

```rust
pub async fn terminate_all_processes(&self)
pub async fn terminate_process(&self, process_id: i32) -> bool
```

Graceful termination with network approval cleanup.

---

## 9. "Shell Enabled" Detection

Shell detection is in **terminal-detection** (`terminal-detection/src/lib.rs`) and **shell-command** (`shell-command/src/shell_detect.rs`).

### Terminal Detection (`terminal-detection/src/lib.rs:288-389`)

Detection priority:
1. `TERM_PROGRAM` (with tmux client info override)
2. Terminal-specific vars: `WEZTERM_VERSION`, `ITERM_SESSION_ID`, `TERM_SESSION_ID`, etc.
3. `TERM` capability string fallback
4. Multiplexer detection: `TMUX`, `ZELLIJ`

```rust
fn detect_terminal_info_from_env(env: &dyn Environment) -> TerminalInfo {
    let multiplexer = detect_multiplexer(env);
    
    if let Some(term_program) = env.var_non_empty("TERM_PROGRAM") {
        if is_tmux_term_program(&term_program) && matches!(multiplexer, Some(Multiplexer::Tmux { .. })) {
            return terminal_from_tmux_client_info(env.tmux_client_info(), multiplexer.clone());
        }
        // ... direct TERM_PROGRAM handling
    }
    // ... other detectors
}
```

### Shell Detection (`shell-command/src/shell_detect.rs:271-295`)

```rust
pub fn default_user_shell_from_path(user_shell_path: Option<PathBuf>) -> DetectedShell {
    if cfg!(windows) {
        get_shell(ShellType::PowerShell, None).unwrap_or_else(ultimate_fallback_shell)
    } else {
        let user_default_shell = user_shell_path
            .and_then(|shell| detect_shell_type(&shell))
            .and_then(|shell_type| get_shell(shell_type, None));

        let shell_with_fallback = if cfg!(target_os = "macos") {
            user_default_shell
                .or_else(|| get_shell(ShellType::Zsh, None))
                .or_else(|| get_shell(ShellType::Bash, None))
        } else {
            user_default_shell
                .or_else(|| get_shell(ShellType::Bash, None))
                .or_else(|| get_shell(ShellType::Zsh, None))
        };

        shell_with_fallback.unwrap_or_else(ultimate_fallback_shell)
    }
}
```

### Shell Integration in Session (`session.rs:1039-1069`)

```rust
let default_shell = if let Some(user_shell_override) = session_configuration.user_shell_override.clone() {
    user_shell_override
} else if use_zsh_fork_shell {
    // Use packaged zsh fork
} else {
    shell::default_user_shell()
};
```

The shell is wrapped in `ThreadEnvironments` and used for command execution.

---

## 10. Input/Output Routing Between Agents

### Event Flow Architecture

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────────┐
│   Client/TUI    │────▶│  App Server      │────▶│  InProcessAppServer │
│   (WebSocket)   │     │  (MessageProc)   │     │  Client             │
└─────────────────┘     └──────────────────┘     └──────────┬──────────┘
                                                             │
                              ┌──────────────────────────────┘
                              ▼
                    ┌───────────────────────┐
                    │   ThreadManager       │
                    │   (Session/Thread)    │
                    └───────────┬───────────┘
                                │
          ┌─────────────────────┼─────────────────────┐
          ▼                     ▼                     ▼
   ┌─────────────┐      ┌─────────────┐       ┌─────────────┐
   │ Main Agent  │      │  Subagent   │       │  Subagent   │
   │ (Session)   │      │  (Session)  │       │  (Session)  │
   └──────┬──────┘      └──────┬──────┘       └──────┬──────┘
          │                    │                     │
          ▼                    ▼                     ▼
   ┌─────────────────────────────────────────────────────────┐
   │              AgentControl (Shared)                      │
   │  - AgentRegistry    - ExecutionLimiter                  │
   │  - V2Residency      - RolloutBudget                     │
   └─────────────────────────────────────────────────────────┘
```

### Request/Response Routing

**InProcessAppServerClient** (`app-server-client/src/lib.rs:310-322`):
```rust
pub struct InProcessAppServerClient {
    command_tx: mpsc::Sender<ClientCommand>,
    event_rx: mpsc::UnboundedReceiver<InProcessServerEvent>,
    worker_handle: tokio::task::JoinHandle<()>,
}
```

- **Commands**: Bounded channel (`DEFAULT_IN_PROCESS_CHANNEL_CAPACITY = 32`)
- **Events**: Unbounded channel (lossless, ordered)
- **Worker Task**: Bridges caller channels to embedded `MessageProcessor`

### Server Request Handling (`message_processor.rs:767-1908`)

`handle_server_request()` routes server-initiated requests:
- `CommandExecutionRequestApproval` → Auto-rejected in exec mode
- `FileChangeRequestApproval` → Auto-rejected
- `ToolRequestUserInput` → Auto-rejected
- `McpServerElicitationRequest` → Auto-cancelled
- Various approval requests → Rejected in exec mode

### Inter-Agent Event Propagation

1. **Thread Events**: `ThreadStarted`, `TurnStarted`, `TurnCompleted` via `ServerNotification`
2. **Subagent Completion**: V2 agents auto-send `InterAgentCommunication::Result` to parent
3. **Background Terminal**: `ThreadBackgroundTerminalsList`/`Terminate` for shell management
4. **Agent Status**: `watch::Receiver<AgentStatus>` for real-time status updates

### Output Routing

- **Human Output**: `EventProcessorWithHumanOutput` formats for terminal
- **JSONL Output**: `EventProcessorWithJsonOutput` for programmatic consumption
- **Streaming Deltas**: `ExecCommandOutputDeltaEvent` for real-time command output
- **Turn Items**: Collected via `TurnCompleted` with optional backfill from rollout

---

## Summary

The Codex agent architecture is a sophisticated multi-agent system with:

1. **Main Agent Loop**: Session-based turn processing with InputQueue and model streaming
2. **Subagent Management**: Hierarchical AgentControl with V2 residency, registry, and execution limiting
3. **Background Shell**: UnifiedExec process manager with PTY support, output buffering, and LRU eviction
4. **Process Management**: Sandbox-integrated execution with timeout/cancellation handling
5. **Agent Communication**: InterAgentCommunication protocol with automatic V2 completion propagation
6. **Task Delegation**: ThreadManager coordinates spawn/fork/resume with capacity enforcement
7. **Persistence**: SQLite thread store + JSONL rollout + fork inheritance
8. **Background Jobs**: Process store with 50-process limit and graceful termination
9. **Shell Detection**: Multi-source terminal/shell detection with platform-specific fallbacks
10. **I/O Routing**: In-process client with bounded commands/unbounded events, worker task bridging

The architecture cleanly separates concerns between the app-server protocol layer, core session/turn management, agent coordination, and sandboxed execution.
