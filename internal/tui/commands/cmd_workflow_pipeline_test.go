package commands

import (
	"strings"
	"testing"
)

// /workflow fronts the pipeline engine: list shows specs, run resolves a
// spec by name and starts it, resume matches a run from history.

func TestWorkflowListShowsSpecs(t *testing.T) {
	m := NewMockHost()
	m.workflowSpecs = []WorkflowSpecInfo{
		{Name: "check", Description: "build then test", Path: "/x/check.yaml"},
	}
	handleWorkflow(m, []string{"list"})
	if !strings.Contains(m.systemMessages[0], "check") || !strings.Contains(m.systemMessages[0], "build then test") {
		t.Fatalf("list should render specs: %v", m.systemMessages)
	}
}

func TestWorkflowListEmptyExplainsSetup(t *testing.T) {
	m := NewMockHost()
	handleWorkflow(m, nil)
	if !strings.Contains(m.systemMessages[0], ".automergent/workflows/") {
		t.Fatalf("empty list should point at the specs dir: %v", m.systemMessages)
	}
}

func TestWorkflowRunResolvesByName(t *testing.T) {
	m := NewMockHost()
	m.workflowSpecs = []WorkflowSpecInfo{
		{Name: "check", Description: "", Path: "/x/check.yaml"},
	}
	handleWorkflow(m, []string{"run", "check", "extra", "args"})
	if len(m.runWorkflowCalls) != 1 {
		t.Fatalf("expected one run call, got %d", len(m.runWorkflowCalls))
	}
	if m.runWorkflowCalls[0].path != "/x/check.yaml" {
		t.Fatalf("wrong spec path: %+v", m.runWorkflowCalls[0])
	}
	if m.runWorkflowCalls[0].args != "extra args" {
		t.Fatalf("args not forwarded: %+v", m.runWorkflowCalls[0])
	}
	if m.runWorkflowCalls[0].resume {
		t.Fatal("run must not resume")
	}
}

func TestWorkflowRunUnknownNameErrors(t *testing.T) {
	m := NewMockHost()
	m.workflowSpecs = []WorkflowSpecInfo{{Name: "check", Path: "/x/check.yaml"}}
	handleWorkflow(m, []string{"run", "nope"})
	if len(m.runWorkflowCalls) != 0 {
		t.Fatal("unknown workflow must not start a run")
	}
	if len(m.errorMessages) == 0 {
		t.Fatal("unknown workflow should report an error")
	}
}

func TestWorkflowResumeMatchesHistory(t *testing.T) {
	m := NewMockHost()
	m.workflowRuns = []WorkflowRunInfo{{RunID: "check-123", Name: "check", Status: "failed", Steps: 3}}
	handleWorkflow(m, []string{"resume", "check-123"})
	if len(m.runWorkflowCalls) != 1 || !m.runWorkflowCalls[0].resume {
		t.Fatalf("resume call missing: %+v", m.runWorkflowCalls)
	}
}

func TestWorkflowResumeUnknownRunErrors(t *testing.T) {
	m := NewMockHost()
	handleWorkflow(m, []string{"resume", "ghost-1"})
	if len(m.runWorkflowCalls) != 0 || len(m.errorMessages) == 0 {
		t.Fatalf("unknown run should error without starting: %+v %v", m.runWorkflowCalls, m.errorMessages)
	}
}

func TestWorkflowHistoryRenders(t *testing.T) {
	m := NewMockHost()
	m.workflowRuns = []WorkflowRunInfo{
		{RunID: "check-1", Name: "check", Status: "completed", Steps: 3},
		{RunID: "check-2", Name: "check", Status: "failed", Steps: 2, Detail: "step b failed"},
	}
	handleWorkflow(m, []string{"history"})
	msg := m.systemMessages[0]
	if !strings.Contains(msg, "check-1") || !strings.Contains(msg, "completed") {
		t.Fatalf("history should list runs: %q", msg)
	}
	if !strings.Contains(msg, "step b failed") {
		t.Fatalf("history should show failure detail: %q", msg)
	}
}

func TestWorkflowCompletionOffersSpecNames(t *testing.T) {
	m := NewMockHost()
	m.workflowSpecs = []WorkflowSpecInfo{
		{Name: "check", Path: "/x/check.yaml"},
		{Name: "audit", Path: "/x/audit.yaml"},
	}
	cmd := workflowCommand()
	got := cmd.Completion(m, "ch")
	if len(got) != 1 || got[0] != "check" {
		t.Fatalf("completion = %v, want [check]", got)
	}
}
