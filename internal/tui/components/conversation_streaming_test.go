package components

import (
	"strings"
	"testing"

	"github.com/iSundram/Automergent/internal/tools"
	"github.com/iSundram/Automergent/internal/tui/themes"
)

func TestSmokeStreamingCache(t *testing.T) {
	c := NewConversation(themes.NewStyles(themes.Get("nord")))
	c.SetSize(80, 24)
	c.AppendToken("hello ")
	for _, w := range []string{"wor", "ld", " more"} {
		c.AppendToken(w)
	}
	if !c.dirty {
		t.Fatal("expected dirty after tokens")
	}
	c.RenderIfDirty()
	if c.dirty {
		t.Fatal("dirty should clear after render")
	}
	c.FinalizeStreamingWithContent("hello **world** done")
	v := c.View()
	if v == "" {
		t.Fatal("empty view")
	}
}

func TestSmokeExpandCycleAndGrouping(t *testing.T) {
	c := NewConversation(themes.NewStyles(themes.Get("catppuccin")))
	c.SetSize(80, 24)
	c.AddToolLifecycleStart("1", "read_file", "{}", "/a.go")
	c.AddToolLifecycleDone("1", "read_file", "/a.go", "ok", 1000000, tools.Result{Content: "out"}, false)
	c.AddToolLifecycleStart("2", "read_file", "{}", "/b.go")
	c.AddToolLifecycleDone("2", "read_file", "/b.go", "ok2", 2000000, tools.Result{Content: "{\"k\":1}"}, false)
	lbl := c.CycleExpand()
	if lbl != "Tool cards: collapsed" {
		t.Fatalf("unexpected label %q", lbl)
	}
	c.RenderIfDirty()
	v := c.View()
	if !strings.Contains(v, "×2") {
		t.Fatalf("expected grouped ×2 card, got:\n%s", v)
	}
}
