package agent

import (
	"strings"
	"testing"

	promptpkg "github.com/iSundram/Automergent/internal/prompt"
	"github.com/iSundram/Automergent/internal/shared"
)

func TestRuleStoreAddAndList(t *testing.T) {
	store := NewRuleStore(t.TempDir())
	if _, err := store.Add("never use tabs"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := store.Add("always write tests first"); err != nil {
		t.Fatalf("add: %v", err)
	}

	rules := store.List()
	if len(rules) != 2 || rules[0] != "never use tabs" {
		t.Fatalf("rules = %v", rules)
	}

	// Idempotent on exact duplicates.
	if _, err := store.Add("Never Use Tabs"); err != nil {
		t.Fatalf("duplicate add: %v", err)
	}
	if got := len(store.List()); got != 2 {
		t.Fatalf("duplicate rule recorded twice: %v", store.List())
	}
}

func TestRuleStoreRemove(t *testing.T) {
	store := NewRuleStore(t.TempDir())
	_, _ = store.Add("never use tabs")
	_, _ = store.Add("always write tests first")

	removed, ok := store.Remove("tabs")
	if !ok || removed != "never use tabs" {
		t.Fatalf("remove = %q, %v", removed, ok)
	}
	rules := store.List()
	if len(rules) != 1 || rules[0] != "always write tests first" {
		t.Fatalf("remaining rules = %v", rules)
	}

	if _, ok := store.Remove("nonexistent"); ok {
		t.Fatal("removing a nonexistent rule must report false")
	}
}

func TestFleetListsRegistryAgents(t *testing.T) {
	Init() // idempotent; populates the global registry with builtins
	out := FleetFromRegistry()
	for _, want := range []string{"## Available Agents", "main", "explore", "general-purpose"} {
		if !strings.Contains(out, want) {
			t.Errorf("fleet listing missing %q", want)
		}
	}
	if !strings.Contains(out, "task tool") {
		t.Error("fleet listing must teach delegation via the task tool")
	}
}

func TestCurrentTaskBlockRouting(t *testing.T) {
	spec := shared.TaskSpec{
		ID:          "p1",
		Description: "see how X is working",
		Agent:       "explore",
		Context: map[string]any{
			"reason":      "needs codebase reading",
			"constraints": []string{"user suggested approach Z"},
		},
	}
	block := currentTaskBlock(spec)
	for _, want := range []string{"## Current Task", "see how X is working", "`explore`", "approach Z", "codebase reading"} {
		if !strings.Contains(block, want) {
			t.Errorf("task block missing %q", want)
		}
	}

	// main-routed tasks carry no delegation hint.
	inline := currentTaskBlock(shared.TaskSpec{Description: "small fix", Agent: "main"})
	if strings.Contains(inline, "Routing:") {
		t.Error("main-routed task must not carry a delegation hint")
	}

	if currentTaskBlock(shared.TaskSpec{}) != "" {
		t.Error("empty task spec must render no block")
	}
}

func TestNoiseSummary(t *testing.T) {
	d := &promptpkg.Decomposition{
		Parts: []promptpkg.DecomposedPart{
			{Kind: promptpkg.PartKindNoise, Text: "I like coffee"},
			{Kind: promptpkg.PartKindNoise, Text: "I don't sleep"},
			{Kind: promptpkg.PartKindTask, Text: "see files"},
		},
	}
	lines := noiseSummary(d)
	if len(lines) != 2 || !strings.Contains(lines[0], "I like coffee") {
		t.Fatalf("noise summary = %v", lines)
	}
}

func TestDirectPartIsActionable(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		// Tool-needing imperatives must run as build tasks, not the
		// tool-less direct path (the "File creation tools are not
		// available" regression).
		{"create file golang.go with an syntax error,", true},
		{"write the code to golang.go", true},
		{"make the plan", true},
		{"implement a retry loop in client.go", true},
		// Questions and conversational parts stay direct.
		{"hey who are you", false},
		{"how do I write tests?", false},
		{"what should I add there more, tell me a comprehensive plan", false},
		{"can you create a file?", false},
		{"tell me what Google does", false},
	}
	for _, tc := range cases {
		if got := directPartIsActionable(tc.text); got != tc.want {
			t.Errorf("directPartIsActionable(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}
