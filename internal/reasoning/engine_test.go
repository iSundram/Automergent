package reasoning

import (
	"context"
	"testing"
	"time"
)

func TestEngine_Analyze(t *testing.T) {
	engine := NewEngine(nil)
	ctx := context.Background()

	tests := []struct {
		name           string
		request        string
		wantType       TaskType
		wantScope      Scope
		wantComplexity Complexity
	}{
		{
			name:           "simple bug fix",
			request:        "Fix the bug in user authentication",
			wantType:       TaskTypeBugFix,
			wantScope:      ScopeSingleFile,
			wantComplexity: ComplexitySimple,
		},
		{
			name:           "new feature",
			request:        "Add a new API endpoint for user profiles",
			wantType:       TaskTypeFeature,
			wantScope:      ScopeSingleFile,
			wantComplexity: ComplexitySimple,
		},
		{
			name:           "project-wide refactor",
			request:        "Refactor the entire codebase to use dependency injection",
			wantType:       TaskTypeRefactor,
			wantScope:      ScopeProjectWide,
			wantComplexity: ComplexityComplex,
		},
		{
			name:           "documentation",
			request:        "Document the API endpoints in README.md",
			wantType:       TaskTypeDocumentation,
			wantScope:      ScopeSingleFile,
			wantComplexity: ComplexitySimple,
		},
		{
			name:           "investigation",
			request:        "Why does the database connection fail intermittently?",
			wantType:       TaskTypeInvestigation,
			wantScope:      ScopeSingleFile,
			wantComplexity: ComplexitySimple,
		},
		{
			name:           "multi-file test",
			request:        "Add tests across multiple files",
			wantType:       TaskTypeTest,
			wantScope:      ScopeMultiFile,
			wantComplexity: ComplexityModerate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis, err := engine.Analyze(ctx, tt.request)
			if err != nil {
				t.Fatalf("Analyze() error = %v", err)
			}

			if analysis.TaskType != tt.wantType {
				t.Errorf("TaskType = %v, want %v", analysis.TaskType, tt.wantType)
			}

			if analysis.Scope != tt.wantScope {
				t.Errorf("Scope = %v, want %v", analysis.Scope, tt.wantScope)
			}

			if analysis.Complexity != tt.wantComplexity {
				t.Errorf("Complexity = %v, want %v", analysis.Complexity, tt.wantComplexity)
			}

			if analysis.EstimatedTime == 0 {
				t.Error("EstimatedTime should not be zero")
			}

			if len(analysis.Intent) == 0 {
				t.Error("Intent should not be empty")
			}
		})
	}
}

func TestEngine_Process(t *testing.T) {
	engine := NewEngine(nil)
	engine.GetExecutor().SetRunner(&mockTaskRunner{})
	ctx := context.Background()

	tests := []struct {
		name    string
		request string
		wantErr bool
	}{
		{
			name:    "simple request",
			request: "Add a hello world function",
			wantErr: false,
		},
		{
			name:    "complex request",
			request: "Refactor entire project structure",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := engine.Process(ctx, tt.request)

			if (err != nil) != tt.wantErr {
				t.Errorf("Process() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if plan == nil {
					t.Error("Process() returned nil plan")
					return
				}

				if len(plan.Tasks) == 0 {
					t.Error("Plan has no tasks")
				}

				if plan.Analysis == nil {
					t.Error("Plan has no analysis")
				}

				trace := engine.GetTrace()
				if trace == nil {
					t.Error("No reasoning trace generated")
				} else if len(trace.Steps) == 0 {
					t.Error("Reasoning trace has no steps")
				}
			}
		})
	}
}

func TestEngine_ClassifyTask(t *testing.T) {
	engine := NewEngine(nil)

	tests := []struct {
		request  string
		wantType TaskType
	}{
		{"fix the bug", TaskTypeBugFix},
		{"Fix broken code", TaskTypeBugFix},
		{"add new feature", TaskTypeFeature},
		{"create a component", TaskTypeFeature},
		{"refactor this code", TaskTypeRefactor},
		{"optimize performance", TaskTypeRefactor},
		{"document the API", TaskTypeDocumentation},
		{"add comments", TaskTypeDocumentation},
		{"why is this failing", TaskTypeInvestigation},
		{"how does this work", TaskTypeInvestigation},
		{"add unit tests", TaskTypeTest},
		{"improve test coverage", TaskTypeTest},
	}

	for _, tt := range tests {
		t.Run(tt.request, func(t *testing.T) {
			got := engine.classifyTask(tt.request)
			if got != tt.wantType {
				t.Errorf("classifyTask(%q) = %v, want %v", tt.request, got, tt.wantType)
			}
		})
	}
}

func TestEngine_DetermineScope(t *testing.T) {
	engine := NewEngine(nil)

	tests := []struct {
		request   string
		wantScope Scope
	}{
		{"fix file.go", ScopeSingleFile},
		{"update multiple files", ScopeMultiFile},
		{"refactor entire project", ScopeProjectWide},
		{"integrate with external API", ScopeExternal},
		{"modify all files in codebase", ScopeProjectWide},
	}

	for _, tt := range tests {
		t.Run(tt.request, func(t *testing.T) {
			got := engine.determineScope(tt.request)
			if got != tt.wantScope {
				t.Errorf("determineScope(%q) = %v, want %v", tt.request, got, tt.wantScope)
			}
		})
	}
}

func TestEngine_AssessComplexity(t *testing.T) {
	engine := NewEngine(nil)

	tests := []struct {
		name           string
		request        string
		taskType       TaskType
		scope          Scope
		wantComplexity Complexity
	}{
		{
			name:           "simple single file",
			request:        "fix typo",
			taskType:       TaskTypeBugFix,
			scope:          ScopeSingleFile,
			wantComplexity: ComplexitySimple,
		},
		{
			name:           "complex refactor",
			request:        "complex architecture redesign",
			taskType:       TaskTypeRefactor,
			scope:          ScopeProjectWide,
			wantComplexity: ComplexityMajor,
		},
		{
			name:           "simple feature",
			request:        "just add a simple function",
			taskType:       TaskTypeFeature,
			scope:          ScopeSingleFile,
			wantComplexity: ComplexityTrivial,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := engine.assessComplexity(tt.request, tt.taskType, tt.scope)
			if got != tt.wantComplexity {
				t.Errorf("assessComplexity() = %v, want %v", got, tt.wantComplexity)
			}
		})
	}
}

func TestEngine_EstimateTime(t *testing.T) {
	engine := NewEngine(nil)

	tests := []struct {
		complexity Complexity
		minTime    time.Duration
		maxTime    time.Duration
	}{
		{ComplexityTrivial, 4 * time.Minute, 6 * time.Minute},
		{ComplexitySimple, 10 * time.Minute, 20 * time.Minute},
		{ComplexityModerate, 30 * time.Minute, 60 * time.Minute},
		{ComplexityComplex, 1 * time.Hour, 3 * time.Hour},
		{ComplexityMajor, 5 * time.Hour, 8 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(string(tt.complexity), func(t *testing.T) {
			got := engine.estimateTime(tt.complexity)
			if got < tt.minTime || got > tt.maxTime {
				t.Errorf("estimateTime(%v) = %v, want between %v and %v",
					tt.complexity, got, tt.minTime, tt.maxTime)
			}
		})
	}
}

func TestEngine_RegisterStrategy(t *testing.T) {
	engine := NewEngine(nil)

	strategy := &mockStrategy{
		name:     "test-strategy",
		taskType: TaskTypeBugFix,
	}

	engine.RegisterStrategy(TaskTypeBugFix, strategy)

	// Verify strategy was registered
	if engine.strategies[TaskTypeBugFix] == nil {
		t.Error("Strategy was not registered")
	}
}

func TestEngine_GetTrace(t *testing.T) {
	engine := NewEngine(nil)

	trace := engine.GetTrace()
	if trace == nil {
		t.Error("GetTrace() returned nil")
	}

	if trace.StartedAt.IsZero() {
		t.Error("Trace StartedAt is zero")
	}
}

func TestEngine_Recovery(t *testing.T) {
	engine := NewEngine(&EngineConfig{
		MaxRetries:      2,
		ParallelWorkers: 1,
	})

	ctx := context.Background()

	// Create a plan that will fail verification
	analysis := &TaskAnalysis{
		Intent:        "Test recovery",
		TaskType:      TaskTypeBugFix,
		Scope:         ScopeSingleFile,
		Complexity:    ComplexitySimple,
		EstimatedTime: 10 * time.Minute,
	}

	plan, err := engine.Plan(ctx, analysis)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}

	// The recovery logic should handle failures gracefully
	// In real implementation, this would test actual recovery behavior
	if plan == nil {
		t.Error("Plan should not be nil")
	}
}

func TestEngineConfig_Defaults(t *testing.T) {
	cfg := DefaultEngineConfig()

	if cfg.MaxRetries <= 0 {
		t.Error("MaxRetries should be positive")
	}

	if cfg.ParallelWorkers <= 0 {
		t.Error("ParallelWorkers should be positive")
	}

	if cfg.ThinkingBudget <= 0 {
		t.Error("ThinkingBudget should be positive")
	}

	if !cfg.EnableExtendedThinking {
		t.Error("EnableExtendedThinking should be true by default")
	}
}

func TestExtractIntent(t *testing.T) {
	tests := []struct {
		request string
		want    string
	}{
		{
			request: "Add a new function.",
			want:    "Add a new function",
		},
		{
			request: "This is a very long request that exceeds one hundred characters and should be truncated at exactly one hundred characters",
			want:    "This is a very long request that exceeds one hundred characters and should be truncated at exactl...",
		},
		{
			request: "Short",
			want:    "Short",
		},
	}

	for _, tt := range tests {
		t.Run(tt.request, func(t *testing.T) {
			got := extractIntent(tt.request)
			if got != tt.want {
				t.Errorf("extractIntent() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Mock strategy for testing
type mockStrategy struct {
	name     string
	taskType TaskType
}

func (s *mockStrategy) Name() string {
	return s.name
}

func (s *mockStrategy) CanHandle(taskType TaskType) bool {
	return taskType == s.taskType
}

func (s *mockStrategy) Decompose(ctx context.Context, analysis *TaskAnalysis) ([]*Task, error) {
	return []*Task{
		{
			ID:          "test-task",
			Description: "Test task",
			Type:        s.taskType,
			Status:      TaskStatusPending,
			CreatedAt:   time.Now(),
		},
	}, nil
}

func (s *mockStrategy) EstimateEffort(analysis *TaskAnalysis) time.Duration {
	return 10 * time.Minute
}

func (s *mockStrategy) Confidence() float64 {
	return 0.8
}

// mockTaskRunner is a TaskRunner that always succeeds for testing.
type mockTaskRunner struct{}

func (m *mockTaskRunner) Run(ctx context.Context, task *Task) (*TaskResult, error) {
	return &TaskResult{
		Success:      true,
		Output:       "Task completed successfully",
		Error:        nil,
		Attempts:     1,
		Duration:     10 * time.Millisecond,
		ToolsUsed:    task.Tools,
		FilesChanged: []string{},
		CompletedAt:  time.Now(),
	}, nil
}

func TestContainsAny(t *testing.T) {
	tests := []struct {
		s       string
		substrs []string
		want    bool
	}{
		{"hello world", []string{"hello"}, true},
		{"hello world", []string{"goodbye"}, false},
		{"hello world", []string{"hello", "goodbye"}, true},
		{"test", []string{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			got := containsAny(tt.s, tt.substrs)
			if got != tt.want {
				t.Errorf("containsAny(%q, %v) = %v, want %v", tt.s, tt.substrs, got, tt.want)
			}
		})
	}
}

func TestUpgradeComplexity(t *testing.T) {
	tests := []struct {
		input Complexity
		want  Complexity
	}{
		{ComplexityTrivial, ComplexitySimple},
		{ComplexitySimple, ComplexityModerate},
		{ComplexityModerate, ComplexityComplex},
		{ComplexityComplex, ComplexityMajor},
		{ComplexityMajor, ComplexityMajor},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			got := upgradeComplexity(tt.input)
			if got != tt.want {
				t.Errorf("upgradeComplexity(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestDowngradeComplexity(t *testing.T) {
	tests := []struct {
		input Complexity
		want  Complexity
	}{
		{ComplexityMajor, ComplexityComplex},
		{ComplexityComplex, ComplexityModerate},
		{ComplexityModerate, ComplexitySimple},
		{ComplexitySimple, ComplexityTrivial},
		{ComplexityTrivial, ComplexityTrivial},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			got := downgradeComplexity(tt.input)
			if got != tt.want {
				t.Errorf("downgradeComplexity(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
