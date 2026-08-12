package coordinator

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// researcherAgent implements the Researcher agent type.
type researcherAgent struct {
	id       string
	model    string
	executor AgentExecutor
}

// coderAgent implements the Coder agent type.
type coderAgent struct {
	id       string
	model    string
	executor AgentExecutor
}

// reviewerAgent implements the Reviewer agent type.
type reviewerAgent struct {
	id       string
	model    string
	executor AgentExecutor
}

// testerAgent implements the Tester agent type.
type testerAgent struct {
	id       string
	model    string
	executor AgentExecutor
}

// documenterAgent implements the Documenter agent type.
type documenterAgent struct {
	id       string
	model    string
	executor AgentExecutor
}

// AgentFactory creates specialized agents.
type AgentFactory struct {
	executor AgentExecutor
	config   *CoordinatorConfig
}

// NewAgentFactory creates a new agent factory.
func NewAgentFactory(executor AgentExecutor, config *CoordinatorConfig) *AgentFactory {
	return &AgentFactory{
		executor: executor,
		config:   config,
	}
}

// CreateAgent creates an agent of the specified role.
func (f *AgentFactory) CreateAgent(role AgentRole) Agent {
	id := fmt.Sprintf("%s-%s", role, uuid.New().String()[:8])
	model := f.getModelForRole(role)

	switch role {
	case RoleResearcher:
		return &researcherAgent{id: id, model: model, executor: f.executor}
	case RoleCoder:
		return &coderAgent{id: id, model: model, executor: f.executor}
	case RoleReviewer:
		return &reviewerAgent{id: id, model: model, executor: f.executor}
	case RoleTester:
		return &testerAgent{id: id, model: model, executor: f.executor}
	case RoleDocumenter:
		return &documenterAgent{id: id, model: model, executor: f.executor}
	default:
		return &researcherAgent{id: id, model: model, executor: f.executor}
	}
}

func (f *AgentFactory) getModelForRole(role AgentRole) string {
	switch role {
	case RoleResearcher:
		return "gemini-3.6-flash"
	case RoleCoder:
		return f.config.Model
	case RoleReviewer:
		return f.config.Model
	case RoleTester:
		return "gemini-3.6-flash"
	case RoleDocumenter:
		return "gemini-3.6-flash"
	default:
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

	// Build researcher-specific prompt
	prompt := buildResearcherPrompt(task)

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

	result, err := a.executor.Execute(ctx, RoleResearcher, prompt, task.Context, a.model)
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

func buildResearcherPrompt(task *Task) string {
	var sb strings.Builder

	sb.WriteString("You are a Researcher agent specialized in exploring codebases and gathering context.\n\n")
	sb.WriteString("Your task is to:\n")
	sb.WriteString("1. Explore and understand the relevant code\n")
	sb.WriteString("2. Gather context and dependencies\n")
	sb.WriteString("3. Identify patterns and architecture\n")
	sb.WriteString("4. Report findings concisely\n\n")

	sb.WriteString("Task: ")
	sb.WriteString(task.Prompt)
	sb.WriteString("\n")

	if task.Context.WorkingDir != "" {
		sb.WriteString("\nWorking Directory: ")
		sb.WriteString(task.Context.WorkingDir)
		sb.WriteString("\n")
	}

	if len(task.Context.Files) > 0 {
		sb.WriteString("\nRelevant Files:\n")
		for _, f := range task.Context.Files {
			sb.WriteString("- ")
			sb.WriteString(f)
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// Coder Agent Implementation

func (a *coderAgent) ID() string      { return a.id }
func (a *coderAgent) Role() AgentRole { return RoleCoder }
func (a *coderAgent) Model() string   { return a.model }

func (a *coderAgent) Execute(ctx context.Context, task *Task) (*TaskResult, error) {
	startTime := time.Now()

	prompt := buildCoderPrompt(task)

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

	result, err := a.executor.Execute(ctx, RoleCoder, prompt, task.Context, a.model)
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

func buildCoderPrompt(task *Task) string {
	var sb strings.Builder

	sb.WriteString("You are a Coder agent specialized in implementing code changes.\n\n")
	sb.WriteString("Your task is to:\n")
	sb.WriteString("1. Implement the requested changes\n")
	sb.WriteString("2. Follow existing code style and patterns\n")
	sb.WriteString("3. Write clean, maintainable code\n")
	sb.WriteString("4. Include necessary imports and dependencies\n\n")

	sb.WriteString("Task: ")
	sb.WriteString(task.Prompt)
	sb.WriteString("\n")

	if len(task.Context.CodeSnippets) > 0 {
		sb.WriteString("\nRelevant Code:\n")
		for path, code := range task.Context.CodeSnippets {
			sb.WriteString(fmt.Sprintf("\n--- %s ---\n%s\n", path, code))
		}
	}

	if len(task.Context.Constraints) > 0 {
		sb.WriteString("\nConstraints:\n")
		for _, c := range task.Context.Constraints {
			sb.WriteString("- ")
			sb.WriteString(c)
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// Reviewer Agent Implementation

func (a *reviewerAgent) ID() string      { return a.id }
func (a *reviewerAgent) Role() AgentRole { return RoleReviewer }
func (a *reviewerAgent) Model() string   { return a.model }

func (a *reviewerAgent) Execute(ctx context.Context, task *Task) (*TaskResult, error) {
	startTime := time.Now()

	prompt := buildReviewerPrompt(task)

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

	result, err := a.executor.Execute(ctx, RoleReviewer, prompt, task.Context, a.model)
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

func buildReviewerPrompt(task *Task) string {
	var sb strings.Builder

	sb.WriteString("You are a Reviewer agent specialized in code review.\n\n")
	sb.WriteString("Your task is to:\n")
	sb.WriteString("1. Review the code for bugs and issues\n")
	sb.WriteString("2. Check for security vulnerabilities\n")
	sb.WriteString("3. Verify adherence to best practices\n")
	sb.WriteString("4. Suggest improvements (only significant ones)\n")
	sb.WriteString("5. Focus on high-impact issues, not style\n\n")

	sb.WriteString("Task: ")
	sb.WriteString(task.Prompt)
	sb.WriteString("\n")

	if len(task.Context.CodeSnippets) > 0 {
		sb.WriteString("\nCode to Review:\n")
		for path, code := range task.Context.CodeSnippets {
			sb.WriteString(fmt.Sprintf("\n--- %s ---\n%s\n", path, code))
		}
	}

	return sb.String()
}

// Tester Agent Implementation

func (a *testerAgent) ID() string      { return a.id }
func (a *testerAgent) Role() AgentRole { return RoleTester }
func (a *testerAgent) Model() string   { return a.model }

func (a *testerAgent) Execute(ctx context.Context, task *Task) (*TaskResult, error) {
	startTime := time.Now()

	prompt := buildTesterPrompt(task)

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

	result, err := a.executor.Execute(ctx, RoleTester, prompt, task.Context, a.model)
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

func buildTesterPrompt(task *Task) string {
	var sb strings.Builder

	sb.WriteString("You are a Tester agent specialized in testing and quality assurance.\n\n")
	sb.WriteString("Your task is to:\n")
	sb.WriteString("1. Write comprehensive tests\n")
	sb.WriteString("2. Cover edge cases and error conditions\n")
	sb.WriteString("3. Run existing tests if requested\n")
	sb.WriteString("4. Report test results and coverage\n\n")

	sb.WriteString("Task: ")
	sb.WriteString(task.Prompt)
	sb.WriteString("\n")

	if len(task.Context.CodeSnippets) > 0 {
		sb.WriteString("\nCode to Test:\n")
		for path, code := range task.Context.CodeSnippets {
			sb.WriteString(fmt.Sprintf("\n--- %s ---\n%s\n", path, code))
		}
	}

	return sb.String()
}

// Documenter Agent Implementation

func (a *documenterAgent) ID() string      { return a.id }
func (a *documenterAgent) Role() AgentRole { return RoleDocumenter }
func (a *documenterAgent) Model() string   { return a.model }

func (a *documenterAgent) Execute(ctx context.Context, task *Task) (*TaskResult, error) {
	startTime := time.Now()

	prompt := buildDocumenterPrompt(task)

	if a.executor == nil {
		return &TaskResult{
			TaskID:     task.ID,
			WorkerID:   a.id,
			Success:    true,
			Output:     fmt.Sprintf("[Documenter %s would document: %s]", a.id, task.Prompt),
			Quality:    0.8,
			Confidence: 0.85,
			Artifacts: []Artifact{
				{Type: ArtifactTypeDoc, Path: "README.md", Content: "# Generated Documentation"},
			},
			Duration: time.Since(startTime),
		}, nil
	}

	result, err := a.executor.Execute(ctx, RoleDocumenter, prompt, task.Context, a.model)
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

func buildDocumenterPrompt(task *Task) string {
	var sb strings.Builder

	sb.WriteString("You are a Documenter agent specialized in documentation.\n\n")
	sb.WriteString("Your task is to:\n")
	sb.WriteString("1. Write clear, concise documentation\n")
	sb.WriteString("2. Include usage examples\n")
	sb.WriteString("3. Document APIs and interfaces\n")
	sb.WriteString("4. Keep docs up-to-date with code\n\n")

	sb.WriteString("Task: ")
	sb.WriteString(task.Prompt)
	sb.WriteString("\n")

	if len(task.Context.CodeSnippets) > 0 {
		sb.WriteString("\nCode to Document:\n")
		for path, code := range task.Context.CodeSnippets {
			sb.WriteString(fmt.Sprintf("\n--- %s ---\n%s\n", path, code))
		}
	}

	return sb.String()
}
