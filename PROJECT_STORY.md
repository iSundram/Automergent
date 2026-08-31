# Project Story: Automergent

## About the Project

Automergent is a terminal-native autonomous AI coding agent built in Go on the Charm v2 stack (Bubble Tea, Lip Gloss, Glamour) with Tree-sitter AST parsing. It reads, writes, searches, tests, and refactors codebases autonomously or interactively, directly inside any terminal or SSH session.

---

## What Inspired This

I wanted an AI coding agent that lives in my terminal — no IDE plugin, no cloud dependency, just me and the command line. I had seen Claude Code and Cursor, but they felt like black boxes. I wanted something I could read, modify, and own. Something built on Go, using the Charm v2 stack — Bubble Tea, Lip Gloss, Glamour — because I believe the terminal deserves beautiful, responsive interfaces.

The name went through a journey too. I originally called it **OweCode** — the working directory is still `/root/OweCode/`. But I never updated the codebase to reflect that name. Instead, I chose **Automergent**: *auto* + *emergent*. The idea that an AI agent should autonomously emerge solutions from your codebase context, without you having to spell out every step. The Go module is `github.com/iSundram/Automergent`. No code references OweCode anymore — only the project directory carries the old name, like a fossil. A few hardcoded test paths still reference `/root/OweCode` as a working directory artifact.

Automergent started as a single `first commit` with 70 files and 4,535 lines. I scaffolded the entire architecture in one shot: agent loop, Gemini client, filesystem/shell/web/LSP/git/MCP tools, multi-theme TUI with Catppuccin and Dracula, session persistence, and OS-level sandboxing via bubblewrap (Linux) and seatbelt (macOS).

I thought I had it all figured out. I was wrong.

---

## How I Built It

### The TUI Saga

The terminal UI went through more rewrites than any other part of the project — at least **seven distinct architectural phases** visible in the git history.

I started with a **rounded-corner TUI**. The original `internal/tui/themes/theme.go` used `lipgloss.RoundedBorder()` extensively — in the input style, border style, and throughout the theme's `NewStyles` function. It looked gorgeous in screenshots but **broke constantly** in real terminals. Border rendering was inconsistent across terminal emulators. LipGloss's rounded styles conflicted with how Bubble Tea handled viewport scrolling. Every fix introduced new layout artifacts.

So I redesigned toward **line-based rendering**. The session picker commit (`c9e233b`) is explicit: *"redesign session picker to line-based theme, render inline, restore full conversation on resume."* The TUI immediately became more stable. I stopped fighting the terminal and started working with it. (Note: rounded corners were never fully purged — `lipgloss.RoundedBorder()` still appears in toasts, conversation panels, the model hub, and the welcome screen. "Scrapped" would be an overstatement; the real change was making line-based the primary structure.)

But I still wasn't satisfied. I threw away component after component — the conversation view, the header, the status bar, the command palette — and regenerated each one, asking the AI to help me design something that actually felt right. The commit history shows this relentless iteration:

- `feattui comprehensive uiux upgrade` — full pass across all components
- `feattui intelligent command palette` — Spotlight-style fuzzy search overlay
- `feattui enhance tool ui` — tool rendering and confirm dialog overhaul
- `feattui implement model thought` — thinking block rendering with `ThoughtSignature` parsing
- `feattui automatically show diff` — auto-open diff pane on file edits
- `feattui include tool context` — pass tool metadata to conversation view

Each one was a new attempt, a new dissatisfaction, a new rewrite.

**Tool cards** went from verbose multi-line displays to a **one-line-per-read design**. Commit `b619f63` introduced per-family renderer files (`tool_read.go`, `tool_edit.go`, `tool_terminal.go`, `tool_box.go`) where reads always show one line with L-range annotations and multi-file grouping, edits render via diff boxes with +/- stats, and terminal output shows tail boxes. Commit `d8936d7` followed up with de-duplication: *"single-read cards collapse to header + one detail line."*

The **command palette** went from a dropdown component (`75c7be4` — `CommandPalette` struct with `visible`, `cursor`, `items []PaletteItem`) to a **Spotlight-style centered overlay** with fuzzy search. Seven commits explicitly renamed it to *"spotlight palette"* in the overhaul message. Commit `4bc2fbb` added background dimming when the palette is visible — Spotlight behavior.

The **bottom dock** (`9ebfd55`) emerged as a way to show background shells and agents without cluttering the main conversation — a tray under the input with down-arrow focus flow, live refresh on agent events, and enter-to-inspect for shell output tails and agent transcripts.

Eventually, I stopped trying to make the TUI look like something it wasn't. I let the AI generate the rendering logic, then I rewrote it myself — component by component, line by line — until every pixel felt intentional. The current TUI is the product of that iteration: not perfect, but mine.

### The Prompt System Evolution

I started with a single monolithic system prompt, like every other AI tool. The original `internal/agent/context.go` contained `buildSystemPrompt()` — one function that concatenated the mode, working directory, `AUTOMERGENT.md` content, and extra context files into a single giant string. I added tool-specific prompts, then "about you" prompts, then context injection prompts. The prompt system grew into a tangled mess of string concatenation.

Then I tried **triage** — splitting the first message into categories. Commit `33d8db3` added `internal/agent/triage.go` with a 5-step triage protocol: Relevance Check → Initial Context → Context Discovery → Validation → Execution. It categorized requests before acting. It was better but still rigid.

The AI suggested I build a **graph system** for dynamic prompt routing. I did. It added a rule-based coordinator, a reasoning engine, a planner, a verifier — a massive machinery of if-then-else logic baked into Go code. The graph engine (`internal/graph/`) grew to 42 files and 17,814 lines, with analysis, continuity, healing, memory, wiring, workflow, tools, store, query, types, and migrations.

I believed it was great. The commit history shows my confidence: `feat: implement new internal/prompt system as primary intelligence pipeline`, `feat: 4-phase agent loop (init→explore→plan→build)`, `feat(prompt): INIT decomposer multi-phase arc`.

Then I started testing.

### The Great Purge

The "comprehensive upgrade" commit (`8d62d35`) was the turning point. I asked the AI to do a comprehensive upgrade of the system, and it delivered **50,336 insertions across 165 files** in a single commit. It added:

- `internal/learning/` — pattern recognition, personalization, feedback loops
- `internal/reasoning/` — rule-based reasoning engine, planner, verifier
- `internal/verification/` — task completion verification gate
- `internal/coordinator/` — multi-agent orchestration
- `internal/accessibility/` — screen reader, audio, keyboard accessibility
- `internal/installer/` — zero-config setup, shell integration
- `config.yaml` — a full configuration file
- Git tools suite and co-author machinery
- Dozens of other subsystems

I trusted it. I merged it. And then I started finding **dead code everywhere**.

**`config.yaml`** was never committed to git — it's a runtime config path (`~/.automergent/config.yaml`) that the code expects but doesn't ship. The actual configuration is handled by `internal/config/` with Viper. The YAML path is referenced in `internal/tui/app/host.go` as a user-facing config location, but it was never a checked-in file.

The **git tools** were removed in `b619f63`. I had added `internal/tools/git/tools.go` early on — git status, diff, commit tools for the agent. But every tool the AI has access to **consumes context tokens**. The git tools were rarely used by the agent and ate into the limited context window. The commit message says it plainly: *"removed git tools suite and fake co-author machinery."* The `internal/tools/git/` directory no longer exists.

The **git co-author** feature was the same story. I built a full TUI component (`coauthor_confirm.go`, 154 lines) and wired it through config and operations. It was polished, tested, and completely unnecessary. The AI doesn't need to know about co-authors to do its job. Removed in the same commit as git tools.

The **learning subsystem** was the biggest conceptual mistake. Commit `c027242` says it all: *"remove learning subsystem, it cannot train the ai model, delete package and its usages."* It deleted 14 files and 5,612 lines from `internal/learning/` — engine, feedback, learner, patterns, personalization, privacy, profile, storage, strategy, support, types. The entire directory is gone. It tried to "learn" from user behavior and adjust the model's responses — but **you cannot change a model's behavior from the outside**. The training happens at the GPU level, not in a Go package.

The **reasoning engine** was next. Commits `9015f24` and `27bec4d` removed the rule-based reasoning from the live path. It was a massive system that tried to pre-analyze every user message, categorize it, and route it through decision trees. But the LLM itself is already a reasoning engine — adding a rule-based reasoning layer on top of it was redundant and slow. Deleted.

The **graph engine** followed. Commits `a6ceaff` and `1130b6f` removed it entirely: *"removed graph engine"* and *"remove graph engine, use prompt pipeline as default."* The 42-file, 17,814-line `internal/graph/` directory was replaced by `internal/prompt/manager.go` (442 lines) and the task state store. The prompt system didn't need to be a graph — it needed to be **phased**.

The **verification gate** was removed in `5016830`: *"dropped shouldVerify/verifyTaskCompletion/triggerRecoveryTurn, consecutiveVerifications field, and the post-stop-condition gate loop."* The `internal/verification/` directory no longer exists.

The **LSP dependency** was replaced with my own `internal/diagnostics/` system — a package that parses compiler errors, detects root causes, and suggests recoveries. The `internal/lsp/` directory was removed. The diagnostics system has its own parsers for JSON output, a cache layer, comparison logic, and a registry.

The commit messages tell the story of a developer realizing that **less is more**: `remove learning subsystem, it cannot train the ai model`, `removed graph engine`, `remove rule-based reasoning from live path`, `remove verification gate`, `remove dead code: installer, lsp, performance, accessibility, sdk, diagnostics subdirs`.

### Background Agents and Shells

While cleaning up, I also **added** things that stayed. The background shell system was a game-changer — persistent PTY sessions that run underneath the main conversation, with live output tails and stall/size watchdogs. Commit `bfbf041` introduced production-parity shell management: file-backed output, process-group kill, persistent working directory, shell discovery. The shell package now contains `async.go`, `live.go`, `process.go`, `wait.go`, `output.go`, `env.go`, and `metadata.go`.

Sub-agents came next. The ability to spawn child agents that run in parallel, each with their own session and transcript, opened up new workflows. A sub-agent could explore the codebase while the main agent continued working. The prompt system's tool prompts reference this directly: *"Delegate to a subagent. Provide a complete, self-contained prompt."* and *"Launch several subagents in parallel with one call."* The build phase prompt instructs: *"Parallelize independent work by delegating to subagents with complete, self-contained prompts."*

The **artifact system** was inspired by tools I'd seen in the AI coding space. I added `/artifact` commands backed by `internal/prompt/workflow.go` — an `Artifact` struct and a workflow system that lets users create, review, and approve plans directly. The behavioral rules document it: *"Plan as artifact: write the implementation plan to `.automergent/artifacts/plan.md` and request review."* Users can approve plans, view diffs, and accept edits without friction — a single keystroke that keeps the agent moving.

The **approval system** (`internal/agent/approval_policy.go`, `internal/tui/app/approval_flow.go`) is a full session-persisted tool-scope permission system. `ToolApproval` records in `internal/session/session.go` let users grant "always allow" decisions per tool scope. The `/approvals` command manages them. Commit `e9c6945` wired persistence, project-scoped keys, mutex+snapshot saves, and crash recovery.

### The Gemini 3.5 Migration

I decided early on to build on the **Gemini 3.5 ecosystem**. The `internal/ai/google/client.go` lists the supported models:

```
gemini-3.5-flash, gemini-3.5-flash-lite, gemini-3.1-flash-lite,
gemini-3.1-pro-preview, gemini-3-flash-preview, gemini-2.5-pro
```

The default model is `gemini-3.6-flash`. Google's models were fast, cheap, and had the best tool-calling support at the time.

But the migration from raw HTTP to the official GenAI SDK (`google.golang.org/genai`) was painful. The raw HTTP client started as 165 lines of hand-rolled SSE parsing. I rewrote it multiple times — the git log for `internal/ai/google/client.go` shows approximately **110 commits** across branches. Key milestones:

1. **Non-streaming HTTP** (`82c8463`) — basic, functional, no streaming
2. **Basic SSE streaming** (`c767a77`) — 272 lines of chunked response handling
3. **Mutex-protected streaming** (`00b2858`) — race condition fixes with `sync.Mutex`
4. **Official GenAI SDK** (`5b5e39b`) — replaced everything with `google.golang.org/genai`

Each migration surfaced new errors. The Gemini API returned **double-v1beta URLs** when the base URL already contained the version path. The `stripAPIVersion()` function (`internal/ai/google/client.go`) strips trailing `/v1beta`, `/v1alpha`, `/v1` segments because *"the genai SDK appends its own API version to the base URL it is given."*

**Empty text parts** in the SSE stream caused 400 errors on the next request. The `isEmptyPart()` function checks every field on a `genai.Part` — text, thought, function call, function response, file data, inline data, code execution, video metadata, audio transcription, thought signature, tool call, tool response, part metadata, media resolution — and skips parts with no meaningful content.

**Thought signatures** needed to be preserved across turns for the thinking blocks to work. Commits `82fd71c` and `f58512b` specifically address: *"fixgoogle preserve thoughtsignature in"* — the `ThoughtSignature` field must round-trip through the raw parts metadata.

The **`receivedFinish` flag** in the streaming goroutine tracks whether the model already delivered its final response. A transport error after that point is just the server closing the SSE connection cleanly, not a failure. Without this flag, the agent would report errors on every successful completion.

I added **retrying with exponential backoff** via `internal/errors/retry.go` — `RetryWithValue[T any]()` with configurable policies: `DefaultRetryPolicy`, `AggressiveRetryPolicy`, `ConservativeRetryPolicy`. The retry logic wraps both non-streaming and streaming requests, with the streaming path using `iter.Pull2` to safely consume the iterator while allowing retries on the first response.

### The Monolith Problem

As features accumulated, certain files became monoliths. `internal/tui/app.go` grew to handle every event type, every view mode, every keyboard shortcut. `internal/tui/components/conversation.go` became a dumping ground for all conversation rendering logic.

I split them. The TUI was restructured into:

```
internal/tui/
├── app/          # Core application event loop, layout, view composition
├── commands/     # Slash command handlers and tips
├── components/   # Modular UI widgets (Header, StatusBar, Tools, Diff, Palette)
├── keys/         # Keyboard binding definitions
├── render/       # ANSI streaming parser, diff engine, Markdown renderer
├── themes/       # 11 color palettes and syntax styling engine
├── tips/         # Contextual tips and help content
└── tui.go        # Root TUI entry point
```

Each component became a focused, testable unit. The tool rendering was split into per-family dispatchers (`tool_read.go`, `tool_edit.go`, `tool_terminal.go`, `tool_box.go`). The agent underwent similar surgery — `TurnContext`, per-tool Meta prompts, and the phased prompt system all live in dedicated packages with clear boundaries.

### Debug Mode and Testing

Many features were added through a **debug mode** that let me test things in isolation before committing. The debug provider (`internal/debug/provider.go`) wraps an `ai.Provider` to log requests and responses. It's wired into the HUD: `primary = debug.NewDebugProvider(primary, logger)`. Commit `fd3f768` introduced it: *"feat: add debug provider, command package, and workflow tests."*

Session testing revealed concurrency bugs in atomic saves. Tool testing exposed argument coercion issues. The verification gate's deadlocks (`c8138fa`) were found through stress testing. Debug logging at `/tmp/opencode/automergent_debug.log` (`c9557f7`) was used during development and later cleaned up because *"file I/O per-token was a perf hazard"* (`382058a`).

The pattern was consistent: **add through debug mode, test in isolation, then either promote to production or delete**. The features that survived — background shells, sub-agents, the artifact system, the phased prompt system — all passed this filter. The ones that didn't — learning, reasoning, graph engine, verification gate — were deleted.

---

## What I Learned

**1. The biggest commits are the most dangerous.**
The 50K-line comprehensive upgrade taught me that scale ≠ quality. I merged it believing everything was great, only to find that most of what was added was dead weight. The lesson: **review the diff, not the commit message.**

**2. You cannot train a model from application code.**
The learning subsystem tried to modify model behavior through feedback loops. It was architecturally impossible. Some problems are not engineering problems — they are research problems.

**3. Rule-based systems fight LLMs, not help them.**
The reasoning engine, the coordinator, the verification gate — all tried to impose rigid logic on a system that thrives on flexibility. The LLM is the reasoning engine. My job is to give it the right context and get out of its way.

**4. The TUI is the product, not a wrapper.**
More commits touch the TUI than any other subsystem. Every component was rewritten multiple times until it felt right. The terminal interface is not incidental — it IS the experience.

**5. Simpler prompt architectures outperform complex ones.**
A single monolithic prompt was bad. A graph-based dynamic prompt system was worse. The phased approach — `init → explore → plan → build` — with per-phase prompt files and behavioral rules is the right balance of structure and flexibility.

**6. Every tool costs context tokens.**
Git tools, co-author prompts, learning systems — they all consume the AI's limited context window. If a tool doesn't directly help the agent do its job, it's not worth having. **Context is the scarcest resource.**

**7. External APIs are never as stable as their documentation.**
Google's Gemini API had multiple bugs that required multiple fixes. Double-v1beta URLs, empty parts, thought signature loss, clean SSE disconnects. Trust the code, not the docs.

**8. Delete aggressively.**
The git history shows a pattern: add, test, realize it's wrong, remove. The learning system, the graph engine, the reasoning engine, the verification gate, the LSP dependency, the git tools, the co-author feature — all removed. **The best code is the code you don't ship.**

---

## Challenges I Faced

1. **TUI rendering across terminals** — Rounded styles broke in many terminal emulators. Line-based rendering became the primary structure for stability.

2. **Race conditions everywhere** — The Gemini streaming client, session persistence, agent event channels, tool execution — all needed mutex protection. Three rounds of concurrency fixes (`00b2858`, `7728fab`, `c8138fa`).

3. **Google API quirks** — Double-v1beta URLs, empty parts causing 400 errors, thought signature preservation, clean SSE disconnects. At least four separate bugfix commits.

4. **The comprehensive upgrade trap** — Trusting a 50K-line AI-generated commit without sufficient review created extensive cleanup work.

5. **Dead code proliferation** — Git tools, co-author features, learning system, reasoning engine — all added with good intentions, all dead on arrival.

6. **Monolith files** — `app.go` and `conversation.go` grew into unmaintainable blobs before I recognized the need to split them into `app/`, `components/`, `render/`, `themes/`.

7. **Scope management** — The project kept growing in every direction. The hardest part was not adding features — it was knowing which features to remove.

8. **Naming confusion** — The project directory is still `OweCode/` from the original name, but the codebase references `Automergent` everywhere. A few test files still hardcode `/root/OweCode` as a working directory.

---

## Architecture

```
Automergent/
├── cmd/
│   └── automergent/          # CLI entrypoint & command bootstrapping
├── internal/
│   ├── agent/                # AI agent loop, orchestration, approval policies
│   ├── ai/                   # Provider abstraction (Google, OpenAI, Anthropic)
│   │   └── google/           # Gemini client with retry, streaming, SDK integration
│   ├── config/               # Viper configuration loader, validation, providers
│   ├── context/              # Budget management, dependency tracking, staleness
│   ├── debug/                # Debug provider for logging requests/responses
│   ├── diagnostics/          # Compiler error parsers, recovery, root cause analysis
│   ├── errors/               # Structured errors, retry policies with backoff
│   ├── git/                  # Git operations (blame, branch, conflict, semantic diff)
│   ├── mcp/                  # Model Context Protocol client & server
│   ├── prompt/               # Phased prompt system (init→explore→plan→build)
│   │   └── phases/           # Per-phase prompt files and behavioral rules
│   ├── sandbox/              # OS-level sandboxing (bubblewrap, seatbelt)
│   ├── session/              # Persistent transcripts, history, undo stack
│   ├── shared/               # Shared types and interfaces
│   ├── taskstate/            # Task state store for agent workflows
│   ├── tools/                # 43+ tool implementations across 13 categories
│   ├── tui/                  # Bubble Tea v2 TUI
│   │   ├── app/              # Core event loop, layout, view composition
│   │   ├── commands/         # Slash command handlers
│   │   ├── components/       # Modular UI widgets
│   │   ├── keys/             # Keyboard bindings
│   │   ├── render/           # ANSI parser, diff engine, Markdown renderer
│   │   ├── themes/           # 11 color palettes
│   │   └── tips/             # Contextual help
│   ├── version/              # Version information
│   └── workflow/             # Artifact system and workflow management
├── go.mod                    # Module: github.com/iSundram/Automergent
├── Makefile                  # Build, test, lint, release
└── install.sh                # Installation script
```

---

## The Numbers

| Metric | Value |
|:---|:---|
| Total commits (first-parent) | ~120+ |
| Largest single commit | 50,336 insertions (`8d62d35`) |
| Days from first commit to v0.1.0 | ~3 |
| Tool files | 43 (across 13 categories) |
| Color themes | 11 |
| AI providers | 5 (Gemini, OpenAI, Anthropic + DeepSeek/Ollama via OpenAI compat) |
| Prompt phases | 4 (init → explore → plan → build) |
| Prompt system files | 29 in `internal/prompt/` |
| Subsystems added then removed | 8+ (learning, graph engine, reasoning, verification, LSP, accessibility, git tools, co-author) |
| Google client rewrites | 4 major (non-streaming → SSE → mutex-protected → GenAI SDK) |
| TUI architectural phases | 7+ |

---

## Mathematical Model of Iteration

The project's evolution follows a **sawtooth pattern** of accumulation and pruning. The codebase size $S$ at time $t$ can be modeled as:

$$
S(t) = S_0 + \sum_{i=1}^{n} \left( \alpha_i \cdot \Delta_{\text{add}}(t_i) - \beta_i \cdot \Delta_{\text{remove}}(t_i') \right)
$$

where $\alpha_i$ is the fraction of added code that survives review, and $\beta_i$ is the fraction of accumulated code that gets pruned. Empirically:

$$
\lim_{t \to \infty} \alpha_i \approx 0.3, \quad \lim_{t \to \infty} \beta_i \approx 0.7
$$

Roughly **70% of any large addition is eventually removed or rewritten**. The project's quality is determined not by what is added, but by what is **kept after honest testing**.

The prompt system's complexity followed an **inverted-U curve**:

$$
P_{\text{complexity}}(t) = \begin{cases}
\text{increasing} & t < t_{\text{graph}} \\
\text{peak} & t = t_{\text{graph}} \\
\text{decreasing} & t > t_{\text{graph}}
\end{cases}
$$

where $t_{\text{graph}}$ marks the point where the graph engine was added — the moment of maximum complexity, before the purge began. The optimal prompt architecture turned out to be the simplest one that works: **phased, not graphed**.

The context efficiency of the toolset scales inversely with tool count:

$$
\eta = \frac{\text{useful tool outputs}}{\text{context tokens consumed}} \propto \frac{1}{|T|}
$$

where $|T|$ is the number of tools registered. Removing git tools, co-author prompts, and dead subsystems **increased** $\eta$ by reducing the search space the AI must navigate to find relevant tools.

The TUI component count oscillates with decreasing amplitude:

$$
C_{\text{TUI}}(t) = C_0 + \epsilon(t) \cdot \sin(\omega t + \phi)
$$

where $\epsilon(t) \to 0$ as $t \to \infty$ — each iteration adds fewer components and removes fewer, converging toward a stable architecture.

---

## Verification Notes

This document was written by reading git diffs and current codebase state. Some claims were verified precisely; others are based on commit messages and may not capture the full picture. Things change across commits — a feature removed in one commit may have been re-added, modified, or relocated in a later one. Nothing here should be taken as absolute without checking the current state of the code yourself.

**What was verified directly:**
- The comprehensive upgrade commit (`8d62d35`) is confirmed as 50,336 insertions across 165 files via `git show --stat`.
- The graph engine removal (`a6ceaff`) is confirmed: 42 files, 17,814 lines deleted from `internal/graph/`.
- The learning subsystem deletion (`c027242`) is confirmed: 14 files, 5,612 lines removed. The directory does not exist at HEAD.
- The verification gate removal (`5016830`) is confirmed: `internal/verification/` does not exist at HEAD.
- The git tools and co-author removal (`b619f63`) is confirmed: `internal/tools/git/` does not exist at HEAD.
- The LSP to diagnostics migration is confirmed: `internal/lsp/` does not exist, `internal/diagnostics/` does.
- The `stripAPIVersion`, `isEmptyPart`, and `receivedFinish` functions exist in `internal/ai/google/client.go` at HEAD.
- The retry system exists in `internal/errors/retry.go` with `RetryWithValue` and multiple policies.
- The phased prompt system exists with 29 files in `internal/prompt/`, including `phases/`, `composer.go`, `phase_classifier.go`.
- The debug provider exists in `internal/debug/provider.go`.
- Background shells and sub-agents are referenced in prompt tool prompts and TUI dock code.
- The artifact system exists in `internal/prompt/workflow.go` and is referenced in behavioral rules.
- The approval system exists across `internal/agent/approval_policy.go`, `internal/tui/app/approval_flow.go`, and `internal/session/session.go`.

**What may be slightly off:**
- **Theme count (11)** was checked against `internal/tui/themes/engine.go` at HEAD. If themes were added or removed in uncommitted changes or very recent commits, this number could be different.
- **Tool file count (43)** counts `.go` files under `internal/tools/` excluding tests. Some files are metadata or shared utilities, not individual tool registrations. The actual registered tool count depends on what `init()` functions register, which could differ from the file count.
- **"5 providers"** — the config system lists DeepSeek and Ollama as hidden/legacy providers using the OpenAI-compatible API. Whether they fully work or are just stubs at this point is unclear from the git history alone.
- **"Rounded TUI scrapped"** — `lipgloss.RoundedBorder()` is still used in many places. The claim that it was "scrapped" is an overstatement. The real change was making line-based the dominant structure, not eliminating rounded corners entirely.
- **"config.yaml dead"** — the file is never committed to git, but the code at `internal/tui/app/host.go` references it as a runtime config location. It's not "dead" in the sense that nothing uses it — it's just not a checked-in file. The original claim was imprecise.
- **The Google client rewrite count ("four times")** is based on major milestone commits. The actual git log for `internal/ai/google/client.go` shows ~110 commits across branches, many of which are small fixes or merges. Calling it "four rewrites" simplifies a messier reality.
- **The "5+ TUI architectures"** claim is based on distinct structural change commits. What counts as a new "architecture" versus an incremental refactor is subjective.

**What could not be fully verified:**
- Whether the phased prompt system (`init → explore → plan → build`) is truly the **current and final** architecture, or whether it was modified after the last commits I read. The git history is deep and branches may have diverged.
- Whether the learning subsystem was truly the only approach that "cannot train a model" — this is the author's stated reasoning, not a technical proof.
- The exact context token cost of each tool — the claim that tools "consume context tokens" is architecturally correct (tools appear in the prompt), but the precise impact was not measured.
- Whether all removed subsystems (graph engine, reasoning, verification) were truly inferior, or whether they were removed due to implementation quality rather than architectural invalidity. A well-implemented graph system might have worked; the one that was built may have just been poorly executed.
- The claim that "70% of large additions are removed" is an impression from the commit history, not a precise measurement.

**Bottom line:** This story is the author's honest account of building the project, verified against git evidence where possible. Some details may be imprecise, some characterizations may be biased by the author's experience, and the current state of the codebase may differ from what the git history suggests. Read the code, not just the story.
