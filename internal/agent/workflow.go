package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/iSundram/Automergent/internal/agent/agentdef"
	"github.com/iSundram/Automergent/internal/agent/builtin"
)

// WorkflowManager manages coordinator workflows.
type WorkflowManager struct {
	mu        sync.RWMutex
	workflows map[string]*builtin.CoordinatorWorkflow
	counter   int
}

// NewWorkflowManager creates a new workflow manager.
func NewWorkflowManager() *WorkflowManager {
	return &WorkflowManager{
		workflows: make(map[string]*builtin.CoordinatorWorkflow),
	}
}

// globalWorkflowManager is the default workflow manager.
var globalWorkflowManager = NewWorkflowManager()

// GlobalWorkflowManager returns the global workflow manager.
func GlobalWorkflowManager() *WorkflowManager {
	return globalWorkflowManager
}

// StartWorkflow creates and starts a new coordinator workflow.
func (wm *WorkflowManager) StartWorkflow(
	ctx context.Context,
	originalTask string,
	agent *Agent,
) (*builtin.CoordinatorWorkflow, error) {
	wm.mu.Lock()
	wm.counter++
	id := fmt.Sprintf("wf-%d", wm.counter)
	wm.mu.Unlock()

	spawnFn := func(ctx context.Context, spec builtin.TaskSpec) (string, error) {
		opts := SubagentOptions{
			AgentType:     spec.AgentType,
			Prompt:        spec.Prompt,
			Name:          spec.Name,
			Description:   spec.Description,
			Mode:          "sync",
			StreamToParent: true,
			Model:         spec.Model,
		}
		result := agent.ExecuteSubagent(ctx, opts)
		if result.Error != nil {
			return "", result.Error
		}
		return result.Output, nil
	}

	wf := builtin.NewCoordinatorWorkflow(originalTask, spawnFn)
	wm.mu.Lock()
	wm.workflows[id] = wf
	wm.mu.Unlock()

	// Run in background
	go func() {
		_ = wf.Run(ctx)
	}()

	return wf, nil
}

// GetWorkflow returns a workflow by ID.
func (wm *WorkflowManager) GetWorkflow(id string) (*builtin.CoordinatorWorkflow, bool) {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	wf, ok := wm.workflows[id]
	return wf, ok
}

// ListWorkflows returns all active workflows.
func (wm *WorkflowManager) ListWorkflows() []*builtin.CoordinatorWorkflow {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	result := make([]*builtin.CoordinatorWorkflow, 0, len(wm.workflows))
	for _, wf := range wm.workflows {
		result = append(result, wf)
	}
	return result
}

// Cleanup removes completed workflows older than maxAge.
func (wm *WorkflowManager) Cleanup(maxAge time.Duration) int {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	removed := 0
	for id, wf := range wm.workflows {
		state := wf.GetState()
		if state.IsComplete() && state.StartedAt.Before(cutoff) {
			delete(wm.workflows, id)
			removed++
		}
	}
	return removed
}

// Mode represents the agent operating mode.
type Mode string

const (
	ModeDefault    Mode = "default"
	ModeTriage     Mode = "triage"
	ModeCoordinator Mode = "coordinator"
)

// CurrentMode returns the current operating mode.
// This is determined by the presence of a coordinator workflow or explicit mode setting.
func CurrentMode() Mode {
	wm := GlobalWorkflowManager()
	if len(wm.ListWorkflows()) > 0 {
		return ModeCoordinator
	}
	return ModeDefault
}

// ModeTools returns the tools available in the given mode.
func ModeTools(mode Mode) []string {
	switch mode {
	case ModeCoordinator:
		return []string{"task", "read_agent", "agent_control"}
	case ModeTriage:
		return []string{"task", "read_agent", "list_agents", "agent_control"}
	default:
		return nil // all tools
	}
}

// FilterToolsByMode restricts the tool set based on the operating mode.
func FilterToolsByMode(allTools map[string]bool, mode Mode) map[string]bool {
	modeTools := ModeTools(mode)
	if modeTools == nil {
		return allTools
	}

	filtered := make(map[string]bool, len(modeTools))
	for _, t := range modeTools {
		if _, ok := allTools[t]; ok {
			filtered[t] = true
		}
	}
	return filtered
}

// IsCoordinatorMode checks if the agent should operate in coordinator mode.
// This can be triggered by:
// 1. Explicit mode setting via /mode coordinator
// 2. Environment variable CLAUDE_CODE_COORDINATOR_MODE=true
// 3. Complex task detection (optional)
func IsCoordinatorMode() bool {
	return CurrentMode() == ModeCoordinator
}

// AgentTypeForTask suggests an agent type based on task content.
func AgentTypeForTask(task string) agentdef.AgentType {
	task = lowercase(task)

	// Research/explore patterns
	if containsAny(task, "find", "search", "explore", "understand", "locate", "where") {
		return agentdef.AgentTypeExplore
	}

	// Review patterns
	if containsAny(task, "review", "check", "audit", "security", "bug", "vulnerability") {
		return agentdef.AgentTypeReview
	}

	// Context management patterns
	if containsAny(task, "compact", "context", "summarize", "memory", "bucket") {
		return agentdef.AgentTypeContexter
	}

	// Coordination patterns
	if containsAny(task, "orchestrate", "coordinate", "manage", "workflow", "parallel") {
		return agentdef.AgentTypeCoordinator
	}

	// Default to general purpose
	return agentdef.AgentTypeGeneral
}

func lowercase(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		result[i] = c
	}
	return string(result)
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if len(sub) > 0 && len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}
