package components_test

import (
	"testing"

	"github.com/iSundram/Automergent/internal/tui/commands"
	"github.com/iSundram/Automergent/internal/tui/components"
	"github.com/iSundram/Automergent/internal/tui/themes"
)

func newInputWithValue(t *testing.T, value string) *components.Input {
	t.Helper()
	in := components.NewInput(themes.NewStyles(themes.Get("modern")))
	in.SetValue(value)
	return &in
}

// TriggerType must use exact token boundaries: longer command names and
// namespaced custom commands must not be hijacked into argument sub-palettes.
// Sub-palettes open only after a trailing space, so a bare "/model" still
// shows the /model command entry.
func TestTriggerTypeUsesTokenBoundaries(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"/model", "command"},
		{"/model gemini-3.6-flash", "model"},
		{"/provider", "command"},
		{"/provider google", "provider"},
		{"/mode", "command"},
		{"/mode plan", "mode"},
		{"/provider-api-key google x", "command"}, // prefix hijack regression
		{"/provider-base-url google https://x", "command"},
		{"/models", "command"},
		{"/modes", "command"},
		{"/model:dump x", "command"}, // namespaced custom command
		{"/providers:foo", "command"},
		{"/deploy prod", "command"},
		{"/", "command"},
		{"?", "help"},
		{"hello @main.go", "file"},
		{"hello world", ""},
	}
	for _, tc := range cases {
		if got := newInputWithValue(t, tc.input).TriggerType(); got != tc.want {
			t.Errorf("TriggerType(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestTriggerValueStripsExactCommandToken(t *testing.T) {
	in := newInputWithValue(t, "/model gemini-3.6-fl")
	if got := in.TriggerValue(); got != "gemini-3.6-fl" {
		t.Fatalf("TriggerValue = %q", got)
	}
}

func TestInsertValueCompletesSubPaletteCommand(t *testing.T) {
	in := newInputWithValue(t, "/mo")
	in.InsertValue("model")
	if got := in.Value(); got != "/model " {
		t.Fatalf("InsertValue from command palette = %q", got)
	}
}

// The sub-palette set and the registry must never drift apart.
func TestSlashSubPalettesMatchRegisteredCommands(t *testing.T) {
	reg := commands.Default()
	for name := range components.SlashSubPalettes {
		cmd, ok := reg.Lookup(name)
		if !ok {
			t.Errorf("SlashSubPalettes references unregistered command %q", name)
			continue
		}
		if cmd.Immediate {
			t.Errorf("sub-palette command %q must not be Immediate (it needs an argument)", name)
		}
	}
}
