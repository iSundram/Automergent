# Comprehensive Analysis: Claude Code Commands System

## Overview
The Claude Code commands system is a sophisticated, multi-layered architecture supporting built-in commands, user-defined skills, plugin commands, MCP (Model Context Protocol) skills, and workflow scripts. The system integrates deeply with the REPL main loop, supports immediate commands that bypass the queue, and provides extensive command discovery, validation, and execution mechanisms.

---

## 1. Command Registration and Discovery

### 1.1 Core Command Registry (`src/commands.ts`)

**Main Registration Point**: The `COMMANDS` memoized function (lines 301-437) declares all built-in commands as a static array. Commands are imported at module load time (lines 2-193) and registered conditionally based on feature flags.

```typescript
// Built-in commands are imported statically (lines 2-193)
import addDir from './commands/add-dir/index.js'
import autofixPr from './commands/autofix-pr/index.js'
...

// Feature-flagged commands use dynamic require() for dead code elimination (lines 67-171)
const proactive = feature('PROACTIVE') || feature('KAIROS')
  ? require('./commands/proactive.js').default
  : null

// COMMANDS array combines static + conditional commands (lines 301-437)
const COMMANDS = memoize((): Command[] => [
  addDir,
  advisor,
  ...
  ...(proactive ? [proactive] : []),
  ...
])
```

**Command Sources** (lines 540-559 in `loadAllCommands`):
1. **Bundled Skills** - Pre-packaged skills shipped with the CLI
2. **Built-in Plugin Skills** - From enabled built-in plugins
3. **Skill Directory Commands** - User/project `.claude/skills/` directories
4. **Workflow Commands** - Named workflow scripts
5. **Plugin Commands** - From installed plugins
6. **Plugin Skills** - Skills exposed by plugins
7. **Built-in Commands** - The static `COMMANDS()` array

### 1.2 Skill Directory Loading (`src/skills/loadSkillsDir.ts`)

**Loading Strategy** (lines 638-804):
- **Managed** (policy): Enterprise-managed skills (`/managed/.claude/skills/`)
- **User**: User-level skills (`~/.claude/skills/`)
- **Project**: Project-level skills (`.claude/skills/`, walks up to home)
- **Additional**: From `--add-dir` flag
- **Legacy**: Deprecated `/commands/` directory format

**Directory Format**: Only supports `skill-name/SKILL.md` (not single `.md` files)

**Frontmatter Parsing** (lines 185-265): Shared parsing for file-based and MCP skills extracts:
- `name`, `description`, `allowed-tools`, `argument-hint`, `arguments`, `when_to_use`, `version`, `model`, `effort`, `user-invocable`, `hooks`, `context` (fork/inline), `agent`, `paths`, `shell`

**Conditional Skills** (lines 771-796): Skills with `paths` frontmatter are stored separately and activated only when matching files are touched (gitignore-style matching via `ignore` library).

**Dynamic Discovery** (lines 861-915): Walks up from file paths to cwd to discover nested `.claude/skills/` directories during file operations.

### 1.3 Plugin Commands (`src/utils/plugins/loadPluginCommands.ts`)

Plugin commands are loaded via `getPluginCommands()` and `getPluginSkills()` (imported at line 219, 221). They are registered with `source: 'plugin'` and `loadedFrom: 'plugin'`.

### 1.4 MCP Skills

Loaded via MCP client connections. Skills from MCP servers have `loadedFrom: 'mcp'` and are filtered by `getMcpSkillCommands()` (lines 638-650) for model invocation.

---

## 2. Command Execution Flow

### 2.1 Entry Points

1. **User Input** → `REPL.tsx` `onSubmit` (line 3992) → `handlePromptSubmit` (src/utils/handlePromptSubmit.ts)
2. **Queued Commands** → `executeQueuedInput` in `useQueueProcessor` hook
3. **Keybinding-Triggered** → `onSubmit` with `fromKeybinding: true`

### 2.2 Processing Pipeline (`src/utils/handlePromptSubmit.ts`)

```
handlePromptSubmit(params)
├── expandPastedTextRefs() - resolve [Pasted text #N] references
├── Immediate Command Check (lines 237-321)
│   └── If `immediate: true` AND (queryGuard.isActive OR isExternalLoading)
│       └── Execute local-jsx command directly via setToolJSX
├── Queue if busy (lines 323-362)
│   └── enqueue() command for later processing
└── Execute directly (lines 364-398)
    └── processUserInput() → processUserInputBase()
```

### 2.3 Command Processing (`src/utils/processUserInput/processUserInput.ts`)

```
processUserInputBase()
├── Handle images (resize, store, metadata)
├── Bridge-safe slash command override (lines 433-467)
├── Ultraplan keyword detection (lines 481-508)
├── Extract attachments (if not slash command)
├── Mode dispatch:
│   ├── 'bash' → processBashCommand()
│   ├── slash command (/) → processSlashCommand()
│   └── default → processTextPrompt()
```

### 2.4 Slash Command Processing (`src/utils/processUserInput/processSlashCommand.tsx`)

```
processSlashCommand()
├── parseSlashCommand() - extracts commandName, args, isMcp
├── hasCommand() validation (line 464)
├── getMessagesForSlashCommand()
│   ├── getCommand() - throws if not found (src/commands.ts:804-819)
│   ├── userInvocable check (lines 710-728)
│   └── Switch on command.type:
│       ├── 'local-jsx' → Promise-based UI rendering (lines 732-856)
│       ├── 'local' → Direct async execution (lines 857-937)
│       └── 'prompt' → getMessagesForPromptSlashCommand() (lines 1084-1212)
│           ├── Coordinator mode shortcut (lines 1101-1127)
│           ├── getPromptForCommand() - generates prompt content
│           ├── Register skill hooks (lines 1136-1145)
│           ├── Record skill invocation (lines 1147-1155)
│           ├── Build messages with metadata, content, attachments, permissions
│           └── Return { messages, shouldQuery: true, allowedTools, model, effort }
```

### 2.5 Forked Sub-Agent Execution (`executeForkedSlashCommand`, lines 108-412)

For commands with `context: 'fork'`:
1. Prepare forked context via `prepareForkedCommandContext()`
2. Optionally run in background (assistant mode, lines 159-293)
3. Run sub-agent via `runAgent()` with dedicated tools
4. Stream progress via `setToolJSX()` with agent progress UI
5. Extract result text and return as user message with `<local-command-stdout>`

---

## 3. Built-in Commands vs Custom Commands

### 3.1 Built-in Commands (`src/commands.ts`)

**Characteristics**:
- Defined in `src/commands/` directory
- Registered in `COMMANDS()` array (lines 301-437)
- `source: 'builtin'`, `loadedFrom: undefined`
- Types: `local`, `local-jsx`, `prompt`
- Cannot be disabled by users (no `isEnabled()` by default)

**Command Types**:
| Type | Interface | Use Case |
|------|-----------|----------|
| `local` | `load() → { call(args, context) }` | Simple sync/async operations (clear, diff) |
| `local-jsx` | `load() → { call(onDone, context, args) }` | Interactive UI (config, theme, plugins) |
| `prompt` | `getPromptForCommand(args, context)` | Skill-like, expands to model prompt (commit, review) |

**Examples**:
- `/clear` (local): Clears conversation history
- `/config` (local-jsx): Opens config UI
- `/commit` (prompt): Generates git commit prompt with context

### 3.2 Custom Commands (Skills)

**Sources**:
- **User Skills**: `~/.claude/skills/name/SKILL.md`
- **Project Skills**: `.claude/skills/name/SKILL.md`
- **Managed Skills**: Enterprise policy
- **Legacy Commands**: `.claude/commands/` (deprecated)
- **Bundled Skills**: Shipped with CLI
- **Plugin Skills**: From installed plugins
- **MCP Skills**: From MCP servers

**Format**: Markdown file with YAML frontmatter
```yaml
---
description: "Command description"
allowed-tools: ["Bash", "Read"]
argument-hint: "<file>"
arguments: ["file"]
when_to_use: "When to use this skill"
model: "sonnet"
effort: "high"
context: "fork"
user-invocable: true
disable-model-invocation: false
paths: ["src/**/*.ts"]
hooks:
  PreToolUse: [...]
---
# Skill content (markdown)
```

**Key Differences from Built-ins**:
- `source`: 'userSettings' | 'projectSettings' | 'policySettings' | 'plugin' | 'bundled' | 'mcp'
- `loadedFrom`: 'skills' | 'commands_DEPRECATED' | 'plugin' | 'bundled' | 'mcp' | 'managed'
- Conditional activation via `paths` frontmatter
- Can be disabled via `user-invocable: false` (model-only)
- Token estimation via `contentLength` from markdown length

### 3.3 Workflow Commands

Loaded via `getWorkflowCommands()` (line 492-497) when `WORKFLOW_SCRIPTS` feature is enabled. Have `kind: 'workflow'` for badging in autocomplete.

---

## 4. Command Aliases and Shortcuts

### 4.1 Alias Definition

Aliases defined in `CommandBase.aliases?: string[]` (src/types/command.ts:198)

**Built-in Examples** (from commands.ts):
- `/clear`: `aliases: ['reset', 'new']` (src/commands/clear/index.ts:14)
- `/cost` and `/stats` → aliases of `/usage` (lines 18, 246)

### 4.2 Alias Resolution

**Finding Commands** (src/commands.ts:788-798):
```typescript
export function findCommand(commandName: string, commands: Command[]): Command | undefined {
  return commands.find(
    _ =>
      _.name === commandName ||
      getCommandName(_) === commandName ||
      _.aliases?.includes(commandName),
  )
}
```

**Suggestions with Aliases** (src/utils/suggestions/commandSuggestions.ts:250-259):
```typescript
function findMatchedAlias(query: string, aliases?: string[]): string | undefined {
  return aliases?.find(alias => alias.toLowerCase().startsWith(query))
}
```
Only shows alias in parentheses if user typed the alias prefix.

### 4.3 Special Shortcuts

- **Exit words**: `exit`, `quit`, `:q`, `:q!`, `:wq`, `:wq!` → triggers `/exit` (handlePromptSubmit.ts:204-221)
- **Ultraplan keyword**: Triggers `/ultraplan` when detected in prompt (processUserInput.ts:481-508)
- **Keybinding commands**: Any command can be bound via keybindings system

---

## 5. Integration with REPL/Main Loop

### 5.1 Command Loading in REPL (`src/screens/REPL.tsx`)

```typescript
// Lines 943-947: Local state for hot-reloadable commands
const [localCommands, setLocalCommands] = useState(initialCommands)

// Line 947: Watch for skill file changes
useSkillsChange(isRemoteSession ? undefined : getProjectRoot(), setLocalCommands)

// Lines 1096-1100: Merge command sources
const commandsWithPlugins = useMergedCommands(localCommands, plugins.commands)
const mergedCommands = useMergedCommands(commandsWithPlugins, mcp.commands)
const commands = useMemo(() => (disableSlashCommands ? [] : mergedCommands), [...])
```

### 5.2 Command Execution in REPL

**Immediate Commands** (lines 4011-4151): Bypass queue when `queryGuard.isActive` AND (`command.immediate` OR `fromKeybinding`). Only for `local-jsx` type.

**Remote Mode Filtering** (lines 4296-4359): In remote mode (`activeRemote.isRemoteMode`), only `local-jsx` slash commands execute locally; others sent to remote.

**Submission Handler** (lines 4364-393): Calls `handlePromptSubmit` with full command list.

### 5.3 Queue Processing (`src/hooks/useQueueProcessor.ts`)

- Commands queued when REPL is busy (`queryGuard.isActive`)
- Processed sequentially via `executeQueuedInput`
- Each queued item goes through `handlePromptSubmit` with `queuedCommands` param

### 5.4 Command Queue (`src/utils/messageQueueManager.ts`)

- `enqueue()` adds commands to queue
- `claimConsumableQueuedAutonomyCommands()` handles autonomy commands
- Queue processed by `useQueueProcessor` hook

---

## 6. Slash Commands vs Regular Commands

### 6.1 Slash Commands (User-Invocable)

**Definition**: Any command starting with `/` that matches a registered command.

**Processing**:
- Parsed by `parseSlashCommand()` (src/utils/slashCommandParsing.ts)
- Dispatched via `processSlashCommand()` 
- User-invocable skills have `userInvocable !== false` (default true)

**User-Invocable Check** (processSlashCommand.tsx:710-728):
```typescript
if (command.userInvocable === false) {
  return {
    messages: [..., "This skill can only be invoked by Claude..."],
    shouldQuery: false,
  }
}
```

### 6.2 Regular Commands (Non-Slash)

- **Bash mode**: Input starting with `!` or when in bash mode
- **Prompt mode**: Plain text input
- **Task notifications**: System-generated

### 6.3 Model-Invocable Skills

**SkillTool Filtering** (commands.ts:654-671):
```typescript
export const getSkillToolCommands = memoize(async (cwd) => {
  const allCommands = await getCommands(cwd)
  return allCommands.filter(cmd =>
    cmd.type === 'prompt' &&
    !cmd.disableModelInvocation &&
    cmd.source !== 'builtin' &&
    (cmd.loadedFrom === 'bundled' || cmd.loadedFrom === 'skills' || 
     cmd.loadedFrom === 'commands_DEPRECATED' || cmd.hasUserSpecifiedDescription || cmd.whenToUse)
  )
})
```

**SlashCommandTool Skills** (commands.ts:677-699): Similar but stricter - requires `hasUserSpecifiedDescription || whenToUse` AND specific `loadedFrom` values.

---

## 7. Command Permissions and Validation

### 7.1 Availability Requirements (`src/commands.ts:508-534`)

Commands can declare `availability?: CommandAvailability[]` (src/types/command.ts:171-175):
- `'claude-ai'`: claude.ai OAuth subscribers
- `'console'`: Direct API key users (api.anthropic.com)

```typescript
export function meetsAvailabilityRequirement(cmd: Command): boolean {
  if (!cmd.availability || cmd.availability.length === 0) return true
  for (const a of cmd.availability) {
    switch (a) {
      case 'claude-ai': if (isClaudeAISubscriber()) return true; break
      case 'console': if (!isClaudeAISubscriber() && !isThirdPartyAPIProvider() && isFirstPartyAnthropicBaseUrl()) return true; break
    }
  }
  return false
}
```

### 7.2 Enable/Disable Logic

**Command-Level** (src/types/command.ts:228-231):
```typescript
isEnabled?: () => boolean  // Defaults to true
```

**Feature Flags**: Many commands conditional on `feature()` (e.g., `PROACTIVE`, `KAIROS`, `BRIDGE_MODE`)

**Enterprise Policy**: 
- `isRestrictedToPluginOnly('skills')` blocks user/project skills
- `isRestrictedToPluginOnly('hooks')` controls skill hook registration

### 7.3 Bridge/Remote Safety (`src/commands.ts:709-786`)

**Remote Safe Commands** (lines 710-728): Explicit allowlist for `--remote` mode:
```typescript
export const REMOTE_SAFE_COMMANDS: Set<Command> = new Set([
  session, exit, clear, help, theme, color, vim, usage, copy, btw,
  feedback, plan, proactive, keybindings, statusline, stickers, mobile
])
```

**Bridge Safe Commands** (lines 742-751): For Remote Control bridge:
```typescript
export const BRIDGE_SAFE_COMMANDS: Set<Command> = new Set([
  compact, clear, usage, summary, releaseNotes, files
].filter(Boolean))
```

**Safety Check** (lines 763-776):
```typescript
export function isBridgeSafeCommand(cmd: Command): boolean {
  if (cmd.type === 'local-jsx') return cmd.bridgeSafe === true
  if (cmd.type === 'prompt') return true
  return cmd.bridgeSafe === true || BRIDGE_SAFE_COMMANDS.has(cmd)
}

export function getBridgeCommandSafety(cmd, args): {ok: true} | {ok: false, reason?: string} {
  if (!isBridgeSafeCommand(cmd)) return { ok: false }
  const reason = cmd.getBridgeInvocationError?.(args)
  return reason ? { ok: false, reason } : { ok: true }
}
```

### 7.4 Validation

**Command Existence**: `hasCommand()` / `getCommand()` throw if not found (src/commands.ts:800-819)

**Argument Validation**: Handled per-command in `getPromptForCommand()` or `call()`

**Sensitive Args**: `isSensitive?: boolean` redacts args from history (src/types/command.ts:214)

**Hooks**: `UserPromptSubmit` hooks can block commands (processUserInput.ts:186-232)

---

## 8. Output Formatting for Commands

### 8.1 Message Types

Commands produce messages added to conversation:

```typescript
// Local command text output
createCommandInputMessage(`<local-command-stdout>${result}</local-command-stdout>`)

// Local command error
createCommandInputMessage(`<local-command-stderr>${error}</local-command-stderr>`)

// Compact result
{ type: 'compact', compactionResult, displayText }

// Skip (no output)
{ type: 'skip' }

// Prompt command expansion
createUserMessage({ content: skillContent, isMeta: true })
createAttachmentMessage({ type: 'command_permissions', allowedTools, model })
```

### 8.2 Display Modes (`CommandResultDisplay`)

```typescript
export type CommandResultDisplay = 'skip' | 'system' | 'user'
```

- `'skip'`: No transcript entry (used for immediate commands in fullscreen)
- `'system'`: System-style message (yellow, not sent to model)
- `'user'`: User bubble (default for local-jsx)

**Local-jsx Decision** (processSlashCommand.tsx:774-809):
```typescript
const skipTranscript = isFullscreenEnvEnabled() && typeof result === 'string' && result.endsWith(' dismissed')
```

### 8.3 Notification System

Immediate commands use `addNotification()` for transient feedback:
```typescript
addNotification({
  key: `immediate-${command.name}`,
  text: result,
  priority: 'immediate',
})
```

### 8.4 Command Input Formatting

**Slash Command Echo** (src/utils/messages.ts:243-245):
```typescript
export function formatCommandInputTags(commandName: string, args: string): string {
  return `<${COMMAND_MESSAGE_TAG}>${commandName}</${COMMAND_MESSAGE_TAG}>\n<${COMMAND_NAME_TAG}>/${commandName}</${COMMAND_NAME_TAG}>\n${args ? `<command-args>${args}</command-args>` : ''}`
}
```

### 8.5 Skill Loading Metadata

```typescript
function formatSkillLoadingMetadata(skillName: string, progressMessage: string): string {
  return [
    `<${COMMAND_MESSAGE_TAG}>${skillName}</${COMMAND_MESSAGE_TAG}>`,
    `<${COMMAND_NAME_TAG}>${skillName}</${COMMAND_NAME_TAG}>`,
    `<skill-format>true</skill-format>`
  ].join('\n')
}
```

---

## 9. Key Architectural Patterns

### 9.1 Lazy Loading
- Commands use `load(): Promise<Module>` for code splitting
- Heavy modules (insights, config, plugins) loaded on first invocation
- Feature flags (`feature()`) enable dead code elimination

### 9.2 Memoization
- `getCommands(cwd)` memoized per cwd (commands.ts:540)
- `getSkillToolCommands` / `getSlashCommandToolSkills` memoized
- Fuse index for suggestions cached by commands array identity

### 9.3 Command Context
All commands receive `ToolUseContext & LocalJSXCommandContext` with:
- `getAppState()`, `setMessages()`, `setToolJSX()`
- `canUseTool`, `options` (theme, mcp, ide)
- `onChangeAPIKey`, `onInstallIDEExtension`, `resume`

### 9.4 Autonomy Integration
- Commands can have `autonomy` metadata for scheduled tasks
- `claimConsumableQueuedAutonomyCommands()` / `finalizeAutonomyCommandsForTurn()`
- Forked commands can run in background (assistant mode)

---

## 10. Summary of Key Files

| File | Purpose |
|------|---------|
| `src/commands.ts` | Main registry, command loading, availability, bridge safety |
| `src/types/command.ts` | Type definitions (Command, PromptCommand, LocalCommand, LocalJSXCommand) |
| `src/commands/*.ts` | Individual built-in command definitions |
| `src/utils/handlePromptSubmit.ts` | Entry point for command execution |
| `src/utils/processUserInput/processUserInput.ts` | Input dispatch (bash/slash/prompt) |
| `src/utils/processUserInput/processSlashCommand.tsx` | Slash command execution logic |
| `src/skills/loadSkillsDir.ts` | Skill discovery, loading, dynamic/conditional skills |
| `src/utils/suggestions/commandSuggestions.ts` | Autocomplete, fuzzy search, alias handling |
| `src/screens/REPL.tsx` | REPL integration, command merging, immediate commands |
| `src/hooks/useQueueProcessor.ts` | Background command queue processing |

---

## 11. Extension Points

1. **Custom Skills**: Add `.claude/skills/name/SKILL.md` files
2. **Plugins**: Export commands via plugin manifest
3. **MCP Servers**: Expose skills via MCP protocol
4. **Workflows**: Named workflow scripts (feature-gated)
5. **Built-in**: Add to `src/commands/` and register in `commands.ts`
6. **Hooks**: `UserPromptSubmit` hooks for validation/blocking
7. **Keybindings**: Bind any command to keyboard shortcuts
