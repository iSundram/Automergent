# Claude Code Context Management: Comprehensive Analysis

This document provides a detailed analysis of the context management system in Claude Code, covering context window management, conversation history handling, token counting and budgeting, context compaction/summarization strategies, file context inclusion, context persistence, how context is passed to the model, and context priorities/truncation.

---

## 1. Context Window Management

### 1.1 Model Context Window Resolution (`src/utils/context.ts`)

The system determines the effective context window size for a given model through `getContextWindowForModel()` (lines 60-121):

```typescript
export function getContextWindowForModel(
  model: string,
  betas?: string[],
): number {
  // 1. Environment override (ant-only)
  if (process.env.USER_TYPE === 'ant' && process.env.CLAUDE_CODE_MAX_CONTEXT_TOKENS) {
    const override = parseInt(process.env.CLAUDE_CODE_MAX_CONTEXT_TOKENS, 10)
    if (!isNaN(override) && override > 0) return override
  }

  // 2. [1m] suffix — explicit client-side opt-in
  if (has1mContext(model)) return 1_000_000

  // 3. GPT-5.6 family context windows (OAuth/Codex ≈ 272k; API key ≈ 1.05M)
  const chatgptContextWindow = getChatGPTModelContextWindow(model)
  if (chatgptContextWindow !== undefined) {
    if (is1mContextDisabled() && chatgptContextWindow > MODEL_CONTEXT_WINDOW_DEFAULT) {
      return MODEL_CONTEXT_WINDOW_DEFAULT
    }
    return chatgptContextWindow
  }

  // 4. Model capability lookup
  const cap = getModelCapability(model)
  if (cap?.max_input_tokens && cap.max_input_tokens >= 100_000) {
    if (cap.max_input_tokens > MODEL_CONTEXT_WINDOW_DEFAULT && is1mContextDisabled()) {
      return MODEL_CONTEXT_WINDOW_DEFAULT
    }
    return cap.max_input_tokens
  }

  // 5. Beta header for 1M context
  if (betas?.includes(CONTEXT_1M_BETA_HEADER) && modelSupports1M(model)) {
    return 1_000_000
  }

  // 6. Sonnet 4.6 experiment treatment
  if (getSonnet1mExpTreatmentEnabled(model)) return 1_000_000

  // 7. Ant model resolution
  if (process.env.USER_TYPE === 'ant') {
    const antModel = resolveAntModel(model)
    if (antModel?.contextWindow) return antModel.contextWindow
  }

  // 8. Default: 200,000 tokens
  return MODEL_CONTEXT_WINDOW_DEFAULT
}
```

**Key constants:**
- `MODEL_CONTEXT_WINDOW_DEFAULT = 200_000` (line 14)
- `COMPACT_MAX_OUTPUT_TOKENS = 20_000` (line 17)

### 1.2 Effective Context Window Calculation (`src/services/compact/autoCompact.ts`)

The effective context window accounts for reserved output tokens:

```typescript
export function getEffectiveContextWindowSize(model: string): number {
  const reservedTokensForSummary = Math.min(
    getMaxOutputTokensForModel(model),
    MAX_OUTPUT_TOKENS_FOR_SUMMARY,  // 20,000
  )
  let contextWindow = getContextWindowForModel(model, getSdkBetas())

  // Allow environment override
  const autoCompactWindow = process.env.CLAUDE_CODE_AUTO_COMPACT_WINDOW
  if (autoCompactWindow) {
    const parsed = parseInt(autoCompactWindow, 10)
    if (!isNaN(parsed) && parsed > 0) {
      contextWindow = Math.min(contextWindow, parsed)
    }
  }

  return contextWindow - reservedTokensForSummary
}
```

---

## 2. Conversation History Handling

### 2.1 History Storage (`src/history.ts`)

History is stored in a JSONL file at `~/.claude/history.jsonl` (line 115). Each entry contains:
- `display`: User-visible text
- `pastedContents`: Inline content or hash references for large pastes
- `timestamp`: Unix timestamp
- `project`: Project root path
- `sessionId`: Current session ID

### 2.2 History Entry Structure

```typescript
type LogEntry = {
  display: string
  pastedContents: Record<number, StoredPastedContent>
  timestamp: number
  project: string
  sessionId?: string
}

type StoredPastedContent = {
  id: number
  type: 'text' | 'image'
  content?: string           // Inline for small pastes (≤ 1024 chars)
  contentHash?: string       // Hash reference for large pastes
  mediaType?: string
  filename?: string
}
```

### 2.3 Paste Store Integration

Large pasted content (>1024 chars) is stored externally via `pasteStore.ts`:
- Content is hashed and stored asynchronously (fire-and-forget)
- History entries store only the hash reference
- On retrieval, content is fetched from the paste store

### 2.4 History Retrieval (`src/history.ts`)

Two main retrieval methods:

1. **`getHistory()`** (lines 190-217): Current project history, current session first
   - Limited to `MAX_HISTORY_ITEMS = 100`
   - Deduplicates by display text
   - Current session entries yielded before other sessions

2. **`getTimestampedHistory()`** (lines 162-180): For Ctrl+R picker
   - Includes timestamps for sorting
   - Lazy resolution of pasted content via `resolve()` function

### 2.5 Write Path

```typescript
let pendingEntries: LogEntry[] = []
let isWriting = false
let currentFlushPromise: Promise<void> | null = null

export function addToHistory(command: HistoryEntry | string): void {
  // ...validation...
  void addToPromptHistory(command)  // Async, non-blocking
}

async function addToPromptHistory(command: HistoryEntry | string): Promise<void> {
  // Convert to LogEntry, store pasted content (inline or hash)
  // Push to pendingEntries
  currentFlushPromise = flushPromptHistory(0)
  void currentFlushPromise
}
```

Flush uses file locking (`lockfile`) with retries. Cleanup registered on first use to flush on exit.

### 2.6 Undo Mechanism

```typescript
export function removeLastFromHistory(): void {
  if (!lastAddedEntry) return
  const entry = lastAddedEntry
  lastAddedEntry = null

  const idx = pendingEntries.lastIndexOf(entry)
  if (idx !== -1) {
    pendingEntries.splice(idx, 1)  // Fast path: still in memory
  } else {
    skippedTimestamps.add(entry.timestamp)  // Slow path: already flushed
  }
}
```

---

## 3. Token Counting and Budgeting

### 3.1 Canonical Token Counting (`src/utils/tokens.ts`)

The primary function for context size measurement is `tokenCountWithEstimation()` (lines 251-292):

```typescript
export function tokenCountWithEstimation(messages: readonly Message[]): number {
  let i = messages.length - 1
  while (i >= 0) {
    const message = messages[i]
    const usage = message ? getTokenUsage(message) : undefined
    if (message && usage) {
      // Walk back to find FIRST sibling with same message.id
      // (handles parallel tool calls split across multiple assistant records)
      const responseId = getAssistantMessageId(message)
      if (responseId) {
        let j = i - 1
        while (j >= 0) {
          const prior = messages[j]
          const priorId = prior ? getAssistantMessageId(prior) : undefined
          if (priorId === responseId) {
            i = j  // Anchor at earliest split
          } else if (priorId !== undefined) {
            break  // Different API response
          }
          j--
        }
      }
      return (
        getTokenCountFromUsage(usage) +
        roughTokenCountEstimationForMessages(messages.slice(i + 1))
      )
    }
    i--
  }
  // No usage found - estimate entire conversation
  return roughTokenCountEstimationForMessages(messages)
}
```

**Why this is canonical:** It combines the last API response's actual token count (input + output + cache) with rough estimates for messages added since that response.

### 3.2 Rough Token Estimation (`src/services/tokenEstimation.ts`)

```typescript
export function roughTokenCountEstimation(
  content: string,
  bytesPerToken: number = 4,
): number {
  return Math.round(content.length / bytesPerToken)
}

// File-type aware estimation
export function bytesPerTokenForFileType(fileExtension: string): number {
  switch (fileExtension) {
    case 'json': case 'jsonl': case 'jsonc': return 2
    default: return 4
  }
}

export function roughTokenCountEstimationForMessages(
  messages: readonly { type: string; message?: { content?: unknown } }[],
): number {
  let totalTokens = 0
  for (const message of messages) {
    totalTokens += roughTokenCountEstimationForMessage(message)
  }
  return totalTokens
}
```

**Block-level estimation** (lines 458-502):
- Text: `content.length / 4`
- Images/documents: Fixed 2000 tokens (resized to 2000x2000 max)
- Tool results: Recursive estimation of content
- Tool use: Name + JSON-stringified input
- Thinking/redacted_thinking: Text length / 4
- Other blocks: JSON stringify length / 4

### 3.3 API-based Token Counting (`src/services/tokenEstimation.ts`)

Two API paths:
1. **`countMessagesTokensWithAPI()`** (lines 147-213): Direct count_tokens API call
2. **`countTokensViaHaikuFallback()`** (lines 263-369): Uses Haiku/Sonnet to generate minimal response and read usage

Providers handled: Anthropic (first-party), Bedrock, Vertex, Gemini.

### 3.4 Token Budget System (`src/query/tokenBudget.ts`)

Referenced in query.ts line 134: `createBudgetTracker()` and `checkTokenBudget()`. Tracks token budgets for auto-continue (+500k tokens feature).

---

## 4. Context Compaction/Summarization Strategies

### 4.1 Auto-Compact Thresholds (`src/services/compact/autoCompact.ts`)

```typescript
export const AUTOCOMPACT_BUFFER_TOKENS = 13_000
export const WARNING_THRESHOLD_BUFFER_TOKENS = 20_000
export const ERROR_THRESHOLD_BUFFER_TOKENS = 20_000
export const MANUAL_COMPACT_BUFFER_TOKENS = 3_000

export function getAutocompactBufferTokens(model: string): number {
  const effectiveWindow = getEffectiveContextWindowSize(model)
  if (effectiveWindow >= 800_000) return 50_000
  if (effectiveWindow >= 400_000) return 30_000
  return AUTOCOMPACT_BUFFER_TOKENS
}

export function getAutoCompactThreshold(model: string): number {
  const effectiveContextWindow = getEffectiveContextWindowSize(model)
  const autocompactThreshold = effectiveContextWindow - getAutocompactBufferTokens(model)

  // Override via env var percentage
  const envPercent = process.env.CLAUDE_AUTOCOMPACT_PCT_OVERRIDE
  if (envPercent) {
    const parsed = parseFloat(envPercent)
    if (!isNaN(parsed) && parsed > 0 && parsed <= 100) {
      const percentageThreshold = Math.floor(effectiveContextWindow * (parsed / 100))
      return Math.min(percentageThreshold, autocompactThreshold)
    }
  }
  return autocompactThreshold
}
```

### 4.2 Auto-Compact Trigger Logic

```typescript
export async function shouldAutoCompact(
  messages: Message[],
  model: string,
  querySource?: QuerySource,
  snipTokensFreed = 0,
): Promise<boolean> {
  // Guards: skip for session_memory, compact, marble_origami, context-collapse queries
  if (querySource === 'session_memory' || querySource === 'compact') return false
  if (feature('CONTEXT_COLLAPSE') && querySource === 'marble_origami') return false
  if (feature('REACTIVE_COMPACT') && getFeatureValue_CACHED_MAY_BE_STALE('tengu_cobalt_raccoon', false)) return false
  if (feature('CONTEXT_COLLAPSE') && isContextCollapseEnabled()) return false

  if (!isAutoCompactEnabled()) return false

  const tokenCount = tokenCountWithEstimation(messages) - snipTokensFreed
  const threshold = getAutoCompactThreshold(model)
  const { isAboveAutoCompactThreshold } = calculateTokenWarningState(tokenCount, model)
  return isAboveAutoCompactThreshold
}
```

### 4.3 Compaction Flow (`src/services/compact/autoCompact.ts:270-380`)

```typescript
export async function autoCompactIfNeeded(...): Promise<{ wasCompacted, compactionResult?, consecutiveFailures? }> {
  // Circuit breaker: stop after 3 consecutive failures
  if (tracking?.consecutiveFailures >= MAX_CONSECUTIVE_AUTOCOMPACT_FAILURES) {
    return { wasCompacted: false }
  }

  const shouldCompact = await shouldAutoCompact(messages, model, querySource, snipTokensFreed)
  if (!shouldCompact) return { wasCompacted: false }

  // Build recompaction info for telemetry
  const recompactionInfo: RecompactionInfo = {
    isRecompactionInChain: tracking?.compacted === true,
    turnsSincePreviousCompact: tracking?.turnCounter ?? -1,
    previousCompactTurnId: tracking?.turnId,
    autoCompactThreshold: getAutoCompactThreshold(model),
    querySource,
  }

  // EXPERIMENT: Try session memory compaction FIRST
  const sessionMemoryResult = await trySessionMemoryCompaction(...)
  if (sessionMemoryResult) {
    setLastSummarizedMessageId(undefined)
    runPostCompactCleanup(querySource)
    return { wasCompacted: true, compactionResult: sessionMemoryResult }
  }

  // Fallback to legacy compaction
  const compactionResult = await compactConversation(
    messages, toolUseContext, cacheSafeParams,
    true,  // suppressFollowUpQuestions
    undefined,  // no custom instructions
    true,  // isAutoCompact
    recompactionInfo
  )
  // ...tracking reset, post-compact cleanup...
}
```

### 4.4 Session Memory Compaction (`src/services/compact/sessionMemoryCompact.ts`)

An experimental alternative to full summarization that uses pre-extracted session memory:

```typescript
// Configuration (lines 56-61)
export const DEFAULT_SM_COMPACT_CONFIG: SessionMemoryCompactConfig = {
  minTokens: 10_000,
  minTextBlockMessages: 5,
  maxTokens: 40_000,
}

// Calculate messages to keep (lines 326-399)
export function calculateMessagesToKeepIndex(
  messages: Message[],
  lastSummarizedIndex: number,
): number {
  let startIndex = lastSummarizedIndex >= 0 ? lastSummarizedIndex + 1 : messages.length
  let totalTokens = 0
  let textBlockMessageCount = 0

  // Count current kept messages
  for (let i = startIndex; i < messages.length; i++) {
    totalTokens += estimateMessageTokens([messages[i]!])
    if (hasTextBlocks(messages[i]!)) textBlockMessageCount++
  }

  // Expand backwards until minimums met or max cap hit
  const floor = lastCompactBoundaryIndex + 1  // Don't cross boundary
  for (let i = startIndex - 1; i >= floor; i--) {
    totalTokens += estimateMessageTokens([messages[i]!])
    if (hasTextBlocks(messages[i]!)) textBlockMessageCount++
    startIndex = i
    if (totalTokens >= config.maxTokens) break
    if (totalTokens >= config.minTokens && textBlockMessageCount >= config.minTextBlockMessages) break
  }

  // Adjust for tool_use/tool_result pairs and thinking blocks
  return adjustIndexToPreserveAPIInvariants(messages, startIndex)
}
```

**Key differences from legacy compaction:**
- Uses pre-extracted session memory (no API call for summary)
- Preserves recent messages verbatim (suffix-preserving)
- Only summarizes the prefix before `lastSummarizedMessageId`
- Validates post-compact token count against autocompact threshold

### 4.5 Legacy Full Compaction (`src/services/compact/compact.ts`)

Main function: `compactConversation()` (lines 411-792)

**Process:**
1. **Pre-compact hooks** (lines 436-447): Execute PreCompact hooks
2. **Build summary request** (lines 464-467): Create compact prompt
3. **Stream compact summary** (lines 474-515): With PTL retry logic
4. **Post-compact processing:**
   - Store file state, clear caches (lines 541-547)
   - Create file attachments for recently read files (lines 556-575)
   - Add plan, skill, tool delta attachments (lines 572-612)
   - Execute SessionStart hooks (lines 618-621)
   - Create compact boundary marker (lines 625-638)
   - Create summary user message (lines 641-651)
   - Calculate `truePostCompactTokenCount` (lines 664-669)
4. **Post-compact hooks & cleanup** (lines 748-777)

**Partial Compaction** (`partialCompactConversation()`, lines 801-1140):
- Direction `'from'`: Summarize messages AFTER pivot, keep earlier (prefix-preserving, cache hit)
- Direction `'up_to'`: Summarize messages BEFORE pivot, keep later (suffix-preserving, cache miss)
- Used by message selector UI

### 4.6 Micro-Compaction (`src/services/compact/microCompact.ts`)

Two modes:

**1. Cached Micro-Compact** (feature-gated, lines 309-403):
- Uses cache editing API (`cache_edits`) to remove tool results without invalidating prompt cache
- Count-based trigger from GrowthBook config
- Only for main thread (`repl_main_thread*` querySource)
- Returns messages unchanged; cache edits applied at API layer

**2. Time-Based Micro-Compact** (lines 416-535):
- Triggers when gap since last assistant message > threshold (configurable)
- Content-clears old tool results (replaces with `[Old tool result content cleared]`)
- Keeps `keepRecent` most recent compactable tool results
- Mutates message content directly (cache is cold anyway)

---

## 5. File Context Inclusion

### 5.1 CLAUDE.md Loading (`src/context.ts:155-188`)

```typescript
export const getUserContext = memoize(async (): Promise<{[k: string]: string}> => {
  const shouldDisableClaudeMd =
    isEnvTruthy(process.env.CLAUDE_CODE_DISABLE_CLAUDE_MDS) ||
    (isBareMode() && getAdditionalDirectoriesForClaudeMd().length === 0)

  const claudeMd = shouldDisableClaudeMd
    ? null
    : getClaudeMds(filterInjectedMemoryFiles(await getMemoryFiles()))
  
  setCachedClaudeMdContent(claudeMd || null)
  
  return {
    ...(claudeMd && { claudeMd }),
    currentDate: `Today's date is ${getLocalISODate()}.`,
  }
})
```

### 5.2 Git Status Context (`src/context.ts:36-111`)

```typescript
export const getGitStatus = memoize(async (): Promise<string | null> => {
  // ...git commands...
  return [
    `This is the git status at the start of the conversation...`,
    `Current branch: ${branch}`,
    `Main branch: ${mainBranch}`,
    ...(userName ? [`Git user: ${userName}`] : []),
    `Status:\n${truncatedStatus || '(clean)'}`,
    `Recent commits:\n${log}`,
  ].join('\n\n')
})
```

Truncated at `MAX_STATUS_CHARS = 1000` (line 20).

### 5.3 Context Injection into Messages (`src/utils/api.ts:443-485`)

```typescript
export function prependUserContext(
  messages: Message[],
  context: { [k: string]: string },
): Message[] {
  const { claudeMd, ...rest } = context
  const result: Message[] = []

  if (claudeMd) {
    result.push(
      createUserMessage({
        content: `<project-instructions>\n${claudeMd}\n</project-instructions>\n`,
        isMeta: true,
      }),
    )
  }

  if (Object.entries(rest).length > 0) {
    result.push(
      createUserMessage({
        content: `<system-reminder>\nAs you answer the user's questions, you can use the following context:\n${restEntries
          .map(([key, value]) => `# ${key}\n${value}`)
          .join('\n')}
        IMPORTANT: this context may or may not be relevant to your tasks. You should not respond to this context unless it is highly relevant to your task.\n</system-reminder>\n`,
        isMeta: true,
      }),
    )
  }

  return [...result, ...messages]
}

export function appendSystemContext(
  systemPrompt: SystemPrompt,
  context: { [k: string]: string },
): string[] {
  return [
    ...systemPrompt,
    Object.entries(context)
      .map(([key, value]) => `${key}: ${value}`)
      .join('\n'),
  ].filter(Boolean)
}
```

### 5.4 Post-Compact File Attachments (`src/services/compact/compact.ts:556-575`)

```typescript
const [fileAttachments, asyncAgentAttachments] = await Promise.all([
  createPostCompactFileAttachments(preCompactReadFileState, context, POST_COMPACT_MAX_FILES_TO_RESTORE),
  createAsyncAgentAttachmentsIfNeeded(context),
])

// Constants (lines 126-134)
export const POST_COMPACT_MAX_FILES_TO_RESTORE = 5
export const POST_COMPACT_TOKEN_BUDGET = 50_000
export const POST_COMPACT_MAX_TOKENS_PER_FILE = 5_000
export const POST_COMPACT_MAX_TOKENS_PER_SKILL = 5_000
export const POST_COMPACT_SKILLS_TOKEN_BUDGET = 25_000
```

---

## 6. Context Persistence Across Sessions

### 6.1 Session Storage (`src/utils/sessionStorage.ts`)

Referenced in compact.ts lines 80, 82: `getTranscriptPath()`, `reAppendSessionMetadata()`

### 6.2 Resume Functionality

- Transcript files store full conversation history
- `--resume` flag loads previous session
- Session metadata (custom title, tag) re-appended after compaction to stay within 16KB tail window (compact.ts lines 735-740)

### 6.3 History Persistence (`src/history.ts`)

Global history file at `~/.claude/history.jsonl` persists across sessions. Project-scoped, session-aware retrieval.

---

## 7. How Context is Passed to the Model

### 7.1 Query Loop Message Preparation (`src/query.ts:523-553`)

```typescript
let messagesForQuery = getMessagesAfterCompactBoundary(messages)

// Release toolUseResult payloads (UI-only, not for API)
messagesForQuery = messagesForQuery.map(msg => {
  if (msg.type !== 'user' || !('toolUseResult' in msg) || msg.toolUseResult === undefined) {
    return msg
  }
  const copy = { ...msg }
  delete copy.toolUseResult
  return copy
})
```

### 7.2 Pre-Query Transformations (in order)

1. **Tool result budget** (`applyToolResultBudget`, query.ts:567-582): Enforce per-message token budget on aggregate tool result size
2. **Snip compact** (query.ts:588-598): Remove older messages if `HISTORY_SNIP` feature enabled
3. **Micro-compact** (query.ts:601-624): Cached or time-based micro-compaction
4. **Context collapse** (query.ts:638-645): Apply collapsed view projection
5. **Auto-compact** (query.ts:652-666): Full or session-memory compaction if threshold exceeded

### 7.3 System Prompt Construction (`src/query.ts:647-649`)

```typescript
const fullSystemPrompt = asSystemPrompt(
  appendSystemContext(systemPrompt, systemContext),
)
```

### 7.4 User Context Prepending (`src/query.ts:899-901`)

```typescript
for await (const message of deps.callModel({
  messages: prependUserContext(messagesForQuery, userContext),
  systemPrompt: fullSystemPrompt,
  ...
}))
```

### 7.5 API Request Options (query.ts:904-948)

Key options passed to the model:
- `thinkingConfig`
- `tools` (with `defer_loading` for tool search)
- `model` (with fallback support)
- `maxOutputTokensOverride`
- `mcpTools`
- `taskBudget` (for API task_budget beta)
- `langfuseTrace`
- `skipCacheWrite`
- `agents`, `allowedAgentTypes`
- `effortValue`, `advisorModel`

### 7.6 Prompt Caching Strategy

**System prompt splitting** (`src/utils/api.ts:317-429`):
- Global cache scope (1P only): Static prefix cached globally, dynamic suffix uncached
- Org cache scope: All blocks cached at org level
- MCP tools present: Skip global cache for system prompt

**Compact cache sharing** (compact.ts:459-462):
```typescript
const promptCacheSharingEnabled = getFeatureValue_CACHED_MAY_BE_STALE(
  'tengu_compact_cache_prefix',  // default true for 3P
  true,
)
```
Forked agent reuses main conversation's cached prefix.

---

## 8. Context Priorities and Truncation

### 8.1 Priority Hierarchy (Implicit in Code)

1. **System Prompt** (highest): Core instructions, tool definitions, immutable per-session
2. **User Context** (high): CLAUDE.md as `<project-instructions>`, git status, date
3. **Recent Conversation** (highest): Messages after last compact boundary
4. **Post-Compact Attachments** (high): Recent file reads, plan, skills, tool deltas
5. **Compact Summary** (medium): Summary of earlier conversation
6. **Older Messages** (lowest): Dropped during compaction/snip

### 8.2 Truncation Strategies

#### A. Auto-Compact (Proactive)
- Triggered when `tokenCount > getAutoCompactThreshold(model)`
- Full summarization via `compactConversation()`
- Replaces ALL prior messages with summary + boundary marker

#### B. Session Memory Compaction (Experimental)
- Keeps recent messages verbatim (suffix-preserving)
- Uses pre-extracted session memory for prefix
- Validates against autocompact threshold

#### C. Partial Compaction (Manual)
- `direction: 'from'`: Summarize suffix, keep prefix (cache hit)
- `direction: 'up_to'`: Summarize prefix, keep suffix (cache miss)

#### D. Snip Compact (`src/services/compact/snipCompact.ts`)
- Removes oldest messages to free tokens
- Preserves compact boundary continuity

#### E. Micro-Compact
- **Cached**: Removes tool results via cache editing (no token count reduction locally)
- **Time-based**: Content-clears old tool results when idle > threshold

#### F. Reactive Compact (`src/services/compact/reactiveCompact.ts`)
- Triggered by API `prompt_too_long` error
- Emergency compaction on 413 response

#### G. Context Collapse (Experimental, `src/services/contextCollapse/`)
- Commits older context to summary store at 90% threshold
- Blocks new turns at 95% threshold
- Projects collapsed view at query time

### 8.3 Blocking Limit (query.ts:790-846)

```typescript
if (!compactionResult && querySource !== 'compact' && querySource !== 'session_memory' && 
    !(reactiveCompact?.isReactiveCompactEnabled() && isAutoCompactEnabled()) &&
    !collapseOwnsIt) {
  const { isAtBlockingLimit } = calculateTokenWarningState(
    tokenCountWithEstimation(messagesForQuery) - snipTokensFreed,
    toolUseContext.options.mainLoopModel,
  )
  if (isAtBlockingLimit) {
    yield createAssistantAPIErrorMessage({
      content: PROMPT_TOO_LONG_ERROR_MESSAGE,
      error: 'invalid_request',
    })
    return { reason: 'blocking_limit' }
  }
}
```
- Hard limit: `effectiveContextWindow - MANUAL_COMPACT_BUFFER_TOKENS` (3,000)
- Reserves space for manual `/compact`
- Skipped if compaction just happened or reactive compact enabled

### 8.4 Predictive Auto-Compact (query.ts:848-888)

```typescript
if (!compactionResult && isAutoCompactEnabled()) {
  const currentTokens = tokenCountWithEstimation(messagesForQuery) - snipTokensFreed
  const estimatedGrowth = estimateMaxTurnGrowth(model)  // maxOutput + 15K tool growth
  const predictiveThreshold = getEffectiveContextWindowSize(model) - estimatedGrowth
  
  if (currentTokens > predictiveThreshold) {
    // Trigger compaction BEFORE API call
  }
}
```

---

## 9. Key Constants Summary

| Constant | Value | Location |
|----------|-------|----------|
| `MODEL_CONTEXT_WINDOW_DEFAULT` | 200,000 | context.ts:14 |
| `COMPACT_MAX_OUTPUT_TOKENS` | 20,000 | context.ts:17 |
| `MAX_OUTPUT_TOKENS_FOR_SUMMARY` | 20,000 | autoCompact.ts:30 |
| `AUTOCOMPACT_BUFFER_TOKENS` | 13,000 | autoCompact.ts:62 |
| `WARNING_THRESHOLD_BUFFER_TOKENS` | 20,000 | autoCompact.ts:63 |
| `ERROR_THRESHOLD_BUFFER_TOKENS` | 20,000 | autoCompact.ts:64 |
| `MANUAL_COMPACT_BUFFER_TOKENS` | 3,000 | autoCompact.ts:65 |
| `MAX_CONSECUTIVE_AUTOCOMPACT_FAILURES` | 3 | autoCompact.ts:99 |
| `MAX_HISTORY_ITEMS` | 100 | history.ts:19 |
| `MAX_PASTED_CONTENT_LENGTH` | 1,024 | history.ts:20 |
| `POST_COMPACT_MAX_FILES_TO_RESTORE` | 5 | compact.ts:126 |
| `POST_COMPACT_TOKEN_BUDGET` | 50,000 | compact.ts:127 |
| `POST_COMPACT_MAX_TOKENS_PER_FILE` | 5,000 | compact.ts:128 |
| `POST_COMPACT_MAX_TOKENS_PER_SKILL` | 5,000 | compact.ts:133 |
| `POST_COMPACT_SKILLS_TOKEN_BUDGET` | 25,000 | compact.ts:134 |
| `IMAGE_MAX_TOKEN_SIZE` | 2,000 | microCompact.ts:38 |
| `TOOL_RESULT_GROWTH_ESTIMATE` | 15,000 | autoCompact.ts:70 |

---

## 10. Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────┐
                        USER INPUT / QUERY
└─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
                    QUERY LOOP (query.ts)
└─────────────────────────────────────────────────────────────────────┘
                                  │
        ┌─────────────────────────┼─────────────────────────┐
        ▼                         ▼                         ▼
┌───────────────┐       ┌───────────────┐       ┌───────────────┐
│ Tool Budget   │       │  Snip Compact │       │ Micro-Compact │
│ (applyTool    │       │  (HISTORY_    │       │  (cached/     │
│  ResultBudget)│       │   SNIP)       │       │   time-based) │
└───────┬───────┘       └───────┬───────┘       └───────┬───────┘
        │                       │                       │
        └───────────────────────┼───────────────────────┘
                                ▼
                    ┌───────────────────────┐
                    │   Context Collapse    │
                    │  (CONTEXT_COLLAPSE)   │
                    └───────────┬───────────┘
                                ▼
                    ┌───────────────────────┐
                    │    Auto-Compact       │
                    │  (autoCompactIfNeeded)│
                    └───────────┬───────────┘
                                ▼
              ┌─────────────────┴─────────────────┐
              ▼                                   ▼
    ┌─────────────────────┐             ┌─────────────────────┐
    │ Session Memory      │             │ Legacy Full         │
    │ Compaction          │             │ Compaction          │
    │ (trySessionMemory   │             │ (compactConversation)│
    │  Compaction)        │             │                     │
    └─────────┬───────────┘             └─────────┬───────────┘
              │                                   │
              └─────────────────┬─────────────────┘
                                ▼
                    ┌───────────────────────┐
                    │  Build Post-Compact   │
                    │  Messages             │
                    │  (buildPostCompact    │
                    │   Messages)           │
                    └───────────┬───────────┘
                                ▼
                    ┌───────────────────────┐
                    │  Prepend User Context │
                    │  (prependUserContext) │
                    │  - CLAUDE.md          │
                    │  - Git status         │
                    │  - Date               │
                    └───────────┬───────────┘
                                ▼
                    ┌───────────────────────┐
                    │  Append System Context│
                    │  (appendSystemContext)│
                    └───────────┬───────────┘
                                ▼
                    ┌───────────────────────┐
                    │   API REQUEST         │
                    │  - messages           │
                    │  - systemPrompt       │
                    │  - tools              │
                    │  - thinkingConfig     │
                    │  - taskBudget         │
                    └───────────────────────┘
```

---

## 11. File References Summary

| Feature | Primary Files |
|---------|--------------|
| Context Window | `src/utils/context.ts` |
| System/User Context | `src/context.ts` |
| History | `src/history.ts` |
| Token Counting | `src/utils/tokens.ts`, `src/services/tokenEstimation.ts` |
| Auto-Compact | `src/services/compact/autoCompact.ts` |
| Full Compaction | `src/services/compact/compact.ts` |
| Session Memory Compaction | `src/services/compact/sessionMemoryCompact.ts` |
| Micro-Compact | `src/services/compact/microCompact.ts` |
| Partial Compaction | `src/services/compact/compact.ts` (partialCompactConversation) |
| Reactive Compact | `src/services/compact/reactiveCompact.ts` |
| Snip Compact | `src/services/compact/snipCompact.ts` |
| Context Collapse | `src/services/contextCollapse/` |
| Query Loop | `src/query.ts` |
| API Request Building | `src/utils/api.ts` |
| Compaction Prompts | `src/services/compact/prompt.ts` |
| Token Budget | `src/query/tokenBudget.ts` |
| Message Normalization | `src/utils/messages.ts` |

---

*Analysis based on codebase at `/tmp/opencode/claude-code` as of August 2026.*
