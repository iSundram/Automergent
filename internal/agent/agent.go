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
	"github.com/iSundram/Automergent/internal/session"
	"github.com/iSundram/Automergent/internal/tools"
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
	firstMessageHandled bool
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
}

type toolCallBatch struct {
	calls    []ai.ToolCall
	parallel bool
}

type executedToolCall struct {
	call       ai.ToolCall
	context    string
	startedAt  time.Time
	finishedAt time.Time
	result     tools.Result
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

	// 1. Initial Triage Phase (Dynamic Workflow)
	// If this is the very first message, we run a hidden triage loop
	isFirstMessage := a.checkAndMarkFirstMessage()
	if isFirstMessage {
		a.Emit(EventStatus, "initiating project triage")
		// Use externalized instruction from triage.go
		prompt = TriageInstruction + "\n\nUser Request: " + prompt
	}

	// In edit mode, check that we are inside a git repository when required.
	if a.cfg.Mode == "edit" && a.cfg.Security.RequireGitForAutoModes {
		cwd, _ := os.Getwd()
		if !gitIsRepo(ctx, cwd) {
			a.Emit(EventStatus, "⚠ Not a git repository — edit mode requires git for safe rollback")
		}
	}

	a.sess.AddMessage(ai.NewTextMessage(ai.RoleUser, prompt))

	for {
		provider := a.Provider()

		// Check context window usage and emit warnings.
		// If usage is high (e.g., > 80%), perform lightweight compaction.
		tokens, _ := provider.TokenCount(a.sess.Messages)
		limit := a.cfg.MaxContextTokens
		if limit <= 0 {
			limit = provider.ContextLimit()
		}
		if limit > 0 && float64(tokens)/float64(limit) > 0.80 {
			a.Emit(EventStatus, "Compacting message history to free up context window...")
			a.sess.Messages = a.CompactSessionMessages(ctx, a.sess.Messages)
		}

		a.checkContextLimit(provider, a.sess.Messages)

		systemPrompt := buildSystemPrompt(a.cfg, a.tools, a.sess.Messages)
		toolSchemas := buildToolSchemas(a.tools)

		req := ai.CompletionRequest{
			Messages:    a.sess.Messages,
			Tools:       toolSchemas,
			System:      systemPrompt,
			Temperature: 0.0,
			MaxTokens:   8192,
			Stream:      true,
			Thinking: &ai.ThinkingConfig{
				Type:         "enabled",
				BudgetTokens: 10000,
			},
		}

		a.Emit(EventStatus, "thinking")
		resp, err := provider.Complete(ctx, req)
		if err != nil {
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

		// Pruning Logic: If we just finished the first turn which had the Triage instruction,
		// remove that instruction from the first message but keep the user's intent.
		if isFirstMessage && len(a.sess.Messages) > 0 {
			if first := &a.sess.Messages[0]; first.Role == ai.RoleUser {
				if parts := strings.Split(first.TextContent(), "User Request: "); len(parts) > 1 {
					first.Content[0].Text = parts[1] // Keep only the user request
				}
			}
			isFirstMessage = false // Only prune once
		}

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

func (a *Agent) partitionToolCalls(toolCalls []ai.ToolCall) []toolCallBatch {
	batches := make([]toolCallBatch, 0, len(toolCalls))
	var current toolCallBatch

	flush := func() {
		if len(current.calls) == 0 {
			return
		}
		batches = append(batches, current)
		current = toolCallBatch{}
	}

	for _, tc := range toolCalls {
		tool, ok := a.tools.Get(tc.Name)
		canParallelize := ok && tool.IsReadOnly(tc.Args) && tool.IsConcurrencySafe(tc.Args) && !tool.RequiresConfirmation(a.cfg.Mode)

		if !canParallelize {
			flush()
			batches = append(batches, toolCallBatch{
				calls:    []ai.ToolCall{tc},
				parallel: false,
			})
			continue
		}

		if !current.parallel {
			flush()
			current.parallel = true
		}
		if len(current.calls) == 10 {
			flush()
			current.parallel = true
		}
		current.calls = append(current.calls, tc)
	}

	flush()
	return batches
}

func (a *Agent) executeToolCallsParallel(ctx context.Context, toolCalls []ai.ToolCall) []executedToolCall {
	batches := a.partitionToolCalls(toolCalls)
	results := make([]executedToolCall, 0, len(toolCalls))

	for _, batch := range batches {
		if !batch.parallel {
			for _, tc := range batch.calls {
				results = append(results, a.executeSingleToolCall(ctx, tc))
			}
			continue
		}

		batchResults := make([]executedToolCall, len(batch.calls))
		var wg sync.WaitGroup
		wg.Add(len(batch.calls))
		for i, tc := range batch.calls {
			i, tc := i, tc
			go func() {
				defer wg.Done()
				batchResults[i] = a.executeSingleToolCall(ctx, tc)
			}()
		}
		wg.Wait()
		results = append(results, batchResults...)
	}

	return results
}

func (a *Agent) executeSingleToolCall(ctx context.Context, tc ai.ToolCall) executedToolCall {
	startedAt := time.Now()
	context := toolCallContext(tc)

	a.Emit(EventToolCall, ToolCallEvent{
		ID:        tc.ID,
		Name:      tc.Name,
		Context:   context,
		Args:      tc.Args,
		StartedAt: startedAt,
	})

	status := fmt.Sprintf("running %s", tc.Name)
	if context != "" {
		status = fmt.Sprintf("running %s (%s)", tc.Name, context)
	}
	a.Emit(EventStatus, status)

	result, err := a.executeTool(ctx, tc)
	if err != nil {
		result = tools.Result{IsError: true, Content: err.Error()}
	}

	return executedToolCall{
		call:       tc,
		context:    context,
		startedAt:  startedAt,
		finishedAt: time.Now(),
		result:     result,
	}
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

	a.mu.RLock()
	allowed := a.sessionAllowedTools[tc.Name]
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
			a.sessionAllowedTools[tc.Name] = true
			a.mu.Unlock()
		}
	}

	return t.Execute(ctx, tc.Args)
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
