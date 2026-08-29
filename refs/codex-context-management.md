# Codex Context Management Analysis

This document provides a comprehensive analysis of the context management system in the Codex codebase (`/tmp/opencode/refs/codex/codex-rs`). The analysis covers all eight requested areas with detailed code references.

---

## 1. Context Window Management

### 1.1 Model Context Window Configuration

The context window is determined by the model's capabilities and user configuration:

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/session/turn_context.rs` (lines 296-303)
```rust
pub(crate) fn model_context_window(&self) -> Option<i64> {
    let effective_context_window_percent = self.model_info.effective_context_window_percent;
    self.model_info
        .resolved_context_window()
        .map(|context_window| {
            context_window.saturating_mul(effective_context_window_percent) / 100
        })
}
```

The model context window is:
- Retrieved from `ModelInfo.resolved_context_window()` 
- Adjusted by `effective_context_window_percent` (default 100%, configurable)
- Used as a hard cap for token budgeting

### 1.2 Context Window Token Status Tracking

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/session/context_window.rs` (lines 5-91)

The `ContextWindowTokenStatus` struct tracks:
- `active_context_tokens`: Full active context usage
- `auto_compact_scope_tokens`: Tokens counted against auto-compact limit
- `auto_compact_scope_limit`: Configured auto-compact token limit
- `full_context_window_limit`: Model's full context window (hard cap)
- `base_window_tokens_remaining`: Remaining tokens against base window
- `auto_compact_window_prefill_tokens`: Prefill tokens for BodyAfterPrefix scope
- `full_context_window_limit_reached`: Whether hard cap is reached
- `token_limit_reached`: Whether buffered auto-compact limit or full window is reached

### 1.3 Auto-Compact Token Limit Scopes

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/session/context_window.rs` (lines 30-51)

Two scopes for auto-compaction:
```rust
match turn_context.config.model_auto_compact_token_limit_scope {
    AutoCompactTokenLimitScope::Total => (
        active_context_tokens,
        turn_context.model_info.auto_compact_token_limit(),
        None,
    ),
    AutoCompactTokenLimitScope::BodyAfterPrefix => {
        let window = sess.auto_compact_window_snapshot().await;
        let baseline = window.prefill_input_tokens.unwrap_or(active_context_tokens);
        // Only count tokens added after initial context prefix
        (
            active_context_tokens.saturating_sub(baseline),
            scope_limit,
            window.prefill_input_tokens,
        )
    }
}
```

### 1.4 Fallback Buffer

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/session/context_window.rs` (lines 66-72)
```rust
let auto_compact_fallback_buffer_tokens = turn_context
    .config
    .token_budget
    .as_ref()
    .map_or(0, crate::config::TokenBudgetConfig::fallback_buffer_tokens);
let buffered_auto_compact_limit = auto_compact_scope_limit
    .map(|limit| limit.saturating_add(auto_compact_fallback_buffer_tokens));
```

The fallback buffer reserves tokens for the auto-compact fallback prompt.

---

## 2. Conversation History Handling

### 2.1 ContextManager Structure

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/context_manager/history.rs` (lines 43-65)

```rust
pub(crate) struct ContextManager {
    items: Arc<Vec<ResponseItemEnvelope>>,
    history_version: u64,
    token_info: Option<TokenUsageInfo>,
    reference_context_item: Option<TurnContextItem>,
    world_state_baseline: Option<WorldStateSnapshot>,
}
```

Key design points:
- **Copy-on-write**: Uses `Arc<Vec<...>>` for efficient snapshots
- **History version**: Bumped on rewrite (compaction, rollback)
- **Token tracking**: Maintains `TokenUsageInfo` for usage accounting
- **Reference context**: For diffing context updates (settings, world state)
- **World state baseline**: Tracks last rendered world state for diffs

### 2.2 History Normalization

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/context_manager/normalize.rs` (lines 21-408)

Normalization enforces invariants before sending to model:
1. **Call/Output pairing**: Every function/tool call must have corresponding output (and vice versa)
2. **Modality stripping**: Remove unsupported images/audio based on `input_modalities`
3. **Synthetic outputs**: Insert "aborted" outputs for calls without responses

```rust
fn normalize_history(&mut self, input_modalities: &[InputModality]) {
    normalize::ensure_call_outputs_present(items);
    normalize::remove_orphan_outputs(items);
    normalize::strip_images_when_unsupported(input_modalities, items);
    normalize::strip_audio_when_unsupported(input_modalities, items);
}
```

### 2.3 History for Model Prompt

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/context_manager/history.rs` (lines 197-214)

```rust
pub(crate) fn for_prompt(self, input_modalities: &[InputModality]) -> Vec<ResponseItem> {
    self.for_prompt_annotated(input_modalities)
        .into_iter()
        .map(ResponseItemEnvelope::into_item)
        .collect()
}

pub(crate) fn for_prompt_annotated(
    mut self,
    input_modalities: &[InputModality],
) -> Vec<ResponseItemEnvelope> {
    self.normalize_history(input_modalities);
    Arc::unwrap_or_clone(self.items)
}
```

### 2.4 Conversation History Snapshot

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/context_manager/history.rs` (lines 67-86)

Provides filtered view for extensions:
```rust
impl ConversationHistorySnapshot for SharedConversationHistory {
    fn items(&self) -> Box<dyn Iterator<Item = &ResponseItem> + Send + '_> {
        Box::new(
            self.items
                .iter()
                .map(|envelope| &envelope.item)
                .filter(|item| {
                    !matches!(
                        item,
                        ResponseItem::Message { role, content, .. }
                            if role == "user" && is_contextual_user_message_content(content)
                    )
                }),
        )
    }
}
```

Filters out contextual user fragments (system-injected context).

---

## 3. Token Counting and Budgeting

### 3.1 Token Estimation

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/context_manager/history.rs` (lines 609-676)

Coarse estimation using byte-based heuristics:
```rust
pub(crate) fn estimate_item_token_count(item: &ResponseItem) -> i64 {
    let model_visible_bytes = estimate_response_item_model_visible_bytes(item);
    approx_tokens_from_byte_count_i64(model_visible_bytes)
}
```

Special handling for:
- **Reasoning/Compaction items**: Encrypted content length × 3/4 - 650
- **Images**: Base64 payload replaced with per-modality estimates (7,373 bytes for resized, patch-based for "original" detail)
- **Audio**: Base64 payload replaced with audio token estimate
- **Encrypted function outputs**: Length × 9/16

### 3.2 History Token Estimation

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/context_manager/history.rs` (lines 247-271)

```rust
pub(crate) fn estimate_token_count_with_base_instructions(
    &self,
    base_instructions: &BaseInstructions,
) -> Option<i64> {
    let base_tokens = i64::try_from(approx_token_count(&base_instructions.text)).unwrap_or(i64::MAX);
    let items_tokens = self
        .items
        .iter()
        .map(|envelope| estimate_item_token_count(&envelope.item))
        .fold(0i64, i64::saturating_add);
    Some(base_tokens.saturating_add(items_tokens))
}
```

### 3.3 Token Usage Tracking

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/context_manager/history.rs` (lines 362-438)

```rust
pub(crate) fn update_token_info(
    &mut self,
    usage: &TokenUsage,
    model_context_window: Option<i64>,
) {
    self.token_info = TokenUsageInfo::new_or_append(
        &self.token_info,
        &Some(usage.clone()),
        model_context_window,
    );
}

pub(crate) fn get_total_token_usage(&self, server_reasoning_included: bool) -> i64 {
    let last_tokens = self.token_info.as_ref().map(|info| info.last_token_usage.total_tokens).unwrap_or(0);
    let items_after_last_model_generated_tokens = self
        .items_after_last_model_generated_item()
        .map(estimate_item_token_count)
        .fold(0i64, i64::saturating_add);
    
    if server_reasoning_included {
        last_tokens.saturating_add(items_after_last_model_generated_tokens)
    } else {
        last_tokens
            .saturating_add(self.get_non_last_reasoning_items_tokens())
            .saturating_add(items_after_last_model_generated_tokens)
    }
}
```

### 3.4 Rollout Budget (Session-Level)

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/rollout_budget.rs` (lines 11-127)

Shared budget across thread tree:
```rust
pub(crate) fn record_usage(&self, usage: &TokenUsage) -> CodexResult<bool> {
    let units = if let Some(units) = usage.codex_rollout_budget_units.as_ref() {
        // Use server-provided budget units
    } else {
        // Weighted calculation: output_tokens * sampling_weight + non_cached_input * prefill_weight
        usage.output_tokens.max(0) as f64 * state.config.sampling_token_weight
            + usage.non_cached_input() as f64 * state.config.prefill_token_weight
    };
    state.weighted_tokens_used += units;
    Ok(state.weighted_tokens_used >= state.config.limit_tokens as f64)
}
```

### 3.5 Token Budget Reminders

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/session/token_budget.rs` (lines 58-112)

```rust
pub(super) async fn maybe_record(
    sess: &Session,
    turn_context: &TurnContext,
    base_window_tokens_remaining: Option<i64>,
    allow_auto_compact_fallback: bool,
) {
    // Reminder when threshold crossed
    if config.reminder_threshold_tokens.is_some_and(|threshold| base_window_tokens_remaining <= threshold) {
        let reminder_due = sess.state.lock().await.claim_token_budget_reminder();
        if reminder_due {
            // Inject TokenBudgetReminder into conversation
        }
    }
    
    // Auto-compact fallback when window exhausted
    if !allow_auto_compact_fallback || base_window_tokens_remaining != 0 { return; }
    let fallback_due = sess.state.lock().await.claim_auto_compact_fallback();
    if fallback_due {
        // Inject AutoCompactFallbackPrompt
    }
}
```

---

## 4. Context Compaction/Summarization Strategies

### 4.1 Compaction Types

Three compaction implementations (from `codex_protocol::protocol::CompactionImplementation`):
1. **Local** (`Responses`): Client-side summarization via model
2. **Remote V1** (`ResponsesCompact`): Server `/responses/compact` endpoint
3. **Remote V2** (`ResponsesCompactionV2`): New server compaction with structured output

### 4.2 Local Compaction (Client-Side Summarization)

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/compact.rs` (lines 111-394)

Process:
1. Build prompt with `SUMMARIZATION_PROMPT` (or custom)
2. Stream to model, collect assistant response
3. Extract last assistant message as summary
4. Build compacted history: selected user messages + summary
5. Replace history via `Session::replace_compacted_history()`

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/compact.rs` (lines 639-717)

```rust
fn build_compacted_history_with_limit(
    mut history: Vec<ResponseItemEnvelope>,
    user_messages: &[CompactedUserMessage],
    summary_text: &str,
    max_tokens: usize,
) -> Vec<ResponseItemEnvelope> {
    // Select newest user messages within token budget
    let mut remaining = max_tokens;
    for message in user_messages.iter().rev() {
        let tokens = approx_token_count(&message.message);
        if tokens <= remaining {
            selected_messages.push(message.clone());
            remaining = remaining.saturating_sub(tokens);
        } else {
            // Truncate oldest retained message
            let truncated = truncate_text(&message.message, TruncationPolicy::Tokens(remaining));
            selected_messages.push(truncated_version);
            break;
        }
    }
    selected_messages.reverse();
    // Add as user messages, then add summary as final user message
}
```

### 4.3 Remote Compaction V1

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/compact_remote.rs` (lines 53-309)

Server-driven compaction via `/responses/compact`:
1. Sends full history to server
2. Server returns compacted history
3. Client processes: filters developer messages, keeps user/assistant/compaction items
4. Truncates function outputs to fit context window

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/compact_remote.rs` (lines 354-397)

```rust
pub(crate) fn should_keep_compacted_history_item(item: &ResponseItem) -> bool {
    match item {
        ResponseItem::Message { role, .. } if role == "developer" => false,
        ResponseItem::Message { role, .. } if role == "user" => {
            matches!(parse_turn_item(item), Some(TurnItem::UserMessage(_) | TurnItem::HookPrompt(_)))
        }
        ResponseItem::Message { role, .. } if role == "assistant" => true,
        ResponseItem::AgentMessage { .. } => true,
        ResponseItem::Compaction { .. } | ResponseItem::ContextCompaction { .. } => true,
        _ => false,
    }
}
```

### 4.4 Remote Compaction V2

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/compact_remote_v2.rs` (lines 58-488)

Enhanced remote compaction with:
- Token budget for retained messages (64,000 tokens default)
- Structured compaction output item
- Better metadata preservation

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/compact_remote_v2.rs` (lines 459-617)

```rust
pub(crate) fn truncate_retained_messages_for_remote_compaction(
    items: Vec<ResponseItemEnvelope>,
    max_tokens: usize,
) -> Vec<ResponseItemEnvelope> {
    // Process from newest to oldest
    for group in v2_history_item_groups(items).rev() {
        // Charge tokens, truncate text if needed, preserve images
        // Keep client-authored developer messages when feature enabled
    }
}
```

### 4.5 Token-Budget Compaction (Manual Reset)

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/compact_token_budget.rs` (lines 21-92)

```rust
pub(crate) async fn run_manual_compact_task(
    sess: Arc<Session>,
    turn_context: Arc<TurnContext>,
) -> CodexResult<()> {
    // Skip model summarization, install fresh context window
    sess.start_new_context_window(step_context, world_state).await;
}
```

### 4.6 Compaction Phases

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/compact.rs` (lines 60-74)

```rust
pub(crate) enum InitialContextInjection {
    BeforeLastUserMessage { world_state, step_context },  // Mid-turn
    DoNotInject,                                           // Pre-turn/Manual
}
```

- **Pre-turn**: Before user message, clears reference context (full reinjection next turn)
- **Mid-turn**: After model output, injects initial context before last user message
- **Manual**: User-triggered, same as pre-turn

---

## 5. File Context Inclusion (Reading Files into Context)

### 5.1 World State System

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/context/world_state/mod.rs` (lines 202-430)

The `WorldState` system manages model-visible context sections:
```rust
pub(crate) trait WorldStateSection: Send + Sync + 'static {
    const ID: &'static str;
    type Snapshot: DeserializeOwned + Serialize;
    
    fn snapshot(&self) -> Self::Snapshot;
    fn render_diff(&self, previous: PreviousSectionState<'_, Self::Snapshot>) 
        -> Option<Box<dyn ContextualUserFragment>>;
}
```

### 5.2 Built-in Context Sections

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/context/world_state/mod.rs` (lines 36-50)

Registered sections:
- `AgentsMdState` - AGENTS.md instructions
- `AppsInstructionsState` - App/integration instructions
- `CollaborationModeState` - Collaboration mode info
- `CompactPermissionsState` - Permission summary
- `ContextWindowGuidanceState` - Token budget guidance
- `EnvironmentsState` - Environment/workspace info
- `EnvironmentsInstructionsState` - Environment-specific instructions
- `ModelInstructionsState` - Model-specific instructions
- `MultiAgentModeState` - Multi-agent configuration
- `MultiAgentUsageHintState` - Usage hints
- `PermissionsState` - Permission profile
- `PersonalityState` - Personality instructions
- `PluginsInstructionsState` - Plugin instructions
- `RealtimeState` - Realtime conversation state
- `ToolsState` - Available tools

### 5.3 Diff-Based Rendering

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/context/world_state/mod.rs` (lines 382-419)

```rust
pub(crate) fn render_diff(
    &self,
    previous: &WorldStateSnapshot,
) -> Vec<Box<dyn ContextualUserFragment>> {
    self.render_with(|id, _| match previous.sections.get(id) {
        Some(previous) => PreviousSectionState::Known(previous),
        None => PreviousSectionState::Absent,
    })
}

pub(crate) fn render_history_diff<'a>(
    &self,
    previous: Option<&WorldStateSnapshot>,
    items: impl IntoIterator<Item = &'a ResponseItem> + Clone,
) -> Vec<Box<dyn ContextualUserFragment>> {
    self.render_with(|id, section| {
        if let Some(previous) = previous.and_then(|p| p.sections.get(id)) {
            if section.has_retained_fragment_matcher() && !has_retained_fragment(items.clone(), section) {
                PreviousSectionState::Absent
            } else {
                PreviousSectionState::Known(previous)
            }
        } else if has_legacy_fragment(items.clone(), section) {
            PreviousSectionState::Unknown
        } else {
            PreviousSectionState::Absent
        }
    })
}
```

### 5.4 Context Injection into History

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/session/turn.rs` (lines 224-228)

```rust
let (world_state, display_roots) = tokio::join!(
    sess.record_context_updates_and_set_reference_context_item(first_step_context.as_ref()),
    turn_diff_display_roots(first_step_context.as_ref()),
);
```

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/session/turn.rs` (lines 345-347)

```rust
world_state = sess
    .record_step_world_state_if_changed(&world_state, step_context.as_ref())
    .await?;
```

### 5.5 Context Fragments

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/context/mod.rs` (lines 41-92)

Exported fragment types include:
- `UserInstructions` - User-provided instructions
- `EnvironmentContext` - Workspace/environment details
- `ApprovedCommandPrefixSaved` - Exec approval prefix
- `ImageResizeNotice` - Image processing notices
- `InternalModelContextFragment` - Internal context
- `NodeReplReviewEvidence` - REPL review evidence
- `HookAdditionalContext` - Hook-injected context
- Various instruction fragments (personality, plugins, etc.)

---

## 6. Context Persistence Across Sessions

### 6.1 Thread Persistence

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/thread_manager.rs` (lines 740-823)

Threads persisted via `LiveThread` with rollout storage:
```rust
let live_thread = match &initial_history {
    InitialHistory::New | InitialHistory::Cleared | InitialHistory::Forked(_) => {
        LiveThread::create(Arc::clone(&thread_store), params).await?
    }
    InitialHistory::Resumed(resumed_history) => {
        LiveThread::resume(Arc::clone(&thread_store), session_configuration.history_mode, params).await?
    }
};
```

### 6.2 History Modes

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/thread_manager.rs` (lines 112-113, 244)

```rust
pub(super) history_mode: ThreadHistoryMode,
```

Modes (from protocol):
- `Full` - Complete history
- `Paginated` - Paginated for long threads
- `None` - No history persistence

### 6.3 Rollout Items

**File:** `/tmp/opencode/refs/codex/codex-rs/codex-rs/core/src/session/rollout_reconstruction.rs`

Rollout items include:
- `ResponseItem` - Model conversation items
- `EventMsg` - Protocol events
- `InterAgentCommunication` - Agent-to-agent messages
- `SessionMeta` - Session metadata
- `Compacted` - Compaction checkpoints
- `ThreadRolledBack` - Rollback markers

### 6.4 Auto-Compact Window Persistence

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/session/session.rs` (lines 693-707)

```rust
let initial_auto_compact_window_ids = AutoCompactWindowIds::new_initial();
let restore_child_window = matches!(&initial_history, InitialHistory::Forked(_))
    && session_configuration.session_source.is_non_root_agent()
    && config.features.enabled(Feature::TokenBudget);
if restore_child_window && let InitialHistory::Forked(items) = &mut initial_history {
    let child_window_id = initial_auto_compact_window_ids.window_id.to_string();
    for item in items {
        if let RolloutItem::Compacted(checkpoint) = item {
            checkpoint.window_number = Some(0);
            checkpoint.first_window_id = Some(child_window_id.clone());
            checkpoint.previous_window_id = None;
            checkpoint.window_id = Some(child_window_id.clone());
        }
    }
}
```

### 6.5 Session Reconstruction

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/session/rollout_reconstruction.rs`

Reconstructs in-memory state from persisted rollout:
- Replays conversation items into `ContextManager`
- Restores world state baselines
- Rebuilds token usage info
- Handles compaction checkpoints

---

## 7. How Context is Passed to the Model

### 7.1 Prompt Construction

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/session/turn.rs` (lines 1294-1310)

```rust
pub(crate) fn build_prompt(
    input: Vec<ResponseItem>,
    router: &ToolRouter,
    turn_context: &TurnContext,
    base_instructions: BaseInstructions,
) -> Prompt {
    Prompt {
        input,
        tools: router.model_visible_specs(),
        parallel_tool_calls: true,
        base_instructions,
        output_schema: turn_context.final_output_json_schema.clone(),
        output_schema_strict: !crate::guardian::is_guardian_reviewer_source(&turn_context.session_source),
    }
}
```

### 7.2 Sampling Request Input Preparation

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/session/turn.rs` (lines 349-356)

```rust
let sampling_request_input: Vec<ResponseItem> = async {
    sess.clone_history()
        .await
        .for_prompt(&turn_context.model_info.input_modalities)
}
.instrument(trace_span!("run_turn.prepare_sampling_request_input"))
.await;
```

The history is normalized for the model's input modalities (stripping unsupported images/audio).

### 7.3 Request Metadata Headers

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/client.rs` (lines 142-150)

Headers passed with each request:
- `X_CODEX_INSTALLATION_ID_HEADER` - Installation ID
- `X_CODEX_ROUTING_HINT_HEADER` - Routing hint
- `X_CODEX_TURN_STATE_HEADER` - Sticky routing token
- `X_CODEX_TURN_METADATA_HEADER` - Turn metadata
- `X_CODEX_PARENT_THREAD_ID_HEADER` - Parent thread ID
- `X_CODEX_WINDOW_ID_HEADER` - Auto-compact window ID

### 7.4 Model Client Session

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/client.rs` (lines 274-288)

```rust
pub struct ModelClientSession {
    client: ModelClient,
    websocket_session: WebsocketSession,
    turn_state: Arc<OnceLock<String>>,  // Sticky routing token
}
```

Per-turn session maintains WebSocket connection and turn state for sticky routing.

---

## 8. Context Priorities and Truncation

### 8.1 Truncation Policy

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/context_manager/history.rs` (lines 460-503)

```rust
fn process_item(item: &ResponseItem, policy: TruncationPolicy) -> ResponseItem {
    let policy_with_serialization_budget = policy * 1.2;  // 20% buffer
    match item {
        ResponseItem::FunctionCallOutput { output, .. } => {
            output: truncate_function_output_payload(output, policy_with_serialization_budget),
        }
        ResponseItem::CustomToolCallOutput { output, .. } => {
            output: truncate_function_output_payload(output, policy_with_serialization_budget),
        }
        // Other items passed through unchanged
    }
}
```

### 8.2 Function Output Truncation

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/context_manager/history.rs` (lines 553-570)

```rust
pub(crate) fn truncate_function_output_payload(
    output: &FunctionCallOutputPayload,
    policy: TruncationPolicy,
) -> FunctionCallOutputPayload {
    let body = match &output.body {
        FunctionCallOutputBody::Text(content) => {
            FunctionCallOutputBody::Text(truncate_text(content, policy))
        }
        FunctionCallOutputBody::ContentItems(items) => FunctionCallOutputBody::ContentItems(
            truncate_function_output_items_with_policy(items, policy, estimate_audio_token_count),
        ),
    };
    FunctionCallOutputPayload { body, success: output.success }
}
```

### 8.3 Context Window Exceeded Handling

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/compact.rs` (lines 309-318)

```rust
Err(e) if matches!(e.details(), CodexErrorDetails::ContextWindowExceeded) => {
    if turn_input_len > 1 {
        // Trim from the beginning to preserve cache (prefix-based) and keep recent messages intact.
        history.remove_first_item();
        retries = 0;
        continue;
    }
    // Single item exceeds window - mark as full
}
```

### 8.4 Pre-Sampling Compaction

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/session/turn.rs` (lines 993-1023)

```rust
async fn run_pre_sampling_compact(...) -> CodexResult<()> {
    let token_status = context_window_token_status(sess, turn_context).await;
    if token_status.token_limit_reached {
        let step_context = sess.capture_step_context(...).await?;
        run_auto_compact(
            sess,
            step_context,
            None,
            client_session,
            InitialContextInjection::DoNotInject,
            CompactionReason::ContextLimit,
            CompactionPhase::PreTurn,
        ).await?;
    }
}
```

### 8.5 Remote Compaction Context Window Trimming

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/compact_remote.rs` (lines 399-455)

```rust
pub(crate) fn trim_function_call_history_to_fit_context_window(
    history: &mut ContextManager,
    turn_context: &TurnContext,
    base_instructions: &BaseInstructions,
) -> (usize, i64) {
    // Iterate from newest to oldest, truncate function outputs
    for group in history_item_groups(...).rev() {
        if estimated_tokens <= context_window { break; }
        // Replace function outputs with "Output exceeded..." placeholder
        let rewritten_item = rewritten_output_for_context_window(...);
    }
}
```

### 8.6 History Item Removal (Rollback/Compaction)

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/context_manager/history.rs` (lines 273-285)

```rust
pub(crate) fn remove_first_item(&mut self) {
    if !self.items.is_empty() {
        let items = Arc::make_mut(&mut self.items);
        let removed = items.remove(0);
        normalize::remove_corresponding_for(items, &removed.item);
        self.world_state_baseline = None;
    }
}
```

### 8.7 Turn Rollback

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/context_manager/history.rs` (lines 298-360)

```rust
pub(crate) fn drop_last_n_user_turns(&mut self, num_turns: u32) {
    // Find user turn boundaries
    let user_positions = user_message_positions(&snapshot);
    // Calculate cut index
    // Trim pre-turn context updates
    // Replace history with truncated version
    self.replace_annotated(retained_items);
}
```

### 8.8 Compaction History Building Priority

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/compact.rs` (lines 652-683)

```rust
// Select messages from newest to oldest within token budget
for message in user_messages.iter().rev() {
    let tokens = approx_token_count(&message.message);
    if tokens <= remaining {
        selected_messages.push(message.clone());
        remaining = remaining.saturating_sub(tokens);
    } else {
        // Truncate the oldest retained message
        let truncated = truncate_text(&message.message, TruncationPolicy::Tokens(remaining));
        selected_messages.push(truncated_version);
        break;
    }
}
selected_messages.reverse();  // Restore chronological order
```

Priority order:
1. **Most recent user messages** (preserved first)
2. **Compaction summary** (always appended last)
3. **Initial context** (injected at model-expected boundary)

### 8.9 Remote Compaction V2 Retention Priority

**File:** `/tmp/opencode/refs/codex/codex-rs/core/src/compact_remote_v2.rs` (lines 512-528)

```rust
fn is_retained_for_remote_compaction_v2(item: &ResponseItem) -> bool {
    if let ResponseItem::AgentMessage { content, .. } = item {
        // Keep non-completion agent messages under 10k tokens
        return !is_completion && estimate_item_token_count(item) <= MAX_RETAINED_AGENT_MESSAGE_TOKENS;
    }
    let ResponseItem::Message { role, .. } = item else { return false; };
    matches!(role.as_str(), "user" | "developer" | "system")
}
```

---

## Summary

The Codex context management system is a sophisticated multi-layered architecture:

1. **Context Window**: Dynamic calculation based on model capabilities with configurable percentage
2. **History Management**: Copy-on-write `ContextManager` with normalization, token tracking, and versioning
3. **Token Budgeting**: Multi-level (per-turn, session-wide rollout budget, token budget reminders)
4. **Compaction**: Three strategies (local, remote v1, remote v2) with different trade-offs, plus token-budget manual reset
5. **File Context**: World State system with diff-based rendering across 15+ context sections
6. **Persistence**: Rollout-based with multiple history modes, auto-compact window tracking
7. **Model Input**: Normalized history + tools + base instructions via `Prompt` struct, with sticky routing
8. **Priorities**: Recent user messages > summary > initial context; function outputs truncated first; pre-sampling compaction prevents overflow

The system is designed for:
- **Efficiency**: Copy-on-write, diff-based context updates, token estimation without full tokenization
- **Correctness**: Call/output pairing invariants, modality-aware normalization
- **Flexibility**: Multiple compaction strategies, configurable scopes, extension points
- **Observability**: Comprehensive analytics, tracing, token usage tracking
