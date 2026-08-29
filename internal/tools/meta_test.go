package tools

import (
	"context"
	"strings"
	"testing"
)

func TestModelFamilyGeminiTargeting(t *testing.T) {
	cases := []struct {
		model string
		want  string
	}{
		{"gemini-3-pro", "gemini3"},
		{"gemini-3-flash", "gemini3"},
		{"Gemini 3 Pro", "gemini3"},
		{"gemini_3_ultra", "gemini3"},
		{"gemini-2.5-pro", "gemini"},
		{"gemini-1.5-flash", "gemini"},
		{"", "default"},
		{"some-future-model", "default"},
	}
	for _, tc := range cases {
		if got := ModelFamily(tc.model); got != tc.want {
			t.Errorf("ModelFamily(%q) = %q, want %q", tc.model, got, tc.want)
		}
	}
}

func TestIsGemini3Boundaries(t *testing.T) {
	yes := []string{"gemini-3-pro", "gemini-3.5-flash", "GEMINI-3", "my-gemini3-experimental"}
	no := []string{"", "gemini-2.5-pro", "gpt-4"}
	for _, m := range yes {
		if !IsGemini3(m) {
			t.Errorf("IsGemini3(%q) = false, want true", m)
		}
	}
	for _, m := range no {
		if IsGemini3(m) {
			t.Errorf("IsGemini3(%q) = true, want false", m)
		}
	}
}

// namedStub is the minimal full Tool implementation for registry-free tests.
type namedStub struct {
	name string
	desc string
	BaseTool
}

func (s *namedStub) Name() string        { return s.name }
func (s *namedStub) Description() string { return s.desc }
func (s *namedStub) Schema() map[string]any {
	return nil
}
func (s *namedStub) Execute(ctx context.Context, args map[string]any) (Result, error) {
	return Result{}, nil
}
func (s *namedStub) RequiresConfirmation(mode string) bool { return false }

func TestInferCategory(t *testing.T) {
	cases := map[string]string{
		"read_file":       "read",
		"list_directory":  "read",
		"grep":            "search",
		"glob":            "search",
		"edit_file":       "edit",
		"write_file":      "edit",
		"multi_edit":      "edit",
		"git_commit":      "git",
		"bash":            "shell",
		"stop_shell":      "shell",
		"web_fetch":       "web",
		"lsp_diagnostics": "lsp",
		"sql":             "db",
		"secrets_scan":    "security",
		"task":            "agents",
		"todo_write":      "memory",
		"mystery_tool":    "general",
	}
	for name, want := range cases {
		if got := InferCategory(name); got != want {
			t.Errorf("InferCategory(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestMetaOfFillsDefaults(t *testing.T) {
	m := &ToolMeta{WhenToUse: "x"}
	fillMetaDefaults(m, &namedStub{name: "todo_write", desc: "Write todos."})
	if m.Category != "memory" || m.DisplayName != "Todo Write" {
		t.Fatalf("unexpected defaults: %+v", m)
	}
	if m.Usage == "" || !strings.Contains(m.Usage, "todos") {
		t.Fatalf("expected usage fallback to description, got %q", m.Usage)
	}
}

func TestSortToolsForPromptDeterministic(t *testing.T) {
	a := namedStub{name: "write_file", desc: ""}
	b := namedStub{name: "read_file", desc: ""}
	c := namedStub{name: "bash", desc: ""}
	first := SortToolsForPrompt([]Tool{&a, &b, &c})
	for i := 0; i < 20; i++ {
		next := SortToolsForPrompt([]Tool{&c, &b, &a})
		for j := range next {
			if next[j].Name() != first[j].Name() {
				t.Fatalf("unstable ordering at %d: %s vs %s", j, next[j].Name(), first[j].Name())
			}
		}
	}
	if first[0].Name() != "read_file" || first[1].Name() != "write_file" || first[2].Name() != "bash" {
		t.Fatalf("category order wrong: %s, %s, %s", first[0].Name(), first[1].Name(), first[2].Name())
	}
}

func TestAliasForGeminiOnly(t *testing.T) {
	m := &ToolMeta{Aliases: map[string]string{"gemini3": "read_file"}}
	if got := m.AliasFor("gemini3"); got != "read_file" {
		t.Errorf("AliasFor(gemini3) = %q", got)
	}
	if got := m.AliasFor("default"); got != "" {
		t.Errorf("expected empty alias for default family, got %q", got)
	}
}
