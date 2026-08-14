package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/config"
	automergentErrors "github.com/iSundram/Automergent/internal/errors"
	"github.com/iSundram/Automergent/internal/reasoning"
	"github.com/iSundram/Automergent/internal/session"
	"github.com/iSundram/Automergent/internal/tools"
	subagent "github.com/iSundram/Automergent/internal/tools/agent"
	"github.com/iSundram/Automergent/internal/verification"
)

// Agent is the core AI coding agent.
type Agent struct {
	cfg                      *config.Config
	provider                 ai.Provider
	sess                     *session.Session
	tools                    *tools.Registry
	events                   chan Event
	closeOnce                sync.Once
	eventsClosed             bool
	sessionPersist           func()
	mu                       sync.RWMutex
	sessionAllowedTools      map[string]bool
	firstMessageHandled      bool
	decisionRecords          []ToolDecisionRecord
	reasoningPreAnalyze      func(context.Context, string) (string, error)
	currentComplexity        reasoning.Complexity
	currentTaskType          reasoning.TaskType
	consecutiveVerifications int
}

// Execute implements the AgentExecutor interface for sub-agents.
func (a *Agent) Execute(ctx context.Context, agentType subagent.AgentType, prompt string, model string) (string, error) {
	// 1. Create a child configuration and provider if a specific model is requested.
	childCfg := *a.cfg
	if model != "" {
		childCfg.Model = model
	}

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

// SetProvider swaps the runtime provider used for subsequent completions.
func (a *Agent) SetProvider(p ai.Provider) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.provider = p
}

// Session returns the current session.
func (a *Agent) Session() *session.Session { return a.sess }

// SetSession replaces the current session (e.g., when loading a saved session).
func (a *Agent) SetSession(sess *session.Session) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sess = sess
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
			a.sess.Messages = a.CompactSessionMessages(ctx, a.sess.Messages)
		} else {
			// Even if not compacting, ghost large tool outputs to keep context lean
			a.sess.Messages = a.GhostLargeOutputs(a.sess.Messages)
		}

		a.checkContextLimit(provider, a.sess.Messages)

		systemPrompt := buildSystemPrompt(a.cfg, a.tools, a.sess.Messages)
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
			if automergentErrors.Is(err, automergentErrors.CodeQuotaExceeded) {
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
		}

		// Pruning Logic: If we just finished the first turn which had the triage instruction,
		// restore the original user message from metadata.
		if isFirstMessage {
			a.pruneFirstMessageTriage()
			isFirstMessage = false // Only prune once
		}

		// 3. Handle Stop Condition (Verification Gate)
		if stop != ai.StopReasonTools || len(toolCalls) == 0 {
			// Even if the model says it's done, we run a verification gate
			if a.consecutiveVerifications < 2 && a.shouldVerify(a.sess.Messages) {
				a.consecutiveVerifications++
				a.Emit(EventStatus, "Verification Gate: Confirming task integrity...")
				vResult, err := a.verifyTaskCompletion(ctx)
				if err != nil || (vResult != nil && vResult.TotalIssues > 0) {
					// Verification failed! Force a recovery turn.
					a.Emit(EventStatus, "Verification Gate: Failures detected. Triggering recovery turn.")
					recoveryMsg := a.triggerRecoveryTurn(vResult, err)
					a.sess.AddMessage(recoveryMsg)
					continue // Back into the loop
				}
				a.Emit(EventStatus, "Verification Gate: Integrity confirmed.")
			}

			a.Emit(EventDone, text)
			a.tryPersist()
			return nil
		}

		// If we are executing tools, reset the verification counter
		a.consecutiveVerifications = 0

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
	approvalScope := toolApprovalScope(tc, t)
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
		}
	}

	return t.Execute(ctx, tc.Args)
}

func toolApprovalScope(tc ai.ToolCall, t tools.Tool) string {
	action, risk := toolApprovalDimensions(tc, t)
	return fmt.Sprintf("name=%q;action=%s;risk=%s", tc.Name, action, risk)
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

// shouldVerify determines if the current session state warrants a verification gate.
func (a *Agent) shouldVerify(messages []ai.Message) bool {
	// Only verify in edit or plan mode
	if a.cfg.Mode != "edit" && a.cfg.Mode != "plan" {
		return false
	}

	// Only verify if there was a tool call that likely modified state
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == ai.RoleAssistant {
			for _, tc := range messages[i].ToolCallParts() {
				name := strings.ToLower(tc.Name)
				if strings.Contains(name, "edit") || strings.Contains(name, "write") ||
					strings.Contains(name, "create") || strings.Contains(name, "delete") ||
					strings.Contains(name, "apply") || strings.Contains(name, "test") {
					return true
				}
			}
		}
	}
	return false
}

// verifyTaskCompletion runs the full verification suite against the current workspace.
func (a *Agent) verifyTaskCompletion(ctx context.Context) (*verification.Result, error) {
	engine := verification.NewDefaultEngine()
	cwd, _ := os.Getwd()

	// Identify changed files from session history for targeted verification
	changedFiles := make([]string, 0)
	for _, m := range a.sess.Messages {
		if m.Role == ai.RoleAssistant {
			for _, tc := range m.ToolCallParts() {
				if path, ok := tc.Args["path"].(string); ok {
					changedFiles = append(changedFiles, path)
				} else if path, ok := tc.Args["file_path"].(string); ok {
					changedFiles = append(changedFiles, path)
				}
			}
		}
	}

	vCtx := &verification.Context{
		WorkingDir:   cwd,
		ChangedFiles: changedFiles,
	}

	return engine.Verify(vCtx)
}

// triggerRecoveryTurn creates a system message explaining the verification failures.
func (a *Agent) triggerRecoveryTurn(result *verification.Result, err error) ai.Message {
	var sb strings.Builder
	sb.WriteString("# Verification Failed\n\n")
	sb.WriteString("I've analyzed your latest response and the state of the workspace. The task is NOT complete because the following issues were detected:\n\n")

	if err != nil {
		sb.WriteString(fmt.Sprintf("## System Error\n%v\n\n", err))
	}

	if result != nil {
		for _, layer := range result.Layers {
			if len(layer.Issues) > 0 {
				sb.WriteString(fmt.Sprintf("## %s Issues\n", layer.Layer))
				for _, issue := range layer.Issues {
					sb.WriteString(fmt.Sprintf("- [%s] %s", issue.Severity, issue.Message))
					if issue.File != "" {
						sb.WriteString(fmt.Sprintf(" (in %s", issue.File))
						if issue.Line > 0 {
							sb.WriteString(fmt.Sprintf(":%d", issue.Line))
						}
						sb.WriteString(")")
					}
					if issue.FixSuggestion != "" {
						sb.WriteString(fmt.Sprintf("\n  *Suggestion:* %s", issue.FixSuggestion))
					}
					sb.WriteString("\n")
				}
				sb.WriteString("\n")
			}
		}
	}

	sb.WriteString("## Requirement\n")
	sb.WriteString("Please analyze these failures, identify the root cause, and continue until all verification layers pass. Do not report completion until the workspace is in a healthy state.")

	return ai.Message{
		Role: ai.RoleSystem,
		Content: []ai.ContentPart{{
			Type: ai.ContentTypeText,
			Text: sb.String(),
		}},
	}
}
