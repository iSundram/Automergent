package coordinator

import (
	"testing"
	"time"

	"github.com/iSundram/Automergent/internal/reasoning"
)

func TestMapTaskTypeToRole(t *testing.T) {
	tests := []struct {
		input    reasoning.TaskType
		expected AgentRole
	}{
		{reasoning.TaskTypeInvestigation, RoleResearcher},
		{reasoning.TaskTypeFeature, RoleCoder},
		{reasoning.TaskTypeBugFix, RoleCoder},
		{reasoning.TaskTypeRefactor, RoleCoder},
		{reasoning.TaskTypeMultiFile, RoleCoder},
		{reasoning.TaskTypeTest, RoleTester},
		{reasoning.TaskTypeDocumentation, RoleDocumenter},
		{reasoning.TaskTypeBuild, RoleReviewer},
		{reasoning.TaskTypeDeployment, RoleReviewer},
		{reasoning.TaskType("unknown"), RoleCoder}, // default
	}

	for _, tt := range tests {
		got := mapTaskTypeToRole(tt.input)
		if got != tt.expected {
			t.Errorf("mapTaskTypeToRole(%s) = %s, want %s", tt.input, got, tt.expected)
		}
	}
}

func TestMapPriority(t *testing.T) {
	tests := []struct {
		input    int
		expected TaskPriority
	}{
		{100, PriorityCritical},
		{95, PriorityHigh},
		{90, PriorityHigh},
		{50, PriorityNormal},
		{10, PriorityLow},
		{0, PriorityLow},
	}

	for _, tt := range tests {
		got := mapPriority(tt.input)
		if got != tt.expected {
			t.Errorf("mapPriority(%d) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

func TestMapPhaseName(t *testing.T) {
	tests := []struct {
		index    int
		total    int
		expected string
	}{
		{0, 3, "research"},
		{1, 3, "plan"},
		{2, 3, "execute"},
		{0, 1, "execute"},
		{0, 2, "research"},
		{1, 2, "execute"},
	}

	for _, tt := range tests {
		got := mapPhaseName(tt.index, tt.total)
		if got != tt.expected {
			t.Errorf("mapPhaseName(%d, %d) = %s, want %s", tt.index, tt.total, got, tt.expected)
		}
	}
}

func TestFromReasoningPlan_NilPlan(t *testing.T) {
	_, err := FromReasoningPlan(nil, nil)
	if err == nil {
		t.Error("expected error for nil plan")
	}
}

func TestFromReasoningPlan_BasicConversion(t *testing.T) {
	now := time.Now()
	rp := &reasoning.ExecutionPlan{
		ID: "plan-1",
		Tasks: []*reasoning.Task{
			{
				ID:           "task-1",
				Type:         reasoning.TaskTypeInvestigation,
				Description:  "explore codebase",
				Priority:     80,
				Dependencies: []string{},
				CreatedAt:    now,
				Estimated:    30 * time.Second,
			},
			{
				ID:           "task-2",
				Type:         reasoning.TaskTypeFeature,
				Description:  "implement feature",
				Priority:     90,
				Dependencies: []string{"task-1"},
				CreatedAt:    now,
				Estimated:    60 * time.Second,
			},
		},
		ExecutionOrder: [][]string{
			{"task-1"},
			{"task-2"},
		},
		Analysis: &reasoning.TaskAnalysis{
			EstimatedTime: 90 * time.Second,
		},
		CreatedAt: now,
	}

	plan, err := FromReasoningPlan(nil, rp)
	if err != nil {
		t.Fatalf("FromReasoningPlan failed: %v", err)
	}

	if plan.ID != "plan-1" {
		t.Errorf("expected plan ID plan-1, got %s", plan.ID)
	}

	if len(plan.Tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(plan.Tasks))
	}

	// Task 1 should be researcher role.
	if plan.Tasks[0].Role != RoleResearcher {
		t.Errorf("expected task 1 role researcher, got %s", plan.Tasks[0].Role)
	}

	// Task 2 should be coder role.
	if plan.Tasks[1].Role != RoleCoder {
		t.Errorf("expected task 2 role coder, got %s", plan.Tasks[1].Role)
	}

	// Task 2 should depend on task 1.
	if len(plan.Tasks[1].Dependencies) != 1 || plan.Tasks[1].Dependencies[0] != "task-1" {
		t.Errorf("expected task 2 to depend on task-1, got %v", plan.Tasks[1].Dependencies)
	}

	// Should have 2 phases.
	if len(plan.Phases) != 2 {
		t.Errorf("expected 2 phases, got %d", len(plan.Phases))
	}
}

func TestCreateDefaultPhases(t *testing.T) {
	tasks := []*Task{
		{ID: "r1", Role: RoleResearcher},
		{ID: "c1", Role: RoleCoder},
		{ID: "c2", Role: RoleCoder},
		{ID: "t1", Role: RoleTester},
	}

	phases := createDefaultPhases(tasks, nil)

	// Should have research, plan (coders), and execute phases.
	if len(phases) < 2 {
		t.Fatalf("expected at least 2 phases, got %d", len(phases))
	}

	// First phase should be research.
	if phases[0].Name != "research" {
		t.Errorf("expected first phase to be research, got %s", phases[0].Name)
	}
}

func TestBuildDependencies(t *testing.T) {
	tasks := []*Task{
		{ID: "a", Dependencies: []string{}},
		{ID: "b", Dependencies: []string{"a"}},
		{ID: "c", Dependencies: []string{"a", "b"}},
	}

	deps := buildDependencies(tasks)

	if len(deps["a"]) != 0 {
		t.Errorf("expected no deps for a, got %v", deps["a"])
	}
	if len(deps["b"]) != 1 || deps["b"][0] != "a" {
		t.Errorf("expected b to depend on [a], got %v", deps["b"])
	}
	if len(deps["c"]) != 2 {
		t.Errorf("expected c to have 2 deps, got %d", len(deps["c"]))
	}
}
