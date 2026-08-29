# Comprehensive Context Management Upgrade Plan

## Executive Summary & Architecture Overview

Automergent's context management system (`internal/context/`) is an advanced, production-grade architecture inspired by state-of-the-art coding agents (Claude Code, Codex, OpenCode). It spans **12 core subsystems**:
1. **Central Manager (`manager.go`)**: Coordinates budget, ranker, selector, staleness detector, dependency graph/analyzer, summarizer, transcript, adaptive calculator, and telemetry.
2. **Context Assembler (`assembly.go`)**: Phase-aware window builder allocating budgets across system prompts, messages, tools, files, and working sets.
3. **Compaction Pipeline (`compaction.go`, `pipeline.go`)**: 6-tier progressive degradation ladder (`ghost` → `truncate_middle` → `distill` → `snapshot` → `microcompact` → `full_compact` with probe verification).
4. **JIT Memory Loader (`jit.go`)**: Tiered loading across Global (`~/.automergent/*.md`), Project (`AUTOMERGENT.md`, `.claude/CLAUDE.md`), and Subdirectory (`.automergent.md`) scopes.
5. **Memory Store (`memory.go`)**: Persistent KV store with intent-based keyword relevance scoring.
6. **Adaptive Token Calculator (`adaptive.go`)**: Char-count heuristic refined via EMA gradient descent against live API ground truth token counts.
7. **Token Budget (`budget.go`)**: Dynamic allocation percentages across model context limits and safety margins.
8. **Staleness Detector (`staleness.go`)**: File freshness and modification tracking (`StateFresh`, `StateModified`, `StateStale`, `StateInvalid`).
9. **Semantic Ranker (`ranking.go`)**: Multi-factor relevance scoring (intent, edit distance, symbol matching, frequency, recency, dependency, freshness).
10. **Transcript Manager (`transcript.go`, `transcript_manager.go`)**: Durable conversation history with provenance links, replacement tracking, and API normalization.
11. **Dependency Analysis (`dependencies.go`)**: Import parsing and dependency graphs for Go, TypeScript, and Python.
12. **Observability & Telemetry (`observability.go`)**: Token breakdowns, compaction events, and cost tracking.

---

## Comprehensive Context Management Upgrade Plan

To elevate Automergent to peak world-class agentic capability, we propose a 6-pillar upgrade plan:

---

### Pillar 1: Semantic Retrieval & Vector/Hybrid RAG Upgrade
* **Current State**: Keyword overlap scoring (`extractKeywords`) and basic symbol indexing.
* **Upgrade Plan**:
  1. **Hybrid Retrieval**: Combine lexical keyword scoring, symbol match, and lightweight embedding-based semantic vector search (local embedding model or provider embedding API).
  2. **Chunk-Level Indexing**: Index large files into semantic chunks (functions, structs, classes) rather than whole-file granularity.
  3. **Reciprocal Rank Fusion (RRF)**: Merge lexical search, dependency graph proximity, and vector distance into a unified relevance score.

---

### Pillar 2: Provider-Side Prompt Caching & KV-Cache Optimization
* **Current State**: Dynamic assembly sent fresh every turn.
* **Upgrade Plan**:
  1. **Static/Dynamic Partitioning**: Strictly partition prompt windows into static prefixes (System Prompts, Tool Definitions, Core Rules in `AUTOMERGENT.md`) and dynamic suffixes (Conversation History, Current File Context).
  2. **Cache Control Integration**: Emit provider-specific cache control headers (e.g. Anthropic `cache_control: {"type": "ephemeral"}`, Gemini context caching blocks) around system prompts and foundational memory to maximize 90%+ cache hit discount rates.
  3. **Incremental Turn Append**: Ensure conversation history appends maintain strict prefix integrity so that provider KV caching remains valid across multi-turn interactions.

---

### Pillar 3: Advanced AST Symbol Intelligence & Structural Outlines
* **Current State**: Basic import parsing for Go, TS, Python.
* **Upgrade Plan**:
  1. **Robust AST Parsers**: Expand parsers to extract exact symbol signatures (functions, methods, interfaces, structs, variables, export declarations) and call graphs.
  2. **JIT Symbol Outlines**: When a file is too large for full context, automatically load its symbol outline (signatures and docstrings) first; fetch full file body on demand (JIT expansion).
  3. **Cross-Language Support**: Add AST parsers for Rust, Java, C++, and Python via treesitter-style lightweight regex/parser fallback.

---

### Pillar 4: Resilient Compaction & Probe Verification
* **Current State**: Tiered ladder with optional LLM distillation and probe verification stub.
* **Upgrade Plan**:
  1. **Autonomous Probe Verification**: Implement rigorous semantic assertions during `FullCompact` and `Snapshot` tiers: prompt the summary generator to verify that all active user goals, pending TODO items, and uncommitted file edits are preserved in the distilled summary.
  2. **Ghost Output Persistence**: Automatically flush large tool outputs (e.g. grep results, test outputs, file dumps > 4KB) to durable disk storage (`.automergent/store/`), injecting compact references that the model can inspect via tool call if needed.
  3. **Compaction Playback Safety**: Ensure rolled-back or compacted transcripts maintain unbroken provenance links so session rewinds (`/rewind`) never orphan summary nodes.

---

### Pillar 5: Multi-Agent Context Sync & Autonomous Memory Evolution
* **Current State**: Context buckets (`bucket_bridge.go`) and persistent memory store.
* **Upgrade Plan**:
  1. **Inter-Agent Context Sync**: Enable sub-agents (e.g. `contexter`, `explore`, `review`) to push structured findings (discoveries, bug hypotheses, test failures) into shared context buckets with atomic locks.
  2. **Self-Evolving Project Memory**: Add an autonomous background daemon that periodically analyzes completed session telemetry and successful task resolutions to propose updates to `AUTOMERGENT.md` or `.automergent.md` rules.

---

### Pillar 6: Observability, Diagnostics & Benchmarking Suite
* **Current State**: Telemetry collector persisting breakdowns and usage events.
* **Upgrade Plan**:
  1. **Context Dashboard UI**: Expand TUI context inspector (`/context`, `/stats`) to display live token allocation pie charts, cache hit ratios, and compaction history.
  2. **Context Efficiency Benchmarks**: Build an automated test suite (`internal/context/benchmark_test.go`) measuring token wastage, retrieval precision @ K, and compaction latency across standardized mock codebases.

---

### Implementation Priority Matrix

| Priority | Feature / Subsystem | Impact | Effort | Target Package |
| :--- | :--- | :--- | :--- | :--- |
| **P0** | Provider Prompt Caching headers (Anthropic/Gemini) | Massive cost & latency reduction | Medium | `internal/ai/`, `internal/context/` |
| **P1** | JIT Symbol Outlines & Structural File Summarization | High context precision | Medium | `internal/context/`, `internal/diagnostics/` |
| **P2** | Rigorous Probe Verification in Compaction Pipeline | Zero data loss during compaction | Low | `internal/context/compaction.go` |
| **P2** | Enhanced Hybrid Retrieval (RRF + Vector/Keyword) | Superior file ranking | High | `internal/context/ranking.go` |
| **P3** | Autonomous Project Memory Evolution (`AUTOMERGENT.md`) | Long-term learning | Medium | `internal/context/memory.go`, `jit.go` |
