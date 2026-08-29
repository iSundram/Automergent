package agent

import (
	"testing"

	"github.com/iSundram/Automergent/internal/agent/agentdef"
	"github.com/iSundram/Automergent/internal/ai"
)

func TestGrantsSharedAcrossAgentTree(t *testing.T) {
	var persisted []string
	parent := newGrants(func(scope string) { persisted = append(persisted, scope) })

	// A child shares the parent's grant object (see Execute).
	child := parent

	if !child.Add("name=\"bash\";cmd=\"git status\"") {
		t.Fatal("first Add must report new")
	}
	if child.Add("name=\"bash\";cmd=\"git status\"") {
		t.Fatal("re-adding an existing scope must not report new")
	}
	if !parent.Has("name=\"bash\";cmd=\"git status\"") {
		t.Fatal("grant made via the child must be visible to the parent")
	}
	if len(persisted) != 1 {
		t.Fatalf("persist called %d times, want 1 (re-adds must not persist)", len(persisted))
	}

	parent.Delete("name=\"bash\";cmd=\"git status\"")
	if child.Has("name=\"bash\";cmd=\"git status\"") {
		t.Fatal("revocation on the parent must hide the scope from the child")
	}
}

func TestGrantsResetReseedsFromSession(t *testing.T) {
	g := newGrants(nil)
	g.Reset([]string{"a", "b"})
	if !g.Has("a") || !g.Has("b") || g.Len() != 2 {
		t.Fatalf("Reset must replace the set exactly, len=%d", g.Len())
	}
	g.Reset(nil)
	if g.Len() != 0 {
		t.Fatal("Reset(nil) must clear")
	}
}

func TestApplyDefinitionMask(t *testing.T) {
	schemas := []ai.ToolSchema{
		{Name: "read_file"}, {Name: "grep"}, {Name: "write_file"}, {Name: "bash"},
	}

	// An empty Tools list (general-purpose) means the full registry.
	if got := applyDefinitionMask(schemas, &agentdef.AgentDefinition{}); len(got) != len(schemas) {
		t.Fatalf("empty Tools must keep all schemas, got %d", len(got))
	}
	if got := applyDefinitionMask(schemas, nil); len(got) != len(schemas) {
		t.Fatalf("nil definition must keep all schemas, got %d", len(got))
	}

	// explore-style read-only definition: write and shell must be gone.
	def := &agentdef.AgentDefinition{Tools: []string{"read_file", "grep"}}
	got := applyDefinitionMask(schemas, def)
	if len(got) != 2 {
		t.Fatalf("read-only definition must keep 2 schemas, got %d", len(got))
	}
	for _, s := range got {
		if s.Name == "write_file" || s.Name == "bash" {
			t.Fatalf("write/bash leaked through the definition mask: %s", s.Name)
		}
	}
}
