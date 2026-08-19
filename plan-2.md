# Autonomous Coding Agent — Architecture & @Thinking Plan

## 0. Product Definition

### Working concept

An autonomous senior software-engineering agent that owns the engineering loop rather than only generating code.

> Give it an engineering goal. It understands the task, selects only the context it needs, creates a workflow, delegates work to specialized agents, executes tools, detects and diagnoses failures, repairs them, verifies the result, remembers durable engineering knowledge, and reports evidence back to the user.

### Core product principle

**The LLM never receives “the conversation” or “the repository” by default. It receives a task-specific context packet assembled by the Context Engine.**

### Core distinction

- **Memory stores knowledge.**
- **Context is a query result.**
- **Conversation is not memory.**
- **Execution history is not automatically context.**
- **A task owns its execution context.**
- **Project and user knowledge persist independently of individual tasks.**

---

# 1. High-Level Architecture

```text
                              USER
                                │
                                ▼
                       ┌─────────────────┐
                       │ Conversation UI │
                       └────────┬────────┘
                                │
                                ▼
                  ┌──────────────────────────┐
                  │  COORDINATOR / BRAIN    │
                  │                          │
                  │ classify                 │
                  │ prioritize               │
                  │ clarify                  │
                  │ create task              │
                  │ create workflow          │
                  │ route / delegate         │
                  └────────────┬─────────────┘
                               │
                ┌──────────────┼─────────────────┐
                │              │                 │
                ▼              ▼                 ▼
         User Memory      Project Memory      Task Memory
                │              │                 │
                └──────────────┼─────────────────┘
                               │
                               ▼
                    ┌────────────────────┐
                    │   CONTEXT ENGINE   │
                    │                    │
                    │ retrieve          │
                    │ rank              │
                    │ compress          │
                    │ deduplicate       │
                    │ budget            │
                    │ assemble          │
                    └─────────┬──────────┘
                              │
              ┌───────────────┼────────────────┐
              ▼               ▼                ▼
           EXPLORER         CODER            TESTER
              │               │                │
              └───────────────┼────────────────┘
                              ▼
                          REVIEWER
                              │
                              ▼
                       TOOL EXECUTION
                              │
                              ▼
                         OBSERVATIONS
                              │
                              ▼
                        MEMORY UPDATE
                              │
                         ┌────┴────┐
                         │         │
                       PASS      FAIL
                         │         │
                         ▼         ▼
                      REVIEW     DEBUGGER
                                   │
                                   ▼
                                  FIX
                                   │
                                   └───────► VERIFY
```

---

# 2. The Two Primary Planes

The system should be divided into two major planes.

## 2.1 Conversation / Coordination Plane

Responsible for interacting with the human.

```text
User
  ↓
Coordinator
  ↓
Task
  ↓
Workflow
  ↓
Compact result
  ↓
User
```

The coordinator should not become a giant coding context.

## 2.2 Execution Plane

Responsible for actually completing work.

```text
Task
  ↓
Workflow Engine
  ↓
Specialized Agents
  ↓
Tools
  ↓
Observations
  ↓
Verification
  ↓
Task Result
```

The user's conversation stays clean while execution can be extremely detailed internally.

---

# 3. Coordinator

The Coordinator is the agent that primarily talks to the user.

## Responsibilities

1. Understand the human request.
2. Detect whether the request is a new task or continuation.
3. Classify intent.
4. Determine urgency and priority.
5. Detect ambiguity.
6. Decide whether to answer, ask, plan, or execute.
7. Create/update a Task object.
8. Create a workflow when execution is required.
9. Start specialized agents.
10. Receive summarized results rather than raw execution history.
11. Decide what the user needs to know.

## Coordinator should NOT

- read the whole repository by default;
- receive all previous coder messages;
- receive every tool output;
- receive every historical task;
- modify files directly unless explicitly designed to have that capability;
- replay old contexts merely because they exist.

---

# 4. Task Classification

Every user request should first become structured metadata.

## Primary intent categories

```text
NEW_FEATURE
BUG_FIX
REFACTOR
ISSUE_INVESTIGATION
QUESTION
DIRECT_COMMAND
INSTALLATION
CONFIGURATION
REVIEW
TEST
EXPLANATION
PLAN
GENERAL_CONVERSATION
```

## Secondary properties

```text
scope:
  file
  module
  subsystem
  repository
  multi_repository

risk:
  low
  medium
  high
  destructive

complexity:
  trivial
  small
  medium
  large
  architectural

execution_mode:
  informational
  single_agent
  sequential
  parallel
  human_approval_required

priority:
  low
  normal
  high
  critical
```

Example:

```json
{
  "intent": "BUG_FIX",
  "scope": "subsystem",
  "risk": "medium",
  "complexity": "medium",
  "execution_mode": "single_agent",
  "priority": "high",
  "requires_clarification": false
}
```

---

# 5. No Giant Repeated System Prompt

Do not repeatedly send a long universal prompt such as:

```text
You are a senior engineer...
Here is who you are...
Here are 200 rules...
Here is the complete conversation...
Here is the complete repository...
```

Instead use small **agent contracts** plus task-specific context.

## Coordinator contract

```text
Interpret the user's goal.
Decide whether clarification is required.
Classify the task.
Produce structured task instructions.
Do not fabricate repository facts.
Do not modify files unless explicitly allowed.
```

## Coder contract

```text
Implement the assigned task.
Respect task scope and constraints.
Use only supplied/retrieved context when possible.
Modify only authorized files.
Run required verification.
Return structured results and evidence.
```

## Tester contract

```text
Verify the expected behavior.
Prefer deterministic tests.
Do not silently change product code.
Return structured failures with evidence.
```

## Reviewer contract

```text
Review the change against requirements.
Check regressions, correctness, security, and architecture.
Do not modify production code.
Return findings with severity and evidence.
```

---

# 6. Memory Architecture

Memory should not be one undifferentiated store.

## L0 — Current Turn Memory

Short-lived data.

```text
current user message
current tool output
current agent result
current observation
```

Usually expires after the current operation unless promoted.

## L1 — Active Task Memory

Exists for the lifetime of a task.

```text
Task ID
goal
intent
constraints
priority
plan
todos
current step
files touched
decisions
errors
attempts
results
open questions
verification state
agent states
```

Example:

```text
TASK-1042

Goal:
Add OAuth login.

Status:
IN_PROGRESS

Completed:
- provider configuration
- user schema
- login endpoint

Current:
callback handler

Blocked:
callback URL mismatch

Files:
auth/provider.go
auth/callback.go
models/user.go

Known issue:
Existing session middleware assumes local login.
```

## L2 — Project Memory

Long-lived repository knowledge.

```text
architecture
modules
services
data flows
technology stack
build commands
test commands
coding conventions
error handling conventions
logging conventions
security constraints
important files
architecture decisions
known limitations
```

## L3 — User Memory

Persistent user-specific preferences.

Examples:

```text
coding style
communication style
approval rules
risk tolerance
preferred architecture patterns
preferred testing strategy
```

Memory must track provenance.

```json
{
  "fact": "Ask before destructive operations",
  "source": "user_explicit",
  "confidence": 1.0,
  "scope": "global",
  "created_at": "...",
  "last_confirmed": "..."
}
```

Inferred preferences should have lower confidence and should be treated as revisable.

## L4 — Episodic Memory

Past task experiences that may be useful later.

```text
problem
attempts
failures
root cause
final solution
lessons
files involved
verification
```

Example:

```text
Past task:
Authentication timeout

Attempts:
1. changed session initialization -> failed
2. changed middleware ordering -> failed

Root cause:
middleware executed before session hydration

Final fix:
move hydration after validation

Lesson:
authentication failures in this project can originate
from request lifecycle ordering
```

## L5 — Repository / Code Knowledge

Structured code intelligence.

### File record

```text
path
language
size
hash
symbols
imports
exports
tests
references
```

### Symbol record

```text
name
type
file
line range
callers
callees
references
```

### Relationship record

```text
imports
calls
inherits
implements
tests
references
```

This allows context retrieval based on code relationships rather than only text similarity.

---

# 7. Memory Promotion

Not everything the model sees should become durable memory.

```text
Observation
    ↓
Candidate memory
    ↓
Is it durable/useful?
    ├── NO  → discard
    └── YES → store
```

Examples:

### Do not store

```text
I misspelled a variable once.
A transient test failed because I used the wrong command.
```

### Store

```text
This project uses UTC timestamps everywhere.
Generated files must never be edited manually.
The billing module requires transactional writes.
The user explicitly prefers small functions.
```

Every durable memory should retain:

```text
what
source
source ID
confidence
scope
created time
last confirmed time
```

---

# 8. Context Engine

The Context Engine is one of the core differentiators of the product.

Its job is to answer:

> **What is the minimum sufficient information this particular agent needs to make the current decision correctly?**

Pipeline:

```text
Current task
    ↓
Determine required context types
    ↓
Retrieve candidate context
    ↓
Score relevance
    ↓
Filter
    ↓
Compress / summarize
    ↓
Deduplicate
    ↓
Apply token budget
    ↓
Build Context Packet
    ↓
Send to agent
```

---

# 9. Context Profiles by Task Type

Different tasks require different context.

## BUG_FIX

```text
required:
- bug description
- observed behavior
- expected behavior
- stack trace
- affected files/symbols
- related tests
- recent changes
- architecture constraints
- similar historical failures
```

Exclude unrelated project areas.

## NEW_FEATURE

```text
required:
- requirements
- acceptance criteria
- relevant architecture
- related modules
- code conventions
- tests
- API boundaries
- user constraints
- relevant architectural decisions
```

## DIRECT_COMMAND

```text
required:
- exact command/request
- execution environment
- target files
- safety constraints
```

Do not add planning history unless required.

## INSTALLATION

```text
required:
- operating system
- runtime/language
- repository configuration
- existing installation state
- project manifest
- required version constraints
```

## REVIEW

```text
required:
- requested requirements
- changed files
- diff
- tests
- relevant architecture
- security constraints
```

---

# 10. Context Packet

Every LLM invocation should receive a structured Context Packet.

```json
{
  "task_id": "TASK-1042",
  "agent_id": "coder-17",
  "agent_role": "coder",
  "objective": "Add OAuth callback support",
  "contract": "CODER_CONTRACT",
  "user_request": "...",
  "constraints": [
    "do not change public API",
    "preserve password login"
  ],
  "acceptance_criteria": [
    "OAuth callback creates session",
    "existing login still works",
    "tests pass"
  ],
  "project_context": [],
  "source_context": [],
  "test_context": [],
  "error_context": [],
  "memory_context": [],
  "tool_observations": [],
  "previous_decisions": [],
  "token_budget": {
    "max_input_tokens": 36000
  }
}
```

This becomes the canonical boundary between memory and model inference.

---

# 11. Context Relevance Scoring

Potential context should be ranked before inclusion.

Conceptually:

```text
relevance =
    semantic_similarity
  + task_scope_match
  + file_relationship
  + symbol_relationship
  + dependency_relationship
  + recency
  + error_relationship
  + historical_relevance
  + user_priority
```

Example:

```text
checkout.go                 0.98
payment.go                  0.91
checkout_test.go            0.89
session.go                  0.84
architecture/billing.md    0.71
README.md                   0.31
dashboard.go                0.01
```

Retrieve until the context budget is filled or the relevance threshold is reached.

---

# 12. Token Budgeting

Every agent invocation should have an explicit budget.

Example:

```text
Agent contract       2K
Task state           2K
Project summary      3K
Relevant source     18K
Relevant tests       5K
Errors               3K
Relevant memory      3K
────────────────────────
Total               36K
```

The goal is **minimum sufficient context**, not maximum context.

The system should measure context usage and effectiveness over time.

---

# 13. Context Receipt

Record exactly what was sent to every model call.

```text
CONTEXT RECEIPT

Task: TASK-1042
Agent: coder
Intent: BUG_FIX

Included:
✓ error log
✓ checkout.go
✓ checkout_test.go
✓ session architecture note
✓ recent auth change
✓ similar historical failure

Excluded:
✗ unrelated frontend
✗ old conversation
✗ unrelated tasks
✗ full repository

Input tokens: 18,432
Output tokens: 4,821
```

This gives the product measurable evidence that context selection is working.

---

# 14. Coordinator → Coder Handoff

Do not hand off using only free-form prose.

Use a typed handoff object.

```json
{
  "task_id": "TASK-1042",
  "objective": "Add OAuth login using existing user/session architecture",
  "intent": "NEW_FEATURE",
  "scope": [
    "auth/",
    "models/user.go",
    "middleware/session.go"
  ],
  "constraints": [
    "Do not change public API",
    "Preserve existing password login",
    "Do not introduce unauthorized runtime dependencies"
  ],
  "acceptance_criteria": [
    "OAuth login succeeds",
    "Existing login still works",
    "Callback errors are handled",
    "Tests pass"
  ],
  "priority": "high",
  "verification_required": true,
  "context_requests": [
    "auth architecture",
    "session middleware",
    "user model",
    "authentication tests"
  ]
}
```

Coder receives:

```text
CODER CONTRACT
+
HANDOFF OBJECT
+
USER ORIGINAL REQUEST
+
RETRIEVED CONTEXT
+
CURRENT TASK STATE
```

Not the complete conversation.

---

# 15. Conversation Context vs Execution Context

The system should deliberately separate them.

```text
USER
  ↕
COORDINATOR
  ↕
TASK / WORKFLOW
  ↕
EXECUTION AGENTS
```

Coder may have 30 internal turns:

```text
grep
read
edit
test
failure
inspect
edit
test
review
```

Those should not automatically become the user's conversation context.

Instead the Coordinator receives a compact structured result:

```text
TASK RESULT

status: completed
summary: ...
root_cause: ...
files_changed: ...
verification: ...
issues: ...
next_action: ...
```

The Coordinator turns that into the appropriate user-facing response.

---

# 16. Workflow Engine

Once a task is classified, the coordinator creates a workflow.

Example:

```text
Understand
   ↓
Inspect
   ↓
Plan
   ↓
Implement
   ↓
Test
   ↓
Review
```

Failure path:

```text
Test
  ↓
FAIL
  ↓
Diagnose
  ↓
Repair
  ↓
Re-test
```

The workflow should be a **state machine / durable execution graph**, not a prompt saying “keep trying until done.”

---

# 17. Workflow State

Example states:

```text
CREATED
CLASSIFYING
PLANNING
WAITING_FOR_APPROVAL
RUNNING
BLOCKED
VERIFYING
REPAIRING
REVIEWING
COMPLETED
FAILED
CANCELLED
PAUSED
```

Each state transition should be persisted.

This makes execution resumable after interruption.

---

# 18. Large Tasks → DAG

When a task is large, the Coordinator/Orchestrator should determine whether subtasks are independent.

Example:

```text
                    BILLING FEATURE
                          │
             ┌────────────┼────────────┐
             ▼            ▼            ▼
          Schema         API          Tests
             │            │            │
             └────────────┼────────────┘
                          ▼
                     Integration
                          │
                          ▼
                        Review
```

Independent work can execute concurrently.

Dependent work waits for prerequisites.

The workflow should explicitly represent:

```text
task dependencies
inputs
outputs
artifacts
required context
agent permissions
completion criteria
```

---

# 19. Specialized Agents

Start with a small set; add roles only when they provide a real benefit.

## Coordinator

Human interaction + task understanding + routing.

## Explorer / Analyst

Repository discovery, architecture mapping, relevant-code identification.

## Planner

Breaks requirements into executable steps and dependencies.

## Coder

Makes implementation changes.

## Tester

Creates/runs tests and verifies behavior.

## Debugger

Analyzes failures and proposes root-cause repairs.

## Reviewer

Checks correctness, regressions, architecture, and security.

## Security Reviewer

Optional specialized role for high-risk changes.

Do not make every role an LLM by default. Use deterministic code where deterministic code is better.

---

# 20. Deterministic vs LLM Responsibilities

Prefer deterministic systems for:

```text
filesystem traversal
Git diff
file hashing
process execution
compiler invocation
test invocation
permission checks
workflow state transitions
token counting
schema validation
policy enforcement
log collection
artifact tracking
```

Use LLM reasoning for:

```text
requirements interpretation
architecture understanding
task decomposition
root-cause hypotheses
repair proposal
code generation
semantic review
ambiguity resolution
```

This keeps the system cheaper, more reliable, and easier to debug.

---

# 21. Agent-to-Agent Communication

Agents should communicate through structured messages rather than uncontrolled natural-language conversations.

Example:

```json
{
  "message_id": "MSG-8821",
  "from": "coder-17",
  "to": "tester-03",
  "type": "TASK_READY",
  "task_id": "TASK-1042",
  "payload": {
    "changed_files": [
      "auth/callback.go"
    ],
    "expected_behavior": [
      "valid OAuth callback creates a session"
    ]
  }
}
```

Tester reply:

```json
{
  "message_id": "MSG-8822",
  "from": "tester-03",
  "to": "coder-17",
  "type": "TEST_FAILURE",
  "task_id": "TASK-1042",
  "payload": {
    "test": "TestOAuthCallbackExpiredState",
    "error": "state mismatch",
    "severity": "high"
  }
}
```

This makes inter-agent communication compact, inspectable, resumable, and auditable.

---

# 22. Peer Context Sharing

If one agent learns something relevant to another agent, do not forward its entire conversation.

Create a small structured discovery.

```text
DISCOVERY

Source:
Explorer agent

Fact:
Session state is initialized in middleware/session.go.

Confidence:
0.94

Relevant to:
auth/*
checkout/*
```

Only agents whose task matches the scope should receive it.

---

# 23. Resumable Bundled Context

Every active task should have a resumable state bundle.

```text
TASK BUNDLE
├── task metadata
├── workflow state
├── current step
├── pending work
├── completed work
├── changed files
├── artifacts
├── decisions
├── unresolved questions
├── current errors
├── test results
├── agent states
├── relevant memories
└── next action
```

If the process crashes at step 7/13, resume from the persisted bundle.

Do not replay the entire LLM conversation.

Example:

```text
TASK-1042

Step: 7/13
Agent: tester
State: FAILED

Current failure:
state mismatch

Already attempted:
- changed callback parsing

Do not repeat:
- callback parsing change

Next action:
inspect session state lifecycle
```

---

# 24. New User Task Boundary

A new user task should not inherit the previous task's execution context.

Example:

```text
Task A:
Fix authentication.

...

User:
Now optimize the image pipeline.
```

The Coordinator creates:

```text
TASK-1043
intent: REFACTOR / OPTIMIZATION
```

Old task context is excluded unless a relevant memory is retrieved.

Persistence rules:

```text
Conversation memory → scoped
Task memory        → scoped
Workflow state     → scoped
Project memory     → persistent
User memory        → persistent
Episodic memory    → retrievable
Code knowledge     → persistent/indexed
```

---

# 25. @Thinking Architecture

Do not store raw hidden chain-of-thought as a product feature.

Instead create a structured reasoning artifact called `@Thinking`.

Purpose:

- make the engineering decision process inspectable;
- preserve useful reasoning state across workflow steps;
- avoid replaying long reasoning transcripts;
- provide structured inputs to other agents.

## @Thinking schema

```text
@Thinking

Objective
Constraints
Known facts
Unknowns
Assumptions
Risk assessment
Relevant context
Hypotheses
Plan
Alternatives considered
Decision
Confidence
Verification strategy
Open questions
```

Example:

```text
@Thinking

Objective:
Fix checkout timeout.

Known facts:
- timeout occurs after payment authorization
- database connection is healthy
- failure appears under retry

Unknowns:
- whether retry loses request state
- whether timeout is local or network-related

Hypothesis:
Retry middleware loses request context.

Plan:
1. inspect retry middleware
2. trace request lifecycle
3. reproduce failure
4. patch smallest root cause
5. add regression test
6. run complete verification

Risk:
Medium

Decision:
Investigate middleware ordering before changing database code.

Confidence:
0.72

Verification:
checkout integration suite + targeted regression test
```

The useful structured fields can be passed to downstream agents.

---

# 26. @Thinking Should Be Updated, Not Rewritten

Each agent can append a new structured reasoning event:

```text
@Thinking.Event

agent: debugger-02
kind: HYPOTHESIS_REJECTED
reason: stack trace shows failure occurs after session hydration
```

Then:

```text
@Thinking.Event

agent: debugger-02
kind: ROOT_CAUSE_IDENTIFIED
root_cause: middleware refresh timestamp calculated before token validation
confidence: 0.91
```

This creates a compact decision history rather than a huge transcript.

---

# 27. Failure Memory

A major differentiator should be preserving failed attempts.

Example:

```text
Failure Memory

Error:
TypeError: user.session is undefined

Attempt 1:
Changed session initialization.
Result: failed.

Attempt 2:
Changed middleware order.
Result: failed.

Root cause:
Middleware executes before session hydration.

Final fix:
Move hydration after validation.
```

Future tasks can retrieve this memory when the problem is similar.

The system should avoid repeating known failed approaches unless new evidence justifies them.

---

# 28. Senior Agent Behavior

“Senior” should be defined behaviorally rather than marketed as a vague model capability.

## Junior-style loop

```text
Request
 ↓
Generate code
 ↓
Done
```

## Senior-style loop

```text
Requirement
 ↓
Clarify ambiguity
 ↓
Inspect architecture
 ↓
Identify constraints
 ↓
Create plan
 ↓
Implement smallest safe change
 ↓
Run verification
 ↓
Analyze failures
 ↓
Fix root cause
 ↓
Run regression tests
 ↓
Review diff
 ↓
Check requirements
 ↓
Report evidence
```

Important senior behavior:

> **Knowing when not to change something.**

If a risky unrelated issue is discovered, the agent should flag it rather than silently expanding scope.

---

# 29. Verification as a First-Class Workflow

The agent should not consider code complete merely because it produced a patch.

Verification pipeline:

```text
EDIT
 ↓
FORMAT / LINT (if available)
 ↓
COMPILE / TYPECHECK
 ↓
UNIT TESTS
 ↓
INTEGRATION TESTS
 ↓
RUNTIME CHECKS
 ↓
DIFF REVIEW
 ↓
REQUIREMENT CHECK
 ↓
SECURITY CHECK (when applicable)
```

Completion requires evidence.

Example final state:

```text
Implementation: COMPLETE

Verification:
✓ type check
✓ 47 existing tests
✓ 12 new tests
✓ integration test
✓ regression check
✓ diff review

Changed:
8 files
+427 lines
-63 lines

Confidence:
HIGH
```

---

# 30. Error Detection and Recovery Loop

```text
Tool / Build / Test
        ↓
Observation
        ↓
Failure classifier
        ↓
Relevant context retrieval
        ↓
Root-cause analysis
        ↓
Repair proposal
        ↓
Policy check
        ↓
Apply repair
        ↓
Re-run verification
```

The system should distinguish:

```text
syntax_error
compile_error
type_error
test_failure
runtime_error
integration_failure
environment_error
network_error
permission_error
tool_error
agent_error
unknown_error
```

Repeated failures should trigger a strategy change rather than endless retries.

---

# 31. Parallel Subagent Orchestration

For a large task:

```text
                 MASTER TASK
                      │
                Coordinator
                      │
                 Planner / DAG
                      │
       ┌──────────────┼──────────────┐
       ▼              ▼              ▼
    Agent A         Agent B        Agent C
    Schema          API            Tests
       │              │              │
       └──────────────┼──────────────┘
                      ▼
                 Integration
                      │
                      ▼
                   Review
```

Each subagent needs:

```text
agent identity
parent task
subtask
authorized scope
required context
input artifacts
output contract
permissions
context budget
memory access
```

---

# 32. Agent Permissions

Each agent should have an explicit capability profile.

Example:

```text
coder:
  read: repository
  write: assigned files
  execute: build/test
  memory: project + task

reviewer:
  read: repository
  write: none
  execute: tests
  memory: project + task

planner:
  read: repository metadata
  write: none
  execute: none
  memory: project + user + task
```

This becomes important for safety and autonomous execution.

---

# 33. Observability

Every important event should be structured.

```text
TaskCreated
TaskClassified
ThinkingUpdated
ContextAssembled
AgentStarted
AgentMessageSent
ToolRequested
ToolExecuted
ArtifactCreated
FileChanged
TestStarted
TestFailed
TestPassed
MemoryCreated
MemoryPromoted
WorkflowTransitioned
HumanApprovalRequested
TaskCompleted
TaskFailed
```

Every event should include:

```text
timestamp
task_id
workflow_id
agent_id
parent_agent_id
message_id
context_id
action
status
latency
input/output references
```

This enables debugging the agent itself.

---

# 34. Suggested Core Data Model

## Task

```text
Task
- id
- parent_task_id?
- user_request
- intent
- scope
- priority
- risk
- complexity
- status
- objective
- acceptance_criteria
- constraints
- created_at
- updated_at
```

## Workflow

```text
Workflow
- id
- task_id
- state
- nodes
- edges
- current_node
- retries
- checkpoints
- created_at
- updated_at
```

## Agent

```text
Agent
- id
- role
- parent_agent_id?
- task_id
- capabilities
- permissions
- state
- context_budget
```

## Memory

```text
Memory
- id
- type
- content
- structured_fields
- source
- source_id
- confidence
- scope
- created_at
- updated_at
- last_confirmed_at
```

## ContextPacket

```text
ContextPacket
- id
- task_id
- agent_id
- objective
- contract
- included_memory_ids
- included_file_ids
- included_observation_ids
- included_message_ids
- excluded_categories
- token_budget
- estimated_tokens
- created_at
```

## Artifact

```text
Artifact
- id
- task_id
- type
- path
- hash
- version
- producer_agent_id
- created_at
```

## AgentMessage

```text
AgentMessage
- id
- from_agent
- to_agent
- task_id
- type
- payload
- priority
- created_at
- status
```

## Observation

```text
Observation
- id
- task_id
- agent_id
- type
- source
- content
- structured_data
- timestamp
```

## Decision

```text
Decision
- id
- task_id
- agent_id
- question
- options
- decision
- rationale_summary
- confidence
- evidence_refs
```

---

# 35. Recommended Execution Lifecycle

```text
1. USER MESSAGE
      ↓
2. TASK BOUNDARY DETECTION
      ↓
3. CLASSIFICATION
      ↓
4. @Thinking INITIALIZATION
      ↓
5. PRIORITY + RISK ASSESSMENT
      ↓
6. CLARIFY OR EXECUTE?
      ↓
7. TASK CREATION
      ↓
8. WORKFLOW GENERATION
      ↓
9. CONTEXT REQUEST
      ↓
10. CONTEXT RETRIEVAL
      ↓
11. CONTEXT RANKING
      ↓
12. CONTEXT PACKET BUILD
      ↓
13. AGENT EXECUTION
      ↓
14. TOOL EXECUTION
      ↓
15. OBSERVATION CAPTURE
      ↓
16. MEMORY UPDATE / PROMOTION
      ↓
17. VERIFICATION
      ↓
18. FAILURE? ── YES → DEBUG / REPAIR → VERIFICATION
      │
      NO
      ↓
19. REVIEW
      ↓
20. TASK RESULT
      ↓
21. COORDINATOR
      ↓
22. USER RESPONSE
```

---

# 36. Example End-to-End Flow

User:

> Fix the login bug where users get randomly logged out.

## Coordinator receives

```text
current message
+
minimal recent conversation context
+
user preferences
+
project summary
+
active task list
```

It determines:

```text
intent = BUG_FIX
priority = HIGH
risk = MEDIUM
scope = authentication subsystem
execution = autonomous
clarification = NO
```

## @Thinking

```text
Objective:
Fix random logout.

Known:
- issue occurs in authentication
- exact trigger not yet known

Need:
- error traces
- session implementation
- relevant tests
- recent auth changes
```

## Context Engine retrieves

```text
auth/session.go
auth/middleware.go
session tests
recent auth changes
session architecture decision
similar historical failure
```

## Workflow

```text
Inspect
 ↓
Reproduce
 ↓
Diagnose
 ↓
Patch
 ↓
Test
 ↓
Review
```

## Coder context

```text
coder contract
+
original request
+
coordinator handoff
+
selected source context
+
selected tests
+
error context
+
task state
```

## Tester receives

```text
changed files
+
expected behavior
+
relevant tests
+
test contract
```

Not the coder's full conversation.

## Tester fails

```text
Test:
TestSessionRefresh

Error:
expected refreshed session; got expired session
```

## Debugger context

```text
failure
+
changed code
+
session lifecycle
+
relevant middleware
+
prior similar failure
```

Debugger finds:

```text
Root cause:
refresh timestamp calculated before token validation.
```

## Repair

Coder receives only the new repair context and applies a focused patch.

## Verification

```text
✓ targeted test
✓ authentication tests
✓ integration suite
✓ diff review
```

## Task result

```text
Completed.

Root cause:
Session refresh timestamp was calculated before token validation.

Changed:
3 files.

Verification:
42 tests passed.

Public API:
unchanged.

Unrelated warning:
1 existing warning remains.
```

Coordinator turns this into the user-facing answer.

---

# 37. What Makes This Different From Claude Code / Codex / Gemini CLI

Do not compete primarily on “better code generation.”

Compete on **engineering orchestration and memory discipline**.

Potential differentiators:

## 1. Selective context

The model sees what it needs rather than everything available.

## 2. Structured project memory

Architecture and decisions become persistent knowledge.

## 3. Task-isolated execution memory

One task does not pollute another.

## 4. Resumable workflows

A crashed process resumes from durable state rather than replaying history.

## 5. Failure memory

The system remembers failed approaches and avoids repeating them.

## 6. Specialized agents

Coordinator, explorer, coder, tester, debugger, reviewer.

## 7. Deterministic orchestration

State transitions, tools, permissions, and verification are controlled outside the LLM.

## 8. Evidence-based completion

The agent is finished only when verification provides evidence.

## 9. Senior behavior

The system knows when to stop, ask, defer, or refuse an unsafe change.

---

# 38. MVP Scope

Do not try to build every possible agent capability first.

## Phase 1 — Core Loop

Build:

```text
Coordinator
Task object
Basic Context Engine
Project memory
Workflow engine
Coder agent
Tool executor
Verification
Task result
```

Supported workflow:

```text
User request
 ↓
Classify
 ↓
Plan
 ↓
Code
 ↓
Test
 ↓
Repair
 ↓
Review
 ↓
Done
```

## Phase 2 — Strong Memory

Add:

```text
Task memory
Episodic memory
Failure memory
User preferences
Code graph
Context ranking
Context receipts
```

## Phase 3 — Multi-Agent

Add:

```text
Explorer
Tester
Debugger
Reviewer
parallel DAG execution
agent message bus
artifact exchange
```

## Phase 4 — Senior Autonomy

Add:

```text
risk assessment
approval policies
long-running tasks
resumable execution
proactive failure recovery
scope control
security review
```

---

# 39. Initial MVP Workflow Templates

Start with a handful of workflow templates.

## Bug Fix

```text
Understand
→ Reproduce
→ Locate
→ Diagnose
→ Patch
→ Test
→ Review
```

## New Feature

```text
Understand
→ Explore
→ Plan
→ Implement
→ Test
→ Integrate
→ Review
```

## Refactor

```text
Understand
→ Map dependencies
→ Define invariants
→ Refactor
→ Compile
→ Test
→ Review
```

## Direct Command

```text
Validate
→ Execute
→ Verify
→ Report
```

## Investigation

```text
Collect evidence
→ Analyze
→ Hypothesize
→ Validate
→ Report
```

---

# 40. Safety and Scope Rules

Autonomy needs explicit boundaries.

The agent should classify actions as:

```text
READ_ONLY
SAFE_WRITE
HIGH_RISK_WRITE
DESTRUCTIVE
EXTERNAL_SIDE_EFFECT
```

Examples:

```text
read file               → READ_ONLY
edit source file        → SAFE_WRITE
modify CI workflow      → HIGH_RISK_WRITE
delete repository data  → DESTRUCTIVE
send external request   → EXTERNAL_SIDE_EFFECT
```

Approval policy can depend on user preferences and project rules.

---

# 41. Human-in-the-Loop Rules

The system should not ask the user about every step.

Instead ask when:

```text
ambiguity blocks progress
risk exceeds policy
external irreversible side effect
architecture choice has multiple valid paths
credentials/secrets are required
scope needs expansion
```

Otherwise continue autonomously.

This is critical to making the product feel autonomous rather than like an elaborate autocomplete tool.

---

# 42. Product UX Principle

The user should interact primarily with the **Coordinator**, while having optional visibility into execution.

Possible views:

```text
Chat
Tasks
Workflow
Changes
Tests
Memory
Agents
Timeline
```

A user could expand:

```text
TASK-1042

✓ Plan
✓ Explore
✓ Implement
⚙ Test
○ Review
```

And optionally inspect:

```text
Why did the agent choose this file?
Why did it retry?
What context was sent?
What memory influenced this decision?
What failed before the final fix?
```

This creates transparency without overwhelming the main interaction.

---

# 43. Internal Golden Rule

The entire architecture can be summarized as:

```text
USER GOAL
   ↓
UNDERSTAND
   ↓
TASK
   ↓
PLAN
   ↓
SELECT CONTEXT
   ↓
DELEGATE
   ↓
EXECUTE
   ↓
OBSERVE
   ↓
VERIFY
   ↓
RECOVER IF NEEDED
   ↓
STORE DURABLE KNOWLEDGE
   ↓
REPORT EVIDENCE
```

And underneath that:

```text
Memory ≠ Context
Context ≠ Conversation
Execution ≠ Conversation
Reasoning ≠ Transcript
```

---

# 44. Success Criteria

The architecture is succeeding when these are true:

1. A simple request creates a small context packet.
2. A complex request automatically creates a workflow.
3. The Coordinator does not need the coder's full transcript.
4. A Coder receives only relevant code and task state.
5. A Tester receives only changed artifacts and verification context.
6. Parallel agents can work without sharing giant histories.
7. A failed task can resume without replaying old conversations.
8. A new user task does not accidentally inherit an unrelated task.
9. Durable project facts survive across tasks.
10. Explicit user preferences survive across projects when appropriate.
11. Failed attempts can be retrieved for similar future failures.
12. Every model call has a measurable Context Receipt.
13. Workflow state is durable and inspectable.
14. Completion requires evidence from verification.
15. The system asks the user only when a decision genuinely requires human input.

---

# 45. @Thinking — Product-Level Summary

```text
@Thinking

Problem:
Current coding agents often treat conversation history and repository
content as the main source of context. This creates huge prompts,
context pollution, weak task isolation, repeated reasoning, and poor
resumability.

Core hypothesis:
An autonomous coding agent should treat context as a dynamically
constructed resource rather than a transcript.

Design decision:
Separate user conversation, task execution, project memory, user memory,
code knowledge, and episodic memory.

Key mechanism:
A Context Engine converts a task + agent role + workflow state into a
minimal sufficient Context Packet.

Coordination model:
One Coordinator talks to the user. Specialized execution agents operate
in isolated task contexts and communicate through structured messages and
artifacts.

Autonomy model:
A durable workflow engine controls state transitions, tool execution,
verification, retries, and parallelism. The LLM proposes reasoning and
actions; deterministic infrastructure enforces execution.

Memory model:
Persist durable facts, architectural decisions, user preferences,
workflow state, failures, and lessons. Do not persist every observation
as permanent memory.

Context model:
Retrieve only relevant source, tests, errors, memories, decisions, and
observations according to task-specific profiles and token budgets.

Reasoning model:
Use structured @Thinking artifacts instead of storing raw hidden
chain-of-thought. Capture objectives, constraints, facts, unknowns,
hypotheses, decisions, confidence, and verification strategy.

Failure model:
Failures become structured observations. The system retrieves relevant
context, forms a root-cause hypothesis, repairs, re-verifies, and stores
useful lessons when appropriate.

Parallelism model:
Large tasks become a dependency graph. Independent subtasks can execute
concurrently. Agents exchange typed messages and artifacts rather than
full conversations.

Resumability model:
A task can be reconstructed from its durable Task Bundle without replaying
its entire LLM transcript.

Senior-agent definition:
A senior agent is not one that writes more code. It understands the goal,
respects constraints, chooses the smallest safe change, verifies its work,
learns from failures, avoids repeating mistakes, and knows when human
judgment is required.

Primary differentiator:

    THINK ONCE
    REMEMBER STRUCTURALLY
    CONTEXTUALIZE SELECTIVELY
    EXECUTE AUTONOMOUSLY
    VERIFY WITH EVIDENCE
```

---

# 46. Immediate Build Order

Build in this exact order to avoid overengineering too early.

```text
1. Task model
2. Coordinator
3. Task classifier
4. Workflow state machine
5. Tool executor
6. Basic Context Packet schema
7. File/context retrieval
8. Coder agent
9. Verification loop
10. Failure/repair loop
11. Task memory
12. Project memory
13. @Thinking
14. Context ranking + budget
15. Episodic/failure memory
16. Tester agent
17. Reviewer agent
18. DAG / parallel agents
19. Agent message bus
20. Resumable execution
21. Observability
22. User-facing execution timeline
```

Do not start with a giant multi-agent framework. Prove the single-task autonomous loop first, then turn each successful boundary into a reusable subsystem.

---
