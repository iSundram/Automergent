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
	"github.com/iSundram/Automergent/internal/config"
	contextmgr "github.com/iSundram/Automergent/internal/context"
	"github.com/iSundram/Automergent/internal/coordinator"
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
	sessionAllowedTools map[string]bool
	approvalSource      string
	workDir             string
	firstMessageHandled bool
	decisionRecords     []ToolDecisionRecord
	skills              []Skill
	skillPaths          *skillTracker
	editReview          *editreview.Store

	// Persistent components
	contextManager     *contextmgr.Manager
	contextManagerRoot string
	coordinator        *coordinator.Engine
	coordinatorOnce    sync.Once
	coordinatorCtx     context.Context
	coordinatorCancel  context.CancelFunc

	// toolProfile is ephemeral per request. It is never persisted, added to
	// session messages, or rendered in the user-facing prompt.
	toolProfile map[string]bool

	// steer carries user messages injected mid-run. The turn loop drains it at
	// tool boundaries, so a queued message reaches the model before the next
	// call instead of waiting for the whole turn to finish.
	steer chan string

	// New prompt system for staged prompt delivery
	promptSystem     *promptpkg.PromptSystem
	promptSystemOnce sync.Once
}

// Execute implements the AgentExecutor interface for sub-agents.
func (a *Agent) Execute(ctx context.Context, agentType subagent.AgentType, prompt string, model string) (string, error) {
	// 1. Create a child configuration and provider if a specific model is requested.
	childCfg := *a.cfg
	if model != "" {
		childCfg.Model = model
	}
	// Child agents must not trigger the coordinator (prevents infinite recursion).
	childCfg.Coordinator.Enabled = false

	// 2. Create a clean child session.
	childSess := session.New()
	childSess.Metadata["parent_id"] = a.sess.ID
	childSess.Metadata["agent_type"] = string(agentType)

	// 3. Create a child agent.
	childAgent := New(&childCfg, a.provider, childSess, a.tools)

	// 4. Run the child agent with proper cancellation handling.
	var finalResponse string
	var finalErr error
	done := make(chan struct{})

	childCtx, childCancel := context.WithCancel(ctx)
	defer childCancel()

	// Drain events to avoid blocking child agent. The drainer will exit when the child agent
	// events channel is closed via childAgent.Close().
	go func() {
		for range childAgent.Events() {
		}
	}()

	// Run the child agent in a goroutine and signal completion on done.
	go func() {
		finalErr = childAgent.Run(childCtx, prompt)
		// Extract the last assistant message as the result
		if len(childSess.Messages) > 0 {
			lastMsg := childSess.Messages[len(childSess.Messages)-1]
			if lastMsg.Role == ai.RoleAssistant {
				finalResponse = lastMsg.TextContent()
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
		events:              make(chan Event, 8192),
		steer:               make(chan string, 8),
		sessionAllowedTools: make(map[string]bool),
		approvalSource:      "tui",
		skillPaths:          newSkillTracker(12),
		editReview:          nil,
	}

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
	// do not re-prompt for tools the user already approved.
	if sess != nil {
		for _, scope := range sess.ApprovalScopes() {
			agent.sessionAllowedTools[scope] = true
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
// Claude-style: this is USER configuration (effort level / explicit budget),
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

// GetModel returns the current model name.
func (a *Agent) GetModel() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg.Model
}

// SetProvider swaps the runtime provider used for subsequent completions.
func (a *Agent) SetProvider(p ai.Provider) {
	a.mu.Lock()
	a.provider = p
	a.mu.Unlock()
	// The new provider has no observer installed yet: without this, retries go
	// silent after any /provider or /model switch.
	a.installRetryObserver(p)
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
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.sessionAllowedTools, scope)
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
	a.sessionAllowedTools = make(map[string]bool)
	if sess != nil {
		for _, scope := range sess.ApprovalScopes() {
			a.sessionAllowedTools[scope] = true
		}
	}
}

// Run executes the agent loop for the given user prompt.
func (a *Agent) Run(ctx context.Context, prompt string) error {
	originalUserPrompt := prompt

	// 1. Initial Triage Phase (Dynamic Workflow)
	// If this is the very first message, we run a hidden triage loop
	isFirstMessage := a.checkAndMarkFirstMessage()
	if isFirstMessage {
		a.Emit(EventStatus, "initiating project triage")
	}

	// Apply triage wrapper for first message if using legacy mode
	firstUserPrompt := originalUserPrompt

	// Persist the user-authored message before any optional coordinator path
	// can return. The triage wrapper exists only in the request copy below.
	userMsg := ai.NewTextMessage(ai.RoleUser, firstUserPrompt)
	if firstUserPrompt != originalUserPrompt {
		userMsg.Metadata = map[string]any{
			triageInjectedMetadataKey:     true,
			originalUserPromptMetadataKey: originalUserPrompt,
		}
	}
	a.sess.AddMessage(userMsg)
	a.recordToTranscript(userMsg)

	// Use new PromptSystem for full intelligent pipeline
	{
		err := a.runPromptSystemPipeline(ctx, originalUserPrompt, isFirstMessage)
		if err == nil {
			return nil
		}
		if err == errPromptSystemPrepared {
			a.Emit(EventStatus, "prompt and graph context prepared; starting tool-capable execution")
		} else {
			a.Emit(EventError, err)
			return fmt.Errorf("agent: prepare prompt system: %w", err)
		}
	}
	// Determine tool profile from identified intents (new intent-based system)
	a.toolProfile = a.selectToolProfileFromIntents(ctx)
	defer func() { a.toolProfile = nil }()
	if a.toolProfile != nil {
		a.Emit(EventStatus, fmt.Sprintf("native tool surface prepared: %d tools", len(a.toolProfile)))
	}

	// In edit mode, check that we are inside a git repository when required.
	if a.cfg.Mode == "edit" && a.cfg.Security.RequireGitForAutoModes {
		cwd, _ := os.Getwd()
		if !gitIsRepo(ctx, cwd) {
			a.Emit(EventStatus, "⚠ Not a git repository — edit mode requires git for safe rollback")
		}
	}

	// Standard agent loop with legacy system prompt
	firstStandardTurn := isFirstMessage
	runMeta := &runMetadata{}
	for {
		provider := a.Provider()
		a.sess.SetMessages(ai.RepairMissingToolResults(a.sess.Messages))

		// Check context window usage
		tokens, _ := provider.TokenCount(a.sess.Messages)
		limit := a.cfg.MaxContextTokens
		if limit <= 0 {
			limit = provider.ContextLimit()
		}

		autoCompressAt := 0.80
		if a.cfg.AutoCompressAt > 0 {
			autoCompressAt = a.cfg.AutoCompressAt
		}

		if limit > 0 && float64(tokens)/float64(limit) > autoCompressAt {
			a.Emit(EventStatus, "Neural Compaction: Freeing up context window...")
			a.sess.SetMessages(a.CompactSessionMessages(ctx, a.sess.Messages))
		} else {
			a.sess.SetMessages(a.GhostLargeOutputs(a.sess.Messages))
		}

		a.checkContextLimit(provider, a.sess.Messages)

		// Use new prompt system for system prompt (categorized, todo-aware, context-aware)
		// Falls back to legacy buildSystemPrompt if prompt system not available
		systemPrompt := a.getSystemPrompt(ctx, provider)
		toolSchemas := a.buildActiveToolSchemas()

		thinkingBudget := a.getThinkingBudget()
		thinkingLevel := a.getThinkingLevel()

		req := ai.CompletionRequest{
			Messages:    a.sess.Messages,
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
		// Tool boundary: a user message inserted here reaches the model on the
		// next call, which is what "steer the run" means.
		a.drainSteer()
		a.injectLongRunContext(runMeta, true)

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
		return "💭 " + string(runes) + "..."
	}
	return "💭 " + string(runes)
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

	a.mu.RLock()
	allowed := a.sessionAllowedTools[approvalScope] || a.sessionAllowedTools[legacyScope]
	if !allowed {
		allowed = a.shellGrantMatches(approvalScope)
	}
	a.mu.RUnlock()

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
			a.mu.Lock()
			a.sessionAllowedTools[approvalScope] = true
			a.mu.Unlock()
			if a.sess != nil {
				a.sess.AddApproval(approvalScope, a.approvalSource)
			}
			a.tryPersist()
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
	return applyModeMask(schemas, a.currentMode())
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
		"read_file": true, "view": true, "list_directory": true,
		"grep": true, "glob": true, "search": true,
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
		"context_bucket_create": true, "context_bucket_list": true,
		"context_bucket_get": true, "context_bucket_update": true,
		"context_share": true, "remember": true,
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

func (a *Agent) Shutdown() error {
	return nil
}

// Close cleans up agent resources, ensuring the events channel is closed exactly once.
func (a *Agent) Close() error {
	var err error
	a.closeOnce.Do(func() {
		// Stop coordinator if running
		if a.coordinatorCancel != nil {
			a.coordinatorCancel()
		}
		if a.coordinator != nil {
			_ = a.coordinator.Stop(context.Background())
		}
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

// Coordinator returns the coordinator engine, initializing it on first use if enabled.
func (a *Agent) Coordinator() *coordinator.Engine {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.coordinator != nil {
		return a.coordinator
	}

	if !a.cfg.Coordinator.Enabled {
		return nil
	}

	// Initialize coordinator
	cfg := coordinator.DefaultConfig()
	cfg.WorkersPerRole = make(map[coordinator.AgentRole]int)
	for roleStr, count := range a.cfg.Coordinator.WorkersPerRole {
		cfg.WorkersPerRole[coordinator.AgentRole(roleStr)] = count
	}
	if a.cfg.Coordinator.DefaultTimeout != "" {
		if d, err := time.ParseDuration(a.cfg.Coordinator.DefaultTimeout); err == nil {
			cfg.DefaultTimeout = d
		}
	}
	cfg.MaxRetries = a.cfg.Coordinator.MaxRetries
	cfg.QualityThreshold = a.cfg.Coordinator.QualityThreshold
	cfg.ConsensusThreshold = a.cfg.Coordinator.ConsensusThreshold
	cfg.ResourceLimits.MaxTokensPerTask = a.cfg.Coordinator.ResourceLimits.MaxTokensPerTask
	cfg.ResourceLimits.MaxConcurrentTasks = a.cfg.Coordinator.ResourceLimits.MaxConcurrentTasks
	cfg.ResourceLimits.MaxMemoryMB = a.cfg.Coordinator.ResourceLimits.MaxMemoryMB
	cfg.ResourceLimits.RateLimitPerMinute = a.cfg.Coordinator.ResourceLimits.RateLimitPerMinute
	cfg.Model = a.cfg.Model
	cfg.FallbackModel = a.cfg.Model

	// Map model overrides from string keys to AgentRole keys.
	if len(a.cfg.Coordinator.ModelOverrides) > 0 {
		cfg.ModelOverrides = make(map[coordinator.AgentRole]string)
		for roleStr, model := range a.cfg.Coordinator.ModelOverrides {
			cfg.ModelOverrides[coordinator.AgentRole(roleStr)] = model
		}
	}

	// Create executor that uses the agent's sub-agent mechanism with context manager.
	// Use NewAgentExecutorAdapterWithModel to avoid deadlock: Coordinator() holds a.mu.Lock()
	// and GetModel() tries a.mu.RLock() which deadlocks on sync.RWMutex.
	exec := coordinator.NewAgentExecutorAdapterWithModel(a, a.cfg.Model)

	a.coordinatorCtx, a.coordinatorCancel = context.WithCancel(context.Background())
	a.coordinator = coordinator.NewEngine(cfg, exec)

	// Start coordinator
	if err := a.coordinator.Start(a.coordinatorCtx); err != nil {
		a.Emit(EventError, fmt.Errorf("coordinator start: %w", err))
		a.coordinator = nil
		return nil
	}

	return a.coordinator
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
			"task_list", "task_get", "task_update",
			"context_bucket_create", "context_bucket_list", "context_bucket_get",
			"context_bucket_set", "context_bucket_delete", "context_list_buckets",
			"context_get_intent", "context_get_init",
			"todo_list", "todo_next", "todo_write",
		)

		// Surface model-driven todo mutations as UI events.
		a.promptSystem.GetTaskState().SetTodoListener(a.notifyTodos)
	} else if a.tools == nil {
		return ContextToolRegistration{Enabled: false, Reason: "tool registry unavailable"}
	} else {
		reason = "prompt state tools registered"
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

// initializeCoordinatorWithPromptSystem initializes the coordinator with the prompt system.
func (a *Agent) initializeCoordinatorWithPromptSystem() error {
	a.coordinatorOnce.Do(func() {
		if a.cfg == nil || !a.cfg.Coordinator.Enabled {
			return
		}

		exec := coordinator.NewAgentExecutorAdapterWithModel(a, a.cfg.Model)
		a.coordinatorCtx, a.coordinatorCancel = context.WithCancel(context.Background())

		// Convert config.CoordinatorConfig to coordinator.CoordinatorConfig
		coordCfg := a.convertCoordinatorConfig(a.cfg.Coordinator)

		// Use NewEngineWithPromptManager to wire the new prompt system
		a.coordinator = coordinator.NewEngineWithPromptManager(&coordCfg, exec, a.promptSystem.Manager)

		if err := a.coordinator.Start(a.coordinatorCtx); err != nil {
			a.Emit(EventError, fmt.Errorf("coordinator start: %w", err))
			a.coordinator = nil
		}
	})
	return nil
}

// getSystemPrompt returns THE system prompt. There is a single builder —
// no legacy fallback, no assistant/coder split (see systemprompt.go).
func (a *Agent) getSystemPrompt(ctx context.Context, provider ai.Provider) string {
	return a.buildUnifiedSystemPrompt(ctx, provider)
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

func (a *Agent) convertCoordinatorConfig(cfg config.CoordinatorConfig) coordinator.CoordinatorConfig {
	return coordinator.CoordinatorConfig{
		MaxWorkers:         10,
		WorkersPerRole:     map[coordinator.AgentRole]int{},
		ModelOverrides:     map[coordinator.AgentRole]string{},
		DefaultTimeout:     5 * time.Minute,
		MaxRetries:         3,
		EnableWorkStealing: true,
		QualityThreshold:   0.7,
		ConsensusThreshold: 2,
		ResourceLimits: coordinator.ResourceLimits{
			MaxTokensPerTask:   100000,
			MaxConcurrentTasks: 5,
			MaxMemoryMB:        512,
			RateLimitPerMinute: 60,
		},
		EventsBufferSize: 1024,
	}
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
