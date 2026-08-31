package commands

import (
	"strings"
	"testing"
)

// /plan is a thin launcher for the plan machinery: the injected prompt must
// drive enter_plan_mode → plan artifact → exit_plan_mode, not a vague
// "confirm before editing" suggestion.

func TestPlanTemplateDrivesRealPlanFlow(t *testing.T) {
	cmd := planCommand()
	prompt := cmd.ExpandPrompt([]string{"refactor the parser"})

	for _, want := range []string{
		"enter_plan_mode",
		"exit_plan_mode",
		".automergent/artifacts/plan.md",
		"refactor the parser",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("plan prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestPlanHandlerFallbackMatchesTemplate(t *testing.T) {
	m := NewMockHost()
	handlePlan(m, []string{"fix the flaky test"})
	if len(m.startAgentCalls) != 1 {
		t.Fatalf("fallback handler must start the agent: %v", m.startAgentCalls)
	}
	prompt := m.startAgentCalls[0]
	for _, want := range []string{"enter_plan_mode", "exit_plan_mode", "fix the flaky test"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("fallback prompt missing %q: %s", want, prompt)
		}
	}
}

func TestPlanHasNoStubSubcommands(t *testing.T) {
	cmd := planCommand()
	if len(cmd.SubCommands) != 0 {
		t.Fatalf("plan should carry no sub-commands (the copy stub is gone): %v", cmd.SubCommands)
	}
	if cmd.SubPalette != "" {
		t.Fatal("plan needs no sub-palette — it takes a free-form task argument")
	}
}
