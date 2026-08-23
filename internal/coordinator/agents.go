package coordinator

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/iSundram/Automergent/internal/prompt"
)

// researcherAgent implements the Researcher agent type.
type researcherAgent struct {
	id            string
	model         string
	executor      AgentExecutor
	promptManager *prompt.PromptManager
}

// coderAgent implements the Coder agent type.
type coderAgent struct {
	id            string
	model         string
	executor      AgentExecutor
	promptManager *prompt.PromptManager
}

// reviewerAgent implements the Reviewer agent type.
type reviewerAgent struct {
	id            string
	model         string
	executor      AgentExecutor
	promptManager *prompt.PromptManager
}

// testerAgent implements the Tester agent type.
type testerAgent struct {
	id            string
	model         string
	executor      AgentExecutor
	promptManager *prompt.PromptManager
}

// documenterAgent implements the Documenter agent type.
type documenterAgent struct {
	id            string
	model         string
	executor      AgentExecutor
	promptManager *prompt.PromptManager
}

// AgentFactory creates specialized agents.
type AgentFactory struct {
	executor      AgentExecutor
	config        *CoordinatorConfig
	promptManager *prompt.PromptManager
}

// NewAgentFactory creates a new agent factory.
func NewAgentFactory(executor AgentExecutor, config *CoordinatorConfig) *AgentFactory {
	return NewAgentFactoryWithPromptManager(executor, config, nil)
}

// NewAgentFactoryWithPromptManager creates a new agent factory with an optional prompt manager.
func NewAgentFactoryWithPromptManager(executor AgentExecutor, config *CoordinatorConfig, pm *prompt.PromptManager) *AgentFactory {
	return &AgentFactory{
		executor:      executor,
		config:        config,
		promptManager: pm,
	}
}

// CreateAgent creates an agent of the specified role.
func (f *AgentFactory) CreateAgent(role AgentRole) Agent {
	id := fmt.Sprintf("%s-%s", role, uuid.New().String()[:8])
	model := f.getModelForRole(role)

	switch role {
	case RoleResearcher:
		return &researcherAgent{id: id, model: model, executor: f.executor, promptManager: f.promptManager}
	case RoleCoder:
		return &coderAgent{id: id, model: model, executor: f.executor, promptManager: f.promptManager}
	case RoleReviewer:
		return &reviewerAgent{id: id, model: model, executor: f.executor, promptManager: f.promptManager}
	case RoleTester:
		return &testerAgent{id: id, model: model, executor: f.executor, promptManager: f.promptManager}
	case RoleDocumenter:
		return &documenterAgent{id: id, model: model, executor: f.executor, promptManager: f.promptManager}
	default:
		return &researcherAgent{id: id, model: model, executor: f.executor, promptManager: f.promptManager}
	}
}

func (f *AgentFactory) getModelForRole(role AgentRole) string {
	// Check configurable overrides first.
	if f.config.ModelOverrides != nil {
		if model, ok := f.config.ModelOverrides[role]; ok && model != "" {
			return model
		}
	}

	// Fall back to role-specific defaults.
	switch role {
	case RoleCoder, RoleReviewer:
		return f.config.Model
	default:
		if f.config.FallbackModel != "" {
			return f.config.FallbackModel
		}
		return f.config.Model
	}
}

// Agent interface for all agent types.
type Agent interface {
	ID() string
	Role() AgentRole
	Model() string
	Execute(ctx context.Context, task *Task) (*TaskResult, error)
}

// Researcher Agent Implementation

func (a *researcherAgent) ID() string      { return a.id }
func (a *researcherAgent) Role() AgentRole { return RoleResearcher }
func (a *researcherAgent) Model() string   { return a.model }

func (a *researcherAgent) Execute(ctx context.Context, task *Task) (*TaskResult, error) {
	startTime := time.Now()

	// Use new prompt system if available
	promptStr := a.buildPrompt(task, RoleResearcher)

	if a.executor == nil {
		return &TaskResult{
			TaskID:     task.ID,
			WorkerID:   a.id,
			Success:    true,
			Output:     fmt.Sprintf("[Researcher %s would explore: %s]", a.id, task.Prompt),
			Quality:    0.8,
			Confidence: 0.8,
			Duration:   time.Since(startTime),
		}, nil
	}

	result, err := a.executor.Execute(ctx, RoleResearcher, promptStr, task.Context, a.model)
	if err != nil {
		return &TaskResult{
			TaskID:   task.ID,
			WorkerID: a.id,
			Success:  false,
			Errors:   []string{err.Error()},
			Duration: time.Since(startTime),
		}, err
	}

	result.TaskID = task.ID
	result.WorkerID = a.id
	result.Duration = time.Since(startTime)

	return result, nil
}

func (a *researcherAgent) buildPrompt(task *Task, role AgentRole) string {
	if a.promptManager != nil {
		adapter := NewPromptAdapter(a.promptManager)
		return adapter.BuildRolePrompt(role, task)
	}
	return buildResearcherPrompt(task)
}

// Coder Agent Implementation

func (a *coderAgent) ID() string      { return a.id }
func (a *coderAgent) Role() AgentRole { return RoleCoder }
func (a *coderAgent) Model() string   { return a.model }

func (a *coderAgent) Execute(ctx context.Context, task *Task) (*TaskResult, error) {
	startTime := time.Now()

	promptStr := a.buildPrompt(task, RoleCoder)

	if a.executor == nil {
		return &TaskResult{
			TaskID:     task.ID,
			WorkerID:   a.id,
			Success:    true,
			Output:     fmt.Sprintf("[Coder %s would implement: %s]", a.id, task.Prompt),
			Quality:    0.85,
			Confidence: 0.85,
			Artifacts: []Artifact{
				{Type: ArtifactTypeCode, Path: "generated.go", Content: "// Generated code"},
			},
			Duration: time.Since(startTime),
		}, nil
	}

	result, err := a.executor.Execute(ctx, RoleCoder, promptStr, task.Context, a.model)
	if err != nil {
		return &TaskResult{
			TaskID:   task.ID,
			WorkerID: a.id,
			Success:  false,
			Errors:   []string{err.Error()},
			Duration: time.Since(startTime),
		}, err
	}

	result.TaskID = task.ID
	result.WorkerID = a.id
	result.Duration = time.Since(startTime)

	return result, nil
}

func (a *coderAgent) buildPrompt(task *Task, role AgentRole) string {
	if a.promptManager != nil {
		adapter := NewPromptAdapter(a.promptManager)
		return adapter.BuildRolePrompt(role, task)
	}
	return buildCoderPrompt(task)
}

// Reviewer Agent Implementation

func (a *reviewerAgent) ID() string      { return a.id }
func (a *reviewerAgent) Role() AgentRole { return RoleReviewer }
func (a *reviewerAgent) Model() string   { return a.model }

func (a *reviewerAgent) Execute(ctx context.Context, task *Task) (*TaskResult, error) {
	startTime := time.Now()

	promptStr := a.buildPrompt(task, RoleReviewer)

	if a.executor == nil {
		return &TaskResult{
			TaskID:     task.ID,
			WorkerID:   a.id,
			Success:    true,
			Output:     fmt.Sprintf("[Reviewer %s would review: %s]", a.id, task.Prompt),
			Quality:    0.9,
			Confidence: 0.9,
			Warnings:   []string{"Example warning: Consider adding error handling"},
			Artifacts: []Artifact{
				{Type: ArtifactTypeAnalysis, Content: "Code review analysis"},
			},
			Duration: time.Since(startTime),
		}, nil
	}

	result, err := a.executor.Execute(ctx, RoleReviewer, promptStr, task.Context, a.model)
	if err != nil {
		return &TaskResult{
			TaskID:   task.ID,
			WorkerID: a.id,
			Success:  false,
			Errors:   []string{err.Error()},
			Duration: time.Since(startTime),
		}, err
	}

	result.TaskID = task.ID
	result.WorkerID = a.id
	result.Duration = time.Since(startTime)

	return result, nil
}

func (a *reviewerAgent) buildPrompt(task *Task, role AgentRole) string {
	if a.promptManager != nil {
		adapter := NewPromptAdapter(a.promptManager)
		return adapter.BuildRolePrompt(role, task)
	}
	return buildReviewerPrompt(task)
}

// Tester Agent Implementation

func (a *testerAgent) ID() string      { return a.id }
func (a *testerAgent) Role() AgentRole { return RoleTester }
func (a *testerAgent) Model() string   { return a.model }

func (a *testerAgent) Execute(ctx context.Context, task *Task) (*TaskResult, error) {
	startTime := time.Now()

	promptStr := a.buildPrompt(task, RoleTester)

	if a.executor == nil {
		return &TaskResult{
			TaskID:     task.ID,
			WorkerID:   a.id,
			Success:    true,
			Output:     fmt.Sprintf("[Tester %s would test: %s]", a.id, task.Prompt),
			Quality:    0.85,
			Confidence: 0.85,
			Artifacts: []Artifact{
				{Type: ArtifactTypeTest, Path: "generated_test.go", Content: "// Generated tests"},
			},
			Duration: time.Since(startTime),
		}, nil
	}

	result, err := a.executor.Execute(ctx, RoleTester, promptStr, task.Context, a.model)
	if err != nil {
		return &TaskResult{
			TaskID:   task.ID,
			WorkerID: a.id,
			Success:  false,
			Errors:   []string{err.Error()},
			Duration: time.Since(startTime),
		}, err
	}

	result.TaskID = task.ID
	result.WorkerID = a.id
	result.Duration = time.Since(startTime)

	return result, nil
}

func (a *testerAgent) buildPrompt(task *Task, role AgentRole) string {
	if a.promptManager != nil {
		adapter := NewPromptAdapter(a.promptManager)
		return adapter.BuildRolePrompt(role, task)
	}
	return buildTesterPrompt(task)
}

// Documenter Agent Implementation

func (a *documenterAgent) ID() string      { return a.id }
func (a *documenterAgent) Role() AgentRole { return RoleDocumenter }
func (a *documenterAgent) Model() string   { return a.model }

func (a *documenterAgent) Execute(ctx context.Context, task *Task) (*TaskResult, error) {
	startTime := time.Now()

	promptStr := a.buildPrompt(task, RoleDocumenter)

	if a.executor == nil {
		return &TaskResult{
			TaskID:   task.ID,
			WorkerID: a.id,
			Success:  false,
			Errors:   []string{"no executor configured"},
			Duration: time.Since(startTime),
		}, nil
	}

	result, err := a.executor.Execute(ctx, RoleDocumenter, promptStr, task.Context, a.model)
	if err != nil {
		return &TaskResult{
			TaskID:   task.ID,
			WorkerID: a.id,
			Success:  false,
			Errors:   []string{err.Error()},
			Duration: time.Since(startTime),
		}, err
	}

	result.TaskID = task.ID
	result.WorkerID = a.id
	result.Duration = time.Since(startTime)

	return result, nil
}

func (a *documenterAgent) buildPrompt(task *Task, role AgentRole) string {
	if a.promptManager != nil {
		adapter := NewPromptAdapter(a.promptManager)
		return adapter.BuildRolePrompt(role, task)
	}
	return BuildRolePrompt(RoleDocumenter, task)
}
