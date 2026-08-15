// Package coordinator provides a multi-agent coordination system.
package coordinator

import (
	"context"
	"fmt"

	"github.com/iSundram/Automergent/internal/reasoning"
	subagent "github.com/iSundram/Automergent/internal/tools/agent"
)

// FromReasoningPlan converts a reasoning.ExecutionPlan to a coordinator.ExecutionPlan.
func FromReasoningPlan(ctx context.Context, rp *reasoning.ExecutionPlan) (*ExecutionPlan, error) {
	if rp == nil {
		return nil, fmt.Errorf("reasoning plan is nil")
	}

	// Convert tasks
	tasks := make([]*Task, 0, len(rp.Tasks))
	taskByID := make(map[string]*Task, len(rp.Tasks))

	for _, rt := range rp.Tasks {
		// Map reasoning task type to coordinator role
		role := mapTaskTypeToRole(rt.Type)

		// Convert context from map to TaskContext
		taskCtx := TaskContext{}
		if rt.Context != nil {
			if files, ok := rt.Context["files"]; ok {
				taskCtx.Files = []string{files}
			}
			if workingDir, ok := rt.Context["working_dir"]; ok {
				taskCtx.WorkingDir = workingDir
			}
		}

		task := &Task{
			ID:          rt.ID,
			Prompt:      rt.Description,
			Role:        role,
			Dependencies: rt.Dependencies,
			Priority:    mapPriority(rt.Priority),
			Context:     taskCtx,
			Status:      TaskStatusPending,
			CreatedAt:   rt.CreatedAt,
			MaxRetries:  3,
			Timeout:     rt.Estimated,
		}
		tasks = append(tasks, task)
		taskByID[rt.ID] = task
	}

	// Convert execution order to phases
	phases := make([]ExecutionPhase, 0, len(rp.ExecutionOrder))
	for i, group := range rp.ExecutionOrder {
		if len(group) == 0 {
			continue
		}
		phaseName := mapPhaseName(i, len(rp.ExecutionOrder))
		phase := ExecutionPhase{
			Index:    i,
			Name:     phaseName,
			TaskIDs:  group,
			Parallel: len(group) > 1,
		}
		phases = append(phases, phase)
	}

	// If no phases from execution order, create default phases
	if len(phases) == 0 && len(tasks) > 0 {
		phases = createDefaultPhases(tasks, rp.Analysis)
	}

	plan := &ExecutionPlan{
		ID:                rp.ID,
		Tasks:             tasks,
		Phases:            phases,
		Dependencies:      buildDependencies(tasks),
		EstimatedDuration: rp.Analysis.EstimatedTime,
		CreatedAt:         rp.CreatedAt,
	}

	return plan, nil
}

func mapTaskTypeToRole(tt reasoning.TaskType) AgentRole {
	switch tt {
	case reasoning.TaskTypeInvestigation:
		return RoleResearcher
	case reasoning.TaskTypeFeature, reasoning.TaskTypeBugFix, reasoning.TaskTypeRefactor, reasoning.TaskTypeMultiFile:
		return RoleCoder
	case reasoning.TaskTypeTest:
		return RoleTester
	case reasoning.TaskTypeDocumentation:
		return RoleDocumenter
	case reasoning.TaskTypeBuild, reasoning.TaskTypeDeployment:
		return RoleReviewer
	default:
		return RoleCoder
	}
}

func mapPriority(p int) TaskPriority {
	switch {
	case p >= 100:
		return PriorityCritical
	case p >= 90:
		return PriorityHigh
	case p >= 50:
		return PriorityNormal
	default:
		return PriorityLow
	}
}

func mapPhaseName(index, total int) string {
	if total <= 1 {
		return "execute"
	}
	switch index {
	case 0:
		return "research"
	case total - 1:
		return "execute"
	default:
		return "plan"
	}
}

func createDefaultPhases(tasks []*Task, analysis *reasoning.TaskAnalysis) []ExecutionPhase {
	if len(tasks) == 0 {
		return nil
	}

	phases := []ExecutionPhase{}

	// Phase 1: Research (investigation tasks)
	researchTasks := filterTasksByRole(tasks, RoleResearcher)
	if len(researchTasks) > 0 {
		ids := make([]string, len(researchTasks))
		for i, t := range researchTasks {
			ids[i] = t.ID
		}
		phases = append(phases, ExecutionPhase{
			Index:    0,
			Name:     "research",
			TaskIDs:  ids,
			Parallel: len(ids) > 1,
		})
	}

	// Phase 2: Plan (analysis/planning tasks)
	planTasks := filterTasksByRole(tasks, RoleCoder)
	if len(planTasks) > 0 {
		ids := make([]string, len(planTasks))
		for i, t := range planTasks {
			ids[i] = t.ID
		}
		phases = append(phases, ExecutionPhase{
			Index:    len(phases),
			Name:     "plan",
			TaskIDs:  ids,
			Parallel: len(ids) > 1,
		})
	}

	// Phase 3: Execute (coder, tester, etc.)
	executeTasks := filterTasksByRole(tasks, RoleCoder, RoleTester, RoleReviewer, RoleDocumenter)
	if len(executeTasks) > 0 {
		ids := make([]string, len(executeTasks))
		for i, t := range executeTasks {
			ids[i] = t.ID
		}
		phases = append(phases, ExecutionPhase{
			Index:    len(phases),
			Name:     "execute",
			TaskIDs:  ids,
			Parallel: len(ids) > 1,
		})
	}

	return phases
}

func filterTasksByRole(tasks []*Task, roles ...AgentRole) []*Task {
	roleSet := make(map[AgentRole]bool)
	for _, r := range roles {
		roleSet[r] = true
	}
	var result []*Task
	for _, t := range tasks {
		if roleSet[t.Role] {
			result = append(result, t)
		}
	}
	return result
}

func buildDependencies(tasks []*Task) map[string][]string {
	deps := make(map[string][]string)
	for _, t := range tasks {
		if len(t.Dependencies) > 0 {
			deps[t.ID] = t.Dependencies
		}
	}
	return deps
}

// AgentExecutorAdapter adapts the coordinator's AgentExecutor to use the agent's execution.
type AgentExecutorAdapter struct {
	agent   interface {
		Execute(ctx context.Context, agentType subagent.AgentType, prompt string, model string) (string, error)
	}
	model string
}

// NewAgentExecutorAdapterWithModel creates an adapter with a pre-fetched model name,
// avoiding the deadlock that occurs when the caller already holds a.mu.Lock()
// and GetModel() tries to acquire a.mu.RLock().
func NewAgentExecutorAdapterWithModel(agent interface {
	Execute(ctx context.Context, agentType subagent.AgentType, prompt string, model string) (string, error)
}, model string) *AgentExecutorAdapter {
	return &AgentExecutorAdapter{
		agent: agent,
		model: model,
	}
}

func (e *AgentExecutorAdapter) Execute(ctx context.Context, role AgentRole, prompt string, taskCtx TaskContext, model string) (*TaskResult, error) {
	modelName := model
	if modelName == "" {
		modelName = e.model
	}

	// Build a role-specific prompt that includes task context (files, code, constraints).
	task := &Task{
		Prompt:  prompt,
		Context: taskCtx,
		Role:    role,
	}
	rolePrompt := BuildRolePrompt(role, task)

	response, err := e.agent.Execute(ctx, subagent.AgentTypeTask, rolePrompt, modelName)
	if err != nil {
		return &TaskResult{
			Success: false,
			Errors:  []string{err.Error()},
		}, err
	}

	return &TaskResult{
		Success: true,
		Output:  response,
		Quality: 0.85,
	}, nil
}