package coordinator

import (
	"context"

	subagent "github.com/iSundram/Automergent/internal/tools/agent"
)

// AgentExecutorAdapter adapts the coordinator's AgentExecutor to use the agent's execution.
type AgentExecutorAdapter struct {
	agent interface {
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
