package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/config"
	contextmgr "github.com/iSundram/Automergent/internal/context"
	"github.com/iSundram/Automergent/internal/coordinator"
	"github.com/iSundram/Automergent/internal/errors"
	promptpkg "github.com/iSundram/Automergent/internal/prompt"
	"github.com/iSundram/Automergent/internal/reasoning"
	"github.com/iSundram/Automergent/internal/session"
	"github.com/iSundram/Automergent/internal/tools"
	"github.com/iSundram/Automergent/internal/version"
	subagent "github.com/iSundram/Automergent/internal/tools/agent"
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
	reasoningPreAnalyze func(context.Context, string) (string, error)
	currentComplexity   reasoning.Complexity
	currentTaskType     reasoning.TaskType

	// Persistent components
	contextManager     *contextmgr.Manager
	contextManagerRoot string
	reasoningEngine    *reasoning.Engine
	coordinator        *coordinator.Engine
	coordinatorOnce    sync.Once
	coordinatorCtx     context.Context
	coordinatorCancel  context.CancelFunc

	// New prompt system for staged prompt delivery
	promptSystem       *promptpkg.PromptSystem
	promptSystemOnce   sync.Once
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
	EventToken     = "token"
	EventThought   = "thought"
	EventToolCall  = "tool_call"
	EventToolStart = "tool_start"
	EventToolDone  = "tool_done"
	EventDone      = "done"
	EventError     = "error"
	EventConfirm   = "confirm"
	EventAskUser   = "ask_user"
	EventNotify    = "notify"
	EventStatus    = "status"
	EventThinking  = "thinking"
	EventCompacted = "compacted"
)

const (
	triageInjectedMetadataKey     = "triage_injected"
	originalUserPromptMetadataKey = "original_user_prompt"
)

// New creates a new Agent.
func New(cfg *config.Config, provider ai.Provider, sess *session.Session, reg *tools.Registry) *Agent {
	agent := &Agent{
		cfg:                 cfg,
		provider:            provider,
		sess:                sess,
		tools:               reg,
		events:              make(chan Event, 8192),
		sessionAllowedTools: make(map[string]bool),
		approvalSource:      "tui",
		promptSystem:        promptpkg.NewPromptSystem(),
	}

	if cfg != nil && cfg.NoTUI {
		agent.approvalSource = "headless"
	}

	// Record the project directory so "always allow" approvals are scoped
	// to this project and do not leak into sessions resumed elsewhere.
	if wd, err := os.Getwd(); err == nil {
		agent.workDir = wd
	}

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

func (a *Agent) runReasoningPreAnalysis(ctx context.Context, prompt string) (string, error) {
	if a.reasoningPreAnalyze != nil {
		return a.reasoningPreAnalyze(ctx, prompt)
	}

	engine := reasoning.NewEngine(nil)
	analysis, err := engine.Analyze(ctx, prompt)
	if err != nil {
		return "", err
	}

	a.mu.Lock()
	a.currentTaskType = analysis.TaskType
	a.currentComplexity = analysis.Complexity
	a.mu.Unlock()

	return fmt.Sprintf("%s/%s", analysis.TaskType, analysis.Scope), nil
}

func (a *Agent) getThinkingBudget() int {
	a.mu.RLock()
	complexity := a.currentComplexity
	a.mu.RUnlock()

	// Default budget
	budget := 10000

	switch complexity {
	case reasoning.ComplexityTrivial:
		budget = 2000
	case reasoning.ComplexitySimple:
		budget = 4000
	case reasoning.ComplexityModerate:
		budget = 8000
	case reasoning.ComplexityComplex:
		budget = 16000
	case reasoning.ComplexityMajor:
		budget = 32000
	}

	// Override from config if explicitly set
	if a.cfg.MaxContextTokens > 0 && budget > a.cfg.MaxContextTokens/4 {
		budget = a.cfg.MaxContextTokens / 4
	}

	return budget
}

// Events returns the channel of agent events.
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
	defer a.mu.Unlock()
	a.provider = p
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
		// Use externalized instruction from triage.go
		prompt = TriageInstruction + "\n\nUser Request: " + prompt
	}

	// 1. Process first message through new prompt system (categorization, planning)
	var categorized *promptpkg.CategorizedRequest
	var promptParts []promptpkg.PromptPart
	var err error
	if isFirstMessage && a.cfg != nil && a.cfg.PromptSystemEnabled {
		a.Emit(EventStatus, "analyzing request with prompt system...")
		categorized, promptParts, err = a.processFirstMessageWithPromptSystem(ctx, originalUserPrompt)
		if err != nil {
			a.Emit(EventStatus, fmt.Sprintf("prompt system: %v, falling back", err))
		} else {
			a.Emit(EventStatus, fmt.Sprintf("categorized as: %s (complexity: %s)", categorized.Category, categorized.Complexity))

			// If needs coder, initialize coordinator with prompt system
			if categorized.RequiresCoder {
				a.Emit(EventStatus, "initializing coordinator with prompt system...")
				a.initializeCoordinatorWithPromptSystem()
			}
		}
	}

	// 2. Coordinator: run in background with a short timeout.
	//    If it doesn't produce a result quickly, fall through to the standard loop.
	coordCh := make(chan string, 1)
	coordErrCh := make(chan error, 1)
	go func() {
		result, err := a.runCoordinatorIfNeeded(ctx, originalUserPrompt)
		if err != nil {
			coordErrCh <- err
		} else {
			coordCh <- result
		}
	}()

	// Wait briefly for coordinator — if it's fast (simple analysis), use it.
	// Otherwise fall through to standard agent loop immediately.
	select {
	case result := <-coordCh:
		if result != "" {
			msg := ai.NewTextMessage(ai.RoleAssistant, result)
			a.sess.AddMessage(msg)
			a.recordToTranscript(msg)
			a.Emit(EventStatus, "Coordinator: task completed")
			return nil
		}
	case err := <-coordErrCh:
		a.Emit(EventStatus, fmt.Sprintf("Coordinator: %v, using standard loop", err))
	case <-time.After(30 * time.Second):
		// Coordinator too slow — proceed with standard loop.
		a.Emit(EventStatus, "Coordinator: taking too long, using standard loop")
	case <-ctx.Done():
		return ctx.Err()
	}

	if a.cfg != nil && a.cfg.ReasoningPreAnalysis {
		a.Emit(EventStatus, "reasoning: pre-analyzing prompt")
		if summary, err := a.runReasoningPreAnalysis(ctx, originalUserPrompt); err != nil {
			a.Emit(EventStatus, "reasoning: unavailable, continuing")
		} else if summary != "" {
			a.Emit(EventStatus, fmt.Sprintf("reasoning: %s", summary))
		}
	}

	// In edit mode, check that we are inside a git repository when required.
	if a.cfg.Mode == "edit" && a.cfg.Security.RequireGitForAutoModes {
		cwd, _ := os.Getwd()
		if !gitIsRepo(ctx, cwd) {
			a.Emit(EventStatus, "⚠ Not a git repository — edit mode requires git for safe rollback")
		}
	}

	userMsg := ai.NewTextMessage(ai.RoleUser, prompt)
	if isFirstMessage {
		userMsg.Metadata = map[string]any{
			triageInjectedMetadataKey:     true,
			originalUserPromptMetadataKey: originalUserPrompt,
		}
	}
	a.sess.AddMessage(userMsg)
	a.recordToTranscript(userMsg)

	// 3. If we have prompt parts from the new system, send them first
	if len(promptParts) > 0 && isFirstMessage && a.cfg != nil && a.cfg.PromptSystemEnabled {
		a.Emit(EventStatus, "delivering staged prompts...")
		for i, part := range promptParts {
			a.Emit(EventStatus, fmt.Sprintf("prompt stage %d/%d: %s", i+1, len(promptParts), part.Stage))
			// Build request with the prompt part as system prompt
			toolSchemas := buildToolSchemas(a.tools)
			thinkingBudget := a.getThinkingBudget()

			req := ai.CompletionRequest{
				Messages:    a.sess.Messages,
				Tools:       toolSchemas,
				System:      part.Content,
				Temperature: 0.0,
				MaxTokens:   8192,
				Stream:      true,
				Thinking: &ai.ThinkingConfig{
					Type:         "enabled",
					BudgetTokens: thinkingBudget,
					Stream:       true,
				},
			}

			resp, err := a.Provider().Complete(ctx, req)
			if err != nil {
				a.Emit(EventError, err)
				break
			}

			text, thought, usage, err := a.drainStream(resp)
			if err != nil {
				a.Emit(EventError, err)
				break
			}

			// Add assistant response to session
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
			if len(msg.Content) > 0 {
				a.sess.AddMessage(msg)
				a.recordToTranscript(msg)
			}
			a.sess.AddUsage(usage)
		}
	}

	for {
		provider := a.Provider()

		// Check context window usage and emit warnings.
		// If usage is high (e.g., > 80% or autoCompressAt), perform intelligent compaction.
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
			// Even if not compacting, ghost large tool outputs to keep context lean
			a.sess.SetMessages(a.GhostLargeOutputs(a.sess.Messages))
		}

		a.checkContextLimit(provider, a.sess.Messages)

		// Get system prompt from new prompt system (staged prompts, todo-aware, context-aware)
		// Falls back to legacy buildSystemPrompt if prompt system not available or exhausted
		systemPrompt := a.getSystemPrompt(ctx, provider)
		toolSchemas := buildToolSchemas(a.tools)

		// Dynamically adjust thinking budget based on complexity
		thinkingBudget := a.getThinkingBudget()

		req := ai.CompletionRequest{
			Messages:    a.sess.Messages,
			Tools:       toolSchemas,
			System:      systemPrompt,
			Temperature: 0.0,
			MaxTokens:   8192,
			Stream:      true,
			Thinking: &ai.ThinkingConfig{
				Type:         "enabled",
				BudgetTokens: thinkingBudget,
				Stream:       true,
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

		// Record ground truth for adaptive token estimation
		if mgr := a.ContextManager(); mgr != nil && usage.InputTokens > 0 {
			// Build the prompt text that was sent to the model
			systemPrompt := buildSystemPrompt(a.cfg, a.tools, a.sess.Messages, mgr)
			toolSchemas := buildToolSchemas(a.tools)
			// Approximate the prompt text
			promptText := systemPrompt
			for _, m := range a.sess.Messages {
				promptText += m.PlaintextForHistory()
			}
			for _, tc := range toolSchemas {
				if b, err := json.Marshal(tc); err == nil {
					promptText += string(b)
				}
			}
			mgr.RecordGroundTruth(promptText, usage.InputTokens)
		}

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

		// Pruning Logic: If we just finished the first turn which had the triage instruction,
		// restore the original user message from metadata.
		if isFirstMessage {
			a.pruneFirstMessageTriage()
			isFirstMessage = false // Only prune once
		}

		// 3. Handle Stop Condition
		if stop != ai.StopReasonTools || len(toolCalls) == 0 {
			a.Emit(EventDone, text)
			a.tryPersist()
			return nil
		}

		for _, executed := range a.executeToolCallsParallel(ctx, toolCalls) {
			a.Emit(EventToolDone, ToolDoneEvent{
				ID:         executed.call.ID,
				Name:       executed.call.Name,
				Context:    executed.context,
				StartedAt:  executed.startedAt,
				FinishedAt: executed.finishedAt,
				Duration:   executed.finishedAt.Sub(executed.startedAt),
				Result:     executed.result,
				Decision:   executed.decision,
			})
			a.Emit(EventStatus, LongTaskStatus{
				TaskID:      executed.call.ID,
				Phase:       executed.call.Name,
				ProgressPct: 100,
				Log:         executed.result.Summary,
			})

			toolMsg := ai.Message{Role: ai.RoleTool}
			toolMsg.Content = append(toolMsg.Content, ai.ContentPart{
				Type: ai.ContentTypeToolResult,
				ToolResult: &ai.ToolResult{
					ToolCallID: executed.call.ID,
					Content:    executed.result.Content,
					IsError:    executed.result.IsError,
				},
			})
			a.sess.AddMessage(toolMsg)
			a.recordToTranscript(toolMsg)
		}
		a.tryPersist()
	}
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

	a.Emit(EventToolCall, ToolCallEvent{
		ID:        tc.ID,
		Name:      tc.Name,
		Context:   context,
		Args:      tc.Args,
		Decision:  decision,
		StartedAt: startedAt,
	})

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
	approvalScope := a.scopedToolApprovalKey(tc, t)
	legacyScope := legacyToolApprovalScope(tc, t)

	a.mu.RLock()
	allowed := a.sessionAllowedTools[approvalScope] || a.sessionAllowedTools[legacyScope]
	a.mu.RUnlock()

	if !allowed && t.RequiresConfirmation(a.cfg.Mode) {
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

	return t.Execute(ctx, tc.Args)
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
	scope := toolApprovalScope(tc, t)
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

// runCoordinatorIfNeeded runs the coordinator for complex tasks that benefit from multi-agent execution.
// Returns (result string, error). If result is non-nil and error is nil, the task is complete.
func (a *Agent) runCoordinatorIfNeeded(ctx context.Context, prompt string) (string, error) {
	// Only use coordinator for complex tasks when enabled
	if !a.cfg.Coordinator.Enabled {
		return "", nil
	}

	// Use reasoning engine to analyze and plan
	re := a.ReasoningEngine()
	if re == nil {
		return "", fmt.Errorf("reasoning engine not available")
	}

	// Analyze the request with a timeout.
	analyzeCtx, analyzeCancel := context.WithTimeout(ctx, 10*time.Second)
	defer analyzeCancel()

	analysis, err := re.Analyze(analyzeCtx, prompt)
	if err != nil {
		return "", fmt.Errorf("analysis failed: %w", err)
	}

	// Only use coordinator for moderate+ complexity tasks
	complexityOrder := map[reasoning.Complexity]int{
		reasoning.ComplexityTrivial:  0,
		reasoning.ComplexitySimple:   1,
		reasoning.ComplexityModerate: 2,
		reasoning.ComplexityComplex:  3,
		reasoning.ComplexityMajor:    4,
	}
	if complexityOrder[analysis.Complexity] < complexityOrder[reasoning.ComplexityModerate] {
		return "", nil // Let standard loop handle simple tasks
	}

	a.Emit(EventStatus, fmt.Sprintf("Coordinator: analyzing task (complexity: %s)", analysis.Complexity))

	// Create execution plan with a timeout (directory walks can be slow on large repos).
	planCtx, planCancel := context.WithTimeout(ctx, 30*time.Second)
	defer planCancel()

	execPlan, err := re.Plan(planCtx, analysis)
	if err != nil {
		return "", fmt.Errorf("planning failed: %w", err)
	}

	a.Emit(EventStatus, fmt.Sprintf("Coordinator: plan created with %d tasks, %d phases", len(execPlan.Tasks), len(execPlan.ExecutionOrder)))

	// Convert to coordinator plan
	coordPlan, err := coordinator.FromReasoningPlan(ctx, execPlan)
	if err != nil {
		return "", fmt.Errorf("plan conversion failed: %w", err)
	}

	a.Emit(EventStatus, fmt.Sprintf("Coordinator: converted plan with %d tasks, %d phases", len(coordPlan.Tasks), len(coordPlan.Phases)))

	// Get coordinator engine
	coord := a.Coordinator()
	if coord == nil {
		return "", fmt.Errorf("coordinator not available")
	}

	a.Emit(EventStatus, fmt.Sprintf("Coordinator: executing plan with %d phases", len(coordPlan.Phases)))

	// Execute the plan with a timeout to prevent blocking forever.
	coordCtx, coordCancel := context.WithTimeout(ctx, 5*time.Minute)
	defer coordCancel()

	result, err := coord.Execute(coordCtx, coordPlan)
	if err != nil {
		return "", fmt.Errorf("coordinator execution failed: %w", err)
	}

	// Format result
	if result != nil && result.FinalOutput != "" {
		return fmt.Sprintf("## Coordinator Result\n\n%s", result.FinalOutput), nil
	}

	return "Coordinator completed task", nil
}

// processFirstMessageWithPromptSystem processes the first user message through the new prompt system.
// Returns the categorized request, initial prompt parts to send to the provider, and any error.
func (a *Agent) processFirstMessageWithPromptSystem(ctx context.Context, userPrompt string) (*promptpkg.CategorizedRequest, []promptpkg.PromptPart, error) {
	// Get available files from context manager
	mgr := a.ContextManager()
	var availableFiles []string
	if mgr != nil {
		availableFiles = mgr.RecentFiles(20)
	}

	// Process through the new prompt system
	parts, err := a.promptSystem.ProcessUserMessage(ctx, userPrompt, a.workDir, availableFiles)
	if err != nil {
		return nil, nil, err
	}

	// Get the categorized request
	categorized := a.promptSystem.Manager.GetCurrentRequest()
	return categorized, parts, nil
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

// getSystemPrompt returns the system prompt using the new prompt system.
// Falls back to legacy buildSystemPrompt if prompt system is not available or exhausted.
func (a *Agent) getSystemPrompt(ctx context.Context, provider ai.Provider) string {
	// If prompt system is enabled and has pending staged prompts, use them
	if a.cfg != nil && a.cfg.PromptSystemEnabled && a.promptSystem != nil {
		// Check for next staged prompt from first message processing
		if nextPrompt := a.promptSystem.GetNextAction(); nextPrompt != nil {
			a.Emit(EventStatus, fmt.Sprintf("using staged prompt: %s", nextPrompt.Stage))
			return nextPrompt.Content
		}

		// If we have a categorized request with todo items, build a todo-aware prompt
		if categorized := a.promptSystem.GetCurrentRequest(); categorized != nil && len(categorized.TodoItems) > 0 {
			// Build context-aware prompt for current todo
			if nextTodo := a.getNextTodoPrompt(); nextTodo != nil {
				return nextTodo.Content
			}
		}

		// Build standard prompt using prompt system (todo-aware, context-aware)
		return a.buildPromptSystemPrompt(ctx, provider)
	}

	// Fallback to legacy buildSystemPrompt
	return buildSystemPrompt(a.cfg, a.tools, a.sess.Messages, a.ContextManager())
}

// getNextTodoPrompt gets the next todo execution prompt from the prompt system
func (a *Agent) getNextTodoPrompt() *promptpkg.PromptPart {
	coderCtx := a.promptSystem.GetCoderContext()
	if coderCtx == nil || len(coderCtx.TodoItems) == 0 {
		return nil
	}

	for i, todo := range coderCtx.TodoItems {
		if todo.Status == promptpkg.TodoStatusPending {
			// Check dependencies
			depsMet := true
			for _, depID := range todo.Dependencies {
				found := false
				for _, t := range coderCtx.TodoItems {
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
				coderCtx.TodoItems[i].Status = promptpkg.TodoStatusInProgress
				return a.promptSystem.Manager.CoderPrompts().BuildExecutionPrompt(coderCtx, &coderCtx.TodoItems[i], a.promptSystem.GetCurrentRequest())
			}
		}
	}
	return nil
}

// buildPromptSystemPrompt builds a system prompt using the new prompt system
func (a *Agent) buildPromptSystemPrompt(ctx context.Context, provider ai.Provider) string {
	var sb strings.Builder

	// Identity
	sb.WriteString("# Identity\n")
	sb.WriteString(fmt.Sprintf("You are Automergent %s, a senior lead software engineer and autonomous agent.\n\n", version.Version))

	// Current task context
	if categorized := a.promptSystem.GetCurrentRequest(); categorized != nil {
		sb.WriteString(fmt.Sprintf("## Current Task: %s\n", categorized.OriginalPrompt))
		sb.WriteString(fmt.Sprintf("Category: %s | Complexity: %s | Strategy: %s\n\n",
			categorized.Category, categorized.Complexity, categorized.Strategy))

		// Todo progress
		if len(categorized.TodoItems) > 0 {
			sb.WriteString("## Todo Progress\n")
			for _, todo := range categorized.TodoItems {
				status := "⏳"
				switch todo.Status {
				case promptpkg.TodoStatusCompleted:
					status = "✅"
				case promptpkg.TodoStatusInProgress:
					status = "🔄"
				case promptpkg.TodoStatusBlocked:
					status = "⚠️"
				}
				sb.WriteString(fmt.Sprintf("%s %s (priority: %d)\n", status, todo.Description, todo.Priority))
			}
			sb.WriteString("\n")
		}

		// Working areas
		if len(categorized.WorkingAreas) > 0 {
			sb.WriteString("## Working Areas\n")
			for _, f := range categorized.WorkingAreas {
				sb.WriteString(fmt.Sprintf("- %s\n", f))
			}
			sb.WriteString("\n")
		}

		// Constraints
		if len(categorized.ContextNeeds) > 0 {
			sb.WriteString("## Context Requirements\n")
			for _, need := range categorized.ContextNeeds {
				if need.InjectTiming != promptpkg.InjectTimingDeferred {
					sb.WriteString(fmt.Sprintf("- %s: %s\n", need.Key, need.Description))
				}
			}
			sb.WriteString("\n")
		}
	}

	// Tools
	sb.WriteString("## Available Tools\n")
	for _, tool := range a.tools.All() {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", tool.Name(), tool.Description()))
	}
	sb.WriteString("\n")

	// Safety protocols (abbreviated)
	sb.WriteString("## Safety & Blast Radius\n")
	sb.WriteString("- **Safe:** Reading, searching, local tests\n")
	sb.WriteString("- **Moderate:** Creating/editing files, adding deps\n")
	sb.WriteString("- **Destructive:** Deleting, force push, rm -rf, drop tables\n")
	sb.WriteString("  - MUST describe risk and wait for confirmation\n\n")

	// Project context
	if mgr := a.ContextManager(); mgr != nil {
		cwd, _ := os.Getwd()
		sb.WriteString(fmt.Sprintf("## Project Context\n- Working Directory: %s\n", cwd))

		// Recent files
		if files := mgr.RecentFiles(10); len(files) > 0 {
			sb.WriteString("- Recent Files:\n")
			for _, f := range files {
				sb.WriteString(fmt.Sprintf("  - %s\n", f))
			}
		}
		sb.WriteString("\n")
	}

	// Conversation summary if long
	if len(a.sess.Messages) > 15 {
		sb.WriteString("[Note: Conversation history is long. Focus on recent state and established plan.]\n\n")
	}

	return sb.String()
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

// ReasoningEngine returns the persistent reasoning engine, initializing it on first use.
func (a *Agent) ReasoningEngine() *reasoning.Engine {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.reasoningEngine != nil {
		return a.reasoningEngine
	}

	cfg := reasoning.DefaultEngineConfig()
	cfg.DefaultTimeout = 5 * time.Minute
	a.reasoningEngine = reasoning.NewEngine(cfg)
	return a.reasoningEngine
}
