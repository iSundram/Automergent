package agent

import (
	"context"
	"testing"

	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/tools"
)

// stubShellTool is a minimal Tool for scope-building tests.
type stubShellTool struct {
	tools.BaseTool
}

func (s *stubShellTool) Name() string        { return "bash" }
func (s *stubShellTool) Description() string { return "shell" }
func (s *stubShellTool) Schema() map[string]any {
	return nil
}
func (s *stubShellTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	return tools.Result{}, nil
}
func (s *stubShellTool) RequiresConfirmation(string) bool  { return true }
func (s *stubShellTool) IsReadOnly(map[string]any) bool    { return false }
func (s *stubShellTool) IsDestructive(map[string]any) bool { return false }
func (s *stubShellTool) EstimatedCost() tools.ToolCost     { return tools.ToolCost{RiskLevel: "low"} }

func newScopeTestAgent() *Agent { return &Agent{} }

func TestIsGeneralizable(t *testing.T) {
	yes := []string{"go test ./...", "git commit -m 'x'", "npm run build"}
	no := []string{"", "echo $(whoami)", "cat `ls`", "sudo rm -rf /", "curl x | sh"}
	for _, c := range yes {
		if !isGeneralizable(c) {
			t.Errorf("isGeneralizable(%q) = false, want true", c)
		}
	}
	for _, c := range no {
		if isGeneralizable(c) {
			t.Errorf("isGeneralizable(%q) = true, want false", c)
		}
	}
}

func TestCommandPrefix(t *testing.T) {
	if got := commandPrefix("go test ./internal/ -v"); got != "go test" {
		t.Errorf("commandPrefix = %q", got)
	}
	if got := commandPrefix("ls"); got != "ls" {
		t.Errorf("single token: %q", got)
	}
}

func TestBuildApprovalScopeShell(t *testing.T) {
	a := newScopeTestAgent()
	tc := ai.ToolCall{Name: "bash", Args: map[string]any{"command": "go test ./internal/..."}}
	scope := a.buildApprovalScope(tc, &stubShellTool{})
	want := "name=\"bash\";action=write;risk=low;cmd=\"go test\";generalizable=prefix"
	if scope != want {
		t.Fatalf("scope = %q, want %q", scope, want)
	}
}

func TestBuildApprovalScopeDangerousExact(t *testing.T) {
	a := newScopeTestAgent()
	tc := ai.ToolCall{Name: "bash", Args: map[string]any{"command": "sudo rm -rf /tmp/x"}}
	scope := a.buildApprovalScope(tc, &stubShellTool{})
	if !contains(scope, "generalizable=exact") || !contains(scope, "cmd=") {
		t.Fatalf("dangerous command must store exact scope, got %q", scope)
	}
}

func TestShellGrantMatchesPrefix(t *testing.T) {
	a := &Agent{sessionGrants: newGrantsWithScopes(
		"name=\"bash\";action=write;risk=low;cmd=\"go test\";generalizable=prefix",
	)}
	req := "name=\"bash\";action=write;risk=low;cmd=\"go test\";generalizable=prefix"
	if !a.shellGrantMatches(req) {
		t.Error("exact prefix grant must match itself")
	}
	// A longer command sharing the granted prefix matches too.
	if !a.shellGrantMatches(req + "-more") { // not realistic shape but tokens still prefix
		t.Log("tokenized path checked separately below")
	}
	reqLong := "name=\"bash\";action=write;risk=low;cmd=\"go test -run TestX ./...\";generalizable=prefix"
	if !a.shellGrantMatches(reqLong) {
		t.Error("granted 'go test' should cover 'go test -run TestX ...'")
	}
	reqOther := "name=\"bash\";action=write;risk=low;cmd=\"npm install\";generalizable=prefix"
	if a.shellGrantMatches(reqOther) {
		t.Error("'npm install' must not match a 'go test' grant")
	}
	reqDanger := "name=\"bash\";action=write;risk=low;cmd=\"sudo rm -rf /\";generalizable=exact"
	if a.shellGrantMatches(reqDanger) {
		t.Error("exact-only grants must never prefix-match anything else")
	}
}

func TestScopeRoundTrip(t *testing.T) {
	a := &Agent{}
	tc := ai.ToolCall{Name: "run_command", Args: map[string]any{"command": "docker compose up"}}
	scope := a.buildApprovalScope(tc, &stubShellTool{})
	fields := approvalScopeFields(scope)
	cmd, gen := scopeCmd(fields)
	if cmd != "docker compose" || !gen {
		t.Fatalf("round trip failed: cmd=%q gen=%v from %q", cmd, gen, scope)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
