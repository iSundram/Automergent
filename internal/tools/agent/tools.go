package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/iSundram/Automergent/internal/tools"
)

// AgentType defines the type of sub-agent.
type AgentType string

const (
	AgentTypeExplore        AgentType = "explore"
	AgentTypeTask           AgentType = "task"
	AgentTypeGeneralPurpose AgentType = "general-purpose"
	AgentTypeCodeReview     AgentType = "code-review"
)

// AgentStatus represents the current state of an agent.
type AgentStatus string

const (
	AgentStatusRunning   AgentStatus = "running"
	AgentStatusIdle      AgentStatus = "idle"
	AgentStatusCompleted AgentStatus = "completed"
	AgentStatusFailed    AgentStatus = "failed"
	AgentStatusCancelled AgentStatus = "cancelled"
)

// AgentInstance represents a running or completed sub-agent.
// Add synchronization primitives for notifying waiters when the agent finishes.
type AgentInstance struct {
	ID          string
	Name        string
	Type        AgentType
	Prompt      string
	Status      AgentStatus
	Result      string
	Error       error
	StartedAt   time.Time
	CompletedAt time.Time
	Turns       []AgentTurn

	mu      sync.Mutex
	done    chan struct{}
	dismiss sync.Once // used to close done channel exactly once
}

// AgentTurn represents a single turn in a multi-turn agent conversation.
type AgentTurn struct {
	Index    int
	Input    string
	Output   string
	Duration time.Duration
}

// AgentManager manages sub-agent instances.
type AgentManager struct {
	mu       sync.RWMutex
	agents   map[string]*AgentInstance
	counter  int
	executor AgentExecutor
	hooks    []func(AgentNotification)
}

// AgentNotification captures a terminal status update for an agent.
type AgentNotification struct {
	AgentID    string
	Name       string
	Type       AgentType
	Status     AgentStatus
	Duration   time.Duration
	Result     string
	ErrMessage string
}

// AgentExecutor is the interface for actually running agents.
// This would be implemented by the main agent loop.
type AgentExecutor interface {
	Execute(ctx context.Context, agentType AgentType, prompt string, model string) (string, error)
}

var globalAgentManager = &AgentManager{
	agents: make(map[string]*AgentInstance),
}

// GetAgentManager returns the global agent manager.
func GetAgentManager() *AgentManager {
	return globalAgentManager
}

// SetExecutor sets the agent executor (called during initialization).
func (m *AgentManager) SetExecutor(e AgentExecutor) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.executor = e
}

func (m *AgentManager) Create(agent *AgentInstance) {
	// initialize agent notification channel before publishing
	if agent.done == nil {
		agent.done = make(chan struct{})
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.agents[agent.ID] = agent
}

func (m *AgentManager) Get(id string) (*AgentInstance, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a, ok := m.agents[id]
	return a, ok
}

func (m *AgentManager) List(includeCompleted bool) []*AgentInstance {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*AgentInstance, 0)
	for _, a := range m.agents {
		if !includeCompleted && (a.Status == AgentStatusCompleted || a.Status == AgentStatusFailed || a.Status == AgentStatusCancelled) {
			continue
		}
		result = append(result, a)
	}
	return result
}

// RegisterCompletionHook registers a callback for terminal status updates.
func (m *AgentManager) RegisterCompletionHook(hook func(AgentNotification)) {
	if hook == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hooks = append(m.hooks, hook)
}

func isTerminalAgentStatus(status AgentStatus) bool {
	return status == AgentStatusCompleted || status == AgentStatusFailed || status == AgentStatusCancelled
}

// UpdateStatus updates agent status and emits completion hooks for terminal states.
func (m *AgentManager) UpdateStatus(id string, status AgentStatus, result string, err error) bool {
	m.mu.RLock()
	agent, ok := m.agents[id]
	hooks := append([]func(AgentNotification){}, m.hooks...)
	m.mu.RUnlock()
	if !ok || agent == nil {
		return false
	}

	agent.mu.Lock()
	previousStatus := agent.Status
	agent.Status = status
	agent.Result = result
	agent.Error = err
	if isTerminalAgentStatus(status) {
		agent.CompletedAt = time.Now()
	}
	duration := time.Since(agent.StartedAt)
	if !agent.CompletedAt.IsZero() {
		duration = agent.CompletedAt.Sub(agent.StartedAt)
	}
	notification := AgentNotification{
		AgentID:  agent.ID,
		Name:     agent.Name,
		Type:     agent.Type,
		Status:   agent.Status,
		Duration: duration,
		Result:   agent.Result,
	}
	if agent.Error != nil {
		notification.ErrMessage = agent.Error.Error()
	}
	agent.mu.Unlock()

	if !isTerminalAgentStatus(status) {
		return true
	}
	if previousStatus == status && isTerminalAgentStatus(previousStatus) {
		return true
	}
	for _, hook := range hooks {
		hook(notification)
	}
	return true
}

// Cleanup removes completed/failed agents older than maxAge to prevent memory leaks.
func (m *AgentManager) Cleanup(maxAge time.Duration) int {
	// Gather candidates without holding the write lock to avoid deadlocks with agent locks.
	cutoff := time.Now().Add(-maxAge)
	var toRemove []string

	m.mu.RLock()
	for id, a := range m.agents {
		// copy pointer; evaluate agent state under its lock
		if a == nil {
			continue
		}
		a.mu.Lock()
		isCompleted := a.Status == AgentStatusCompleted || a.Status == AgentStatusFailed || a.Status == AgentStatusCancelled
		completedBefore := a.CompletedAt.Before(cutoff)
		a.mu.Unlock()
		if isCompleted && completedBefore {
			toRemove = append(toRemove, id)
		}
	}
	m.mu.RUnlock()

	if len(toRemove) == 0 {
		return 0
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	removed := 0
	for _, id := range toRemove {
		if _, ok := m.agents[id]; ok {
			delete(m.agents, id)
			removed++
		}
	}
	return removed
}

func (m *AgentManager) NextID(name string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counter++
	if name == "" {
		return fmt.Sprintf("agent-%d", m.counter)
	}
	return fmt.Sprintf("%s-%d", name, m.counter)
}

// validate agent type
func isValidAgentType(t AgentType) bool {
	switch t {
	case AgentTypeExplore, AgentTypeTask, AgentTypeGeneralPurpose, AgentTypeCodeReview:
		return true
	default:
		return false
	}
}

// TaskTool spawns sub-agents for specialized tasks.
type TaskTool struct{}

func (t *TaskTool) Name() string { return "task" }
func (t *TaskTool) Description() string {
	return `Launch specialized sub-agents for complex tasks.

Agent types:
- explore: Fast agent for codebase exploration, finding files, answering questions (Gemini model)
- task: Execute commands with verbose output, returns summary on success (Gemini model)
- general-purpose: Full capabilities in separate context, for complex multi-step tasks (Gemini model)
- code-review: Review code changes, surfaces only important issues (All tools, Gemini model)

Use mode="background" for long tasks, you'll be notified on completion.
Use mode="sync" for quick tasks where you need immediate results.`
}
func (t *TaskTool) RequiresConfirmation(mode string) bool { return false }

func (t *TaskTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"agent_type": map[string]any{
				"type":        "string",
				"enum":        []string{"explore", "task", "general-purpose", "code-review"},
				"description": "Type of agent to spawn.",
			},
			"prompt": map[string]any{
				"type":        "string",
				"description": "Task for the agent. Be specific and provide complete context.",
			},
			"name": map[string]any{
				"type":        "string",
				"description": "Short name for the agent (used in agent_id).",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Short (3-5 word) description for UI.",
			},
			"mode": map[string]any{
				"type":        "string",
				"enum":        []string{"sync", "background"},
				"description": "sync: wait for result, background: run async and notify on completion.",
			},
			"model": map[string]any{
				"type":        "string",
				"description": "Optional model override.",
			},
		},
		"required": []string{"agent_type", "prompt"},
	}
}

func (t *TaskTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	agentTypeStr, ok := tools.StringArg(args, "agent_type")
	if !ok {
		return tools.Result{IsError: true, Content: "agent_type is required"}, nil
	}

	prompt, ok := tools.StringArg(args, "prompt")
	if !ok || prompt == "" {
		return tools.Result{IsError: true, Content: "prompt is required"}, nil
	}

	name, _ := tools.StringArg(args, "name")
	description, _ := tools.StringArg(args, "description")
	model, _ := tools.StringArg(args, "model")

	mode := "sync"
	if m, ok := tools.StringArg(args, "mode"); ok {
		mode = m
	}

	agentType := AgentType(agentTypeStr)
	if !isValidAgentType(agentType) {
		return tools.Result{IsError: true, Content: fmt.Sprintf("invalid agent_type: %s", agentTypeStr)}, nil
	}

	agentID := GetAgentManager().NextID(name)

	agent := &AgentInstance{
		ID:        agentID,
		Name:      name,
		Type:      agentType,
		Prompt:    prompt,
		Status:    AgentStatusRunning,
		StartedAt: time.Now(),
		done:      make(chan struct{}),
	}

	GetAgentManager().Create(agent)

	finish := func(result string, err error, status AgentStatus) {
		if err != nil {
			_ = GetAgentManager().UpdateStatus(agent.ID, AgentStatusFailed, err.Error(), err)
			return
		}
		_ = GetAgentManager().UpdateStatus(agent.ID, status, result, nil)
	}

	if mode == "background" {
		// Run in background. Use provided ctx so cancellation is propagated.
		go func() {
			result, err := executeAgent(ctx, agent, model)
			if err != nil {
				finish("", err, AgentStatusFailed)
			} else {
				finish(result, nil, AgentStatusCompleted)
			}
			// notify waiters exactly once
			agent.dismiss.Do(func() { close(agent.done) })
		}()

		return tools.Result{
			Content: fmt.Sprintf("started background agent: %s\nType: %s\nDescription: %s\nUse read_agent to get results", agentID, agentType, description),
			Metadata: map[string]any{
				"agent_id":   agentID,
				"agent_type": string(agentType),
				"mode":       "background",
			},
		}, nil
	}

	// Sync mode - wait for result
	result, err := executeAgent(ctx, agent, model)
	if err != nil {
		finish("", err, AgentStatusFailed)
		// notify any waiters
		agent.dismiss.Do(func() { close(agent.done) })
		return tools.Result{
			IsError: true,
			Content: fmt.Sprintf("agent %s failed: %v", agentID, err),
		}, nil
	}
	finish(result, nil, AgentStatusCompleted)
	// notify any waiters
	agent.dismiss.Do(func() { close(agent.done) })

	return tools.Result{
		Content: result,
		Metadata: map[string]any{
			"agent_id":   agentID,
			"agent_type": string(agentType),
			"duration":   agent.CompletedAt.Sub(agent.StartedAt).String(),
		},
	}, nil
}

func executeAgent(ctx context.Context, agent *AgentInstance, model string) (string, error) {
	manager := GetAgentManager()
	// read executor under RLock to avoid races
	manager.mu.RLock()
	exec := manager.executor
	manager.mu.RUnlock()
	if exec == nil {
		// Fallback: return a placeholder (in real implementation, this would call the AI)
		return fmt.Sprintf("[Agent %s would execute: %s]", agent.Type, agent.Prompt), nil
	}
	return exec.Execute(ctx, agent.Type, agent.Prompt, model)
}

// ReadAgentTool retrieves results from a background agent.
type ReadAgentTool struct{}

func (t *ReadAgentTool) Name() string { return "read_agent" }
func (t *ReadAgentTool) Description() string {
	return `Get results from a background agent.
- Use agent_id from task tool
- Use wait=true to block until completion
- Use since_turn to get only new turns in multi-turn agents`
}
func (t *ReadAgentTool) RequiresConfirmation(mode string) bool { return false }

func (t *ReadAgentTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"agent_id": map[string]any{
				"type":        "string",
				"description": "Agent ID from task tool.",
			},
			"wait": map[string]any{
				"type":        "boolean",
				"description": "Wait for agent to complete (default: false).",
			},
			"timeout": map[string]any{
				"type":        "integer",
				"description": "Max seconds to wait if wait=true (default: 10, max: 60).",
			},
			"since_turn": map[string]any{
				"type":        "integer",
				"description": "Return only turns after this index.",
			},
		},
		"required": []string{"agent_id"},
	}
}

func (t *ReadAgentTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	agentID, ok := tools.StringArg(args, "agent_id")
	if !ok || agentID == "" {
		return tools.Result{IsError: true, Content: "agent_id is required"}, nil
	}

	wait := false
	if v, ok := tools.ArgBool(args, "wait"); ok {
		wait = v
	}

	// validate timeout: ensure positive and within bounds
	timeout := 10
	if tIn, ok := tools.ArgInt(args, "timeout"); ok {
		if tIn <= 0 {
			timeout = 10
		} else if tIn > 60 {
			timeout = 60
		} else {
			timeout = tIn
		}
	}

	agent, ok := GetAgentManager().Get(agentID)
	if !ok {
		return tools.Result{IsError: true, Content: fmt.Sprintf("agent not found: %s", agentID)}, nil
	}

	if wait {
		// quick check if already done
		agent.mu.Lock()
		status := agent.Status
		agent.mu.Unlock()
		if status != AgentStatusCompleted && status != AgentStatusFailed && status != AgentStatusCancelled {
			// wait using agent.done channel and context; timeout respected
			timer := time.NewTimer(time.Duration(timeout) * time.Second)
			defer timer.Stop()
			select {
			case <-agent.done:
				// finished
			case <-ctx.Done():
				// respect caller cancellation
				return tools.Result{IsError: true, Content: "read_agent cancelled"}, nil
			case <-timer.C:
				// timeout
			}
		}
	}

	agent.mu.Lock()
	defer agent.mu.Unlock()

	return tools.Result{
		Content: fmt.Sprintf("Agent: %s\nType: %s\nStatus: %s\nDuration: %s\n\nResult:\n%s",
			agent.ID, agent.Type, agent.Status,
			agent.CompletedAt.Sub(agent.StartedAt).Truncate(time.Millisecond),
			agent.Result),
		Metadata: map[string]any{
			"agent_id": agent.ID,
			"status":   string(agent.Status),
		},
	}, nil
}

// ListAgentsTool lists all agents.
type ListAgentsTool struct{}

func (t *ListAgentsTool) Name() string                          { return "list_agents" }
func (t *ListAgentsTool) Description() string                   { return "List all active and completed agents." }
func (t *ListAgentsTool) RequiresConfirmation(mode string) bool { return false }

func (t *ListAgentsTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"include_completed": map[string]any{
				"type":        "boolean",
				"description": "Include completed/failed agents (default: true).",
			},
		},
	}
}

func (t *ListAgentsTool) Execute(_ context.Context, args map[string]any) (tools.Result, error) {
	includeCompleted := true
	if v, ok := tools.ArgBool(args, "include_completed"); ok {
		includeCompleted = v
	}

	agents := GetAgentManager().List(includeCompleted)

	if len(agents) == 0 {
		return tools.Result{Content: "no agents found"}, nil
	}

	var lines []string
	for _, a := range agents {
		a.mu.Lock()
		duration := time.Since(a.StartedAt).Truncate(time.Second)
		if a.Status == AgentStatusCompleted || a.Status == AgentStatusFailed {
			duration = a.CompletedAt.Sub(a.StartedAt).Truncate(time.Second)
		}
		lines = append(lines, fmt.Sprintf("- %s [%s] %s (%s)", a.ID, a.Type, a.Status, duration))
		a.mu.Unlock()
	}

	return tools.Result{
		Content: fmt.Sprintf("%d agent(s):\n%s", len(agents), joinStrings(lines, "\n")),
	}, nil
}

func joinStrings(strs []string, sep string) string {
	result := ""
	for i, s := range strs {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}

// EstimatedCost returns cost estimates for the task tool.
func (t *TaskTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{TokensApprox: 2000, LatencyMs: 30000, RiskLevel: "medium"}
}

// EstimatedCost returns cost estimates for the read agent tool.
func (t *ReadAgentTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{TokensApprox: 500, LatencyMs: 100, RiskLevel: "low"}
}

// EstimatedCost returns cost estimates for the list agents tool.
func (t *ListAgentsTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{TokensApprox: 100, LatencyMs: 50, RiskLevel: "low"}
}
