# Comprehensive Context Management Upgrade Plan

This document outlines a comprehensive upgrade plan for the Automergent context management subsystem (`internal/context/`), focusing on advanced token budgeting, JIT assembly, resilient compaction, AST-aware staleness detection, and fine-grained observability.

---

## 1. Executive Summary

The existing context management architecture in Automergent (`internal/context/manager.go`, `budget.go`, `compaction.go`, `jit.go`, `ranking.go`, etc.) provides a solid foundation for token allocation, staleness detection, and conversation history transcripting. However, as codebase complexity and agent autonomy scale, several opportunities for architectural refinement emerge:
- **Dynamic Semantic Density Weighting**: Moving beyond static token budgets to weight context items by structural relevance and semantic density.
- **AST-Aware JIT Assembly**: Upgrading Just-In-Time assembly to extract and inject precise symbol definitions and type signatures rather than raw file snippets.
- **Hierarchical Multi-Pass Compaction**: Implementing tiered conversation summarization that preserves critical decisions, failing test signatures, and active invariants across long multi-turn sessions.
- **Incremental Dependency Caching**: Enhancing dependency analysis and staleness tracking with AST change-hashing to avoid redundant re-analysis.
- **Enhanced Telemetry & Budget Probing**: Providing granular per-turn token utilization metrics, context window saturation alerts, and efficiency reporting.

---

## 2. Current Architecture Overview

The current subsystem comprises several modular components under `internal/context/`:
- **`Manager` (`manager.go`)**: Central coordinator integrating ranker, budget, detector, graph, analyzer, selector, summarizer, and transcripts.
- **`TokenBudget` (`budget.go`)**: Manages token quotas based on model limits (e.g., Gemini Flash limits).
- **`Ranker` (`ranking.go`)**: Scores files and context snippets based on recency, relevance, and frequency.
- **`StalenessDetector` (`staleness.go`)**: Detects stale files based on modification times and access patterns.
- **`DependencyAnalyzer` & `DependencyGraph` (`dependencies.go`)**: Maps code dependencies between packages and files.
- **`ContextSelector` / `assembly.go` / `jit.go`**: Selects and assembles relevant code for the active prompt.
- **`ContextSummarizer` / `compaction.go` (`compaction.go`, `summarizer.go`)**: Compresses transcripts and context when token limits approach thresholds.
- **`TranscriptManager` (`transcript_manager.go`, `transcript.go`)**: Maintains durable conversation logs.

---

## 3. Proposed Upgrade Pillars

### Pillar 1: Advanced Semantic Density & Adaptive Budgeting
- **Semantic Weighting**: Update `Ranker` (`ranking.go`) and `TokenBudget` (`budget.go`) to calculate token value density (ratio of symbols/definitions to boilerplate/comments).
- **Adaptive Allocation**: Dynamically reallocate token budgets between prompt context, reference files, and conversation history based on the current agent phase (Init, Plan, Build).

### Pillar 2: AST-Aware JIT Code Assembly
- **Symbol-Level Extraction**: Enhance `jit.go` and `assembly.go` to parse Go/TypeScript/Python ASTs when available, extracting only referenced functions, types, and interfaces rather than entire files.
- **Cross-File Pruning**: Integrate dependency graph weights directly into JIT selection to prune transitive dependencies that are unlikely to be modified.

### Pillar 3: Hierarchical Multi-Pass Compaction
- **Tiered Summarization**: Refine `compaction.go` and `summarizer.go` to use a 3-tier compaction strategy:
  1. *Micro-compaction*: Prune redundant tool outputs and superseded intermediate search results.
  2. *Macro-compaction*: Summarize completed task turns into immutable milestone blocks.
  3. *Invariant Retention*: Maintain a dedicated scratchpad for active file paths, failing test states, and explicit constraints.

### Pillar 4: AST-Aware Caching & Staleness Detection
- **Content Hashing**: Upgrade `StalenessDetector` (`staleness.go`) from file modification timestamps (`mtime`) to content-hash and symbol-hash tracking, preventing false positives on whitespace or comment-only edits.
- **Memoized Analysis**: Expand analysis cache in `Manager` with fine-grained dependency invalidation.

### Pillar 5: Deep Observability & Telemetry
- **Token Efficiency Metrics**: Extend `observability.go` to track token ROI (tokens consumed vs. successful tool actions/test passes).
- **Saturation Alarms**: Implement proactive warning triggers when context utilization exceeds 75%, 85%, and 95% thresholds.

---

## 4. Implementation Roadmap

1. **Phase 1: Analysis & Metrics Foundation**
   - Enhance `observability.go` and `budget.go` with detailed token breakdown telemetry and saturation warning hooks.
2. **Phase 2: AST-Aware JIT & Ranking Upgrades**
   - Implement symbol-level extraction in JIT assembly and update ranking heuristics with semantic density weights.
3. **Phase 3: Hierarchical Compaction Refinement**
   - Upgrade summarizer and compaction pipelines with tiered retention for constraints and test states.
4. **Phase 4: Testing & Verification**
   - Add comprehensive unit tests in `internal/context/` covering edge cases for compaction, adaptive budgeting, and AST JIT assembly.
