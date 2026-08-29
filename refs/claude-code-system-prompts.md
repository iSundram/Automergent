# Comprehensive Analysis: claude-code System Prompts

This document provides a thorough analysis of the claude-code system prompt architecture, based on code exploration of `/tmp/opencode/claude-code`.

---

## 1. System Prompt Construction

### Core Entry Point: `getSystemPrompt()` 
**File:** `src/constants/prompts.ts:423` (exported async function)

The primary system prompt factory function that assembles the complete system prompt as a `string[]` (branded as `SystemPrompt` type from `packages/@ant/model-provider/src/types/systemPrompt.ts`).

### Structure: Static vs Dynamic Sections
The prompt is divided by a boundary marker `SYSTEM_PROMPT_DYNAMIC_BOUNDARY` (`src/constants/prompts.ts:114-115`):

| Phase | Content | Cache Strategy |
|-------|---------|----------------|
| **Static (before boundary)** | Intro, System Rules, Doing Tasks, Actions, Using Tools, Communication Style | Cross-org cacheable (`scope: 'global'`) |
| **BOUNDARY** | `__SYSTEM_PROMPT_DYNAMIC_BOUNDARY__` | Removed before API call, never seen by model |
| **Dynamic (after boundary)** | Session Guidance, Memory, Model Override, Env Info, Language, Output Style, MCP Instructions, Scratchpad, Summarize Tool Results, Token Budget, Brief | Per-session (`scope: 'org'` or no cache) |

### Static Sections (always included, cacheable)
1. **`getSimpleIntroSection()`** (line 175) - Identity framing
2. **`getSimpleSystemSection()`** (line 186) - System rules, tool behavior, hooks
3. **`getSimpleDoingTasksSection()`** (line 201) - Software engineering task guidance
4. **`getActionsSection()`** (line 245) - Reversibility/risk guidance
5. **`getUsingYourToolsSection()`** (line 259) - Tool usage guidance (Bash/PowerShell, search)
6. **`getOutputEfficiencySection()`** (line 391) - Communication style (prose, no emojis)

### Dynamic Sections (registered via Section Registry)
**File:** `src/constants/prompts.ts:469-522`

Each dynamic section is registered using:
- `systemPromptSection(name, computeFn)` - cached, computed once per session
- `DANGEROUS_uncachedSystemPromptSection(name, computeFn, reason)` - recomputed every turn, breaks cache

Sections include:
- `mode_persona` - Custom mode system prompt
- `session_guidance` - Session-specific tool/agent guidance  
- `memory` - Auto-memory (MEMORY.md) content
- `ant_model_override` - Ant-internal model config
- `env_info_simple` - Environment info (CWD, platform, model)
- `language` - User language preference
- `output_style` - Output style configuration
- `mcp_instructions` - MCP server instructions (uncached, reconnects change it)
- `scratchpad` - Scratchpad directory instructions
- `summarize_tool_results` - Tool result summarization reminder
- `token_budget` - Token budget guidance (feature-gated)
- `brief` - Brief mode instructions (feature-gated)

---

## 2. How System Prompts Are Loaded/Configured

### Loading Flow
```
getSystemPrompt(tools, model, additionalDirs, mcpClients)
    ↓
buildEffectiveSystemPrompt() [src/utils/systemPrompt.ts:41]
    ↓
Priority selection:
  0. Override (loop mode) → replaces ALL
  1. Coordinator mode → coordinator prompt
  2. Agent (mainThreadAgentDefinition) → proactive: append, else: replace
  3. Custom (--system-prompt) → replaces default
  4. Default → getSystemPrompt() full output
    ↓
appendSystemPrompt always added at end (except Override)
```

### Configuration Sources
1. **Environment Variables:**
   - `CLAUDE_CODE_SIMPLE` - Minimal prompt (line 429)
   - `CLAUDE_CODE_DISABLE_CLAUDE_MDS` - Disable CLAUDE.md loading
   - `CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS` - Disables global cache boundary
   - `USER_TYPE=ant` - Ant-internal features (model override, undercover mode)

2. **Settings (via `getInitialSettings()`):**
   - `language` - Language preference
   - `outputStyle` - Output style config
   - `modelOverrides` - Bedrock/Vertex model ID overrides

3. **CLI Arguments:**
   - `--system-prompt` - Custom system prompt (replaces default)
   - `--append-system-prompt` - Appends to effective prompt
   - `--model` - Model selection affects marketing name in env section
   - `--add-dir` - Additional CLAUDE.md directories

4. **Feature Flags (GrowthBook/bun:bundle):**
   - `PROACTIVE`, `KAIROS` - Autonomous mode
   - `TOKEN_BUDGET` - Token budget section
   - `VERIFICATION_AGENT` - Verification agent guidance
   - `TEAMMEM` - Team memory

---

## 3. Dynamic Prompt Injection

### System Context Injection
**File:** `src/context.ts:116` (`getSystemContext`)

Memoized per-session, provides:
- **Git Status** - Branch, main branch, status (truncated to 2000 chars), recent commits
- **Cache Breaker** (ant-only) - `systemPromptInjection` variable for manual cache busting

Injected at query time via `appendSystemContext()` in `src/query.ts:449`

### User Context Injection
**File:** `src/context.ts:155` (`getUserContext`)

Memoized per-session, provides:
- **CLAUDE.md content** - Merged from all hierarchy levels (Managed → User → Project → Local)
- **Current Date** - "Today's date is YYYY-MM-DD."

Injected as first user message via `prependUserContext()` in `src/utils/api.ts:449`, wrapped in `<system-reminder>` tags

### CLAUDE.md Hierarchy (Priority Order - Later Overrides Earlier)
**File:** `src/utils/claudemd.ts:1-26`

1. **Managed** - `/etc/claude-code/CLAUDE.md` (global policy)
2. **User** - `~/.claude/CLAUDE.md` + `~/.claude/rules/*.md` (personal)
3. **Project** - `CLAUDE.md`, `.claude/CLAUDE.md`, `.claude/rules/*.md` (walked from CWD to root)
4. **Local** - `CLAUDE.local.md` (private, gitignored)
5. **Additional Directories** - Via `--add-dir` flag
6. **AutoMem** - MEMORY.md entrypoint (feature-gated)
7. **TeamMem** - Shared team memory (feature-gated, ant-only)

**Key features:**
- `@include` directive support for file inclusion (max depth 5)
- Frontmatter `paths:` for conditional rules (glob matching)
- HTML comment stripping (`<!-- note -->`)
- Exclusion patterns via `claudeMdExcludes` setting
- Symlink-aware path resolution for excludes

---

## 4. Prompt Templates and Variables

### Template Functions in `prompts.ts`
Each section is a function returning `string | null`:

```typescript
// Environment info template (line 572)
export async function computeEnvInfo(modelId: string, additionalDirs?: string[]): Promise<string>

// Simple env info for dynamic section (line 617)
export async function computeSimpleEnvInfo(modelId: string, additionalDirs?: string[]): Promise<string>

// Knowledge cutoff per model (line 679)
function getKnowledgeCutoff(modelId: string): string | null

// MCP instructions template (line 545)
function getMcpInstructions(mcpClients: MCPServerConnection[]): string | null

// Scratchpad instructions (line 765)
export function getScratchpadInstructions(): string | null
```

### Variable Interpolation Points
| Variable | Source | Used In |
|----------|--------|---------|
| `{{CWD}}` | `getCwd()` | Env sections, intro |
| `{{DATE}}` | `getSessionStartDate()` | Env sections, intro |
| `{{MODEL_NAME}}` | `getMarketingNameForModel()` | Env sections |
| `{{MODEL_ID}}` | Resolved model ID | Env sections |
| `{{KNOWLEDGE_CUTOFF}}` | `getKnowledgeCutoff()` | Env sections |
| `{{SHELL}}` | `process.env.SHELL` | Env sections |
| `{{PLATFORM}}` | `env.platform` | Env sections |
| `{{OS_VERSION}}` | `getUnameSR()` | Env sections |
| `{{LANGUAGE}}` | Settings | Language section |
| `{{OUTPUT_STYLE}}` | Output style config | Output style section |

---

## 5. Model-Specific Prompt Variations

### Model Marketing Names
**File:** `src/utils/model/model.ts:672` (`getMarketingNameForModel`)

Maps canonical model IDs to human-readable names:
- `claude-opus-4-7` → "Opus 4.7" (cutoff: Jan 2026)
- `claude-opus-4-6` → "Opus 4.6" (cutoff: May 2025)
- `claude-sonnet-4-6` → "Sonnet 4.6" (cutoff: Aug 2025)
- `claude-haiku-4-5` → "Haiku 4.5" (cutoff: Feb 2025)
- Supports `[1m]` suffix for 1M context variants

### Knowledge Cutoffs
**File:** `src/constants/prompts.ts:679` (`getKnowledgeCutoff`)

Per-model knowledge cutoff dates embedded in env section:
- Opus 4.7: January 2026
- Opus 4.6: May 2025
- Sonnet 4.6: August 2025
- Haiku 4.5: February 2025
- 3.5/3.7 Sonnet: January 2025

### Model-Aware Sections
- **Env section** includes model name/ID and knowledge cutoff
- **Token budget** section only shown when feature enabled
- **Embedded search tools** detection affects tool guidance (Bash vs Glob/Grep)

### Provider-Specific Behavior
| Provider | Global Cache | Token Counting | Beta Headers |
|----------|-------------|----------------|--------------|
| First Party (Anthropic) | ✅ (`scope: 'global'`) | Exact API | Full |
| Bedrock | ❌ (org only) | Separate endpoint | Limited |
| Vertex | ❌ (org only) | Estimation | Limited |
| OpenAI/Gemini/Grok | ❌ (org only) | Estimation | None |

---

## 6. How User Instructions Merge with System Prompts

### Priority Hierarchy (Highest → Lowest)
**File:** `src/utils/systemPrompt.ts:29-39`

| Priority | Source | Behavior |
|----------|--------|----------|
| 0 | `overrideSystemPrompt` (loop mode) | **Complete replacement** - returns only `[override]` |
| 1 | Coordinator Mode | Uses coordinator-specific prompt |
| 2 | Main Thread Agent | Proactive: **appends** to default; Standard: **replaces** default |
| 3 | `--system-prompt` (customSystemPrompt) | **Replaces** default |
| 4 | Default | Full `getSystemPrompt()` output |
| — | `appendSystemPrompt` | **Always appended** (except when Override is set) |

### Agent System Prompts
**File:** `packages/builtin-tools/src/tools/AgentTool/loadAgentsDir.ts:136-159`

Two types:
- **Built-in Agents** (`source: 'built-in'`): `getSystemPrompt(params: { toolUseContext })` - dynamic
- **Custom Agents** (`source: 'userSettings'|'projectSettings'|...`): `getSystemPrompt()` - static closure over markdown content

Custom agents parse markdown frontmatter + body:
```markdown
---
name: "my-agent"
description: "When to use this agent"
tools: ["Read", "Write", "Bash"]
model: "sonnet"
---
# Agent Instructions
Your custom prompt here...
```
**File:** `packages/builtin-tools/src/tools/AgentTool/loadAgentsDir.ts:714` - body becomes `systemPrompt`

### CLAUDE.md as User Instructions
CLAUDE.md files are **not** part of the system prompt array. They're injected as:
- **System Context** (git status) → appended to system prompt
- **User Context** (CLAUDE.md) → prepended as first user message with `<system-reminder>` tag

This separation allows:
- System prompt to be cached independently
- CLAUDE.md to vary per project without busting system prompt cache

---

## 7. Prompt Versioning/Changes

### Version Tracking Mechanisms

1. **Attribution Header** (`src/constants/system.ts`)
   - `cc_version`: Version + fingerprint
   - `cc_entrypoint`: REPL/SDK/pipe
   - `cch`: Native client attestation (Bun-native)

2. **Boundary Marker Stability**
   - `SYSTEM_PROMPT_DYNAMIC_BOUNDARY` constant must not change without updating:
     - `src/utils/api.ts` (`splitSysPromptPrefix`)
     - `src/services/api/claude.ts` (`buildSystemPromptBlocks`)

3. **Section Registry Cache Keys**
   - Section names are cache keys (`systemPromptSection('name', fn)`)
   - Renaming a section = cache bust
   - Adding/removing sections = cache bust for that session

4. **Model Launch Updates**
   - `FRONTIER_MODEL_NAME` constant (line 118)
   - `CLAUDE_LATEST_MODEL_IDS` object (line 121)
   - `getKnowledgeCutoff()` mappings (line 679)
   - `getMarketingNameForModel()` mappings (line 672)

5. **Feature-Gated Sections**
   - Sections wrapped in `feature('FLAG')` checks
   - Feature flag changes = different prompt assembly
   - DCE (Dead Code Elimination) removes unused sections in external builds

### Change Detection & Cache Invalidation
- **System Prompt Sections**: Cached via `getSystemPromptSectionCache()` (memoized Map)
- **Cleared on**: `/clear`, `/compact`, `resetGetMemoryFilesCache()`
- **Session Context**: Memoized per-session (`lodash.memoize`)
- **Attribution Header**: Always `cacheScope: null` (busts on version change)

### Dump Script for Inspection
**File:** `scripts/dump-prompt.ts`

Generates full system prompt for manual review:
```bash
bun run scripts/dump-prompt.ts
# Output: scripts/system-prompt-dump.txt
```

---

## Key Files Reference

| File | Purpose |
|------|---------|
| `src/constants/prompts.ts` | Core prompt factory, all section functions |
| `src/constants/systemPromptSections.ts` | Section registry, caching logic |
| `src/utils/systemPrompt.ts` | Priority-based prompt selection |
| `src/utils/systemPromptType.ts` | `SystemPrompt` branded type |
| `src/context.ts` | System/User context injection |
| `src/utils/claudemd.ts` | CLAUDE.md loading, merging, @include |
| `src/utils/model/model.ts` | Model resolution, marketing names, cutoffs |
| `src/services/api/claude.ts` | `buildSystemPromptBlocks()`, `splitSysPromptPrefix()` |
| `src/utils/api.ts` | `splitSysPromptPrefix()`, cache control logic |
| `src/utils/betas.ts` | `shouldUseGlobalCacheScope()` |
| `packages/builtin-tools/src/tools/AgentTool/loadAgentsDir.ts` | Agent definition loading |
| `packages/builtin-tools/src/tools/AgentTool/built-in/*.ts` | Built-in agent prompts |
| `scripts/dump-prompt.ts` | Prompt dumping utility |

---

## Architecture Summary

The claude-code system prompt is a **cache-optimized, dynamically assembled string array** with:

1. **Two-tier caching**: Static prefix (cross-org) + Dynamic sections (per-session)
2. **Section registry**: Memoized dynamic sections with explicit cache-break annotations
3. **Five-level priority**: Override → Coordinator → Agent → Custom → Default
4. **Context separation**: System prompt ≠ CLAUDE.md (injected separately)
5. **Model awareness**: Marketing names, knowledge cutoffs, provider capabilities
6. **Feature-gated**: Sections conditionally included via build-time flags
7. **Version-tracked**: Attribution header + boundary marker for cache control

This design minimizes token costs via prompt caching while allowing rich per-session customization through dynamic sections and user-provided instructions (CLAUDE.md, agents, custom prompts).
