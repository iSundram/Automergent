package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/agent/agentdef"
	"github.com/iSundram/Automergent/internal/session"
)

// SubagentProgress tracks real-time progress of a subagent.
type SubagentProgress struct {
	AgentID       string
	AgentType     string
	ToolUseCount  int
	TokenCount    int
	LastActivity  time.Time
	Summary       string
	CurrentTool   string
	RecentActions []string // last N tool calls
}

// SubagentStreamEvent is emitted during subagent execution for parent notification.
type SubagentStreamEvent struct {
	Type      string // "progress", "tool_start", "tool_done", "token", "done", "error"
	AgentID   string
	Payload   any
	Timestamp time.Time
}

// SubagentOptions configures how a subagent is spawned.
type SubagentOptions struct {
	AgentType     agentdef.AgentType
	Model         string
	Prompt        string
	Name          string
	Description   string
	Mode          string // "sync" or "background"
	StreamToParent bool
	MaxTokens     int
	Timeout       time.Duration
	ParentAgentID string
	WorkDir       string
}

// SubagentResult holds the outcome of a subagent execution.
type SubagentResult struct {
	AgentID   string
	Output    string
	Error     error
	Duration  time.Duration
	ToolCalls int
	Events    []SubagentStreamEvent
}

// ExecuteSubagent is the enhanced subagent execution path with streaming,
// progress tracking, context isolation, and notification support.
func (a *Agent) ExecuteSubagent(ctx context.Context, opts SubagentOptions) *SubagentResult {
	startTime := time.Now()

	// 1. Resolve agent definition from registry
	def, _ := a.resolveAgentDefinition(opts.AgentType)

	// 2. Build isolated context
	childCtx, childCancel, childSess, childAgent := a.createIsolatedChild(ctx, opts, def)
	defer childCancel()

	// 3. Set up progress tracking
	progress := &SubagentProgress{
		AgentID:      opts.Name,
		AgentType:    string(opts.AgentType),
		LastActivity: startTime,
	}
	var progressMu sync.Mutex
	var streamEvents []SubagentStreamEvent
	var streamMu sync.Mutex

	// 4. Set up streaming to parent if enabled
	var progressCh chan SubagentStreamEvent
	if opts.StreamToParent {
		progressCh = make(chan SubagentStreamEvent, 64)
		go func() {
			for evt := range progressCh {
				streamMu.Lock()
				streamEvents = append(streamEvents, evt)
				streamMu.Unlock()
				a.emitSubagentProgress(evt, progress, &progressMu)
			}
		}()
	}

	// 5. Set up cancellation chain
	childCtx, childCancel = context.WithCancel(ctx)
	cancelKey := fmt.Sprintf("subagent-%s", opts.Name)
	a.registerChildCancel(cancelKey, childCancel)
	defer a.unregisterChildCancel(cancelKey)

	// 6. Run child agent with progress monitoring
	var finalResponse string
	var finalErr error
	done := make(chan struct{})

	// Event drainer + progress extractor
	go func() {
		for evt := range childAgent.Events() {
			a.extractProgress(evt, progress, &progressMu)
			if progressCh != nil {
				progressCh <- SubagentStreamEvent{
					Type:      "progress",
					AgentID:   progress.AgentID,
					Payload:   progress,
					Timestamp: time.Now(),
				}
			}
		}
	}()

	// Main execution
	go func() {
		var runErr error
		if def != nil && def.SystemPrompt != "" {
			// Prepend agent system prompt to the task
			fullPrompt := buildSubagentPrompt(def, opts.Prompt)
			runErr = childAgent.Run(childCtx, fullPrompt)
		} else {
			runErr = childAgent.Run(childCtx, opts.Prompt)
		}

		// Extract result
		if len(childSess.Messages) > 0 {
			for i := len(childSess.Messages) - 1; i >= 0; i-- {
				if childSess.Messages[i].Role == ai.RoleAssistant {
					finalResponse = childSess.Messages[i].TextContent()
					break
				}
			}
		}
		finalErr = runErr
		_ = childAgent.Close()
		close(done)
	}()

	// 7. Wait with timeout and cancellation
	var timeoutCh <-chan time.Time
	if opts.Timeout > 0 {
		timer := time.NewTimer(opts.Timeout)
		defer timer.Stop()
		timeoutCh = timer.C
	}

	select {
	case <-done:
		// Completed
	case <-ctx.Done():
		childCancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			finalErr = ctx.Err()
		}
	case <-timeoutCh:
		childCancel()
		finalErr = fmt.Errorf("subagent timed out after %s", opts.Timeout)
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}

	// 8. Close progress channel
	if progressCh != nil {
		close(progressCh)
	}

	// 9. Emit completion event
	duration := time.Since(startTime)
	result := &SubagentResult{
		AgentID:  progress.AgentID,
		Output:   finalResponse,
		Error:    finalErr,
		Duration: duration,
	}

	streamMu.Lock()
	result.Events = streamEvents
	streamMu.Unlock()

	// 10. Emit notification for parent
	a.emitSubagentNotification(result)

	return result
}

// resolveAgentDefinition looks up the agent definition from the registry.
func (a *Agent) resolveAgentDefinition(agentType agentdef.AgentType) (*agentdef.AgentDefinition, bool) {
	reg := GlobalRegistry()
	return reg.Get(agentType)
}

// createIsolatedChild creates a child agent with isolated context.
func (a *Agent) createIsolatedChild(
	ctx context.Context,
	opts SubagentOptions,
	def *agentdef.AgentDefinition,
) (context.Context, context.CancelFunc, *session.Session, *Agent) {
	childCfg := *a.cfg
	if opts.Model != "" {
		childCfg.Model = opts.Model
	} else if def != nil && def.Model != "" {
		childCfg.Model = def.Model
	}

	childSess := session.New()
	childSess.Metadata["parent_id"] = a.sess.ID
	childSess.Metadata["agent_type"] = string(opts.AgentType)
	childSess.Metadata["subagent_name"] = opts.Name
	if opts.ParentAgentID != "" {
		childSess.Metadata["parent_agent_id"] = opts.ParentAgentID
	}

	childAgent := New(&childCfg, a.provider, childSess, a.tools)

	childCtx, childCancel := context.WithCancel(ctx)
	return childCtx, childCancel, childSess, childAgent
}

// buildSubagentPrompt constructs the full prompt for a subagent.
func buildSubagentPrompt(def *agentdef.AgentDefinition, taskPrompt string) string {
	var sb strings.Builder

	sb.WriteString("## Agent Role\n")
	sb.WriteString(def.SystemPrompt)
	sb.WriteString("\n\n## Task\n")
	sb.WriteString(taskPrompt)

	if len(def.Tools) > 0 {
		sb.WriteString("\n\n## Available Tools\n")
		sb.WriteString("You have access to: ")
		sb.WriteString(strings.Join(def.Tools, ", "))
		sb.WriteString("\n")
	}

	return sb.String()
}

// extractProgress extracts progress information from agent events.
func (a *Agent) extractProgress(evt Event, progress *SubagentProgress, mu *sync.Mutex) {
	mu.Lock()
	defer mu.Unlock()

	switch evt.Type {
	case EventToolStart:
		if tc, ok := evt.Payload.(ToolCallEvent); ok {
			progress.CurrentTool = tc.Name
			progress.ToolUseCount++
			progress.LastActivity = time.Now()
			action := fmt.Sprintf("%s(%s)", tc.Name, truncateArgs(tc.Args, 50))
			progress.RecentActions = append(progress.RecentActions, action)
			if len(progress.RecentActions) > 5 {
				progress.RecentActions = progress.RecentActions[len(progress.RecentActions)-5:]
			}
		}
	case EventToolDone:
		progress.CurrentTool = ""
	case EventToken:
		progress.TokenCount++
		progress.LastActivity = time.Now()
	}
}

// emitSubagentProgress sends a progress event to the parent agent's event channel.
func (a *Agent) emitSubagentProgress(evt SubagentStreamEvent, progress *SubagentProgress, mu *sync.Mutex) {
	mu.Lock()
	snap := *progress
	mu.Unlock()

	a.Emit("subagent_progress", snap)
}

// emitSubagentNotification sends a completion notification for the subagent.
func (a *Agent) emitSubagentNotification(result *SubagentResult) {
	notif := TaskNotification{
		TaskID:     result.AgentID,
		Status:     "completed",
		Summary:    fmt.Sprintf("Agent %s completed", result.AgentID),
		Result:     result.Output,
		DurationMs: result.Duration.Milliseconds(),
	}
	if result.Error != nil {
		notif.Status = "failed"
		notif.Summary = fmt.Sprintf("Agent %s failed: %v", result.AgentID, result.Error)
	}

	a.Emit(EventNotify, notif)
}

// truncateArgs truncates tool call arguments for display.
func truncateArgs(args map[string]any, maxLen int) string {
	if args == nil {
		return ""
	}
	s := fmt.Sprintf("%v", args)
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
