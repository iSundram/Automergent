package commands

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/iSundram/Automergent/internal/tui/components"
)

func TestRegistryNamesAndAliasesAreUnique(t *testing.T) {
	r := Default()
	seen := map[string]string{}
	for _, command := range r.List() {
		if command.Name == "" || command.Category == "" || command.Description == "" || command.Icon == "" {
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
		"session":     "sessions",
		"pr":          "review",
		"usage":       "cost",
		"approvals":   "permissions",
	}
	for alias, want := range wants {
		got, ok := r.Lookup(alias)
		if !ok || got.Name != want {
			t.Fatalf("Lookup(%q) = %#v, %v; want %q", alias, got, ok, want)
		}
	}
}

func TestPaletteItemsExposeArgsHintAndSearchTerms(t *testing.T) {
	r := Default()
	items := r.PaletteItems(nil)
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
		t.Fatal("run command is missing args hint or searchable metadata")
	}
}

func TestEveryCommandHasHandler(t *testing.T) {
	r := Default()
	for _, cmd := range r.List() {
		if !r.HasHandler(cmd.Name) {
			t.Fatalf("command %q registered without handler", cmd.Name)
		}
	}
}

func TestSessionCommandsHaveHandlers(t *testing.T) {
	r := Default()
	sessionCmds := []string{"new", "sessions", "resume", "clear", "reset", "export", "permissions", "rewind", "branch", "summary"}
	for _, name := range sessionCmds {
		if !r.HasHandler(name) {
			t.Fatalf("session command %q should have a handler in command package", name)
		}
	}
}

func TestDispatchReturnsErrUnknownCommand(t *testing.T) {
	r := Default()
	m := NewMockHost()
	if _, err := r.Dispatch(m, "nonexistent", []string{}); err == nil {
		t.Fatal("expected error for unknown command")
	} else if _, ok := err.(ErrUnknownCommand); !ok {
		t.Fatalf("expected ErrUnknownCommand, got %T: %v", err, err)
	}
}

func TestDispatchCallsHandler(t *testing.T) {
	r := Default()
	m := NewMockHost()
	result, err := r.Dispatch(m, "help", []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.showHelpCalls != 1 {
		t.Fatalf("expected ShowHelp called, got %d", m.showHelpCalls)
	}
	if result.Cmd != nil || result.Text != "" {
		t.Fatalf("help should produce an empty result, got %#v", result)
	}
}

func TestDispatchResolvesAliasesBeforeDispatch(t *testing.T) {
	r := Default()
	m := NewMockHost()
	m.thinking = true
	if _, err := r.Dispatch(m, "stop", []string{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.cancelActiveRuns) == 0 {
		t.Fatal("alias /stop did not reach cancel handler")
	}
}

func TestDispatchRespectsEnabledGate(t *testing.T) {
	r := NewRegistry()
	called := false
	r.MustRegister(Command{
		Name:           "gated",
		Description:    "Gated command",
		Category:       "Test",
		Icon:           "x",
		Enabled:        func(h Host) bool { return false },
		DisabledReason: func(h Host) string { return "not now" },
	}, func(h Host, args []string) Result {
		called = true
		return Done(nil)
	})
	m := NewMockHost()

	result, err := r.Dispatch(m, "gated", []string{})
	if called {
		t.Fatal("handler must not run when disabled")
	}
	var disabled ErrCommandDisabled
	if !errors.As(err, &disabled) || disabled.Reason != "not now" {
		t.Fatalf("expected ErrCommandDisabled with reason, got %#v", err)
	}
	if !result.IsZero() {
		t.Fatalf("disabled dispatch should return empty result, got %#v", result)
	}

	// Enabled commands run normally through the same path.
	r2 := NewRegistry()
	r2.MustRegister(Command{Name: "ok", Description: "d", Category: "Test", Icon: "x"}, func(h Host, args []string) Result {
		return TextResult("ran")
	})
	res, err := r2.Dispatch(NewMockHost(), "ok", []string{})
	if err != nil || res.Text != "ran" {
		t.Fatalf("enabled dispatch failed: res=%#v err=%v", res, err)
	}
}

func TestDefaultCancelIsStateGated(t *testing.T) {
	r := Default()
	idle := NewMockHost()
	_, err := r.Dispatch(idle, "cancel", []string{})
	var disabled ErrCommandDisabled
	if !errors.As(err, &disabled) || disabled.Reason != "No active request" {
		t.Fatalf("idle host should disable /cancel with reason, got %v", err)
	}

	busy := NewMockHost()
	busy.thinking = true
	if _, err := r.Dispatch(busy, "cancel", []string{}); err != nil {
		t.Fatalf("thinking host should allow /cancel: %v", err)
	}
	if len(busy.cancelActiveRuns) != 1 {
		t.Fatal("cancel handler did not run for busy host")
	}
}

func TestPaletteDecorationsComeFromRegistry(t *testing.T) {
	items := Default().PaletteItems(NewMockHost())
	byValue := map[string]components.PaletteItem{}
	for _, item := range items {
		byValue[item.Value] = item
	}

	cancel := byValue["cancel"]
	if !cancel.Disabled || cancel.DisabledReason != "No active request" {
		t.Fatalf("cancel should be disabled with reason for idle host: %#v", cancel.Disabled)
	}
	if byValue["review-mode"].Current {
		t.Fatal("review should not be current for idle mock host")
	}
	if byValue["tree"].Current || byValue["diff"].Current || byValue["lsp"].Current {
		t.Fatal("pane toggles should not be current for idle mock host")
	}
}

func TestHiddenCommandsExcludedFromPalette(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(Command{Name: "visible", Description: "d", Category: "Test", Icon: "x"}, func(h Host, a []string) Result { return Done(nil) })
	r.MustRegister(Command{Name: "secret", Description: "d", Category: "Test", Icon: "x", Hidden: true}, func(h Host, a []string) Result { return Done(nil) })

	items := r.PaletteItems(nil)
	if len(items) != 1 || items[0].Value != "visible" {
		t.Fatalf("hidden commands must be excluded from palette items: %#v", items)
	}
	if _, ok := r.Lookup("secret"); !ok {
		t.Fatal("hidden commands must remain dispatchable")
	}
}

func TestRegisterPanicsOnNilHandlerAndDuplicates(t *testing.T) {
	panics := func(name string, fn func()) {
		t.Helper()
		defer func() {
			if r := recover(); r == nil {
				t.Fatalf("expected panic for %s", name)
			}
		}()
		fn()
	}
	noop := func(h Host, a []string) Result { return Done(nil) }

	r := NewRegistry()
	r.MustRegister(Command{Name: "a", Aliases: []string{"z"}, Description: "d", Category: "Test", Icon: "x"}, noop)

	t.Run("duplicate-name", func(t *testing.T) {
		panics("duplicate name", func() { r.Register(Command{Name: "a", Description: "d", Category: "Test", Icon: "x"}, noop) })
	})
	t.Run("duplicate-alias", func(t *testing.T) {
		panics("duplicate alias", func() {
			r.Register(Command{Name: "b", Aliases: []string{"z"}, Description: "d", Category: "Test", Icon: "x"}, noop)
		})
	})
	t.Run("nil-handler", func(t *testing.T) {
		panics("nil handler", func() { r.Register(Command{Name: "c", Description: "d", Category: "Test", Icon: "x"}, nil) })
	})
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
	items := r.PaletteItems(nil)
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

	check := map[string]struct {
		icon     string
		category string
	}{
		"model":       {"󰊕", "AI & Model"},
		"theme":       {"󰏘", "Configuration"},
		"keybindings": {"󰌌", "Configuration"},
		"compact":     {"󰕳", "AI & Model"},
		"run":         {"󰆍", "Workflow"},
		"new":         {"󰐕", "Session"},
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

func TestCommandImmediateFlags(t *testing.T) {
	r := Default()
	immediateCmds := []string{"context", "tree", "diff", "lsp", "test", "build", "review-mode", "cancel", "stats", "help", "quit", "compact",
		"new", "sessions", "resume", "clear", "reset", "export", "permissions",
		"init", "recap", "memory", "env", "version", "doctor",
		"rewind", "cost", "config", "context-files", "security-review"}
	nonImmediateCmds := []string{"model", "provider", "mode", "search", "run", "api-key", "base-url", "effort", "provider-api-key", "provider-base-url", "theme", "keybindings", "rename",
		"review", "branch", "summary", "issue", "pr-comments", "add-dir",
		// /commit is non-Immediate: it opens a scope sub-palette first.
		"commit"}

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

func TestSensitiveCommandsFlagged(t *testing.T) {
	r := Default()
	for _, name := range []string{"api-key", "provider-api-key"} {
		cmd, ok := r.Lookup(name)
		if !ok {
			t.Fatalf("command %q not found", name)
		}
		if !cmd.Sensitive {
			t.Errorf("command %q should be Sensitive=true", name)
		}
	}
}

// compile-time guard that Done keeps wrapping tea.Cmd values.
var _ = func() Result { return Done(tea.Quit) }
