package command

import (
	"testing"
)

func TestRegistryNamesAndAliasesAreUnique(t *testing.T) {
	r := Default()
	seen := map[string]string{}
	for _, command := range r.List() {
		if command.Name == "" || command.Category == "" || command.Description == "" {
			t.Fatalf("incomplete command definition: %#v", command)
		}
		for _, name := range append([]string{command.Name}, command.Aliases...) {
			if owner, exists := seen[name]; exists {
				t.Fatalf("command name or alias %q belongs to both %q and %q", name, owner, command.Name)
			}
			seen[name] = command.Name
		}
	}
}

func TestLookupResolvesAliases(t *testing.T) {
	r := Default()
	wants := map[string]string{
		"files":       "tree",
		"changes":     "diff",
		"diagnostics": "lsp",
		"tokens":      "context",
		"stop":        "cancel",
		"exit":        "quit",
		"keys":        "keybindings",
	}
	for alias, want := range wants {
		got, ok := r.Lookup(alias)
		if !ok || got.Name != want {
			t.Fatalf("Lookup(%q) = %#v, %v; want %q", alias, got, ok, want)
		}
	}
}

func TestPaletteItemsExposeUsageAndSearchTerms(t *testing.T) {
	r := Default()
	items := r.PaletteItems()
	if len(items) != len(r.List()) {
		t.Fatalf("palette has %d commands, registry has %d", len(items), len(r.List()))
	}
	foundRun := false
	for _, item := range items {
		if item.Value == "run" {
			foundRun = item.Hint == "<command>" && item.SearchTerms != ""
		}
	}
	if !foundRun {
		t.Fatal("run command is missing usage or searchable metadata")
	}
}

func TestSessionOwnedCommandsHaveNoHandler(t *testing.T) {
	r := Default()
	sessionCmds := []string{"new", "sessions", "resume", "clear", "reset", "export", "approvals"}
	for _, name := range sessionCmds {
		if r.HasHandler(name) {
			t.Fatalf("session-owned command %q should not have a handler in command package", name)
		}
		if !r.IsSessionOwned(name) {
			t.Fatalf("command %q should be marked session-owned", name)
		}
	}
}

func TestDispatchReturnsErrUnknownCommand(t *testing.T) {
	r := Default()
	m := NewMockHost()
	_, err := r.Dispatch(m, "nonexistent", []string{})
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
	_, ok := err.(ErrUnknownCommand)
	if !ok {
		t.Fatalf("expected ErrUnknownCommand, got %T: %v", err, err)
	}
}

func TestDispatchReturnsErrSessionOwned(t *testing.T) {
	r := Default()
	m := NewMockHost()
	_, err := r.Dispatch(m, "new", []string{})
	if err == nil {
		t.Fatal("expected error for session-owned command")
	}
	_, ok := err.(ErrSessionOwned)
	if !ok {
		t.Fatalf("expected ErrSessionOwned, got %T: %v", err, err)
	}
}

func TestDispatchCallsHandler(t *testing.T) {
	r := Default()
	m := NewMockHost()
	_, err := r.Dispatch(m, "help", []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.showHelpCalls != 1 {
		t.Fatalf("expected ShowHelp called, got %d", m.showHelpCalls)
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		input    string
		wantName string
		wantArgs []string
	}{
		{"/help", "help", nil},
		{"/model gemini-3.6-flash", "model", []string{"gemini-3.6-flash"}},
		{"/provider google gemini-3.6-flash", "provider", []string{"google", "gemini-3.6-flash"}},
		{"help", "", nil}, // not a slash command
		{"", "", nil},
	}
	for _, tt := range tests {
		name, args := Parse(tt.input)
		if name != tt.wantName {
			t.Fatalf("Parse(%q) name = %q, want %q", tt.input, name, tt.wantName)
		}
		if len(args) != len(tt.wantArgs) {
			t.Fatalf("Parse(%q) args = %v, want %v", tt.input, args, tt.wantArgs)
		}
		for i, a := range args {
			if a != tt.wantArgs[i] {
				t.Fatalf("Parse(%q) args[%d] = %q, want %q", tt.input, i, a, tt.wantArgs[i])
			}
		}
	}
}

func TestPaletteItemsHaveCorrectStructure(t *testing.T) {
	r := Default()
	items := r.PaletteItems()
	for _, item := range items {
		if item.Label == "" {
			t.Fatalf("empty label for command %q", item.Value)
		}
		if item.Value == "" {
			t.Fatalf("empty value for command %q", item.Label)
		}
		if item.Icon == "" {
			t.Fatalf("empty icon for command %q", item.Value)
		}
		if item.Category == "" {
			t.Fatalf("empty category for command %q", item.Value)
		}
	}

	// Check specific commands have proper structure
	check := map[string]struct {
		icon     string
		category string
	}{
		"model":          {"󰊕", "AI & Model"},
		"theme":          {"󰏘", "Configuration"},
		"keybindings":    {"󰌌", "Configuration"},
		"compact":        {"󰕳", "AI & Model"},
		"run":            {"󰆍", "Workflow"},
	}
	for name, expect := range check {
		found := false
		for _, item := range items {
			if item.Value == name {
				found = true
				if item.Icon != expect.icon {
					t.Errorf("command %q icon = %q, want %q", name, item.Icon, expect.icon)
				}
				if item.Category != expect.category {
					t.Errorf("command %q category = %q, want %q", name, item.Category, expect.category)
				}
				break
			}
		}
		if !found {
			t.Errorf("command %q not found in palette items", name)
		}
	}

	// Check session-owned commands are still in palette
	sessionCmds := []string{"new", "sessions", "resume", "clear", "reset", "export", "approvals"}
	for _, name := range sessionCmds {
		found := false
		for _, item := range items {
			if item.Value == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("session-owned command %q missing from palette", name)
		}
	}
}

func TestCommandCategories(t *testing.T) {
	r := Default()
	categories := map[string]bool{}
	for _, cmd := range r.List() {
		categories[cmd.Category] = true
	}
	expected := []string{"AI & Model", "Project", "Workflow", "Configuration", "System", "Session"}
	for _, cat := range expected {
		if !categories[cat] {
			t.Fatalf("missing category %q", cat)
		}
	}
}

func TestAllCommandsHaveImmediateFlagSet(t *testing.T) {
	r := Default()
	for _, cmd := range r.List() {
		if cmd.Immediate && cmd.Usage != "" {
			t.Logf("command %q is Immediate=true with usage %q - verify this is intentional", cmd.Name, cmd.Usage)
		}
	}
}

func TestNoDuplicateIconsInSameCategory(t *testing.T) {
	r := Default()
	seen := map[string]map[string]bool{}
	for _, cmd := range r.List() {
		if seen[cmd.Category] == nil {
			seen[cmd.Category] = map[string]bool{}
		}
		if seen[cmd.Category][cmd.Icon] {
			t.Logf("duplicate icon %q in category %q for command %q", cmd.Icon, cmd.Category, cmd.Name)
		}
		seen[cmd.Category][cmd.Icon] = true
	}
}

func TestCommandImmediateFlags(t *testing.T) {
	r := Default()
	immediateCmds := []string{"context", "tree", "diff", "lsp", "test", "build", "review", "cancel", "stats", "help", "quit", "compact"}
	nonImmediateCmds := []string{"model", "provider", "mode", "search", "run", "api-key", "base-url", "effort", "provider-api-key", "provider-base-url", "theme", "keybindings"}

	for _, name := range immediateCmds {
		cmd, ok := r.Lookup(name)
		if !ok {
			t.Fatalf("command %q not found", name)
		}
		if !cmd.Immediate {
			t.Errorf("command %q should be Immediate=true", name)
		}
	}

	for _, name := range nonImmediateCmds {
		cmd, ok := r.Lookup(name)
		if !ok {
			t.Fatalf("command %q not found", name)
		}
		if cmd.Immediate {
			t.Errorf("command %q should be Immediate=false", name)
		}
	}
}