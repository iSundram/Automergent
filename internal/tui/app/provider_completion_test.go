package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/iSundram/Automergent/internal/tui/components"
)

// /provider setup|use|test <name> must complete provider names from the same
// list the /provider picker uses — typing "setup goo" used to dead-end in
// "No matching providers".
func TestProviderSubcommandCompletesProviderNames(t *testing.T) {
	app := sizedTestApp(t)

	app.input.SetValue("/provider setup goo")
	app.updatePalette()

	items := app.palette.Items()
	if len(items) == 0 {
		t.Fatal("expected provider-name completions after '/provider setup goo', got none")
	}
	found := false
	for _, it := range items {
		if it.Value == "google-aistudio" {
			found = true
		}
		if !strings.HasPrefix(strings.ToLower(it.Value), "goo") {
			t.Fatalf("unexpected completion candidate %q", it.Value)
		}
	}
	if !found {
		values := make([]string, 0, len(items))
		for _, it := range items {
			values = append(values, it.Value)
		}
		t.Fatalf("google-aistudio must be offered, got %v", values)
	}

	// use and test complete provider names too.
	for _, sub := range []string{"use", "test"} {
		app.input.SetValue("/provider " + sub + " g")
		app.updatePalette()
		if len(app.palette.Items()) == 0 {
			t.Fatalf("expected provider completions for '/provider %s g'", sub)
		}
	}

	// Subcommands without a value domain (fallback) offer no provider list.
	app.input.SetValue("/provider fallback l")
	app.updatePalette()
	for _, it := range app.palette.Items() {
		if it.Value == "google-aistudio" {
			t.Fatal("provider names must not complete for /provider fallback")
		}
	}
}

// Searching the command palette must also search subcommands: "fallback"
// surfaces "/provider fallback".
func TestCommandSearchFindsSubcommands(t *testing.T) {
	app := sizedTestApp(t)

	app.input.SetValue("/fallback")
	app.updatePalette()

	var hit *components.PaletteItem
	items := app.palette.Items()
	for i := range items {
		if items[i].Value == "provider fallback" {
			hit = &items[i]
		}
	}
	if hit == nil {
		values := make([]string, 0, len(items))
		for _, it := range items {
			values = append(values, it.Value)
		}
		t.Fatalf("searching 'fallback' must surface /provider fallback, got %v", values)
	}

	// Selecting the subcommand hit dispatches the composed command: the
	// input is consumed and the palette closes (the handler's own result
	// may legitimately carry no tea.Cmd). The parent "/provider" command
	// also matches "fallback" through its description, so walk the cursor
	// down to the subcommand hit first.
	idx := -1
	for i, it := range items {
		if it.Value == "provider fallback" {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatal("subcommand item not selectable")
	}
	for i := 0; i < idx; i++ {
		app.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if app.palette.Visible() {
		t.Fatal("subcommand hit must close the palette")
	}
}

// The unfiltered command list stays the top-level surface: subcommand items
// join only while a filter is typed.
func TestUnfilteredCommandListHasNoSubcommandItems(t *testing.T) {
	app := sizedTestApp(t)

	app.input.SetValue("/")
	app.updatePalette()
	for _, it := range app.palette.Items() {
		if strings.Contains(it.Value, " ") {
			t.Fatalf("unfiltered command list must not contain subcommand items, got %q", it.Value)
		}
	}
}
