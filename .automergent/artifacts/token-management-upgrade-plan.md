# Token Management and Context Architecture Upgrade Plan

This document outlines a comprehensive architectural upgrade plan for token management, context budgeting, progressive compaction, and adaptive memory pruning across Automergent's codebase (`internal/context/`, `internal/agent/`, `internal/session/`).

## 1. Executive Summary & Objectives

Automergent handles large developer workflows with extensive file inspection, tool execution transcripts, and multi-turn subagent runs. As conversations grow, precise token budgeting, tiered compaction, and intelligent context ranking become critical to prevent context window exhaustion, reduce latency, and maintain reasoning fidelity. 

The primary objectives of this upgrade plan are:
- **Zero-Drop Context Preservation**: Ensure critical user goals, recent tool outputs, and architectural decisions are never lost during compaction.
- **Dynamic Token Budgeting**: Automatically adjust system reserves, tool definition allocations, and conversation limits based on the active model's exact context window (e.g., Gemini Flash 1M+, Gemini Pro 2M+, local/default 128k).
- **Multi-Tier Progressive Compaction**: Refine the existing 6-tier ladder (`ghost`, `truncate_middle`, `distill`, `snapshot`, `microcompact`, `full_compact`) with semantic clustering and dependency-aware pruning.
- **Advanced Observability & Telemetry**: Provide real-time token tracking per turn, per tool invocation, and per subagent chain.

---

## 2. Current State Analysis

The current token and context architecture spans several well-structured modules under `internal/context/`:
- `budget.go`: Defines `TokenBudget`, `ModelTokenLimits`, and percentage-based allocation configs (`DefaultBudgetConfig`, `StreamingBudgetConfig`).
- `compaction.go` & `pipeline.go`: Implements the tiered compaction ladder (`CompactionTier`, `CompactionStrategy`) and orchestration pipeline.
- `ranking.go` & `staleness.go`: Evaluates item importance, recency, and dependency chains.
- `summarizer.go` & `transcript.go`: Handles transcript slicing, formatting, and LLM-based distillation.
- `manager.go`: Coordinates context assembly and state updates.

### Identified Gaps & Areas for Improvement
1. **Static Percentage Assumptions**: Default percentage splits (`0.05` system, `0.40` conversation, `0.30` files) do not dynamically adapt when file inspection volume surges.
2. **Tool Result Bloat**: Large directory listings or file reads can quickly saturate `ConversationUsed` before compaction triggers at high thresholds (e.g., 75%-90%).
3. **Model Tier Interoperability**: Switching between models with vastly different window sizes (e.g., downshifting from 2M to 128k) needs smoother state projection and state refactoring.

---

## 3. Phase 1: Dynamic Token Budgeting & Model-Aware Allocation

### 3.1 Adaptive Budget Rebalancing
- Introduce runtime dynamic adjustment in `internal/context/budget.go` where context file budgets scale logarithmically with project size and active file count.
- Implement token usage feedback loops from tokenizer estimators to dynamically shift headroom to output reserves during long code generation tasks.

### 3.2 Granular Token Counting
- Integrate precise tokenizer approximations matching target providers (Gemini, Anthropic, OpenAI) to prevent truncation errors at API boundaries.
- Add real-time token telemetry hooks in `internal/context/observability.go`.

---

## 4. Phase 2: Enhanced Progressive Compaction & Distillation Pipelines

### 4.1 Semantic Distillation in Compaction Pipeline
- Upgrade `StrategyDistill` in `internal/context/compaction.go` to extract semantic dependency graphs rather than naive summarization.
- Retain exact error logs, failed test commands, and unresolved todos verbatim while abstracting repetitive intermediate file reads.

### 4.2 Microcompaction Tuning
- Optimize `StrategyMicrocompact` to target tool results older than $N$ turns, replacing bulky raw outputs with structured cryptographic diff summaries and file hashes.

---

## 5. Phase 3: Intelligent Context Pruning & Dependency Ranking

### 5.1 Dependency-Aware Retention
- Enhance `internal/context/dependencies.go` and `ranking.go` to compute graph centrality for referenced files and tools.
- Ensure files referenced in active todos or pending compiler diagnostics are exempt from eviction during compaction tiers.

### 5.2 Staleness Decay Function
- Implement exponential time-decay combined with reference-count weighting in `internal/context/staleness.go` to prioritize recently touched symbols.

---

## 6. Phase 4: Observability, Metrics, and Benchmarking

### 6.1 Token Dashboard & TUI Integration
- Expand context inspection tools (`internal/tools/ctxinfo/inspect.go`) and TUI statusbars (`tui/app/hud.go`, `tui/components/statusbar.go`) to display real-time token budget breakdown (System, Tools, Conversation, Files, Output, Available).
- Add automated diagnostic warnings when context utilization crosses 80%.

### 6.2 Automated Benchmarking Suite
- Add regression tests in `internal/context/` measuring compression ratio, semantic retention rate, and latency impact across synthetic large codebase transcripts.

---

## 7. Migration & Rollout Strategy

1. **Sprint 1 (Budgeting & Telemetry)**: Implement dynamic budget rebalancing and enhanced telemetry hooks.
2. **Sprint 2 (Compaction & Distillation)**: Upgrade distillation strategies and tool-result microcompaction.
3. **Sprint 3 (Ranking & Pruning)**: Wire dependency-aware retention into the ranking engine.
4. **Sprint 4 (TUI & Verification)**: Integrate HUD stats and run end-to-end integration tests.
