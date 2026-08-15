package coordinator

import (
	"strings"
	"testing"
)

func TestBuildRolePrompt_Researcher(t *testing.T) {
	task := &Task{
		Prompt: "explore the auth module",
		Context: TaskContext{
			WorkingDir: "/src",
			Files:      []string{"auth.go", "auth_test.go"},
		},
	}

	prompt := BuildRolePrompt(RoleResearcher, task)

	if !strings.Contains(prompt, "Researcher") {
		t.Error("expected researcher prompt to mention Researcher")
	}
	if !strings.Contains(prompt, "explore the auth module") {
		t.Error("expected prompt to contain task description")
	}
	if !strings.Contains(prompt, "auth.go") {
		t.Error("expected prompt to contain file paths")
	}
	if !strings.Contains(prompt, "/src") {
		t.Error("expected prompt to contain working directory")
	}
}

func TestBuildRolePrompt_Coder(t *testing.T) {
	task := &Task{
		Prompt: "implement login",
		Context: TaskContext{
			CodeSnippets: map[string]string{"auth.go": "func Login() {}"},
			Constraints:  []string{"must be thread-safe"},
		},
	}

	prompt := BuildRolePrompt(RoleCoder, task)

	if !strings.Contains(prompt, "Coder") {
		t.Error("expected coder prompt to mention Coder")
	}
	if !strings.Contains(prompt, "func Login() {}") {
		t.Error("expected prompt to contain code snippets")
	}
	if !strings.Contains(prompt, "must be thread-safe") {
		t.Error("expected prompt to contain constraints")
	}
}

func TestBuildRolePrompt_Reviewer(t *testing.T) {
	task := &Task{
		Prompt: "review the PR",
		Context: TaskContext{
			CodeSnippets: map[string]string{"main.go": "package main"},
		},
	}

	prompt := BuildRolePrompt(RoleReviewer, task)

	if !strings.Contains(prompt, "Reviewer") {
		t.Error("expected reviewer prompt to mention Reviewer")
	}
	if !strings.Contains(prompt, "package main") {
		t.Error("expected prompt to contain code")
	}
}

func TestBuildRolePrompt_Tester(t *testing.T) {
	task := &Task{
		Prompt: "write tests for auth",
		Context: TaskContext{
			CodeSnippets: map[string]string{"auth.go": "func Login() {}"},
		},
	}

	prompt := BuildRolePrompt(RoleTester, task)

	if !strings.Contains(prompt, "Tester") {
		t.Error("expected tester prompt to mention Tester")
	}
}

func TestBuildRolePrompt_Documenter(t *testing.T) {
	task := &Task{
		Prompt: "write API docs",
		Context: TaskContext{
			CodeSnippets: map[string]string{"api.go": "func Handle() {}"},
		},
	}

	prompt := BuildRolePrompt(RoleDocumenter, task)

	if !strings.Contains(prompt, "Documenter") {
		t.Error("expected documenter prompt to mention Documenter")
	}
}

func TestBuildRolePrompt_Generic(t *testing.T) {
	task := &Task{
		Role:   AgentRole("unknown"),
		Prompt: "do something",
	}

	prompt := BuildRolePrompt(AgentRole("unknown"), task)

	if !strings.Contains(prompt, "unknown") {
		t.Error("expected generic prompt to contain role name")
	}
	if !strings.Contains(prompt, "do something") {
		t.Error("expected prompt to contain task description")
	}
}

func TestBuildRolePrompt_EmptyContext(t *testing.T) {
	task := &Task{
		Prompt: "simple task",
	}

	// Should not panic with empty context.
	for _, role := range AllRoles() {
		prompt := BuildRolePrompt(role, task)
		if prompt == "" {
			t.Errorf("expected non-empty prompt for role %s", role)
		}
	}
}
