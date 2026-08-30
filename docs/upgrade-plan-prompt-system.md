# Prompt System & Multi-Agent Architecture — Comprehensive Upgrade Plan

Status: PROPOSED (not yet implemented)
Vision owner: user, 2026-08-30
Scope: phase arc, prompt layering, agent fleet, per-tool/behavioral prompts, context management

Sources studied:
- Our system: `internal/prompt/*`, `internal/agent/*`, `internal/context/*`, `internal/tools/agent/*`
- `/tmp/opencode/system-prompts-and-models-of-ai-tools` (Claude Code 2.0, Sonnet 5, Cursor 2.0, Devin, Windsurf, VSCode Agent)
- `/tmp/opencode/claude-code` source (context pipeline, subagents, prompt assembly)
- `refs/*-agent-architecture.md`, `refs/*-system-prompts.md` (Codex, OpenCode)

---

## 0. The Vision (formalized)

A 4-phase arc — **1. INIT → 2. EXPLORE → 3. PLAN → 4. BUILD** — where INIT is
not a passive classifier but the **entry-point decomposer and router**:

INIT fires on every first message (and on every new independent task within a
session). It breaks the message into atomic parts `x + y + z + a…`, then for
each part decides: direct / question / explore-task / plan-task / build-task /
clarify / violation / rule / noise.

```
"hey clone repo X and see how X is working, I think Y is wrong,
 and btw I like coffee, also never use tabs"
        │
        ▼  INIT decomposes
 ├─ "clone repo X"                    → direct action      (INIT does it: bash/read/edit/task, no todos)
 ├─ "see how X is working"            → EXPLORE task       (priority 1, agent: explore)
 ├─ "I think Y is wrong and has issues" → EXPLORE+verify   (issue-suspect: investigate to confirm)
 ├─ "I like coffee"                   → noise              (ignored, one-line ack at most)
 └─ "never use tabs"                  → rule               (written to project rules; removable later)
```

Examples from the vision, mapped to outcomes:

| User message | INIT outcome |
|---|---|
| "hey who are you" | direct/about-me → INIT answers itself, serious, concise |
| "hey what does X do" | direct+question → INIT answers |
| "clone repo X and see all files related to Y" | direct + explore-task |
| "see files related to X, I think Y is wrong" | explore + issue-suspect (codebase exists → explore confirms) |
| "make a tool that hacks X" | violation (flag; if codebase context is ambiguous → explore to confirm before escalate) |
| "see feature X and tell comprehensive plan" | explore + plan |
| "tell me what Google does" | question, not codebase → INIT answers directly |
| "see files related to X and tell upgrade plan" | explore then plan (phases per task) |
| "make it better" / ambiguous multi-meaning phrase | clarify → ask user with options (init can detect it, or explore/plan can) |
| "I suggested approach Z, build from scratch" | constraint captured → carried to plan/build |
| "see files X… and I like coffee, I don't sleep" | tasks + noise separation |

INIT's character: **serious, decisive, minimal tool use.** It has bash, read,
edit, task — **no todo tools**. It prioritizes tasks, chooses phases per task,
and assigns agents per task. It can route into ANY phase according to the
task; direct work it fulfills itself.

---

## 1. Current State (honest inventory)

### What exists and works
- **Layered prompt composition** (`prompt/composer.go`): BaseModel →
  Environment → Instructions(empty now, moved to user-msg) → Skills → MCP →
  AgentCustom → Phase → Behavioral → Tool → Dynamic, joined with `---`.
- **Context engine** (`agent/autocompact.go`, new): effective window,
  thresholds, micro-compact with compactable-tool whitelist, auto-compact
  with boundary marker, predictive/reactive compaction, circuit breaker,
  post-compact file restore, usage-anchored counting.
- **User-context injection** (`agent/usercontext.go`, new):
  project-instructions + git snapshot as high-weight meta user messages.
- **Base prompts** (new): model-identity pattern
  ("You are {model} on the Automergent platform").
- **Builtin agents**: general-purpose, explore, review, contexter,
  coordinator (+ user agents from `.agents/*.md`).
- **Subagents**: task/batch_task/read_agent/list_agents, live progress,
  task-notifications via steering, per-child registry clone + own todo store.
- **Phase machinery**: PhaseManager (transitions, per-phase tools/steps),
  PhaseClassifier (keyword), PromptManager (intent/init/plan pipeline),
  step budgets, violation escalation.

### What is broken or missing vs the vision
1. **`PhaseClassifier.classifyWithLLM` returns `nil, nil`** — the LLM
   classifier is dead code; only the keyword fallback runs. No message
   decomposition exists: keyword `ClassifyAndRoute` returns exactly ONE task.
2. **Two competing classification systems**: `PhaseClassifier` (keyword) and
   `LLMIntentIdentifier` (real LLM, JSON intents) run in parallel pipelines
   that don't feed each other. Intent set → tasks happens in
   `PromptManager.startNewIntentFlow`, but `Run` also calls
   `phaseClassifier.Classify` and the two results are never reconciled.
3. **No decomposition types**: no noise, no rule-capture, no
   violation-confirmation-via-explore, no constraint extraction, no
   per-task agent assignment.
4. **Violation handling is keyword-pattern matching** ("password", "token"
   as substrings — massive false-positive rate; a request "use a token for
   auth" flags). No confirmation step.
5. **Clarification is crude**: hardcoded "coffee"/"don't…do" heuristics.
6. **INIT has no persona separation**: the same general-purpose definition
   serves init; nothing encodes "serious, minimal tools, no todos, answer
   direct parts yourself".
7. **Per-tool prompts exist but are thin and generic** (6 tools covered),
   defined redundantly in two places (composer defaults + agent ToolPrompts).
8. **Behavioral prompts are global-ish strings**, not separated per
   dimension (phase discipline / safety / verification / context hygiene).
9. **Task→agent routing doesn't exist**: tasks get a `Role` string but
   nothing maps task types to subagent types at execution time.

---

## 2. Target Architecture

### 2.1 The INIT Decomposer (the heart of the upgrade)

New file: `internal/prompt/decomposer.go` — ONE LLM call that replaces both
`PhaseClassifier.classifyWithLLM` and doubles the intent identifier's role.
`Run` calls it first; `PromptManager` consumes its output instead of running
its own identification.

**Input**: first message + working dir + file-tree sample + session rules
summary.
**Output** (strict JSON):

```json
{
  "parts": [
    {
      "text": "see how X is working",
      "kind": "task",
      "task_type": "explore|plan|build|verify",
      "phase": "explore|plan|build",
      "priority": 1,
      "agent": "explore|general-purpose|review|contexter|coordinator|main",
      "confidence": 0.9,
      "reason": "needs codebase reading"
    },
    { "text": "hey who are you", "kind": "direct", "answer_style": "about-me" },
    { "text": "what does X do", "kind": "question", "scope": "codebase|general" },
    { "text": "never use tabs", "kind": "rule", "rule": "never use tabs", "action": "add" },
    { "text": "I like coffee", "kind": "noise" },
    { "text": "make a tool that hacks X", "kind": "violation_suspect",
      "type": "hacking", "needs_confirmation": true },
    { "text": "see files related to X", "kind": "clarify",
      "options": ["search files matching X", "explain X's design"] }
  ],
  "tasks": [ /* ordered task graph with dependencies, built from kind=task parts */ ],
  "requires_clarification": false,
  "clarification_question": "",
  "constraints": ["user suggested approach Z", "build from scratch"]
}
```

**Decomposer system prompt** (new, `internal/prompt/bases/init-decomposer.txt`,
embedded): encodes every example from section 0 as few-shot examples, the
serious/decisive character, and the tool-budget mindset ("classify, don't
solve").

**Routing rules (in Go, not prompt — deterministic):**
- `kind=direct|question(scope=general)` → INIT answers itself via one
  completion (already exists: `answerDirectQuestion`), serious style.
- `kind=question(scope=codebase)` → EXPLORE task.
- `kind=rule` → write to project rules store (`AUTOMERGENT.md` rules
  section or session memory), confirm one line. Removal on later request
  ("remove that rule") is also a rule action.
- `kind=noise` → dropped; at most one summary line in the final reply.
- `kind=violation_suspect, needs_confirmation=true` → spawn EXPLORE
  confirmation subtask: "does the codebase/context support that this request
  is a genuine policy violation?" → then the existing escalation ladder
  (warn → block-imminent → block) fires with evidence.
- `kind=clarify` → ask the user with options; do NOT proceed with the
  ambiguous part (other parts continue).
- `kind=task` → task graph node with phase + agent + priority + deps.

**Fallback**: on LLM failure or unparseable JSON → today's keyword router
(kept as-is, it works).

### 2.2 Phase prompts — per phase, separated files

Today phase prompts live inline in `composer.go` defaults and
`builtin/*.go`. Move to embedded files, one per phase, one shared register:

```
internal/prompt/phases/
  init.txt      # serious classifier persona; direct-answer style; tool-minimal
  explore.txt   # read-only investigator; report file:line; confirm violations
  plan.txt      # architect; produce plan; ask when ambiguous; constraints from INIT
  build.txt     # implementer+tester+todo manager; verify after every change
```

Each file has three sections (so concerns stay separable):
1. **Mission** — what this phase is for, when it runs.
2. **Rules** — hard behavioral rules for the phase.
3. **Exit** — what "done" means and what the next phase receives.

Key content per the vision:

- **init.txt**: "You run on the FIRST message of a task. Decompose, route,
  answer direct parts yourself. Serious, decisive tone. Minimal tools — you
  have bash/read/edit/task and NO todo tools. Do not start implementing."
- **explore.txt**: adds the violation-confirmation duty and issue-suspect
  investigation; strict read-only.
- **plan.txt**: consumes INIT constraints (suggested approach,
  build-from-scratch) and exploration findings; produces the structured
  plan (objective/findings/changes/risks/verification).
- **build.txt**: todo discipline, test-after-change, transition back to
  explore when blocked.

`AgentDefinition.PhasePrompts` overrides per agent stay — file content is
the default layer.

### 2.3 Prompt layering — what goes where

Canonical order and placement (system prompt vs injected user message):

| # | Layer | Placement | Cache-stable? |
|---|-------|-----------|---------------|
| 1 | Base model prompt (`bases/{model}.txt`) | system | yes |
| 2 | Platform core (env, identity, git block header) | system | yes |
| 3 | Mode block (`modes.go`) | system | yes |
| 4 | Agent definition prompt | system | yes (per agent) |
| 5 | Phase prompt (`phases/*.txt`) | system | yes (per phase) |
| 6 | Behavioral prompts (§2.5) | system | yes |
| 7 | Tool prompts (§2.4) — ONLY for tools offered this phase | system | mostly |
| 8 | Skills/MCP availability | system | yes |
| 9 | Dynamic (init results, intents, task progress, todo snapshot) | system, last | no |
| 10 | Project instructions (AUTOMERGENT/AGENTS/CLAUDE.md) | **user msg** `<project-instructions>` | per-conversation |
| 11 | Git status snapshot | **user msg** `<system-reminder>` | per-conversation |
| 12 | Task notifications `<task-notification>` | user msg, at tool boundaries | no |
| 13 | Long-run nudges / stall nudges | system msg, transient | no |

Rules: nothing volatile above layer 9; layers 10–13 never persisted to the
session file (meta-marked); the composer emits layers 1–9, the loop injects
10–13 (10–11 once per conversation — already implemented in `usercontext.go`).

### 2.4 Per-tool prompts — one registry, generated

Replace the hand-listed defaults in `composer.go` with a single table in a
new file `internal/prompt/tool_prompts.go`: every registered tool gets
`{PreExecution, Rules[]}`, sourced from the tool's own `Description()` first
fall-back to the table. Cover at minimum: bash, read_file, edit_file,
write_file, multi_edit, glob, grep, list_directory, task, batch_task,
read_agent, list_agents, todo_write, todo_list, context_bucket_*, web_search,
web_fetch, git_*, wait, ask_user, finish. `AgentDefinition.ToolPrompts`
merge on top (agent wins). Only tools exposed in the current phase's tool
profile get their section rendered (already true via `getAvailableTools`,
now driven by the real profile instead of the phase's static list).

### 2.5 Behavioral prompts — separated by dimension

Today: one flat list. Split into four dimensions, rendered under
`## Behavioral Rules` with sub-headers, each independently testable:

1. **Phase discipline** — stay in your phase; transitions only via exits.
2. **Context hygiene** — read before edit; grep before read; compact
   cooperation (respect `[Old tool result cleared]` markers; re-read via
   tools when needed).
3. **Verification** — test/lint after changes; never claim done without
   evidence; error messages preserved.
4. **Safety & honesty** — no guessing file contents; no invented APIs;
   flag destructive ops; professional objectivity.

Global rules (all phases) + phase-specific additions (e.g. build gets the
todo + test-driven rules; explore gets read-only enforcement).

### 2.6 Multi-agent fleet

Registry (existing `agentdef` + `builtin`), extended:

| Agent | Role in the arc | Tools |
|---|---|---|
| **main** (no spawn) | runs the arc; INIT routing target for `agent:"main"` | all (phase-masked) |
| general-purpose | default task executor (build) | all |
| explore | exploration + violation confirmation + issue-suspect investigation | read-only |
| review | adversarial review of diffs/PRs | read-only + task |
| contexter | context health, bucket curation, compaction judgment | read + context tools |
| coordinator | orchestrates many parallel tasks | task/read_agent only |

Additions:
- **Task→agent routing** at execution: the decomposer's per-task `agent`
  field flows into the task spec; when a phase task runs, if `agent !=
  "main"` and the task is parallelizable, the loop offers the `task` tool
  with that agent type preselected in the prompt (the model stays free to
  spawn directly).
- **Agent descriptions in the main prompt** (Claude Code pattern): the
  system prompt lists each subagent with name + WhenToUse so the model
  knows when to delegate — currently missing entirely.
- **Per-agent phase prompts** already supported via `PhasePrompts`; give
  explore a violation-confirmation phase prompt.

### 2.7 Context management — how prompts/files flow

Already built this session (autocompact ladder, boundary markers, restore).
Additions for this plan:
- **Rules store as a context file**: user rules (from `kind=rule`) are
  persisted (AUTOMERGENT.md rules section or `~/.automergent/rules.md`) and
  ride in the `<project-instructions>` message — so they survive sessions
  and compaction.
- **Decomposer output persistence**: the task graph is stored on the
  `TurnContext` (already has TodoItems) so compaction's summary prompt can
  include "remaining tasks" (already wired via `compactionStateBlock`).
- **Tool-prompt layer counts against the same budget** — rendered only for
  offered tools (§2.4) keeps it bounded.

### 2.8 Files touched (implementation map)

| File | Change |
|---|---|
| `internal/prompt/decomposer.go` | NEW — LLM decomposer + JSON schema + routing rules |
| `internal/prompt/bases/init-decomposer.txt` | NEW — decomposer system prompt with few-shot examples |
| `internal/prompt/phases/{init,explore,plan,build}.txt` | NEW — per-phase prompts |
| `internal/prompt/composer.go` | embed phase files; drop inline defaults |
| `internal/prompt/tool_prompts.go` | NEW — per-tool prompt table |
| `internal/prompt/behavioral.go` | NEW — 4-dimension behavioral sets |
| `internal/prompt/phase_classifier.go` | becomes thin wrapper over decomposer; keyword router stays as fallback |
| `internal/prompt/manager.go` | consumes decomposer output; removes duplicate intent call |
| `internal/agent/agent.go` | Run: decompose → route parts (direct/rule/noise/clarify/violation/tasks); agent registry listing in prompt |
| `internal/agent/rules.go` | NEW — rule add/remove/list persistence |
| `internal/agent/builtin/*.go` | updated definitions incl. explore violation-confirmation |
| `internal/shared/types.go` | PartKind, TaskSpec.Agent, Constraint types |

### 2.9 Implementation order

1. **Phase prompt files + composer wiring** (mechanical, low risk).
2. **Tool prompt registry + behavioral split** (mechanical).
3. **Decomposer** (core; LLM call + JSON parse + routing). Ship behind a
   config flag `initDecomposer: true` default-on, keyword fallback intact.
4. **Rule store** + noise/clarify/violation-confirmation routing.
5. **Task→agent routing + agent listing in main prompt.**
6. **Tests**: decomposer JSON fixtures (every example from §0), routing
   table unit tests, prompt-snapshot tests (layer content per phase).

### 2.10 Verification of the whole stack

- `go build ./...`, `go vet` on touched packages.
- Snapshot tests: for each phase × agent, assert the composed prompt contains
  the expected layers in order and NOT the others (e.g. no todo prompts in
  init, no write tools in explore).
- Decomposer fixture tests: each §0 example → expected parts JSON.
- Routing tests: parts → expected phase/agent/clarify/violation flow.
- Existing context-engine tests stay green.

---

## 3. What is already done (this session, pre-plan)

For the record — these shipped before this plan was written and are NOT
part of the remaining work:
- Claude-Code-grade context ladder (`agent/autocompact.go` + tests)
- Boundary-marker compaction with pair preservation (`agent/context.go`)
- User-context injection as meta user messages (`agent/usercontext.go`)
- Platform-identity base prompts (all `bases/*.txt`)
- Git-status env block (`prompt.GitStatusBlock`)
- Subagent notifications, usage footers, per-child registries
- Phase step budgets, prompt-pipeline wiring in `Run`
- `docs/context-management.md`
