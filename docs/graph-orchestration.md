# Graph Orchestration Contract

Automergent treats conversation, execution, memory, and context as separate
planes. The graph stores their relationships; it does not execute edits by
itself.

## Request Lifecycle

```text
user message
  |
  v
graph continuity analysis
  |-- relation, scope, risk
  |-- new / follow-up / related
  |-- context share policy
  |-- entry-point hints
  v
persisted task graph
  |-- task
  |-- workflow
  |-- todo-owned context buckets
  |-- decisions and lifecycle events
  v
staged prompt preparation (no tools)
  v
tool-capable agent loop
  |-- tool decision event
  |-- tool observation event
  |-- todo completion or block
  v
verification and assistant report
  v
ephemeral prompt cleanup
```

## Context Rules

- New tasks start isolated (`none`).
- Follow-ups resume the active task (`full` by default).
- Related tasks get a new task and share selected keys (`partial`).
- Summary-only requests share summaries, not raw execution history.
- Each todo owns a bucket. Dependencies share only declared keys.
- Injected prompts are ephemeral and are removed after use or expiry.
- User messages, decisions, verification evidence, and promoted memories are
  durable graph data.

## Execution Rules

- Analysis may suggest tools but cannot call or pre-fill them.
- Prompt stages run without tools and never claim task completion.
- Product wiring and code generation require an explicit execution call.
- A successful tool batch completes the active todo.
- A failed tool batch blocks the active todo and preserves failure context.
- Feature work always includes product entry-point review and verification.
- Follow-up work reuses unfinished workflows instead of duplicating them.

## Graph Visibility

The TUI command `/graph` shows the latest task's:

- relation, risk, and context policy;
- current intent and product entry-point hints;
- suggested tools, confidence, and deferred state;
- workflow and todo status;
- recorded lifecycle/tool event count.

The view exposes structured decisions and state. It does not expose hidden
model chain-of-thought.

## Extension Points

Tool categories are selected privately by the model for one request only and
mapped to a temporary native-tool profile. They are not written to session
messages, prompt context, graph metadata, or memories. New graphical views
should consume persisted graph nodes and edges rather than conversation text.
New tools should record observations through `GraphEngine` so relevance,
failures, and verification remain attributable to a task and context bucket.
