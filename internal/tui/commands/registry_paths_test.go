package commands

import (
	"testing"

	"github.com/iSundram/Automergent/internal/tui/components"
)

// CCB-ported registry capabilities: Fork execution mode, Paths visibility
// gating, and ShouldQuery results.

func TestPathGatingHidesCommandUntilFileTouched(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(Command{
		Name:        "gated",
		Description: "only for Go files",
		Category:    "Project",
		Paths:       []string{"*.go"},
	}, func(h Host, args []string) Result { return Done(nil) })
	r.MustRegister(Command{
		Name:        "open",
		Description: "always visible",
		Category:    "Project",
	}, func(h Host, args []string) Result { return Done(nil) })

	m := NewMockHost()

	// No touched files: gated command absent, open command present.
	names := paletteNames(r.PaletteItems(m))
	if names["gated"] {
		t.Fatal("path-gated command must be hidden before a matching file is touched")
	}
	if !names["open"] {
		t.Fatal("ungated command must always be visible")
	}

	// A matching base name surfaces it.
	m.recentFilePaths = []string{"internal/agent/agent.go"}
	names = paletteNames(r.PaletteItems(m))
	if !names["gated"] {
		t.Fatal("*.go glob must match the touched .go file")
	}

	// Non-matching files keep it hidden.
	m.recentFilePaths = []string{"README.md", "docs/guide.txt"}
	names = paletteNames(r.PaletteItems(m))
	if names["gated"] {
		t.Fatal("gated command must stay hidden for non-matching files")
	}
}

func TestPathGatingMatchesFullPathAndBasename(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(Command{
		Name:   "gomod",
		Paths:  []string{"go.mod"},
		Category: "Project",
	}, func(h Host, args []string) Result { return Done(nil) })
	r.MustRegister(Command{
		Name:   "deep",
		Paths:  []string{"internal/*/modes.go"},
		Category: "Project",
	}, func(h Host, args []string) Result { return Done(nil) })

	m := NewMockHost()
	m.recentFilePaths = []string{"go.mod", "internal/agent/modes.go"}
	names := paletteNames(r.PaletteItems(m))
	if !names["gomod"] || !names["deep"] {
		t.Fatalf("exact and glob path matching both failed: %v", names)
	}
}

func TestPathGatingStillDispatchableWhenHidden(t *testing.T) {
	// Visibility is palette-only: a typed /gated still dispatches.
	r := NewRegistry()
	called := false
	r.MustRegister(Command{
		Name:   "gated",
		Paths:  []string{"*.go"},
		Category: "Project",
	}, func(h Host, args []string) Result { called = true; return Done(nil) })

	m := NewMockHost()
	m.recentFilePaths = nil
	if _, err := r.Dispatch(m, "gated", nil); err != nil {
		t.Fatalf("dispatch failed: %v", err)
	}
	if !called {
		t.Fatal("hidden command must still execute when typed explicitly")
	}
}

func TestShouldQueryResultSentToAgent(t *testing.T) {
	// ShouldQuery is a data flag consumed by the app glue; the contract to
	// pin here is that it round-trips through Done-style construction.
	res := Result{Text: "gathered state", ShouldQuery: true}
	if !res.ShouldQuery || res.IsZero() {
		t.Fatal("ShouldQuery result must carry text and the flag")
	}
	plain := TextResult("just info")
	if plain.ShouldQuery {
		t.Fatal("TextResult must not trigger a follow-up query by default")
	}
}

func paletteNames(items []components.PaletteItem) map[string]bool {
	out := make(map[string]bool)
	for _, it := range items {
		out[it.Label] = true
	}
	return out
}
