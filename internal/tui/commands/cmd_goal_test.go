package commands

import (
	"strings"
	"testing"
)

// /goal now drives a real autonomy loop: the handler installs state via
// SetGoal, lifecycle actions go through GoalAction, and the old
// "StartAgent with a 'set the thread goal' prompt" behavior is gone.

func TestGoalSetInstallsObjective(t *testing.T) {
	m := NewMockHost()
	handleGoal(m, []string{"make", "all", "tests", "pass"})

	if len(m.setGoalCalls) != 1 {
		t.Fatalf("expected one SetGoal call, got %d", len(m.setGoalCalls))
	}
	if m.setGoalCalls[0].objective != "make all tests pass" {
		t.Fatalf("objective not joined: %q", m.setGoalCalls[0].objective)
	}
	if m.setGoalCalls[0].tokenBudget != 0 {
		t.Fatalf("no budget given, got %d", m.setGoalCalls[0].tokenBudget)
	}
	if len(m.startAgentCalls) != 0 {
		t.Fatalf("setting a goal must not start an agent run: %v", m.startAgentCalls)
	}
	if !strings.Contains(m.systemMessages[0], "Goal set") {
		t.Fatalf("missing confirmation: %v", m.systemMessages)
	}
}

func TestGoalSetWithBudget(t *testing.T) {
	m := NewMockHost()
	handleGoal(m, []string{"budget", "50000", "refactor", "the", "parser"})

	if len(m.setGoalCalls) != 1 {
		t.Fatalf("expected one SetGoal call, got %d", len(m.setGoalCalls))
	}
	if m.setGoalCalls[0].tokenBudget != 50000 {
		t.Fatalf("budget = %d, want 50000", m.setGoalCalls[0].tokenBudget)
	}
	if m.setGoalCalls[0].objective != "refactor the parser" {
		t.Fatalf("objective = %q", m.setGoalCalls[0].objective)
	}
}

func TestGoalLifecycleActions(t *testing.T) {
	m := NewMockHost()
	handleGoal(m, []string{"ship it"})

	for _, action := range []string{"pause", "resume", "continue", "clear"} {
		handleGoal(m, []string{action})
		if len(m.goalActions) == 0 || m.goalActions[len(m.goalActions)-1] != action {
			t.Fatalf("action %q not routed to GoalAction: %v", action, m.goalActions)
		}
	}
}

func TestGoalStatusShowsSnapshot(t *testing.T) {
	m := NewMockHost()
	handleGoal(m, []string{"write the docs"})
	handleGoal(m, []string{"status"})
	if !strings.Contains(m.systemMessages[len(m.systemMessages)-1], "write the docs") {
		t.Fatalf("status should render the objective: %v", m.systemMessages)
	}
}
