package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/agent/agentdef"
	"github.com/iSundram/Automergent/internal/agent/builtin"
	"github.com/iSundram/Automergent/internal/config"
	contextmgr "github.com/iSundram/Automergent/internal/context"
	"github.com/iSundram/Automergent/internal/editreview"
	"github.com/iSundram/Automergent/internal/errors"
	promptpkg "github.com/iSundram/Automergent/internal/prompt"
	"github.com/iSundram/Automergent/internal/session"
	"github.com/iSundram/Automergent/internal/shared"
	"github.com/iSundram/Automergent/internal/tools"
	subagent "github.com/iSundram/Automergent/internal/tools/agent"
	toolsFS "github.com/iSundram/Automergent/internal/tools/filesystem"
	toolsShell "github.com/iSundram/Automergent/internal/tools/shell"
)

// Agent is the core AI coding agent.
type Agent struct {
	cfg                 *config.Config
	provider            ai.Provider
	sess                *session.Session
	tools               *tools.Registry
	events              chan Event
	closeOnce           sync.Once
	eventsClosed        bool
	sessionPersist      func()
	mu                  sync.RWMutex
	// sessionGrants is shared with every subagent this agent spawns (see
	// Execute), so "always allow" decisions cover the whole agent tree.
	sessionGrants *grants
	approvalSource      string
	workDir             string
	firstMessageHandled bool
	decisionRecords     []ToolDecisionRecord
	skills              []Skill
	skillPaths          *skillTracker
	commandHints        []CommandHint
	commandHintsMu      sync.RWMutex
	editReview          *editreview.Store

	// Persistent components
	contextManager     *contextmgr.Manager
	contextManagerRoot string

	// toolProfile is ephemeral per request. It is never persisted, added to
	// session messages, or rendered in the user-facing prompt.
	toolProfile map[string]bool

	// steer carries user messages injected mid-run. The turn loop drains it at
	// tool boundaries, so a queued message reaches the model before the next
	// call instead of waiting for the whole turn to finish.
	steer chan string

	// Context-window management state (see autocompact.go). usageAnchor is
	// the message count covered by the last provider-reported usage;
	// usageAnchoredTokens is that usage total. Compaction rewrites messages
	// in place, which invalidates the anchor.
	usageAnchor         int
	usageAnchoredTokens int
	compactionFailures  int
	lastCompactedAt     time.Time

	// userCtx is the conversation-scoped user context (project instructions,
	// git snapshot) injected as meta user messages at request time. See
	// usercontext.go.
	userCtx     map[string]string
	userCtxOnce sync.Once

	// rules persists user-stated rules captured by the INIT decomposer.
	rules *RuleStore

	// omitProjectContext drops project instructions + git snapshot from the
	// user context — read-only subagents don't need them (see subagents.go).
	omitProjectContext bool

	// agentMemory is this agent's persistent memory (subagents with a
	// MemoryScope; see agentmemory.go).
	agentMemory *AgentMemory

	// childHandles keeps spawned child agents addressable for resume
	// (continue a completed/stopped agent's conversation). Keyed by the
	// tracked subagent instance ID.
	childHandles map[string]*Agent

	// boundary is the working-directory path scope (see pathboundary.go).
	boundary *tools.PathScope

	// New prompt system for staged prompt delivery
	promptSystem     *promptpkg.PromptSystem
	promptSystemOnce sync.Once

	// Phase-aware agent loop
	phaseManager    *promptpkg.PhaseManager
	phaseClassifier *promptpkg.PhaseClassifier
	decomposer      *promptpkg.InitDecomposer
	promptComposer  *promptpkg.PromptComposer
	currentAgentDef *agentdef.AgentDefinition

	// decomposeDisabled skips the INIT decomposer: subagent children are
	// routed by their parent already, and their definition (explore is
	// read-only, general-purpose builds) decides their phase.
	decomposeDisabled bool

	// childCancels tracks cancellation functions for spawned child agents.
	childCancels map[string]context.CancelFunc
}

// Execute implements the AgentExecutor interface for sub-agents.
func (a *Agent) Execute(ctx context.Context, agentType subagent.AgentType, prompt string, model string) (string, error) {
	trackedID := subagent.AgentIDFrom(ctx)

	// Resume path: the caller asked to continue an existing agent's
	// conversation rather than spawn a fresh one. The stored child's event
	// channel is closed after its first run, so rebuild a live agent around
	// its persisted session (sidechain transcript reloads from disk).
	if resumeID := resumeAgentIDFrom(ctx); resumeID != "" {
		if child, ok := a.resumeChild(resumeID); ok {
			resumed := New(child.cfg, a.provider, child.Session(), child.tools)
			resumed.sessionGrants = a.sessionGrants
			if child.currentAgentDef != nil {
				resumed.SetDefinition(child.currentAgentDef)
			}
			a.prepareChild(resumed, child.currentAgentDef, resumeID)
			return a.runChild(ctx, resumed, prompt, trackedID, string(agentType))
		}
		// Unknown handle: fall through and spawn fresh (the model gets a new
		// agent rather than an error it cannot act on).
	}

	// 0. Resolve the agent definition first: it drives model routing and
	// the read-only context slimming.
	var def *agentdef.AgentDefinition
	if d, ok := GlobalRegistry().Get(AgentType(agentType)); ok {
		def = d
	}

	// 1. Create a child configuration. Read-only agents (explore/review)
	// run on the configured FastModel when neither the call nor the
	// definition pins one — the reference agent's Explore→haiku routing.
	childCfg := *a.cfg
	if resolved := a.resolveChildModel(model, def); resolved != "" {
		childCfg.Model = resolved
	}

	// 2. Create a clean child session. A fork child inherits the parent's
	// conversation (repaired so no tool call dangles) instead of starting
	// cold.
	childSess := session.New()
	childSess.Metadata["parent_id"] = a.sess.ID
	childSess.Metadata["agent_type"] = string(agentType)
	if forkContextFrom(ctx) {
		childSess.SetMessages(a.forkContextMessages())
	}

	// 3. Create a child agent. The child runs on a CLONE of the tool registry
	// with the task-state tools re-registered against the child's own prompt
	// system: without this, a subagent's todo_write would mutate the parent's
	// task list (the registry was shared, and those tools close over the
	// parent's task store).
	childTools := a.tools.Clone()
	childAgent := New(&childCfg, a.provider, childSess, childTools)
	if childAgent.promptSystem != nil {
		tools.RegisterTaskStateTools(childTools, childAgent.promptSystem.GetTaskState())
	}

	// The child answers to the same always-allow set as its parent: grants
	// made while answering the main agent's asks cover subagents, and an
	// "always allow" granted on a subagent's ask persists to the real session
	// instead of dying with the child's throwaway one.
	childAgent.sessionGrants = a.sessionGrants

	// Apply the agent-type definition: system prompt, phase config, and the
	// per-type tool allowlist (explore is read-only; general-purpose's empty
	// Tools means the full registry, bash included).
	if def != nil {
		childAgent.SetDefinition(def)
	}

	// Parity behaviors: read-only context slimming, sidechain transcript,
	// agent memory, resume handle.
	a.prepareChild(childAgent, def, trackedID)

	// Children do not re-run the INIT decomposer: the parent already
	// decomposed and routed; the definition picks the phase.
	childAgent.decomposeDisabled = true

	return a.runChild(ctx, childAgent, prompt, trackedID, string(agentType))
}

// runChild drives a built child agent to completion and formats the result.
// Shared by the fresh-spawn and resume paths.
func (a *Agent) runChild(ctx context.Context, childAgent *Agent, prompt string, trackedID, agentType string) (string, error) {
	childSess := childAgent.Session()
	var finalResponse string
	var finalErr error
	done := make(chan struct{})

	childCtx, childCancel := context.WithCancel(ctx)
	defer childCancel()

	// The child's events used to be drained into nothing, which is why a running
	// subagent had no observable state: everything it did was read off the
	// channel and dropped. The drainer still exists to keep the child from
	// blocking, but on the way past it records which tool the child is in and
	// what it last said, so the dock has something true to show. The drainer
	// exits when the child's channel closes via childAgent.Close().
	//
	// Confirmation asks are the exception: re-emitted on the parent's channel
	// with provenance, they reach the UI that is still pumping this agent's
	// events while it sits blocked inside the task tool. Without this, a
	// subagent's permission ask disappeared into the drain and expired after
	// the ten-minute timeout, silently denied.
	go func() {
		for ev := range childAgent.Events() {
			switch ev.Type {
			case EventConfirm, EventAskUser:
				if p, ok := ev.Payload.(map[string]any); ok {
					p["agent_type"] = agentType
					p["agent_id"] = trackedID
					p["agent_name"] = subagentDisplayName(trackedID, subagent.AgentType(agentType))
					a.Emit(ev.Type, p)
				}
				continue
			}
			if trackedID == "" {
				continue
			}
			reportSubagentProgress(trackedID, ev)
		}
	}()

	// Run the child agent in a goroutine and signal completion on done.
	go func() {
		started := time.Now()
		finalErr = childAgent.Run(childCtx, prompt)
		// Extract the last assistant message as the result
		if len(childSess.Messages) > 0 {
			lastMsg := childSess.Messages[len(childSess.Messages)-1]
			if lastMsg.Role == ai.RoleAssistant {
				finalResponse = lastMsg.TextContent()
			}
		}
		if trackedID != "" {
			subagent.GetAgentManager().NoteTokens(trackedID,
				childSess.TotalInputTokens, childSess.TotalOutputTokens)
			subagent.GetAgentManager().NoteTool(trackedID, "")

			// Task-notification footer: the parent model sees what the child
			// cost, the same way the reference agent reports usage with every
			// background completion. Applied only when the child succeeded so
			// failures keep their error text unobstructed.
			if finalErr == nil {
				var toolCount int
				if inst, ok := subagent.GetAgentManager().Get(trackedID); ok {
					toolCount = inst.Snapshot().ToolCount
				}
				finalResponse += fmt.Sprintf(
					"\n\n<usage>\ntotal_tokens: %d\ninput_tokens: %d\noutput_tokens: %d\ntool_uses: %d\nduration_ms: %d\n</usage>",
					childSess.TotalInputTokens+childSess.TotalOutputTokens,
					childSess.TotalInputTokens, childSess.TotalOutputTokens,
					toolCount, time.Since(started).Milliseconds())
			}
		}
		// Ensure we close the child's event channel so drainers exit.
		_ = childAgent.Close()
		close(done)
	}()

	select {
	case <-done:
		return finalResponse, finalErr
	case <-ctx.Done():
		// Parent cancelled: request child cancellation and wait for a short grace period
		childCancel()
		select {
		case <-done:
			return finalResponse, finalErr
		case <-time.After(5 * time.Second):
			// Give up waiting to avoid blocking indefinitely; return the context error
			return "", ctx.Err()
		}
	}
}

// subagentDisplayName resolves the instance's chosen name, falling back to
// the agent type when the instance is untracked (direct Execute calls).
func subagentDisplayName(id string, agentType subagent.AgentType) string {
	if id != "" {
		if inst, ok := subagent.GetAgentManager().Get(id); ok {
			if snap := inst.Snapshot(); snap.Name != "" {
				return snap.Name
			}
		}
	}
	return string(agentType)
}

// reportSubagentProgress translates one child event into the live fields the
// dock reads, and appends the steps worth remembering to the activity log the
// agent viewer shows as the subagent's own short conversation.
func reportSubagentProgress(agentID string, ev Event) {
	mgr := subagent.GetAgentManager()
	switch ev.Type {
	case EventToolCall:
		switch p := ev.Payload.(type) {
		case ToolCallEvent:
			mgr.NoteTool(agentID, p.Name)
			mgr.NoteActivity(agentID, subagent.ToolActivityLine(p.Name, p.Args))
		case ai.ToolCall:
			mgr.NoteTool(agentID, p.Name)
			mgr.NoteActivity(agentID, subagent.ToolActivityLine(p.Name, p.Args))
		}
	case EventToolDone:
		mgr.NoteTool(agentID, "")
	case EventToken:
		if tok, ok := ev.Payload.(string); ok {
			mgr.NoteOutput(agentID, tok)
		}
	case EventStatus:
		if s, ok := ev.Payload.(string); ok {
			mgr.NoteOutput(agentID, s)
		}
	}
}

type ConfirmationResponse struct {
	Allow    bool
	Always   bool
	Feedback string
}

// Event is an agent lifecycle event.
type Event struct {
	Type    string
	Payload any
}

type ToolCallEvent struct {
	ID        string
	Name      string
	Context   string
	Args      map[string]any
	Decision  ToolDecisionRecord
	StartedAt time.Time
}

type ToolDoneEvent struct {
	ID         string
	Name       string
	Context    string
	StartedAt  time.Time
	FinishedAt time.Time
	Duration   time.Duration
	Result     tools.Result
	Decision   ToolDecisionRecord
}

// TodoEvent carries a todo snapshot/update to the UI.
type TodoEvent struct {
	Items []shared.TodoItem
}

// notifyTodos emits a todo snapshot event; safe to call from any goroutine.
func (a *Agent) notifyTodos(items []shared.TodoItem) {
	a.emitTodoEvent(context.Background(), EventTodoSnapshot, TodoEvent{Items: items})
}

type toolCallBatch struct {
	calls     []ai.ToolCall
	decisions []ToolDecisionRecord
	parallel  bool
}

type executedToolCall struct {
	call       ai.ToolCall
	context    string
	startedAt  time.Time
	finishedAt time.Time
	result     tools.Result
	decision   ToolDecisionRecord
}

const (
	EventToken        = "token"
	EventThought      = "thought"
	EventToolCall     = "tool_call"
	EventToolStart    = "tool_start"
	EventToolDone     = "tool_done"
	EventDone         = "done"
	EventError        = "error"
	EventConfirm      = "confirm"
	EventAskUser      = "ask_user"
	EventNotify       = "notify"
	EventStatus       = "status"
	EventThinking     = "thinking"
	EventCompacted    = "compacted"
	EventTodoSnapshot = "todo_snapshot"
	EventTodoUpdate   = "todo_update"
	EventInitAction   = "init_action"
	// EventRetry reports one retried provider API attempt. Payload is an
	// ai.RetryInfo. Emitted while the retry is pending, so the UI can show
	// progress instead of appearing to hang through the backoff.
	EventRetry = "retry"
	// EventSteered reports that a queued user message was injected mid-run at
	// a tool boundary. Payload is the message text.
	EventSteered = "steered"
)

const (
	triageInjectedMetadataKey     = "triage_injected"
	originalUserPromptMetadataKey = "original_user_prompt"
)

var errPromptSystemPrepared = fmt.Errorf("prompt system prepared execution context")

// New creates a new Agent.
func New(cfg *config.Config, provider ai.Provider, sess *session.Session, reg *tools.Registry) *Agent {
	agent := &Agent{
		cfg:                 cfg,
		provider:            provider,
		sess:                sess,
		tools:               reg,
		events:          make(chan Event, 8192),
		steer:           make(chan string, 8),
		approvalSource:  "tui",
		skillPaths:      newSkillTracker(12),
		editReview:      nil,
		currentAgentDef: builtin.GeneralAgent(),
	}

	// Grants persist into this agent's session; subagents spawned later share
	// the object (see Execute) so the tree answers to one allow set.
	agent.sessionGrants = newGrants(func(scope string) {
		if agent.sess != nil {
			agent.sess.AddApproval(scope, agent.approvalSource)
		}
		agent.tryPersist()
	})

	// Surface provider-internal retries as events so the UI can show that a
	// request is being retried rather than appearing to hang.
	agent.installRetryObserver(provider)

	if cfg != nil && cfg.NoTUI {
		agent.approvalSource = "headless"
	}

	// Record the project directory so "always allow" approvals are scoped
	// to this project and do not leak into sessions resumed elsewhere.
	if wd, err := os.Getwd(); err == nil {
		agent.workDir = wd
	}

	// Initialize prompt system with context manager
	llmClient := promptpkg.NewAIProviderAdapter(agent.provider, "")
	agent.promptSystem = promptpkg.NewPromptSystemWithLLM(promptpkg.DefaultPromptConfig(), agent.ContextManager(), agent.workDir, llmClient, agent.tools)

	// Initialize phase-aware components
	agent.phaseManager = promptpkg.NewPhaseManager(agent.promptSystem, agent.currentAgentDef)
	agent.phaseClassifier = promptpkg.NewPhaseClassifier(agent.phaseManager, llmClient)

	// Surface prompt-system pipeline stages (intents, init actions, task plan)
	// in the TUI conversation so the pre-execution work is visible.
	agent.promptSystem.Manager.SetProgress(func(stage, detail string) {
		agent.Emit(EventNotify, map[string]any{
			"level":   "info",
			"title":   stage,
			"message": detail,
		})
	})

	// Surface init-phase tool executions as structured events so the TUI can
	// render them as native tool-call cards inside the conversation log.
	agent.promptSystem.SetActionObserver(func(evt shared.InitActionEvent) {
		agent.Emit(EventInitAction, evt)
	})

	// Seed always-allow approvals persisted in the session so resumed runs
	// do not re-prompt for tools the user already approved. Directory grants
	// (path-boundary always-allows) also re-seed the boundary scope.
	if sess != nil {
		agent.sessionGrants.Reset(sess.ApprovalScopes())
		for _, scope := range sess.ApprovalScopes() {
			if dir, ok := tools.IsDirGrant(scope); ok {
				agent.pathScope().AddGrantedDir(stripProjectPrefix(scope, dir))
			}
		}
	}

	return agent
}

// SetSessionPersist registers a callback invoked after meaningful session updates
// (e.g. completed turns). Used to save conversations without waiting for process exit.
func (a *Agent) SetSessionPersist(fn func()) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sessionPersist = fn
}

func (a *Agent) tryPersist() {
	a.mu.RLock()
	fn := a.sessionPersist
	a.mu.RUnlock()
	if fn != nil {
		fn()
	}
}

// checkAndMarkFirstMessage safely determines whether the current run is the first
// message handled for this agent and marks it as handled to avoid races.
func (a *Agent) checkAndMarkFirstMessage() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.firstMessageHandled {
		return false
	}
	isFirst := len(a.sess.Messages) == 0
	// Mark handled regardless so subsequent runs don't treat this as first.
	a.firstMessageHandled = true
	return isFirst
}

// getThinkingBudget returns the Gemini thinking token budget.
// This is USER configuration (effort level / explicit budget),
// never inferred from keyword analysis of the prompt.
func (a *Agent) getThinkingBudget() int {
	budget := 10000
	if a.cfg == nil {
		return budget
	}
	if a.cfg.ThinkingBudget > 0 {
		return a.cfg.ThinkingBudget
	}
	switch strings.ToLower(strings.TrimSpace(a.cfg.Effort)) {
	case "minimal", "none":
		budget = 0
	case "low":
		budget = 4000
	case "medium":
		budget = 8000
	case "high":
		budget = 16000
	case "max", "ultra":
		budget = 32000
	}
	if a.cfg.MaxContextTokens > 0 && budget > a.cfg.MaxContextTokens/4 {
		budget = a.cfg.MaxContextTokens / 4
	}
	return budget
}

// getThinkingLevel returns the Gemini 3 thinking level from user effort config.
func (a *Agent) getThinkingLevel() string {
	if a.cfg == nil {
		return "high"
	}
	switch strings.ToLower(strings.TrimSpace(a.cfg.Effort)) {
	case "minimal", "none", "off":
		return "low"
	case "low":
		return "low"
	case "medium":
		return "medium"
	default:
		return "high"
	}
}

func (a *Agent) Events() <-chan Event { return a.events }

// Provider returns the AI provider.
func (a *Agent) Provider() ai.Provider {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.provider
}

// Tools returns the agent's tool registry. Callers building side agents
// (memory consolidation, one-shot workers) share the registry so their tool
// calls obey the same policies.
func (a *Agent) Tools() *tools.Registry {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.tools
}

// GetModel returns the current model name.
func (a *Agent) GetModel() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg.Model
}

// SetProvider swaps the runtime provider used for subsequent completions.
// Every component that captured the OLD provider's adapter is rebuilt or
// re-pointed: the INIT decomposer, the phase classifier, the prompt
// system's staged pipelines (intent identification, task planning), the
// context manager's model limits, and the usage anchor. Without this, a
// /model or /provider switch left all the routing machinery calling the
// retired provider.
func (a *Agent) SetProvider(p ai.Provider) {
	a.mu.Lock()
	a.provider = p
	// The decomposer and classifier are rebuilt lazily from a.provider on
	// next use (see decomposeFirstMessage / ensurePhaseComponents), so
	// dropping them here picks up the new provider without rebuild cost on
	// every switch.
	a.decomposer = nil
	a.phaseClassifier = nil
	newModel := ""
	if a.cfg != nil {
		newModel = a.cfg.Model
	}
	ps := a.promptSystem
	cm := a.contextManager
	a.mu.Unlock()

	// The new provider has no observer installed yet: without this, retries go
	// silent after any /provider or /model switch.
	a.installRetryObserver(p)

	// Staged pipelines (intent identification, task planning) cache their
	// own adapters — repoint them at the new provider.
	if ps != nil {
		ps.SetLLMClient(promptpkg.NewAIProviderAdapter(p, ""))
	}

	// Context ladder limits are a function of the model: a switch from a
	// 1M-token model to a 128k one must shrink the budgets immediately,
	// not on the next cwd change.
	if cm != nil && newModel != "" {
		cm.SetModel(contextmgr.GetModelLimits(newModel))
	}

	// The anchor describes the previous provider's token accounting.
	a.invalidateUsageAnchor()
}

// installRetryObserver wires provider-internal retry reporting into the event
// stream. Providers that do not implement ai.RetryObserver are left alone.
func (a *Agent) installRetryObserver(p ai.Provider) {
	ro, ok := p.(ai.RetryObserver)
	if !ok {
		return
	}
	ro.SetRetryObserver(func(info ai.RetryInfo) {
		a.Emit(EventRetry, info)
	})
}

// Steer queues a user message for injection at the next tool boundary of the
// running turn. It never blocks: a full buffer means the user has already
// queued more than the run can absorb, and reports false so the caller can
// keep the message in its own queue instead.
func (a *Agent) Steer(text string) bool {
	if strings.TrimSpace(text) == "" {
		return false
	}
	select {
	case a.steer <- text:
		return true
	default:
		return false
	}
}

// drainSteer moves any queued steering messages into the conversation as user
// messages. Called at tool boundaries, where inserting a user turn is valid.
// Reports how many messages were injected.
func (a *Agent) drainSteer() int {
	injected := 0
	for {
		select {
		case text := <-a.steer:
			msg := ai.NewTextMessage(ai.RoleUser, text)
			a.sess.AddMessage(msg)
			a.recordToTranscript(msg)
			a.Emit(EventSteered, text)
			injected++
		default:
			return injected
		}
	}
}

// ClearSteerQueue discards pending steering messages. Called on cancellation so
// a queued message cannot leak into the next, unrelated run.
func (a *Agent) ClearSteerQueue() {
	for {
		select {
		case <-a.steer:
		default:
			return
		}
	}
}

// Session returns the current session.
func (a *Agent) Session() *session.Session { return a.sess }

// Approvals returns a copy of the always-allow approvals recorded in the
// current session.
func (a *Agent) Approvals() []session.ToolApproval {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.sess == nil {
		return nil
	}
	snap := a.sess.Snapshot()
	return append([]session.ToolApproval(nil), snap.AllowedTools...)
}

// RevokeApproval removes an always-allow approval from both the in-memory
// cache and the persisted session. Returns true if the scope was present.
// EditReviewStore returns the pending-edit proposal store (nil when the
// edit-review mode is disabled).
func (a *Agent) EditReviewStore() *editreview.Store { return a.editReview }

func (a *Agent) RevokeApproval(scope string) bool {
	a.sessionGrants.Delete(scope)
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.sess == nil {
		return false
	}
	return a.sess.RemoveApproval(scope)
}

// SetSession replaces the current session (e.g., when loading a saved session).
// Always-allow approvals are re-seeded from the incoming session so resumed
// sessions keep their granted tool permissions.
func (a *Agent) SetSession(sess *session.Session) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sess = sess
	a.firstMessageHandled = sess != nil && len(sess.Messages) > 0
	if sess != nil {
		a.sessionGrants.Reset(sess.ApprovalScopes())
	} else {
		a.sessionGrants.Reset(nil)
	}
}

// Run executes the agent loop for the given user prompt using phase-aware execution.
func (a *Agent) Run(ctx context.Context, prompt string) error {
	originalUserPrompt := prompt

	// Agents constructed directly (tests, embedders) skip New's wiring; make
	// sure the phase machinery exists before it is used.
	a.ensurePhaseComponents()

	// 1. Initial Triage Phase - classify first message
	isFirstMessage := a.checkAndMarkFirstMessage()
	if isFirstMessage {
		a.Emit(EventStatus, "initiating project triage")
	}

	// Persist the user-authored message
	userMsg := ai.NewTextMessage(ai.RoleUser, originalUserPrompt)
	a.sess.AddMessage(userMsg)
	a.recordToTranscript(userMsg)

	// Deliver task notifications queued while the agent was idle (background
	// subagents that finished between runs) ahead of the model's first
	// response, the same way they would have landed mid-run.
	a.drainSteer()

	// 2. INIT: decompose the message into parts and route each one. The
	// decomposer is the primary path (LLM-driven, understands multi-part
	// messages, rules, noise, violations); the keyword classifier stays as
	// the fallback when it is unavailable. It runs on EVERY top-level
	// message: each user turn is a new instruction for the arc to route,
	// not just the first. Subagent children skip it (their parent already
	// routed them; their definition decides their phase).
	if !a.decomposeDisabled {
		if decomposition := a.decomposeFirstMessage(ctx, originalUserPrompt); decomposition != nil {
			return a.executeDecomposition(ctx, decomposition, originalUserPrompt)
		}
	}

	classification, err := a.phaseClassifier.Classify(ctx, originalUserPrompt, a.getAvailableFiles())
	if err != nil {
		a.Emit(EventError, fmt.Errorf("classification failed: %w", err))
		// Fall back to standard loop
		return a.runStandardLoop(ctx, originalUserPrompt, isFirstMessage)
	}

	// Keyword-fallback arc completion: the keyword router sends nearly every
	// coding request to a lone EXPLORE task, which strands the arc before
	// any implementation happens. When the message also carries build
	// intent, chain BUILD after EXPLORE so the fallback still walks the arc.
	if classification.PrimaryPhase == shared.PhaseExplore && looksLikeBuildWork(originalUserPrompt) {
		classification.SecondaryPhases = append(classification.SecondaryPhases, shared.PhaseBuild)
		for i := range classification.Tasks {
			if classification.Tasks[i].Type == "explore" && classification.Tasks[i].Description != "" {
				build := classification.Tasks[i]
				build.ID = build.ID + "-build"
				build.Type = "build"
				build.Phase = shared.PhaseBuild
				build.Description = "Implement: " + originalUserPrompt
				build.Dependencies = append(build.Dependencies, classification.Tasks[i].ID)
				classification.Tasks = append(classification.Tasks, build)
				break
			}
		}
	}

	// Handle violations detected during classification
	if len(classification.Violations) > 0 {
		for _, v := range classification.Violations {
			a.handleViolation(&v)
		}
		if classification.Violations[0].Action == "blocked" {
			return fmt.Errorf("session blocked due to policy violation")
		}
	}

	// Handle clarification requests
	if classification.RequiresClarification {
		a.askClarification(classification.ClarificationQuestions)
		return nil
	}

	// Handle direct questions in init phase
	if classification.IsDirectQuestion {
		return a.answerDirectQuestion(ctx, originalUserPrompt)
	}

	// Run the prompt-system pipeline (intent identification, init exploration,
	// task planning) so phase execution starts with pre-computed context:
	// intents, discovered files, and a task plan. A pipeline failure degrades
	// to the keyword-classified phases instead of failing the run.
	if a.promptSystem != nil && a.provider != nil {
		if _, err := a.promptSystem.ProcessUserMessage(ctx, originalUserPrompt, a.workDir, a.getAvailableFiles()); err != nil {
			a.Emit(EventStatus, "prompt pipeline degraded to keyword routing: "+err.Error())
		}
	}

	// 3. Execute phases in sequence. A classification without tasks must not
	// silently do nothing: it degenerates to one direct task for the primary
	// phase.
	tasks := classification.Tasks
	if len(tasks) == 0 {
		tasks = []shared.TaskSpec{{
			Type:        "direct",
			Description: originalUserPrompt,
			Role:        "assistant",
			Priority:    1,
		}}
	}
	phases := append([]shared.AgentPhase{classification.PrimaryPhase}, classification.SecondaryPhases...)
	for _, phase := range phases {
		for _, task := range tasks {
			// A repeat ExecutePhase for the same phase is not a valid
			// transition (explore→explore); the manager already sits in the
			// phase after the first task, so later tasks reuse its config.
			var result promptpkg.PhaseResult
			if a.phaseManager.CurrentPhase() == phase {
				cfg := a.phaseManager.GetPhaseConfig(phase)
				result = promptpkg.PhaseResult{
					Phase:    phase,
					TaskSpec: task,
					Config:   cfg,
					ToolSet:  cfg.ToolSet,
					MaxSteps: cfg.MaxSteps,
				}
			} else {
				result = a.phaseManager.ExecutePhase(phase, task)
			}

			if result.Violation != nil {
				a.handleViolation(result.Violation)
				if result.Violation.Action == "blocked" {
					return fmt.Errorf("session blocked due to policy violation")
				}
			}

			if result.Error != nil {
				a.Emit(EventError, result.Error)
				return result.Error
			}

			// Make the arc visible: the UI shows which phase is taking over.
			a.Emit(EventStatus, fmt.Sprintf("phase %s: %s", phase, task.Description))

			// Execute the phase using the standard loop with phase-specific config
			if err := a.runPhaseLoop(ctx, phase, result); err != nil {
				return err
			}
		}
	}

	return nil
}

// runPhaseLoop executes a single phase with its specific configuration.
func (a *Agent) runPhaseLoop(ctx context.Context, phase shared.AgentPhase, result promptpkg.PhaseResult) error {
	// Update tool profile for this phase
	a.toolProfile = a.toolSetToProfile(result.ToolSet)
	defer func() { a.toolProfile = nil }()

	// Build phase-specific system prompt using PromptComposer
	a.promptComposer = promptpkg.NewPromptComposer(
		shared.ModelInfo{Name: a.cfg.Model, Provider: a.provider.Name()},
		a.currentAgentDef,
		phase,
		a.workDir,
		a.convertSkillsForPrompt(a.skills),
		a.getMCPServers(),
		a.promptSystem.GetInitResults(),
		a.promptSystem.GetCurrentIntentSet(),
		a.promptSystem.GetCurrentTasks(),
	)
	// Live registry: the tool layer renders each tool's own Meta()
	// documentation for the tools this phase offers.
	a.promptComposer.SetRegistry(a.tools)

	// The fleet roster: who can be delegated to and when (task tool).
	if fleet := FleetFromRegistry(); fleet != "" {
		a.promptComposer.AddLayer(shared.LayerDynamic, fleet)
	}

	// The task this phase run is executing, with its routing hint — the
	// model must know which part of a decomposed message it is on, and
	// which subagent the INIT router chose for it.
	if block := currentTaskBlock(result.TaskSpec); block != "" {
		a.promptComposer.AddLayer(shared.LayerDynamic, block)
	}

	firstStandardTurn := true
	runMeta := &runMetadata{}
	phaseSteps := 0
	maxSteps := result.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 20
	}
	reactiveCompactions := 0

	for {
		provider := a.Provider()
		a.sess.SetMessages(ai.RepairMissingToolResults(a.sess.Messages))

		// Context-window ladder — ghost oversized outputs, micro-compact old
		// tool results, auto-compact past the threshold, then warn/block.
		// See autocompact.go for the thresholds and their rationale.
		if err := a.manageContextWindow(ctx, provider); err != nil {
			a.Emit(EventError, err)
			a.tryPersist()
			return err
		}

		// Predictive trigger: compact BEFORE the request when one more full
		// turn (reply + tool growth) would overflow the window, instead of
		// discovering it from a failed call.
		if a.predictContextOverflow(provider) && a.compactionFailures < maxConsecutiveCompactionFailures {
			a.Emit(EventStatus, "Neural Compaction: freeing context window before overflow")
			compacted := a.CompactSessionMessages(ctx, a.sess.Messages)
			a.sess.SetMessages(compacted)
			a.invalidateUsageAnchor()
			a.lastCompactedAt = time.Now()
			a.Emit(EventCompacted, map[string]any{
				"tokens_after": a.tokenCountWithEstimation(compacted),
				"predictive":   true,
			})
		}

		// Use layered system prompt from PromptComposer
		systemPrompt := a.promptComposer.Compose()
		toolSchemas := a.buildActiveToolSchemas()

		thinkingBudget := a.getThinkingBudget()
		thinkingLevel := a.getThinkingLevel()

		// Project instructions + git snapshot ride as a leading meta user
		// message (never persisted; see usercontext.go).
		messagesForQuery := prependUserContext(a.sess.Messages, a.userContext())
		// Strict providers require role alternation: merged history,
		// steered notifications, or compacted blocks can produce adjacent
		// same-role messages, so coalesce them before the request.
		messagesForQuery = mergeConsecutiveRoles(messagesForQuery)

		req := ai.CompletionRequest{
			Messages:    messagesForQuery,
			Tools:       toolSchemas,
			System:      systemPrompt,
			Temperature: 0.0,
			MaxTokens:   8192,
			Stream:      true,
			Thinking: &ai.ThinkingConfig{
				Type:            "enabled",
				BudgetTokens:    thinkingBudget,
				ThinkingLevel:   thinkingLevel,
				Stream:          true,
				IncludeThoughts: true,
			},
		}

		a.Emit(EventStatus, "thinking")
		resp, err := provider.Complete(ctx, req)
		if err != nil {
			if errors.Is(err, errors.CodeQuotaExceeded) {
				if a.handleQuotaExceeded(ctx, err) {
					continue
				}
			}
			// Reactive compaction: the provider rejected the prompt as too
			// long (estimation drifted from its real counting). Compact and
			// retry once rather than failing the whole phase.
			if reactiveCompactions < 1 && a.reactiveCompact(ctx, err, provider) {
				reactiveCompactions++
				continue
			}
			a.Emit(EventError, err)
			a.tryPersist()
			return fmt.Errorf("agent: complete: %w", err)
		}

		text, thought, usage, err := a.drainStream(resp)
		if err != nil {
			a.Emit(EventError, err)
			a.tryPersist()
			return fmt.Errorf("agent: stream: %w", err)
		}
		toolCalls := resp.ToolCalls()
		stop := resp.StopReason()
		a.sess.AddUsage(usage)

		msg := ai.Message{
			Role:     ai.RoleAssistant,
			Metadata: resp.GetMetadata(),
		}
		if thought != "" {
			msg.Content = append(msg.Content, ai.ContentPart{Type: ai.ContentTypeThought, Thought: thought})
		}
		if text != "" {
			msg.Content = append(msg.Content, ai.ContentPart{Type: ai.ContentTypeText, Text: text})
		}
		for _, tc := range toolCalls {
			tcCopy := tc
			msg.Content = append(msg.Content, ai.ContentPart{
				Type:     ai.ContentTypeToolCall,
				ToolCall: &tcCopy,
			})
		}
		if len(msg.Content) > 0 {
			a.sess.AddMessage(msg)
			a.recordToTranscript(msg)
			// Anchor token counting on this response's real usage: the
			// request covered exactly the messages now in the session.
			a.recordUsageAnchor(usage, len(a.sess.Messages))
		}

		if stop != ai.StopReasonTools || len(toolCalls) == 0 {
			if continueTurn := a.injectLongRunContext(runMeta, false); continueTurn {
				continue // anti-stall: nudge injected, loop again
			}
			a.Emit(EventDone, text)
			a.tryPersist()
			if firstStandardTurn {
				a.pruneFirstMessageTriage()
				firstStandardTurn = false
			}
			return nil
		}

		executedCalls := a.executeToolCallsParallel(ctx, toolCalls)
		for _, executed := range executedCalls {
			done := ToolDoneEvent{
				ID:         executed.call.ID,
				Name:       executed.call.Name,
				Context:    executed.context,
				StartedAt:  executed.startedAt,
				FinishedAt: executed.finishedAt,
				Duration:   executedDuration(executed),
				Result:     executed.result,
				Decision:   executed.decision,
			}
			a.Emit(EventToolDone, done)
			a.Emit(EventStatus, LongTaskStatus{
				TaskID:      executed.call.ID,
				Phase:       executed.call.Name,
				ProgressPct: 100,
				Log:         fmt.Sprintf("Completed %s", executed.call.Name),
			})
		}
		resultMsg := buildToolResultMessage(toolCalls, executedCalls)
		a.sess.AddMessage(resultMsg)
		a.recordToTranscript(resultMsg)
		a.drainSteer()
		a.injectLongRunContext(runMeta, true)

		// Phase step budget: past MaxSteps the model gets one wrap-up nudge,
		// and past twice that the phase is cut off so a runaway loop cannot
		// burn the whole session (subagents inherit the same cap, which is
		// what bounds their cost).
		phaseSteps++
		if phaseSteps == maxSteps {
			a.sess.AddMessage(ai.Message{
				Role: ai.RoleSystem,
				Content: []ai.ContentPart{{Type: ai.ContentTypeText, Text: fmt.Sprintf(
					"[Phase step budget] You have used %d of %d steps in the %s phase. Wrap up: finish the current action, verify what you can, and produce your best partial result now.",
					phaseSteps, maxSteps, phase)}},
			})
			a.Emit(EventStatus, fmt.Sprintf("phase %s hit step budget (%d); requesting wrap-up", phase, maxSteps))
		}
		if phaseSteps >= maxSteps*2 {
			return fmt.Errorf("phase %s exceeded %d steps without completing; aborting to protect the session", phase, phaseSteps)
		}
	}
}

func (a *Agent) emitTodoEvent(ctx context.Context, eventType string, state TodoEvent) {
	a.Emit(eventType, state)
}

// executedDuration returns the tool's true execution time: the
// post-approval measurement when the tool recorded one (all native tools
// do), otherwise the wall-clock span as fallback.
func executedDuration(e executedToolCall) time.Duration {
	if e.result.Metadata != nil {
		if ms, ok := e.result.Metadata["execDurationMs"].(int64); ok && ms >= 0 {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return e.finishedAt.Sub(e.startedAt)
}

func buildToolResultMessage(requested []ai.ToolCall, executed []executedToolCall) ai.Message {
	results := make(map[string]tools.Result, len(executed))
	for _, item := range executed {
		results[item.call.ID] = item.result
	}
	parts := make([]ai.ContentPart, 0, len(requested))
	for _, call := range requested {
		result, ok := results[call.ID]
		if !ok {
			result = tools.Result{IsError: true, Content: fmt.Sprintf("tool %q was interrupted before producing a result", call.Name)}
		}
		parts = append(parts, ai.ContentPart{
			Type: ai.ContentTypeToolResult,
			ToolResult: &ai.ToolResult{
				ToolCallID: call.ID,
				Content:    result.Content,
				IsError:    result.IsError,
			},
		})
	}
	return ai.Message{Role: ai.RoleTool, Content: parts}
}

func (a *Agent) executeToolCallsParallel(ctx context.Context, toolCalls []ai.ToolCall) []executedToolCall {
	decisionByCallID := make(map[string]ToolDecisionRecord, len(toolCalls))
	decisionRecords := make([]ToolDecisionRecord, 0, len(toolCalls))
	requestCalls := make([]tools.OrchestrationCall, len(toolCalls))

	for i, tc := range toolCalls {
		decision := a.evaluateToolDecision(tc)
		decisionByCallID[tc.ID] = decision
		decisionRecords = append(decisionRecords, decision)
		requestCalls[i] = tools.OrchestrationCall{
			ID:   tc.ID,
			Name: tc.Name,
			Args: tc.Args,
		}
	}
	a.storeDecisionRecords(decisionRecords)

	mode := ""
	if a.cfg != nil {
		mode = a.cfg.Mode
	}

	orchestrator := tools.NewOrchestrator(
		func(name string) (tools.Tool, bool) { return a.tools.Get(name) },
		func(execCtx context.Context, call tools.OrchestrationCall) (tools.Result, error) {
			return a.executeOrchestrationCall(execCtx, call)
		},
	)

	response := orchestrator.Execute(ctx, tools.ExecutionRequest{
		Calls:            requestCalls,
		Mode:             mode,
		MaxParallelBatch: 10,
	})

	results := make([]executedToolCall, 0, len(response.Records))
	for _, record := range response.Records {
		tc := ai.ToolCall{
			ID:   record.Call.ID,
			Name: record.Call.Name,
			Args: record.Call.Args,
		}
		decision, ok := decisionByCallID[tc.ID]
		if !ok {
			decision = a.evaluateToolDecision(tc)
		}
		results = append(results, executedToolCall{
			call:       tc,
			context:    toolCallContext(tc),
			startedAt:  record.StartedAt,
			finishedAt: record.FinishedAt,
			result:     record.Result,
			decision:   decision,
		})
	}
	return results
}

func (a *Agent) executeOrchestrationCall(ctx context.Context, call tools.OrchestrationCall) (tools.Result, error) {
	tc := ai.ToolCall{
		ID:   call.ID,
		Name: call.Name,
		Args: call.Args,
	}

	startedAt := time.Now()
	context := toolCallContext(tc)
	decision := a.evaluateToolDecision(tc)

	event := ToolCallEvent{
		ID:        tc.ID,
		Name:      tc.Name,
		Context:   context,
		Args:      tc.Args,
		Decision:  decision,
		StartedAt: startedAt,
	}
	a.Emit(EventToolCall, event)

	status := fmt.Sprintf("running %s", tc.Name)
	if context != "" {
		status = fmt.Sprintf("running %s (%s)", tc.Name, context)
	}
	a.Emit(EventStatus, LongTaskStatus{
		TaskID:      tc.ID,
		Phase:       tc.Name,
		ProgressPct: 0,
		Message:     status,
	})

	return a.executeTool(ctx, tc)
}

func toolCallContext(tc ai.ToolCall) string {
	if path, ok := tc.Args["path"].(string); ok {
		return path
	}
	if cmd, ok := tc.Args["command"].(string); ok {
		return cmd
	}
	if pattern, ok := tc.Args["pattern"].(string); ok {
		return pattern
	}
	if dir, ok := tc.Args["dir"].(string); ok {
		return dir
	}
	return ""
}

// drainStream reads all chunks from the response, emitting EventToken for each text chunk.
func (a *Agent) drainStream(resp ai.CompletionResponse) (string, string, ai.Usage, error) {
	var text string
	var thought string
	ch := resp.Stream()
	for chunk := range ch {
		if chunk.Error != nil {
			a.Emit(EventError, chunk.Error)
			return text, thought, resp.Usage(), fmt.Errorf("stream chunk: %w", chunk.Error)
		}
		if chunk.Done {
			break
		}
		if chunk.Thought != "" {
			a.Emit(EventThought, chunk.Thought)
			formatted := formatThinking(chunk.Thought)
			a.Emit(EventThinking, formatted)
			thought += chunk.Thought
		}
		if chunk.Text != "" {
			a.Emit(EventToken, chunk.Text)
			text += chunk.Text
		}
	}
	return text, thought, resp.Usage(), nil
}

// formatThinking formats thinking chunks for display.
func formatThinking(thought string) string {
	const maxRunes = 100
	runes := []rune(strings.TrimSpace(thought))
	if len(runes) > maxRunes {
		runes = runes[:maxRunes]
		return "● " + string(runes) + "..."
	}
	return "● " + string(runes)
}

func (a *Agent) executeTool(ctx context.Context, tc ai.ToolCall) (tools.Result, error) {
	t, ok := a.tools.Get(tc.Name)
	if !ok {
		return tools.Result{IsError: true, Content: fmt.Sprintf("unknown tool: %s", tc.Name)}, nil
	}

	// Pre-tool hooks may veto before any approval or execution work happens.
	if blocked, reason := a.runPreToolHooks(ctx, tc); blocked {
		return tools.Result{IsError: true, Content: reason}, nil
	}

	approvalScope := a.scopedToolApprovalKey(tc, t)
	legacyScope := legacyToolApprovalScope(tc, t)

	allowed := a.sessionGrants.Has(approvalScope) || a.sessionGrants.Has(legacyScope)
	if !allowed {
		allowed = a.shellGrantMatches(approvalScope)
	}

	// Working-directory boundary: paths outside the project (or protected
	// locations inside it) need their own approval, whatever the mode-based
	// policy says. "Always allow" answers grant the path's DIRECTORY, not
	// the whole tool.
	if !allowed {
		if decision := a.checkPathBoundary(tc, t); !decision.Allowed {
			res := a.requestPathConfirmation(tc, decision)
			if !res.Allow {
				msg := "user declined: " + decision.Reason
				if res.Feedback != "" {
					msg = fmt.Sprintf("user declined: %s", res.Feedback)
				}
				return tools.Result{IsError: true, Content: msg}, nil
			}
			if res.Always {
				if grant := a.pathGrantFor(tc, t, decision); grant != "" {
					a.sessionGrants.Add(grant)
					if dir, ok := tools.IsDirGrant(grant); ok {
						a.pathScope().AddGrantedDir(dir)
					}
				}
			}
		}
	}

	if !allowed && a.needsConfirmation(tc, t) {
		res := a.requestConfirmation(tc)
		if !res.Allow {
			msg := "user declined tool execution"
			if res.Feedback != "" {
				msg = fmt.Sprintf("user declined: %s", res.Feedback)
			}
			return tools.Result{IsError: true, Content: msg}, nil
		}
		if res.Always {
			a.sessionGrants.Add(approvalScope) // persist hook writes session + storage
		}
	}

	// Per-tool timeout from metadata; zero keeps the caller's deadline.
	if meta := tools.MetaOf(t); meta.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, meta.Timeout)
		defer cancel()
	}

	// Permission-aware timing: the clock starts HERE — after pre-tool
	// hooks and user approval — so waiting on a permission prompt never
	// counts against the tool's duration.
	execStart := time.Now()
	result, err := t.Execute(ctx, tc.Args)
	execDuration := time.Since(execStart)

	a.runPostToolHooks(ctx, tc, result)
	if path := toolAccessedPath(tc.Name, tc.Args); path != "" && a.skillPaths != nil {
		a.skillPaths.record(path)
	}
	if result.Metadata == nil {
		result.Metadata = make(map[string]any)
	}
	result.Metadata["execDurationMs"] = execDuration.Milliseconds()
	return result, err
}

func toolApprovalScope(tc ai.ToolCall, t tools.Tool) string {
	action, risk := toolApprovalDimensions(tc, t)
	return fmt.Sprintf("name=%q;action=%s;risk=%s", tc.Name, action, risk)
}

// scopedToolApprovalKey returns the approval scope key for this agent,
// prefixed with the project directory so approvals granted in one project
// never apply in another. When the work dir is unknown the plain scope is
// used so lookups stay consistent within a session.
func (a *Agent) scopedToolApprovalKey(tc ai.ToolCall, t tools.Tool) string {
	scope := a.buildApprovalScope(tc, t)
	if a.workDir == "" {
		return scope
	}
	return "project=" + a.workDir + ";" + scope
}

func legacyToolApprovalScope(tc ai.ToolCall, t tools.Tool) string {
	action, risk := toolApprovalDimensions(tc, t)
	return fmt.Sprintf("%s|%s|%s", tc.Name, action, risk)
}

func toolApprovalDimensions(tc ai.ToolCall, t tools.Tool) (action string, risk string) {
	action = "read"
	if !t.IsReadOnly(tc.Args) {
		action = "write"
	}
	if t.IsDestructive(tc.Args) {
		action = "destructive"
	}
	risk = strings.TrimSpace(strings.ToLower(t.EstimatedCost().RiskLevel))
	if risk == "" {
		risk = "unknown"
	}
	return action, risk
}

func (a *Agent) pruneFirstMessageTriage() {
	if len(a.sess.Messages) == 0 {
		return
	}
	first := &a.sess.Messages[0]
	if first.Role != ai.RoleUser || first.Metadata == nil {
		return
	}
	original, ok := first.Metadata[originalUserPromptMetadataKey].(string)
	if ok {
		first.Content = []ai.ContentPart{{Type: ai.ContentTypeText, Text: original}}
	}
	delete(first.Metadata, triageInjectedMetadataKey)
	delete(first.Metadata, originalUserPromptMetadataKey)
	if len(first.Metadata) == 0 {
		first.Metadata = nil
	}
}

func (a *Agent) requestConfirmation(tc ai.ToolCall) ConfirmationResponse {
	ch := make(chan ConfirmationResponse, 1)
	a.Emit(EventConfirm, map[string]any{"tool_call": tc, "reply": ch})
	// Use configurable timeout with a sensible default (10 minutes)
	timeout := 10 * time.Minute
	if a.cfg != nil && a.cfg.ConfirmationTimeout != "" {
		if d, err := time.ParseDuration(a.cfg.ConfirmationTimeout); err == nil {
			timeout = d
		}
	}
	select {
	case res := <-ch:
		return res
	case <-time.After(timeout):
		return ConfirmationResponse{Allow: false}
	}
}

// handleQuotaExceeded prompts the user when AI quota is exceeded.
func (a *Agent) handleQuotaExceeded(ctx context.Context, err error) bool {
	a.Emit(EventStatus, "AI quota exceeded for "+a.provider.Name())

	ch := make(chan string, 1)
	a.Emit(EventAskUser, map[string]any{
		"question": "AI quota exceeded. Press Enter to retry once you've resolved it (e.g., upgraded plan or waited), or type 'abort' to stop:",
		"reply":    ch,
	})

	select {
	case res := <-ch:
		if strings.ToLower(strings.TrimSpace(res)) == "abort" {
			return false
		}
		a.Emit(EventStatus, "retrying after quota resolution...")
		return true
	case <-ctx.Done():
		return false
	case <-time.After(1 * time.Hour): // Wait up to an hour for user intervention
		return false
	}
}

func (a *Agent) Emit(eventType string, payload any) {
	// Recover from potential send-on-closed-channel panics; this guards against
	// concurrent Close() racing with Emit().
	defer func() {
		if r := recover(); r != nil {
			// swallow panic - channel closed while sending
		}
	}()

	// If events channel is closed, drop events to avoid panic
	a.mu.RLock()
	closed := a.eventsClosed
	a.mu.RUnlock()
	if closed {
		return
	}

	e := Event{Type: eventType, Payload: payload}
	// Critical events should not be dropped. Block with a short timeout to avoid deadlock.
	critical := map[string]bool{
		EventConfirm:   true,
		EventToolCall:  true,
		EventToolStart: true,
		EventToolDone:  true,
		EventError:     true,
		EventDone:      true,
	}

	if critical[eventType] {
		// Try an immediate send first.
		select {
		case a.events <- e:
			return
		default:
		}
		// Block briefly to ensure delivery for critical events.
		select {
		case a.events <- e:
			return
		case <-time.After(5 * time.Second):
			// Give up after timeout to avoid deadlock.
			return
		}
	}

	// Non-critical events: attempt non-blocking send, dropping oldest if buffer is full.
	select {
	case a.events <- e:
		return
	default:
		// Drop oldest and retry once
		select {
		case <-a.events:
		default:
		}
		select {
		case a.events <- e:
			return
		default:
			// Still full; drop event
			return
		}
	}
}

// recordToTranscript appends a message to the durable transcript.
func (a *Agent) recordToTranscript(msg ai.Message) {
	if mgr := a.ContextManager(); mgr != nil {
		if tm := mgr.TranscriptManager(); tm != nil {
			tm.Append(msg)
		}
	}
}

func buildToolSchemas(reg *tools.Registry) []ai.ToolSchema {
	var schemas []ai.ToolSchema
	for _, t := range reg.All() {
		schemas = append(schemas, ai.ToolSchema{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Schema(),
		})
	}
	return schemas
}

func (a *Agent) buildActiveToolSchemas() []ai.ToolSchema {
	all := buildToolSchemas(a.tools)
	var schemas []ai.ToolSchema
	if a.toolProfile != nil {
		schemas = filterToolSchemas(all, a.toolProfile)
	} else {
		schemas = all
	}
	schemas = applyModeMask(schemas, a.currentMode())
	return applyDefinitionMask(schemas, a.currentAgentDef)
}

// ensurePhaseComponents lazily wires the prompt system, phase manager, and
// classifier when an Agent was built without New (tests, embedders). Safe to
// call repeatedly; existing components are left alone.
func (a *Agent) ensurePhaseComponents() {
	if a.promptSystem == nil && a.provider != nil {
		llmClient := promptpkg.NewAIProviderAdapter(a.provider, "")
		a.promptSystem = promptpkg.NewPromptSystemWithLLM(promptpkg.DefaultPromptConfig(), a.ContextManager(), a.workDir, llmClient, a.tools)
	}
	if a.phaseManager == nil {
		def := a.currentAgentDef
		if def == nil {
			def = builtin.GeneralAgent()
		}
		a.phaseManager = promptpkg.NewPhaseManager(a.promptSystem, def)
	}
	if a.phaseClassifier == nil && a.provider != nil {
		a.phaseClassifier = promptpkg.NewPhaseClassifier(a.phaseManager, promptpkg.NewAIProviderAdapter(a.provider, ""))
	}
}

// SetDefinition applies an agent-type definition to this agent: the
// definition's system prompt and phase configuration take over from the
// general-purpose default, and its Tools list (when non-empty) becomes the
// agent's capability mask. Called on children before they run (see Execute);
// the general-purpose definition's empty list means the full registry.
func (a *Agent) SetDefinition(def *agentdef.AgentDefinition) {
	if def == nil {
		return
	}
	a.currentAgentDef = def
	if a.promptSystem != nil && a.provider != nil {
		llmClient := promptpkg.NewAIProviderAdapter(a.provider, "")
		a.phaseManager = promptpkg.NewPhaseManager(a.promptSystem, def)
		a.phaseClassifier = promptpkg.NewPhaseClassifier(a.phaseManager, llmClient)
	}
}

func (a *Agent) selectToolProfile(ctx context.Context, userPrompt string) map[string]bool {
	all := buildToolSchemas(a.tools)
	if len(all) == 0 || a.Provider() == nil {
		return nil
	}
	system := `Classify the current user request for tool personalization only.
Return exactly one JSON object and no other text: {"category":"..."}.
Allowed categories: feature_addition, bug_fix, issue_investigation, review, test, plan, question, direct_command, conversation, unknown.
Do not answer the request and do not call tools.`
	request := ai.CompletionRequest{
		Messages:    []ai.Message{ai.NewTextMessage(ai.RoleUser, userPrompt)},
		System:      system,
		Temperature: 0,
		MaxTokens:   128,
		Stream:      true,
	}
	routerCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	resp, err := a.Provider().Complete(routerCtx, request)
	if err != nil || resp == nil {
		return nil
	}
	var text strings.Builder
	for chunk := range resp.Stream() {
		if chunk.Text != "" {
			text.WriteString(chunk.Text)
		}
	}
	var selected struct {
		Category string `json:"category"`
	}
	if json.Unmarshal([]byte(extractJSONObject(text.String())), &selected) != nil {
		return nil
	}
	switch selected.Category {
	case "feature_addition", "bug_fix", "direct_command":
		return allToolNames(all)
	case "test":
		return verificationToolNames()
	case "issue_investigation", "review", "plan", "question":
		return readOnlyToolNames()
	case "conversation":
		return contextToolNames()
	default:
		return allToolNames(all)
	}
}

func extractJSONObject(value string) string {
	value = strings.TrimSpace(value)
	start := strings.IndexByte(value, '{')
	end := strings.LastIndexByte(value, '}')
	if start < 0 || end < start {
		return value
	}
	return value[start : end+1]
}

// selectToolProfileFromIntents determines tool access based on identified intents from the prompt system.
// This replaces the old selectToolProfile which made a separate LLM call for categorization.
func (a *Agent) selectToolProfileFromIntents(ctx context.Context) map[string]bool {
	all := buildToolSchemas(a.tools)
	if len(all) == 0 || a.Provider() == nil {
		return nil
	}

	intentSet := a.promptSystem.GetCurrentIntentSet()
	if intentSet == nil {
		// No intents identified yet, allow all tools
		return allToolNames(all)
	}

	// If the init phase already explored the codebase, remove broad discovery
	// tools so the model reads the pre-found files instead of re-searching.
	initResults := a.promptSystem.GetInitResults()
	initSatisfied := initResults != nil && len(initResults.FilesFound) > 0

	// Determine tool access based on intents
	hasWriteIntents := false
	hasReadOnlyIntents := false
	hasTestIntents := false

	for _, intent := range intentSet.Intents {
		switch intent.Type {
		case promptpkg.IntentImplement, promptpkg.IntentFix, promptpkg.IntentRefactor, promptpkg.IntentCommit, promptpkg.IntentDocument:
			hasWriteIntents = true
		case promptpkg.IntentTest:
			hasTestIntents = true
		case promptpkg.IntentExplore, promptpkg.IntentDebug, promptpkg.IntentReview, promptpkg.IntentPlan, promptpkg.IntentQuestion, promptpkg.IntentDirect:
			hasReadOnlyIntents = true
		}
	}

	var profile map[string]bool
	switch {
	case hasWriteIntents:
		profile = allToolNames(all)
	case hasTestIntents:
		profile = verificationToolNames()
	case hasReadOnlyIntents:
		profile = readOnlyToolNames()
	default:
		profile = allToolNames(all)
	}

	if initSatisfied {
		delete(profile, "glob")
		delete(profile, "list_directory")
	}

	return profile
}

func allToolNames(all []ai.ToolSchema) map[string]bool {
	names := make(map[string]bool, len(all))
	for _, schema := range all {
		names[schema.Name] = true
	}
	return names
}

func readOnlyToolNames() map[string]bool {
	names := map[string]bool{
		"read_file": true, "list_directory": true,
		"grep": true, "glob": true,
		"lsp_diagnostics": true, "list_shells": true, "read_shell": true,
		"list_agents": true, "read_agent": true,
		"web_search": true, "web_fetch": true,
	}
	for name := range contextToolNames() {
		names[name] = true
	}
	return names
}

func verificationToolNames() map[string]bool {
	names := readOnlyToolNames()
	names["bash"] = true
	names["write_shell"] = true
	names["stop_shell"] = true
	names["todo_write"] = true
	names["wait"] = true
	return names
}

func contextToolNames() map[string]bool {
	return map[string]bool{
		"context_bucket_get": true, "context_bucket_set": true,
		"context_bucket_delete": true, "context_get": true,
	}
}

func filterToolSchemas(all []ai.ToolSchema, allowed map[string]bool) []ai.ToolSchema {
	filtered := make([]ai.ToolSchema, 0, len(all))
	for _, schema := range all {
		if allowed[schema.Name] {
			filtered = append(filtered, schema)
		}
	}
	// A registry mismatch must not strand the model without tools. Returning
	// the registered surface is safer than silently sending an empty list.
	if len(filtered) == 0 {
		return all
	}
	return filtered
}

// checkContextLimit emits warning/critical events when context usage is high.
// It uses the approximate token count so no provider API call is needed.
func (a *Agent) checkContextLimit(provider ai.Provider, messages []ai.Message) {
	tokens, err := provider.TokenCount(messages)
	if err != nil {
		return
	}

	limit := a.cfg.MaxContextTokens
	if limit <= 0 {
		limit = provider.ContextLimit()
	}
	if limit <= 0 {
		return
	}

	fraction := float64(tokens) / float64(limit)
	switch {
	case fraction >= 0.95:
		a.Emit(EventStatus, fmt.Sprintf(
			"⚠ Context is %d%% full (%d/%d tokens). Next request may fail — use /compress to reduce context.",
			int(fraction*100), tokens, limit,
		))
	case fraction >= 0.80:
		a.Emit(EventStatus, fmt.Sprintf(
			"Context is %d%% full (%d/%d tokens). Consider using /compress.",
			int(fraction*100), tokens, limit,
		))
	}
}

// gitIsRepo reports whether dir is inside a git repository.
func gitIsRepo(ctx context.Context, dir string) bool {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--is-inside-work-tree")
	cmd.Env = os.Environ()
	return cmd.Run() == nil
}

// runStandardLoop is the fallback when first-message classification fails:
// the request still executes through the build phase with the default
// general-purpose configuration rather than silently doing nothing.
func (a *Agent) runStandardLoop(ctx context.Context, prompt string, isFirstMessage bool) error {
	result := a.phaseManager.ExecutePhase(shared.PhaseBuild, shared.TaskSpec{
		Type:        "build",
		Description: prompt,
		Role:        "implementer",
		Priority:    1,
	})
	if result.Error != nil {
		a.Emit(EventError, result.Error)
		return result.Error
	}
	return a.runPhaseLoop(ctx, shared.PhaseBuild, result)
}

// getAvailableFiles returns a list of available files for classification.
func (a *Agent) getAvailableFiles() []string {
	mgr := a.ContextManager()
	if mgr != nil {
		return mgr.RecentFiles(20)
	}
	return []string{}
}

// handleViolation handles a detected violation.
func (a *Agent) handleViolation(v *shared.ViolationCheck) {
	a.Emit(EventNotify, map[string]any{
		"level":   "warning",
		"title":   "Policy Violation Detected",
		"message": fmt.Sprintf("Type: %s, Severity: %s, Action: %s", v.Type, v.Severity, v.Action),
	})
	
	// Record violation in phase manager
	a.phaseManager.RecordViolation(v.Type, v.Severity, v.UserMessage, v.AgentResponse)
}

// askClarification asks the user for clarification.
func (a *Agent) askClarification(questions []string) {
	for _, q := range questions {
		a.Emit(EventAskUser, map[string]any{
			"question": q,
		})
	}
}

// answerDirectQuestion answers a direct question in init phase.
func (a *Agent) answerDirectQuestion(ctx context.Context, question string) error {
	// Use a simple completion for direct questions
	provider := a.Provider()
	systemPrompt := a.getSystemPrompt(ctx, provider)
	
	req := ai.CompletionRequest{
		Messages: prependUserContext([]ai.Message{ai.NewTextMessage(ai.RoleUser, question)}, a.userContext()),
		Tools:    []ai.ToolSchema{},
		System:   systemPrompt,
		Temperature: 0.0,
		MaxTokens:  1024,
		Stream:     true,
	}
	
	resp, err := provider.Complete(ctx, req)
	if err != nil {
		return err
	}
	
	text, _, _, err := a.drainStream(resp)
	if err != nil {
		return err
	}

	// Record the answer so it survives compaction and session resume.
	msg := ai.NewTextMessage(ai.RoleAssistant, text)
	a.sess.AddMessage(msg)
	a.recordToTranscript(msg)
	a.tryPersist()

	a.Emit(EventDone, text)
	return nil
}

// toolSetToProfile converts a ToolSet to a tool profile map.
func (a *Agent) toolSetToProfile(ts shared.ToolSet) map[string]bool {
	all := buildToolSchemas(a.tools)
	profile := make(map[string]bool)
	
	switch ts {
	case shared.ToolSetContextOnly:
		return map[string]bool{}
	case shared.ToolSetReadOnly:
		for _, schema := range all {
			if schema.Name == "read_file" || schema.Name == "list_directory" ||
				schema.Name == "grep" || schema.Name == "glob" ||
				schema.Name == "lsp_diagnostics" || schema.Name == "list_shells" || schema.Name == "read_shell" ||
				schema.Name == "list_agents" || schema.Name == "read_agent" ||
				schema.Name == "web_search" || schema.Name == "web_fetch" {
				profile[schema.Name] = true
			}
		}
	case shared.ToolSetBasic:
		// INIT's toolset: bash, read, edits, task — enough to fulfill
		// direct requests itself, deliberately no todo tools.
		for _, schema := range all {
			if schema.Name == "read_file" || schema.Name == "list_directory" ||
				schema.Name == "grep" || schema.Name == "glob" ||
				schema.Name == "bash" || schema.Name == "task" ||
				schema.Name == "edit_file" || schema.Name == "write_file" ||
				schema.Name == "web_search" || schema.Name == "web_fetch" {
				profile[schema.Name] = true
			}
		}
	case shared.ToolSetModerate:
		for _, schema := range all {
			if schema.Name != "agent_control" {
				profile[schema.Name] = true
			}
		}
	case shared.ToolSetFull:
		for _, schema := range all {
			profile[schema.Name] = true
		}
	}
	
	return profile
}

// getMCPServers returns MCP server info for prompt composition.
func (a *Agent) getMCPServers() []promptpkg.MCPServerInfo {
	// This would integrate with actual MCP server registry
	return []promptpkg.MCPServerInfo{}
}

func (a *Agent) Shutdown() error {
	return nil
}

// Close cleans up agent resources, ensuring the events channel is closed exactly once.
func (a *Agent) Close() error {
	var err error
	a.closeOnce.Do(func() {
		// Mark closed under lock then close the channel
		a.mu.Lock()
		if !a.eventsClosed {
			a.eventsClosed = true
			close(a.events)
		}
		a.mu.Unlock()
	})
	return err
}

// ContextToolRegistration reports whether context operations were made available.
type ContextToolRegistration struct {
	Enabled bool
	Names   []string
	Reason  string
}

// RegisterContextTools exposes context operations to the model.
func (a *Agent) RegisterContextTools() ContextToolRegistration {
	var names []string
	reason := "task state tools registered"

	// Register task/todo/context state tools (prompt-backed, no graph dependency)
	if a.tools != nil && a.promptSystem != nil {
		tools.RegisterTaskStateTools(a.tools, a.promptSystem.GetTaskState())
		names = append(names,
			"todo_write", "todo_list",
			"context_bucket_get", "context_bucket_set", "context_bucket_delete",
			"context_get",
		)

		// Surface model-driven todo mutations as UI events.
		a.promptSystem.GetTaskState().SetTodoListener(a.notifyTodos)
	} else if a.tools == nil {
		return ContextToolRegistration{Enabled: false, Reason: "tool registry unavailable"}
	} else {
		reason = "prompt state tools registered"
	}

	// Register violation detection tools
	if a.tools != nil {
		a.tools.Register(tools.NewViolationTool())
		a.tools.Register(tools.NewBlockSessionTool())
		a.tools.Register(tools.NewOverrideViolationTool())
		names = append(names,
			"violation_detected", "block_session", "override_violation",
		)
	}

	// Register the builtin expansion suite: git, wait, multi_edit, finish.
	if a.tools != nil {
		if a.cfg != nil && a.cfg.EditReview && a.editReview == nil {
			a.editReview = editreview.NewStore()
			editreview.WrapWriteTools(a.tools, a.editReview)
		}
		a.tools.Register(toolsShell.NewWaitTool())
		a.tools.Register(toolsFS.NewMultiEditTool(a.cfg))
			subagent.RegisterControlTool(a.tools)
		names = append(names,
			"git_status", "git_diff", "git_log", "git_add", "git_commit",
			"git_branch", "git_checkout", "git_stash",
			"wait", "multi_edit", "agent_control",
		)

		// Load user-defined agents from .agents/*.md in the workspace.
		if loaded, err := subagent.LoadAgentDefinitions(filepath.Join(a.workDir, ".agents")); err == nil && len(loaded) > 0 {
			a.Emit(EventStatus, fmt.Sprintf("loaded %d custom agent(s): %s", len(loaded), strings.Join(loaded, ", ")))
		}
	}

	return ContextToolRegistration{Enabled: len(names) > 0, Names: names, Reason: reason}
}

// getNextTodoPrompt gets the next todo execution prompt from the prompt system
func (a *Agent) getNextTodoPrompt() *promptpkg.PromptPart {
	turnCtx := a.promptSystem.GetTurnContext()
	if turnCtx == nil || len(turnCtx.TodoItems) == 0 {
		return nil
	}

	for i, todo := range turnCtx.TodoItems {
		if todo.Status == promptpkg.TodoStatusPending {
			// Check dependencies
			depsMet := true
			for _, depID := range todo.Dependencies {
				found := false
				for _, t := range turnCtx.TodoItems {
					if t.ID == depID && t.Status == promptpkg.TodoStatusCompleted {
						found = true
						break
					}
				}
				if !found {
					depsMet = false
					break
				}
			}
			if depsMet {
				// Mark as in progress
				turnCtx.TodoItems[i].Status = promptpkg.TodoStatusInProgress
				return a.promptSystem.Manager.TaskPrompts().BuildExecutionPrompt(turnCtx, &turnCtx.TodoItems[i], nil)
			}
		}
	}
	return nil
}

// ContextManager returns the persistent context manager, initializing it on first use.
func (a *Agent) ContextManager() *contextmgr.Manager {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.contextManager != nil {
		// Recreate if the working directory changed (manager is bound to a root dir).
		if cwd, _ := os.Getwd(); cwd != a.contextManagerRoot {
			managerCfg := contextmgr.DefaultManagerConfig()
			managerCfg.ModelLimits = contextmgr.GetModelLimits(a.cfg.Model)
			managerCfg.BudgetConfig = contextmgr.DefaultBudgetConfig()
			a.contextManager = contextmgr.NewManager(cwd, managerCfg)
			a.contextManagerRoot = cwd
		}
		return a.contextManager
	}

	cwd, _ := os.Getwd()
	managerCfg := contextmgr.DefaultManagerConfig()
	managerCfg.ModelLimits = contextmgr.GetModelLimits(a.cfg.Model)
	managerCfg.BudgetConfig = contextmgr.DefaultBudgetConfig()
	a.contextManager = contextmgr.NewManager(cwd, managerCfg)
	a.contextManagerRoot = cwd
	return a.contextManager
}

// Telemetry returns the telemetry collector for context observability.
func (a *Agent) Telemetry() *contextmgr.TelemetryCollector {
	if mgr := a.ContextManager(); mgr != nil {
		return mgr.Telemetry()
	}
	return nil
}

// AdaptiveCalculator returns the adaptive token calculator.
func (a *Agent) AdaptiveCalculator() *contextmgr.AdaptiveTokenCalculator {
	if mgr := a.ContextManager(); mgr != nil {
		return mgr.AdaptiveCalculator()
	}
	return nil
}

// runPromptSystemPipeline runs the new internal/prompt system pipeline for a user request.
// Returns nil if the pipeline completed the task, error if it failed (fallback to standard loop).
func (a *Agent) runPromptSystemPipeline(ctx context.Context, prompt string, isFirstMessage bool) error {
	a.Emit(EventStatus, "🧠 Preparing prompt context")

	// Get available files from context manager
	mgr := a.ContextManager()
	var availableFiles []string
	if mgr != nil {
		availableFiles = mgr.RecentFiles(20)
	}

	// Process through the new prompt system to identify intents and generate tasks
	if a.promptSystem != nil {
		_, err := a.promptSystem.ProcessUserMessage(ctx, prompt, a.workDir, availableFiles)
		if err != nil {
			return fmt.Errorf("prompt system processing failed: %w", err)
		}
		if initResults := a.promptSystem.GetInitResults(); initResults != nil {
			a.Emit(EventStatus, fmt.Sprintf("init phase done: %d files found, %d read, %d errors; tasks: %d",
				len(initResults.FilesFound), len(initResults.CodeSnippets), len(initResults.Errors),
				len(a.promptSystem.GetCurrentTasks())))
		} else {
			a.Emit(EventStatus, "intent identification complete (no init phase required)")
		}
	}

	return errPromptSystemPrepared
}

func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func isCasualMessage(message string) bool {
	normalized := strings.ToLower(strings.TrimSpace(message))
	normalized = strings.Trim(normalized, "!.,? ")
	switch normalized {
	case "hi", "hello", "hey", "hiya", "good morning", "good afternoon", "good evening":
		return true
	default:
		return false
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// convertSkillsForPrompt converts agent skills to prompt.Skill interface.
func (a *Agent) convertSkillsForPrompt(skills []Skill) []promptpkg.Skill {
	result := make([]promptpkg.Skill, len(skills))
	for i, s := range skills {
		result[i] = agentSkillAdapter{skill: s}
	}
	return result
}

// agentSkillAdapter adapts agent.Skill to prompt.Skill interface.
type agentSkillAdapter struct {
	skill Skill
}

func (a agentSkillAdapter) Name() string {
	return a.skill.Name
}

func (a agentSkillAdapter) Description() string {
	return a.skill.Description
}
