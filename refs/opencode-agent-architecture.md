# OpenCode Agent Architecture - Comprehensive Analysis

This document provides a thorough analysis of the opencode agent architecture based on the codebase at `/tmp/opencode/refs/opencode`. It covers the main agent loop, subagent spawning and management, background shell execution, process management, PTY handling, agent-to-agent communication, task delegation, shell session persistence, background job handling, shell-enabled detection, and input/output routing.

---

## Table of Contents

1. [Main Agent Loop](#1-main-agent-loop)
2. [Subagent Spawning and Management](#2-subagent-spawning-and-management)
3. [Background Shell Execution](#3-background-shell-execution)
4. [Process Management and PTY Handling](#4-process-management-and-pty-handling)
5. [Agent-to-Agent Communication](#5-agent-to-agent-communication)
6. [Task Delegation and Coordination](#6-task-delegation-and-coordination)
7. [Shell Session Persistence](#7-shell-session-persistence)
8. [Background Job Handling](#8-background-job-handling)
9. [Shell-Enabled Detection](#9-shell-enabled-detection)
10. [Input/Output Routing Between Agents](#10-inputoutput-routing-between-agents)

---

## 1. Main Agent Loop

The main agent loop is implemented in the **SessionRunner** (`packages/core/src/session/runner/llm.ts`) and **SessionPrompt** (`packages/opencode/src/session/prompt.ts`). The loop runs durable coding-agent sessions until they settle.

### 1.1 Core Loop Structure (SessionRunner.run)

**File:** `packages/core/src/session/runner/llm.ts:383-406`

```typescript
const run = Effect.fn("SessionRunner.run")(function* (input: {
  readonly sessionID: SessionSchema.ID
  readonly force: boolean
}) {
  const hasSteer = yield* SessionInput.hasPending(db, input.sessionID, "steer")
  const hasQueue = hasSteer ? false : yield* SessionInput.hasPending(db, input.sessionID, "queue")
  if (!input.force && !hasSteer && !hasQueue) return
  
  yield* failInterruptedTools(input.sessionID)
  let promotion: SessionInput.Delivery | undefined = hasSteer ? "steer" : hasQueue ? "queue" : undefined
  let shouldRun = input.force || hasSteer || hasQueue
  
  while (shouldRun) {
    let needsContinuation = true
    let step = 1
    while (needsContinuation) {
      const result = yield* runTurn(input.sessionID, promotion, step)
      needsContinuation = result.needsContinuation
      step = result.step + 1
      promotion = "steer"
      if (!needsContinuation) needsContinuation = yield* SessionInput.hasPending(db, input.sessionID, "steer")
    }
    shouldRun = yield* SessionInput.hasPending(db, input.sessionID, "queue")
    promotion = shouldRun ? "queue" : undefined
  }
})
```

**Key characteristics:**
- **Turn-based execution**: Each "turn" is one LLM provider call
- **Step limiting**: Agents can have `steps` config limiting max turns (`agent.info?.steps`)
- **Continuation logic**: Continues while tool calls need results or new steering input arrives
- **Promotion system**: Background jobs can be "promoted" to foreground

### 1.2 Turn Execution (runTurnAttempt)

**File:** `packages/core/src/session/runner/llm.ts:173-348`

The turn execution flow:
1. **Session validation** - Verify session belongs to current location
2. **Agent selection** - Load agent config and permissions
3. **System context initialization** - Load system prompts, skills, references
4. **History loading** - Get projected V2 session messages
5. **Tool materialization** - Resolve tool definitions based on agent permissions
6. **Compaction check** - Auto-compact if context overflow detected
7. **LLM streaming** - Single `llm.stream(request)` provider turn
8. **Tool settlement** - Execute and await all local tool calls before continuation
9. **Post-turn processing** - Snapshots, cost tracking, step completion events

### 1.3 SessionPrompt Loop (User-facing)

**File:** `packages/opencode/src/session/prompt.ts:1081-1341`

```typescript
const runLoop: (sessionID: SessionID) => Effect.Effect<SessionV1.WithParts> = Effect.fn("SessionPrompt.run")(
  function* (sessionID: SessionID) {
    while (true) {
      yield* status.set(sessionID, { type: "busy" })
      let msgs = yield* MessageV2.filterCompactedEffect(sessionID)
      const { user: lastUser, assistant: lastAssistant, finished: lastFinished, tasks } = MessageV2.latest(msgs)
      
      // Check for subtask (subagent delegation)
      const task = tasks.pop()
      if (task?.type === "subtask") {
        yield* handleSubtask({ task, model, lastUser, sessionID, session, msgs })
        continue
      }
      
      // Check for compaction
      if (task?.type === "compaction") { ... }
      
      // Check for overflow compaction
      if (lastFinished && ... isOverflow) { ... }
      
      // Normal LLM processing via SessionProcessor
      const handle = yield* processor.create({ assistantMessage: msg, sessionID, model })
      const outcome = yield* handle.process({ ... })
      if (outcome === "break") break
    }
  }
)
```

---

## 2. Subagent Spawning and Management

Subagents are spawned via the **TaskTool** (`packages/opencode/src/tool/task.ts`). Each subagent runs in its own session with inherited permissions.

### 2.1 TaskTool Execution Flow

**File:** `packages/opencode/src/tool/task.ts:92-347`

```typescript
const run = Effect.fn("TaskTool.execute")(function* (params, ctx) {
  // 1. Check subagent depth limit
  let current = parent
  let depth = 0
  while (current.parentID) {
    depth++
    current = yield* sessions.get(current.parentID)
  }
  if (depth >= (cfg.subagent_depth ?? 1)) { fail }
  
  // 2. Permission check for task tool
  yield* ctx.ask({ permission: id, patterns: [params.subagent_type], ... })
  
  // 3. Get subagent config
  const next = yield* agent.get(params.subagent_type)
  
  // 4. Resume existing or create new session
  const session = params.task_id
    ? yield* sessions.get(SessionID.make(params.task_id))
    : yield* sessions.create({
        parentID: ctx.sessionID,
        title: params.description + ` (@${next.name} subagent)`,
        agent: next.name,
        permission: [ ...childPermission, ...childToolDenies ],
      })
  
  // 5. Run task (foreground or background)
  if (runInBackground) {
    // Background: start background job, return immediately
    const info = yield* background.start({ id: nextSession.id, run: runTask(), ... })
    yield* notify(info.id)
    return backgroundResult()
  } else {
    // Foreground: wait for completion
    return yield* Effect.raceFirst(
      background.wait({ id: nextSession.id }),
      background.waitForPromotion(nextSession.id)
    )
  }
})
```

### 2.2 Subagent Permission Derivation

**File:** `packages/opencode/src/agent/subagent-permissions.ts:14-27`

```typescript
export function deriveSubagentSessionPermission(input: {
  parentSessionPermission: PermissionV1.Ruleset
  subagent: Agent.Info
}): PermissionV1.Ruleset {
  const canTask = input.subagent.permission.some((rule) => rule.permission === "task")
  const canTodo = input.subagent.permission.some((rule) => rule.permission === "todowrite")
  return [
    // Parent deny rules and external_directory rules propagate
    ...input.parentSessionPermission.filter(
      (rule) => rule.permission === "external_directory" || rule.action === "deny",
    ),
    // Deny todowrite if subagent doesn't explicitly allow
    ...(canTodo ? [] : [{ permission: "todowrite", pattern: "*", action: "deny" }]),
    // Deny task if subagent doesn't explicitly allow
    ...(canTask ? [] : [{ permission: "task", pattern: "*", action: "deny" }]),
  ]
}
```

### 2.3 Subagent Session Creation

**File:** `packages/opencode/src/tool/task.ts:136-172`

```typescript
const childPermission = deriveSubagentSessionPermission({
  parentSessionPermission: parent.permission ?? [],
  subagent: next,
})
const childToolDenies = [
  // Deny todowrite unless subagent explicitly allows
  ...(next.permission.some((rule) => rule.permission === "todowrite") ? [] : [{ permission: "todowrite", pattern: "*", action: "deny" }]),
  // Deny task tool (prevent recursive subagents) unless subagent explicitly allows
  ...(next.permission.some((rule) => rule.permission === id) ? [] : [{ permission: id, pattern: "*", action: "deny" }]),
  // Deny experimental primary tools
  ...(cfg.experimental?.primary_tools?.map((permission) => ({
    permission, pattern: "*", action: "deny"
  })) ?? []),
]

const nextSession = session ?? (yield* sessions.create({
  parentID: ctx.sessionID,
  title: params.description + ` (@${next.name} subagent)`,
  agent: next.name,
  permission: [ ...childPermission, ...childToolDenies.filter(...) ],
}))
```

### 2.4 Built-in Subagent Types

**File:** `packages/opencode/src/agent/agent.ts:182-218`

Default subagents defined in `Agent.Service`:
- **general** (mode: "subagent"): General-purpose agent for researching complex questions and executing multi-step tasks
- **explore** (mode: "subagent"): Fast agent specialized for exploring codebases (grep, glob, list, bash, read, webfetch, websearch)
- **compaction** (mode: "primary", hidden): Used for context compaction
- **title** (mode: "primary", hidden): Generates session titles
- **summary** (mode: "primary", hidden): Generates summaries

---

## 3. Background Shell Execution

Background execution is handled through the **BackgroundJob** service and the **TaskTool**'s background mode.

### 3.1 BackgroundJob Service

**File:** `packages/core/src/background-job.ts:1-365`

The BackgroundJob service manages asynchronous, long-running operations with:
- **Job registry**: In-memory map of jobs with status tracking
- **Fork/extend model**: Jobs can be extended with additional work
- **Promotion**: Background jobs can be promoted to foreground
- **Output streaming**: Captured output with sequence numbers

```typescript
type Active = {
  info: Info
  done: Deferred.Deferred<Info>
  scope: Scope.Closeable
  token: object
  pending: number
  next: number
  output?: { sequence: number; text: string }
  tail: Deferred.Deferred<void>
  promoted: Deferred.Deferred<Info>
  onPromote?: Effect.Effect<void>
}
```

### 3.2 Background Subagent Execution

**File:** `packages/opencode/src/tool/task.ts:273-308`

```typescript
const info = yield* background.start({
  id: nextSession.id,
  type: id,
  title: params.description,
  metadata,
  onPromote: Effect.all([
    ctx.metadata({ title: params.description, metadata: { ...metadata, background: true, jobId: nextSession.id } }),
    notify(nextSession.id),
  ]),
  run: runTask().pipe(Effect.onInterrupt(() => ops.cancel(nextSession.id))),
})
```

**Background flow:**
1. Create session with `parentID` linking to parent
2. Start background job with `runTask()` effect
3. Return immediately with "running" status
4. Fork notification handler that waits for job completion
5. On completion, inject result into parent session via synthetic message

### 3.3 Background Job Extension

**File:** `packages/core/src/background-job.ts:256-290`

```typescript
const extend: Interface["extend"] = Effect.fn("BackgroundJob.extend")(function* (input) {
  // Add more work to existing running job
  // Chains via previous tail Deferred
  // Increments pending count and sequence
})
```

Used by TaskTool when sending additional context to running background task (`task.ts:256-270`).

---

## 4. Process Management and PTY Handling

Process management is handled at multiple levels: **AppProcess** for simple commands, **PTY** for interactive terminals, and **ShellTool** for command execution.

### 4.1 AppProcess (Simple Command Execution)

**File:** `packages/core/src/process.ts:1-261`

```typescript
export interface Interface = ChildProcessSpawner["Service"] & {
  readonly run: (command: ChildProcess.Command, options?: RunOptions) => Effect.Effect<RunResult, AppProcessError>
  readonly runStream: (command: ChildProcess.Command, options?: RunStreamOptions) => Stream.Stream<string, AppProcessError>
}
```

Features:
- Spawns child processes via `ChildProcessSpawner`
- Collects stdout/stderr with configurable byte limits
- Supports timeout, abort signals, stdin piping
- Returns structured `RunResult` with exit code, output, truncation info

### 4.2 PTY Service (Interactive Terminals)

**File:** `packages/core/src/pty.ts:1-318`

The PTY service manages persistent terminal sessions using the `#pty` native module.

**Session lifecycle:**
```typescript
// Create session
const proc = yield* Effect.sync(() => spawn(command, args, { name: "xterm-256color", cwd, env }))
const session: Active = {
  info: { id, title, command, args, cwd, status: "running", pid: proc.pid },
  process: proc,
  buffer: "",           // Output buffer (capped at 2MB)
  bufferCursor: 0,      // Start of retained buffer
  cursor: 0,            // Absolute output cursor
  subscribers: new Map(), // Active attachments
  listeners: [],        // Native event listeners
}
```

**Key features:**
- **Buffer management**: 2MB ring buffer with cursor tracking
- **Subscriber model**: Multiple clients can attach with replay from cursor
- **Exit tracking**: Retains exited sessions (up to 25) for inspection
- **Resize support**: `update()` handles terminal resize
- **Write support**: `write()` sends input to running process

**Attachment protocol:**
```typescript
type AttachInput = {
  readonly cursor?: number  // -1 = tail, number = absolute, undefined = full replay
  readonly onData: (chunk: string) => void
  readonly onEnd: (event: { exitCode?: number }) => void
}
```

### 4.3 ShellTool (Command Execution in Sessions)

**File:** `packages/opencode/src/tool/shell.ts:1-645`

The ShellTool executes commands within the context of a session:
- Parses bash/PowerShell commands using tree-sitter
- Resolves file paths for permission checks
- Runs commands via `ChildProcessSpawner`
- Streams output with truncation (configurable limits)
- Handles timeout, abort, and cleanup

**Execution flow:**
```typescript
const run = Effect.fn("ShellTool.run")(function* (input, ctx) {
  const handle = yield* spawner.spawn(cmd(input.shell, input.command, input.cwd, input.env))
  yield* Stream.runForEach(Stream.decodeText(handle.all), (chunk) => {
    // Accumulate output, handle truncation
    // Update metadata for real-time UI
  })
  // Race: exitCode vs abort vs timeout
  const exit = yield* Effect.raceAll([...])
  // Build result with metadata
  return { title, metadata: { output, exit, truncated, outputPath }, output }
})
```

---

## 5. Agent-to-Agent Communication

Communication between agents (parent ↔ subagent) occurs through several mechanisms:

### 5.1 Session Hierarchy

**File:** `packages/opencode/src/session/session.ts:222-245`

```typescript
export const Info = Schema.Struct({
  id: SessionID,
  // ...
  parentID: optional(SessionID),  // Links to parent session
  // ...
})
```

### 5.2 TaskTool Result Injection

**File:** `packages/opencode/src/tool/task.ts:216-243`

```typescript
const inject = Effect.fn("TaskTool.injectBackgroundResult")(function* (
  state: "completed" | "error",
  text: string,
) {
  const currentParent = yield* sessions.get(ctx.sessionID)
  yield* ops.prompt({
    sessionID: ctx.sessionID,
    agent: currentParent.agent ?? ctx.agent,
    variant,
    parts: [{
      type: "text",
      synthetic: true,
      text: renderOutput({
        sessionID: nextSession.id,
        state,
        summary: state === "completed" ? `Background task completed: ${params.description}` : `Background task failed: ${params.description}`,
        text,
      }),
    }],
  }).pipe(Effect.ignore, Effect.forkIn(scope, { startImmediately: true }))
})
```

### 5.3 Background Job Metadata

**File:** `packages/opencode/src/tool/task.ts:185-190`

```typescript
const metadata = {
  parentSessionId: ctx.sessionID,
  sessionId: nextSession.id,
  model,
  ...(runInBackground ? { background: true } : {}),
}
```

Passed to background job for tracking relationships.

### 5.4 Event Bus (EventV2)

**File:** `packages/core/src/event.ts` (referenced throughout)

All agents publish/subscribe to `EventV2.Service` for:
- Session events (Created, Updated, Deleted, MessageUpdated, PartUpdated)
- Tool events (Tool.Failed, Tool.Succeeded)
- Step events (Step.Started, Step.Ended)
- Compaction events

---

## 6. Task Delegation and Coordination

Task delegation is primarily through the **task** tool (subagent spawning) and **command** system.

### 6.1 Subtask Delegation (TaskTool)

**File:** `packages/opencode/src/session/prompt.ts:255-449`

```typescript
const handleSubtask = Effect.fn("SessionPrompt.handleSubtask")(function* (input) {
  // Create assistant message with tool call
  const assistantMessage = yield* sessions.updateMessage({ ... })
  let part = yield* sessions.updatePart({
    type: "tool",
    tool: TaskTool.id,
    state: { status: "running", input: taskArgs, time: { start: Date.now() } },
  })
  
  // Execute task tool with promptOps from parent
  const result = yield* taskTool.execute(taskArgs, {
    agent: task.agent,
    messageID: assistantMessage.id,
    sessionID,
    abort: taskAbort.signal,
    callID: part.callID,
    extra: { bypassAgentCheck: true, promptOps },
    messages: msgs,
    metadata: (val) => sessions.updatePart({ ...part, state: { ...part.state, ...val } }),
    ask: (req) => permission.ask({ ...req, ruleset: Permission.merge(taskAgent.permission, session.permission ?? []) }),
  })
  
  // Update tool part with result
  yield* sessions.updatePart({
    ...part,
    state: { status: "completed", output: result.output, metadata: result.metadata, ... }
  })
  
  // Inject summary prompt for parent to continue
  const summaryUserMsg: SessionV1.User = { ... }
  yield* sessions.updateMessage(summaryUserMsg)
  yield* sessions.updatePart({ type: "text", synthetic: true, text: "Summarize the task tool output above and continue with your task." })
})
```

### 6.2 Command Delegation

**File:** `packages/opencode/src/session/prompt.ts:1356-1481`

Commands can delegate to subagents via `subtask: true` in command config:

```typescript
const isSubtask = (agent.mode === "subagent" && cmd.subtask !== false) || cmd.subtask === true
const parts = isSubtask
  ? [{
      type: "subtask",
      agent: agent.name,
      description: cmd.description ?? "",
      command: input.command,
      model: { providerID: taskModel.providerID, modelID: taskModel.modelID },
      prompt: templateParts.find((y) => y.type === "text")?.text ?? "",
    }]
  : [...uniqueTemplateParts, ...(input.parts ?? [])]
```

### 6.3 Coordination via SessionRunState

**File:** `packages/opencode/src/session/run-state.ts:1-151`

```typescript
export interface Interface {
  readonly assertNotBusy: (sessionID: SessionID) => Effect.Effect<void, Session.BusyError>
  readonly cancel: (sessionID: SessionID) => Effect.Effect<void>
  readonly ensureRunning: (sessionID, onInterrupt, work) => Effect.Effect<SessionV1.WithParts>
  readonly startShell: (sessionID, onInterrupt, work, ready?) => Effect.Effect<SessionV1.WithParts, Session.BusyError>
}
```

Uses `Runner` (`packages/opencode/src/effect/runner.ts`) for state machine:
- **Idle**: No work running
- **Running**: LLM turn in progress
- **Shell**: Shell command executing
- **ShellThenRun**: Shell completed, LLM turn queued

---

## 7. Shell Session Persistence

Shell sessions are persisted through the **PTY service** and **Session** database records.

### 7.1 PTY Session Persistence

**File:** `packages/core/src/pty.ts:1-318`

```typescript
const create = Effect.fn("Pty.create")(function* (input: CreateInput) {
  const proc = yield* Effect.sync(() => spawn(command, args, { name: "xterm-256color", cwd, env }))
  const info: Info = {
    id,
    title: input.title || `Terminal ${id.slice(-4)}`,
    command,
    args,
    cwd,
    status: "running",
    pid: proc.pid,
  }
  const session: Active = {
    info,
    process: proc,
    buffer: "",        // Retained output buffer
    bufferCursor: 0,   // Start of retained buffer
    cursor: 0,         // Absolute cursor
    subscribers: new Map(),
    listeners: [],
  }
  sessions.set(id, session)
  
  // Data listener - updates buffer and notifies subscribers
  proc.onData((chunk) => {
    session.cursor += chunk.length
    // Notify active subscribers
    for (const [token, subscriber] of session.subscribers.entries()) {
      if (!subscriber.active) { subscriber.pending.push(chunk); continue }
      subscriber.onData(chunk)
    }
    // Ring buffer management (2MB limit)
    session.buffer += chunk
    if (session.buffer.length > BUFFER_LIMIT) {
      const excess = session.buffer.length - BUFFER_LIMIT
      session.buffer = session.buffer.slice(excess)
      session.bufferCursor += excess
    }
  })
  
  // Exit listener
  proc.onExit(({ exitCode }) => {
    session.info.status = "exited"
    session.info.exitCode = exitCode
    notifyEnd(session, { exitCode })
    exitOrder.push(id)
    // Cleanup old exited sessions (keep 25)
    while (exitOrder.length > EXITED_LIMIT) { yield* removeSession(exitOrder[0]) }
  })
  
  return info
})
```

**Attachment with replay:**
```typescript
const attach = Effect.fn("Pty.attach")(function* (id, input) {
  const session = yield* requireSession(id)
  if (session.info.status !== "running") return yield* new ExitedError({ ptyID: id })
  
  const token = {}
  const subscriber: Subscriber = { onData: input.onData, onEnd: input.onEnd, active: false, detached: false, pending: [] }
  session.subscribers.set(token, subscriber)
  
  // Calculate replay from cursor
  const start = session.bufferCursor
  const end = session.cursor
  const from = input.cursor === -1 ? end : Math.max(0, input.cursor ?? 0)
  const replay = session.buffer.slice(Math.max(0, from - start))
  
  return {
    replay,
    cursor: end,
    write: (data) => { if (session.info.status === "running") session.process.write(data) },
    activate: () => { /* flush pending */ },
    detach: () => { session.subscribers.delete(token) },
  }
})
```

### 7.2 Session Database Persistence

**File:** `packages/core/src/session.ts:1-486` and `packages/opencode/src/session/session.ts:1-1018`

Sessions stored in SQLite via Drizzle ORM:
- `SessionTable`: Core session metadata (id, project, directory, agent, model, permissions, timestamps)
- `SessionMessageTable`: Messages with parts (JSON serialized)
- `PartTable`: Individual message parts

```typescript
// Session creation persists to DB
yield* db.insert(ProjectTable).values({ id: project.id, worktree: project.directory, ... })
const info = SessionV1.SessionInfo.make({ id: sessionID, slug, version, projectID, directory, ... })
yield* events.publish(SessionV1.Event.Created, { sessionID, info }, { location: input.location })
```

---

## 8. Background Job Handling

### 8.1 BackgroundJob Registry

**File:** `packages/core/src/background-job.ts:120-361`

```typescript
export const make = Effect.gen(function* () {
  const state: State = {
    jobs: yield* SynchronizedRef.make(new Map()),
    scope: yield* Scope.Scope,
  }
  
  // Job settlement with sequence tracking
  const settle = Effect.fn("BackgroundJob.settle")(function* (id, token, sequence, exit) {
    // Decrements pending, updates output if successful
    // When pending reaches 0, marks completed/error/cancelled
    // Succeeds done Deferred, closes scope
  })
  
  // Fork with token-based ownership
  const fork = Effect.fn("BackgroundJob.fork")(function* (scope, id, token, sequence, run) {
    return yield* run.pipe(
      Effect.matchCauseEffect({ onSuccess, onFailure }),
      Effect.forkIn(scope, { startImmediately: true })
    )
  })
  
  // Start new job or return existing
  const start: Interface["start"] = Effect.fn("BackgroundJob.start")(function* (input) {
    return yield* Effect.uninterruptibleMask((restore) =>
      Effect.gen(function* () {
        // Create job entry with scope, token, done Deferred
        // Fork the run effect
        // Return info
      })
    )
  })
  
  // Extend running job with more work
  const extend: Interface["extend"] = ...
  
  // Wait for completion with timeout
  const wait: Interface["wait"] = ...
  
  // Wait for promotion (background -> foreground)
  const waitForPromotion: Interface["waitForPromotion"] = ...
  
  // Promote background job to foreground
  const promote: Interface["promote"] = ...
  
  // Cancel running job
  const cancel: Interface["cancel"] = ...
  
  return Service.of({ list, get, start, extend, wait, waitForPromotion, promote, cancel })
})
```

### 8.2 Integration with TaskTool

**File:** `packages/opencode/src/tool/task.ts:256-347`

```typescript
// Check if job already running (extend)
if (yield* background.extend({ id: nextSession.id, run: runTask() })) {
  return { title: params.description, metadata: { ...metadata, background: true, jobId: nextSession.id }, output: ... }
}

// Start new background job
const info = yield* background.start({
  id: nextSession.id,
  type: id,
  title: params.description,
  metadata,
  onPromote: Effect.all([ ctx.metadata({...}), notify(nextSession.id) ]),
  run: runTask().pipe(Effect.onInterrupt(() => ops.cancel(nextSession.id))),
})

if (runInBackground) {
  yield* notify(info.id)  // Fork waiter for completion
  return backgroundResult()
}

// Foreground: wait for completion or promotion
const result = yield* Effect.raceFirst(
  background.wait({ id: nextSession.id }).pipe(Effect.map((w) => w.info)),
  background.waitForPromotion(nextSession.id),
)
```

### 8.3 SessionRunState Background Job Cleanup

**File:** `packages/opencode/src/session/run-state.ts:111-143`

```typescript
const cancelBackgroundJobs = Effect.fn("SessionRunState.cancelBackgroundJobs")(function* (background, sessionID) {
  const jobs = yield* background.list()
  const pending = new Set<string>([sessionID])
  const cancelled = new Set<string>()
  
  const matches = (job) => {
    if (job.status !== "running") return false
    if (cancelled.has(job.id)) return false
    if (pending.has(job.id)) return true
    if (job.metadata?.sessionId && pending.has(job.metadata.sessionId)) return true
    return job.metadata?.parentSessionId && pending.has(job.metadata.parentSessionId)
  }
  
  let batch = jobs.filter(matches)
  while (batch.length > 0) {
    yield* Effect.forEach(batch, (job) => background.cancel(job.id), { concurrency: "unbounded", discard: true })
    batch = jobs.filter(matches)
  }
})
```

---

## 9. Shell-Enabled Detection

Shell-enabled detection determines if a shell is available and acceptable for command execution.

### 9.1 Shell Detection Logic

**File:** `packages/core/src/shell.ts:1-226`

```typescript
const META: Record<string, { deny?: boolean; login?: boolean; posix?: boolean; ps?: boolean }> = {
  bash: { login: true, posix: true },
  dash: { login: true, posix: true },
  fish: { deny: true, login: true },
  ksh: { login: true, posix: true },
  nu: { deny: true },
  powershell: { ps: true },
  pwsh: { ps: true },
  sh: { login: true, posix: true },
  zsh: { login: true, posix: true },
}

function ok(file: string) {
  return meta(file)?.deny !== true
}

function resolve(file: string) {
  const shell = full(file)
  if (rooted(shell)) {
    if (stat(shell)?.isFile()) return shell
    return
  }
  return which(shell) ?? undefined
}

function select(file: string | undefined, opts?: { acceptable?: boolean }) {
  if (file && (!opts?.acceptable || ok(file))) {
    const shell = resolve(file)
    if (shell) return shell
  }
  if (process.platform === "win32") return win()[0]
  return fallback()
}

export function preferred(configShell?: string) {
  if (configShell) return select(configShell)
  defaultPreferred ??= select(process.env.SHELL)
  return defaultPreferred
}

export function acceptable(configShell?: string) {
  if (configShell) return select(configShell, { acceptable: true })
  defaultAcceptable ??= select(process.env.SHELL, { acceptable: true })
  return defaultAcceptable
}
```

### 9.2 Platform-Specific Resolution

**Windows (`packages/core/src/shell.ts:98-106`):**
```typescript
function win() {
  return Array.from(new Set(
    [which("pwsh"), which("powershell"), gitbash(), process.env.COMSPEC || "cmd.exe"]
      .filter((item): item is string => Boolean(item))
      .map(full),
  ))
}
```

**Unix (`packages/core/src/shell.ts:108-112`):**
```typescript
async function unix() {
  const text = await readFile("/etc/shells", "utf8").catch(() => "")
  if (text) return Array.from(new Set(text.split("\n").filter((line) => line.trim() && !line.startsWith("#"))))
  return ["/bin/bash", "/bin/zsh", "/bin/sh"]
}
```

### 9.3 Shell Argument Construction

**File:** `packages/core/src/shell.ts:166-200`

```typescript
export function args(file: string, command: string, cwd: string) {
  const n = name(file)
  if (n === "nu" || n === "fish") return ["-c", command]
  if (n === "zsh") {
    return ["-l", "-c", `[[ -f ~/.zshenv ]] && source ~/.zshenv ... cd -- "$1" eval ${JSON.stringify(command)}`, "opencode", cwd]
  }
  if (n === "bash") {
    return ["-l", "-c", `shopt -s expand_aliases [[ -f ~/.bashrc ]] && source ~/.bashrc ... cd -- "$1" eval ${JSON.stringify(command)}`, "opencode", cwd]
  }
  if (n === "cmd") return ["/c", command]
  if (ps(file)) return ["-NoProfile", "-Command", command]
  return ["-c", command]
}
```

### 9.4 Usage in ShellTool

**File:** `packages/opencode/src/tool/shell.ts:597-604`

```typescript
return () => Effect.gen(function* () {
  const cfg = yield* config.get()
  const shell = Shell.acceptable(cfg.shell)  // Uses acceptable() for security
  const name = Shell.name(shell)
  const prompt = ShellPrompt.render(name, process.platform, limits, defaultTimeoutMs)
  // ...
})
```

---

## 10. Input/Output Routing Between Agents

### 10.1 Parent → Subagent (Task Delegation)

**File:** `packages/opencode/src/tool/task.ts:200-214`

```typescript
const runTask = Effect.fn("TaskTool.runTask")(function* () {
  const parts = yield* ops.resolvePromptParts(params.prompt)
  const result = yield* ops.prompt({
    messageID: MessageID.ascending(),
    sessionID: nextSession.id,
    model: { modelID: model.modelID, providerID: model.providerID },
    variant: next.model ? undefined : variant,
    agent: next.name,
    parts,
  })
  return result.parts.findLast((item) => item.type === "text")?.text ?? ""
})
```

The parent's prompt is resolved and sent as the initial user message to the subagent's session.

### 10.2 Subagent → Parent (Result Return)

**File:** `packages/opencode/src/tool/task.ts:216-243`

```typescript
const inject = Effect.fn("TaskTool.injectBackgroundResult")(function* (state, text) {
  const currentParent = yield* sessions.get(ctx.sessionID)
  yield* ops.prompt({
    sessionID: ctx.sessionID,
    agent: currentParent.agent ?? ctx.agent,
    variant,
    parts: [{
      type: "text",
      synthetic: true,
      text: renderOutput({ sessionID: nextSession.id, state, summary, text }),
    }],
  }).pipe(Effect.ignore, Effect.forkIn(scope, { startImmediately: true }))
})
```

Results injected as synthetic user messages in parent session.

### 10.3 Foreground Subagent Result Handling

**File:** `packages/opencode/src/tool/task.ts:310-334`

```typescript
return yield* Effect.acquireUseRelease(
  Effect.sync(() => { ctx.abort.addEventListener("abort", onAbort) }),
  () => Effect.gen(function* () {
    const result = yield* Effect.raceFirst(
      background.wait({ id: nextSession.id }).pipe(Effect.map((w) => w.info)),
      background.waitForPromotion(nextSession.id),
    )
    if (result?.metadata?.background === true) return backgroundResult()
    if (result?.status === "error") return yield* Effect.fail(new Error(result.error ?? "Task failed"))
    if (result?.status === "cancelled") return yield* Effect.fail(new Error("Task cancelled"))
    return {
      title: params.description,
      metadata,
      output: renderOutput({ sessionID: nextSession.id, state: "completed", text: result?.output ?? "" }),
    }
  }),
  (_, exit) => Effect.gen(function* () {
    if (Exit.hasInterrupts(exit))
      yield* Effect.all([cancel, background.cancel(nextSession.id)], { discard: true })
  }).pipe(Effect.ensuring(Effect.sync(() => ctx.abort.removeEventListener("abort", onAbort))))
)
```

### 10.4 SessionPrompt I/O Processing

**File:** `packages/opencode/src/session/prompt.ts:635-1050` (`createUserMessage`)

```typescript
const createUserMessage = Effect.fn("SessionPrompt.createUserMessage")(function* (input: PromptInput) {
  // Resolve parts: files, agents, text
  const resolvedParts = yield* Effect.forEach(input.parts, resolvePart, { concurrency: "unbounded" })
  
  // File parts: read content, create text + file parts
  // Agent parts: add synthetic text prompting task tool usage
  
  yield* sessions.updateMessage(info)
  for (const part of parts) yield* sessions.updatePart(part)
  return { info, parts }
})
```

### 10.5 LLM Stream Event Handling

**File:** `packages/opencode/src/session/processor.ts:278-537` (`handleEvent`)

Routes LLM stream events to session parts:
- `text-start`/`text-delta`/`text-end` → TextPart
- `tool-input-start`/`tool-input-delta`/`tool-input-end`/`tool-call` → ToolPart (pending/running)
- `tool-result`/`tool-error` → ToolPart (completed/error)
- `reasoning-start`/`reasoning-delta`/`reasoning-end` → ReasoningPart
- `step-start`/`step-finish` → StepStartPart/StepFinishPart (with snapshots, cost, tokens)
- `provider-error` → throws error

### 10.6 Tool Execution Routing

**File:** `packages/opencode/src/session/tools.ts` (referenced in prompt.ts:1226-1241)

```typescript
const tools = yield* SessionTools.resolve({
  agent,
  session,
  model,
  processor: handle,
  bypassAgentCheck,
  messages: msgs,
  promptOps,
}).pipe(
  Effect.provideService(Plugin.Service, plugin),
  Effect.provideService(Permission.Service, permission),
  Effect.provideService(ToolRegistry.Service, registry),
  Effect.provideService(MCP.Service, mcp),
  Effect.provideService(Truncate.Service, truncate),
  Effect.provideService(RuntimeFlags.Service, flags),
)
```

Tool execution flows through:
1. `ToolRegistry` - resolves tool definitions
2. `Permission.Service` - checks permissions via `ask()`
3. Tool's `execute()` - runs the actual implementation
4. `SessionProcessor.completeToolCall()` / `failToolCall()` - records result

---

## Summary of Key Architectural Patterns

| Pattern | Implementation |
|---------|---------------|
| **Agent Loop** | Turn-based LLM streaming with tool settlement (SessionRunner + SessionPrompt) |
| **Subagent Spawning** | TaskTool creates child sessions with derived permissions, runs via BackgroundJob |
| **Background Execution** | BackgroundJob service with fork/extend/promote model |
| **Process Management** | AppProcess for commands, PTY for interactive terminals |
| **Session Persistence** | SQLite (Drizzle) for sessions/messages; PTY buffer for terminal output |
| **Inter-Agent Comm** | Session hierarchy (parentID), EventV2 bus, synthetic message injection |
| **Task Delegation** | TaskTool (subagents), Command system (subtask: true), SessionRunState coordination |
| **Shell Detection** | Shell.preferred/acceptable with platform-specific resolution |
| **I/O Routing** | SessionPrompt.resolvePart → LLM → Processor.handleEvent → ToolRegistry → Session.updatePart |

---

## Key File References

| Component | Primary Files |
|-----------|---------------|
| Main Agent Loop | `packages/core/src/session/runner/llm.ts`, `packages/opencode/src/session/prompt.ts` |
| Subagent Spawning | `packages/opencode/src/tool/task.ts`, `packages/opencode/src/agent/subagent-permissions.ts` |
| Background Jobs | `packages/core/src/background-job.ts`, `packages/opencode/src/background/job.ts` |
| Process/PTY | `packages/core/src/process.ts`, `packages/core/src/pty.ts`, `packages/opencode/src/tool/shell.ts` |
| Session Persistence | `packages/core/src/session.ts`, `packages/opencode/src/session/session.ts` |
| Session Runner | `packages/core/src/session/runner/llm.ts`, `packages/opencode/src/session/processor.ts` |
| Agent Config | `packages/opencode/src/agent/agent.ts`, `packages/core/src/agent.ts` |
| Shell Detection | `packages/core/src/shell.ts` |
| Run Coordination | `packages/opencode/src/session/run-state.ts`, `packages/opencode/src/effect/runner.ts` |
| Event Bus | `packages/core/src/event.ts` |

---

*Analysis based on opencode codebase at `/tmp/opencode/refs/opencode` as of August 2026.*
