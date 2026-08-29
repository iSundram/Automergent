# OpenCode System Prompts Analysis

## Overview

OpenCode uses a sophisticated system prompt construction mechanism that combines multiple layers:
1. **Model-specific base prompts** (packages/opencode/src/session/prompt/*.txt)
2. **Dynamic environment context** (working directory, git status, platform, date)
3. **Project references** (additional directories with descriptions)
4. **User instructions** (AGENTS.md, CLAUDE.md, custom instruction files/URLs)
5. **Agent-specific prompts** (custom prompts per agent configuration)
6. **MCP server instructions** (from configured MCP servers)
7. **Skills** (available specialized skills with descriptions)
8. **Plan mode reminders** (when in read-only planning mode)
9. **Structured output prompts** (when JSON schema output is requested)

---

## 1. System Prompt Construction

### Core Components (packages/opencode/src/session/system.ts)

The `SystemPrompt` service constructs the system prompt from multiple sources:

```typescript
// Line 67-103: environment() - Builds environment context
const environment = [
  `You are powered by the model named ${model.api.id}. The exact model ID is ${model.providerID}/${model.api.id}`,
  `Here is some useful information about the environment you are running in:`,
  `<env>`,
  `  Working directory: ${ctx.directory}`,
  `  Workspace root folder: ${ctx.worktree}`,
  `  Is directory a git repo: ${ctx.project.vcs === "git" ? "yes" : "no"}`,
  `  Platform: ${process.platform}`,
  `  Today's date: ${new Date().toDateString()}`,
  `</env>`,
].join("\n")

// Plus project references if available
```

```typescript
// Line 105-117: skills() - Adds available skills
return [
  "Skills provide specialized instructions and workflows for specific tasks.",
  "Use the skill tool to load a skill when a task matches its description.",
  Skill.fmt(list, { verbose: true }),
].join("\n")
```

```typescript
// Line 119-135: mcp() - Adds MCP server instructions
return [
  "<mcp_instructions>",
  ...instructions.flatMap((item) => [
    `  <server name="${item.name}">`,
    ...item.instructions.split("\n").map((line) => `    ${line}`),
    "  </server>",
  ]),
  "</mcp_instructions>",
].join("\n")
```

### Assembly in Session Loop (packages/opencode/src/session/prompt.ts:1257-1269)

```typescript
const [skills, env, instructions, mcpInstructions, modelMsgs] = yield* Effect.all([
  sys.skills(agent),                    // Skills list
  sys.environment(model),               // Environment context
  instruction.system(),                 // User instructions (AGENTS.md, etc.)
  sys.mcp(agent, session.permission),   // MCP instructions
  MessageV2.toModelMessagesEffect(msgs, model),
])

const system = [
  ...env,
  ...instructions,
  ...(mcpInstructions ? [mcpInstructions] : []),
  ...(skills ? [skills] : []),
]
```

---

## 2. Model-Specific Base Prompts

Located in `packages/opencode/src/session/prompt/*.txt`. Selected via `SystemPrompt.provider()`:

| Model Pattern | Prompt File | Description |
|---------------|-------------|-------------|
| `muse-glimmer` / `muse-spark` | `meta.txt` | Meta's Muse models with `{{MODEL_NAME}}` template |
| `gpt-4`, `o1`, `o3` | `beast.txt` | Extended reasoning/autonomous prompt |
| `gpt-*` (incl. codex) | `gpt.txt` or `codex.txt` | OpenAI models |
| `gemini-*` | `gemini.txt` | Google Gemini models |
| `claude*` | `anthropic.txt` | Anthropic Claude models |
| `trinity*` | `trinity.txt` | Trinity model |
| `kimi*` / `moonshotai` | `kimi.txt` | Kimi/Moonshot models |
| `copilot-gpt-5` | `copilot-gpt-5.txt` | GitHub Copilot GPT-5 |
| Default | `default.txt` | Fallback for unknown models |

### Key Differences Between Prompts

**default.txt** (95 lines): Concise, minimal, emphasizes brevity (<4 lines), no comments unless asked

**anthropic.txt** (105 lines): Emphasizes TodoWrite for task management, Task tool for subagents, professional objectivity

**gpt.txt** (107 lines): Deeply pragmatic, editing constraints (ASCII, apply_patch), autonomy/persistence, frontend guidelines

**gemini.txt** (155 lines): Structured workflows (Understand→Plan→Implement→Verify), path construction rules, security rules

**beast.txt** (147 lines): Autonomous agent emphasis, extensive internet research requirement, detailed workflow steps

**codex.txt** (79 lines): Editing constraints, tool usage preferences, git hygiene, frontend design, final answer formatting

**trinity.txt** (97 lines): Similar to default but with single-tool-per-message constraint

**kimi.txt** (95 lines): General AI agent guidelines, AGENTS.md emphasis, working environment cautions

**copilot-gpt-5.txt** (143 lines): Structured workflow, todo tracking, code search instructions, output formatting with links

**meta.txt** (65 lines): Meta-specific, verification emphasis, preciseness, tool use guidelines

---

## 3. How System Prompts Are Loaded/Configured

### Agent Configuration (packages/opencode/src/agent/agent.ts:35-56)

Agents can have custom prompts via configuration:

```typescript
export const Info = Schema.Struct({
  name: Schema.String,
  description: Schema.optional(Schema.String),
  mode: Schema.Literals(["subagent", "primary", "all"]),
  // ...
  prompt: Schema.optional(Schema.String),  // Custom system prompt
  // ...
})
```

### Default Agents with Custom Prompts (agent.ts:196-264)

| Agent | Prompt Source | Purpose |
|-------|---------------|---------|
| `explore` | `PROMPT_EXPLORE` (explore.txt) | File search specialist |
| `compaction` | `PROMPT_COMPACTION` (compaction.txt) | Context summarization |
| `title` | `PROMPT_TITLE` (title.txt) | Title generation |
| `summary` | `PROMPT_SUMMARY` (summary.txt) | Conversation summarization |

### Configuration Override (agent.ts:267-294)

```typescript
for (const [key, value] of Object.entries(cfg.agent ?? {})) {
  // ...
  item.prompt = value.prompt ?? item.prompt  // User config overrides default
  // ...
}
```

---

## 4. Dynamic Prompt Injection

### User Instructions (packages/opencode/src/session/instruction.ts)

**File Discovery** (Line 64-68):
```typescript
const instructionFiles = [
  "AGENTS.md",
  ...(!flags.disableClaudeCodePrompt ? ["CLAUDE.md"] : []),
  "CONTEXT.md", // deprecated
]
```

**Global Files** (Line 60-63):
```typescript
const globalFiles = [
  path.join(global.config, "AGENTS.md"),
  ...(!flags.disableClaudeCodePrompt ? [path.join(global.home, ".claude", "CLAUDE.md")] : []),
]
```

**System Instructions Loading** (Line 155-169):
```typescript
const system = Effect.fn("Instruction.system")(function* () {
  const config = yield* cfg.get()
  const paths = yield* systemPaths()
  const urls = (config.instructions ?? []).filter(
    (item) => item.startsWith("https://") || item.startsWith("http://"),
  )

  const files = yield* Effect.forEach(Array.from(paths), read, { concurrency: 8 })
  const remote = yield* Effect.forEach(urls, fetch, { concurrency: 4 })

  return [
    ...Array.from(paths).flatMap((item, i) => (files[i] ? [`Instructions from: ${item}\n${files[i]}`] : [])),
    ...urls.flatMap((item, i) => (remote[i] ? [`Instructions from: ${item}\n${remote[i]}`] : [])),
  ]
})
```

**Context-Aware Instruction Resolution** (Line 179-221):
When reading files, nearby instruction files (AGENTS.md, CLAUDE.md) are automatically attached once per message by walking up the directory tree.

### Plan Mode Injection (packages/opencode/src/session/reminders.ts)

```typescript
// Line 26-47: Non-experimental plan mode
if (!flags.experimentalPlanMode) {
  if (input.agent.name === "plan") {
    // Inject PLAN_MODE reminder (plan.txt)
  }
  // Switch from plan to build
  if (wasPlan && input.agent.name === "build") {
    // Inject BUILD_SWITCH reminder (build-switch.txt)
  }
}

// Line 50-89: Experimental plan mode
if (input.agent.name === "plan") {
  // Inject PLAN_MODE with dynamic plan file path (plan-mode.txt)
}
```

---

## 5. Prompt Templates and Variables

### Template Variables

| Variable | Source | Usage |
|----------|--------|-------|
| `{{MODEL_NAME}}` | `system.ts:30` | Meta prompt template replacement |
| `${planInfo}` | `reminders.ts:81-84` | Plan mode dynamic path |
| `${ctx.directory}` | `system.ts:77` | Working directory |
| `${ctx.worktree}` | `system.ts:78` | Workspace root |
| `${ctx.project.vcs}` | `system.ts:79` | Git repo status |
| `${process.platform}` | `system.ts:80` | OS platform |
| `${new Date().toDateString()}` | `system.ts:81` | Current date |

### Template Resolution (packages/opencode/src/session/prompt.ts:157-191)

```typescript
const resolvePromptParts = Effect.fn("SessionPrompt.resolvePromptParts")(function* (template: string) {
  const parts: PromptInput["parts"] = [{ type: "text", text: template }]
  const files = ConfigMarkdown.files(template)  // Extracts @file references
  // Resolves files, agents, MCP resources into parts
})
```

### Command Template Processing (packages/opencode/src/session/prompt.ts:1372-1410)

```typescript
// Placeholder replacement: $1, $2, ... $ARGUMENTS
const withArgs = templateCommand.replaceAll(placeholderRegex, (_, index) => {
  const position = Number(index)
  const argIndex = position - 1
  if (argIndex >= args.length) return ""
  if (position === last) return args.slice(argIndex).join(" ")
  return args[argIndex]
})
```

---

## 6. How User Instructions Merge with System Prompts

### Merge Order (packages/opencode/src/session/llm/request.ts:58-66)

```typescript
const system = [
  [
    ...(input.agent.prompt ? [input.agent.prompt] : SystemPrompt.provider(input.model)),  // 1. Agent prompt OR model-specific
    ...input.system,                                                                       // 2. System prompt from SystemPrompt service (env, instructions, mcp, skills)
    ...(input.user.system ? [input.user.system] : []),                                     // 3. User message system field
  ]
    .filter((x) => x)
    .join("\n"),
]
```

### Plugin Transformation (request.ts:68-78)

```typescript
yield* input.plugin.trigger(
  "experimental.chat.system.transform",
  { sessionID: input.sessionID, model: input.model },
  { system },
)
// Allows plugins to modify the system prompt
```

### OpenAI OAuth Special Handling (request.ts:99, 101-112)

```typescript
if (isOpenaiOauth) options.instructions = system.join("\n")
// For OpenAI OAuth, system prompt goes to `instructions` parameter instead of messages

const messages =
  isOpenaiOauth || input.isWorkflow
    ? input.messages
    : [
        ...system.map((x): ModelMessage => ({ role: "system", content: x })),
        ...input.messages,
      ]
```

---

## 7. Prompt Versioning/Changes

### Configuration-Based Versioning

1. **Config Schema** (packages/core/src/v1/config/config.ts:124-126):
```typescript
instructions: Schema.optional(Schema.mutable(Schema.Array(Schema.String))).annotate({
  description: "Additional instruction files or patterns to include",
}),
```

2. **Agent Prompt Override** (agent.ts:52):
```typescript
prompt: Schema.optional(Schema.String),
```

3. **Config Merge Strategy** (config.ts:45-51):
```typescript
function mergeConfigConcatArrays(target: Info, source: Info): Info {
  const merged = mergeConfig(target, source)
  if (target.instructions && source.instructions) {
    merged.instructions = Array.from(new Set([...target.instructions, ...source.instructions]))
  }
  return merged
}
```

### Built-in Prompt Files

All base prompts are static files in `packages/opencode/src/session/prompt/` that are bundled at compile time. Changes require code modification and rebuild.

### Runtime Flags Affecting Prompts (packages/opencode/src/effect/runtime-flags.ts)

```typescript
// Line 47
experimentalPlanMode: enabledByExperimental("OPENCODE_EXPERIMENTAL_PLAN_MODE"),

// Affects which plan reminder is injected (plan.txt vs plan-mode.txt)
```

---

## 8. Code References Summary

| File | Line | Description |
|------|------|-------------|
| `packages/opencode/src/session/system.ts` | 27-49 | `provider()` - Model-specific prompt selection |
| `packages/opencode/src/session/system.ts` | 67-103 | `environment()` - Dynamic environment context |
| `packages/opencode/src/session/system.ts` | 105-117 | `skills()` - Available skills injection |
| `packages/opencode/src/session/system.ts` | 119-135 | `mcp()` - MCP server instructions |
| `packages/opencode/src/session/prompt.ts` | 1257-1269 | System prompt assembly in session loop |
| `packages/opencode/src/session/llm/request.ts` | 58-66 | System prompt merge order |
| `packages/opencode/src/session/llm/request.ts` | 68-78 | Plugin system prompt transformation |
| `packages/opencode/src/session/instruction.ts` | 64-68 | Instruction file patterns (AGENTS.md, CLAUDE.md) |
| `packages/opencode/src/session/instruction.ts` | 155-169 | Loading system instructions (files + URLs) |
| `packages/opencode/src/session/instruction.ts` | 179-221 | Context-aware instruction resolution |
| `packages/opencode/src/session/reminders.ts` | 13-89 | Plan mode reminders injection |
| `packages/opencode/src/agent/agent.ts` | 35-56 | Agent schema with optional prompt field |
| `packages/opencode/src/agent/agent.ts` | 196-264 | Default agents with custom prompts |
| `packages/opencode/src/agent/agent.ts` | 267-294 | Config override for agent prompts |
| `packages/opencode/src/session/prompt/*.txt` | - | Model-specific base prompt templates |

---

## 9. Prompt Flow Summary

```
User Message
     │
     ▼
┌─────────────────────────────────────┐
│ Session Loop (prompt.ts)            │
│   ├─ Get last user message          │
│   ├─ Get agent config               │
│   ├─ Get model                      │
└─────────────────────────────────────┘
     │
     ▼
┌─────────────────────────────────────┐
│ SystemPrompt Service (system.ts)    │
│   ├─ environment(model)             │  ← Working dir, git, platform, date
│   ├─ skills(agent)                  │  ← Available skills list
│   ├─ instruction.system()           │  ← AGENTS.md, CLAUDE.md, URLs
│   └─ mcp(agent, permission)         │  ← MCP server instructions
└─────────────────────────────────────┘
     │
     ▼
┌─────────────────────────────────────┐
│ LLMRequestPrep.prepare()            │
│   ├─ Agent prompt OR provider()     │  ← Model-specific base prompt
│   ├─ SystemPrompt service output    │  ← Environment, skills, instructions, MCP
│   ├─ User message system field      │  ← Per-message system override
│   └─ Plugin transform               │  ← experimental.chat.system.transform
└─────────────────────────────────────┘
     │
     ▼
┌─────────────────────────────────────┐
│ LLM.stream()                        │
│   ├─ Messages: system + history     │
│   ├─ Tools                          │
│   └─ Provider-specific options      │
└─────────────────────────────────────┘
```

---

## 10. Key Design Patterns

1. **Layered Composition**: System prompt = Base prompt + Environment + Instructions + MCP + Skills
2. **Model Adaptation**: Different base prompts per model family for optimal performance
3. **Configurable Overrides**: Users can override agent prompts via config
4. **Dynamic Injection**: Plan mode, instruction files resolved at runtime
5. **Plugin Extensibility**: `experimental.chat.system.transform` hook for modifications
6. **Context-Aware**: Instructions loaded based on file being read (directory walk-up)
7. **Deduplication**: Instruction files tracked per message to avoid repetition
8. **Template Variables**: `${var}` substitution in command templates and plan mode
