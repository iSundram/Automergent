package components

import (
	"strings"
	"testing"
	"time"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

func expandTestConversation(t *testing.T) Conversation {
	t.Helper()
	c := NewConversation(themes.NewStyles(themes.Get("catppuccin")))
	c.SetSize(100, 40)
	return c
}

func TestSetExpandTogglesBlocks(t *testing.T) {
	c := expandTestConversation(t)

	collapsed := c.SetExpand(false)
	if c.ExpandMode() != ExpandCompact {
		t.Fatalf("SetExpand(false) left mode at %v", c.ExpandMode())
	}
	if !strings.Contains(collapsed, "/expand") {
		t.Errorf("collapsed label must advertise /expand: %q", collapsed)
	}

	expanded := c.SetExpand(true)
	if c.ExpandMode() != ExpandFull {
		t.Fatalf("SetExpand(true) left mode at %v", c.ExpandMode())
	}
	if !strings.Contains(expanded, "/collapse") {
		t.Errorf("expanded label must advertise /collapse: %q", expanded)
	}
}

func TestCollapsedThoughtShowsHeaderOnly(t *testing.T) {
	c := expandTestConversation(t)
	c.SetExpand(false)

	// Seed through the real API, then set the thought fields the way
	// FinalizeStreamingWithContent does for a settled assistant turn.
	c.AddMessage("assistant", "The answer.", false)
	c.messages[len(c.messages)-1].Thought = "Let me consider several approaches here.\nOne of them is quite long and detailed with many lines."
	c.messages[len(c.messages)-1].ThoughtDuration = 4 * time.Second
	c.invalidateAll()

	view := c.View()
	if !strings.Contains(view, "Thought for 4s") {
		t.Errorf("collapsed thought missing duration header:\n%s", view)
	}
	if strings.Contains(view, "Let me consider") {
		t.Errorf("collapsed thought leaked its body — only the header line may show:\n%s", view)
	}
	if !strings.Contains(view, "/expand") {
		t.Errorf("collapsed thought must advertise /expand:\n%s", view)
	}

	// Expanded: the body returns.
	c.SetExpand(true)
	view = c.View()
	if !strings.Contains(view, "Let me consider") {
		t.Errorf("expanded thought lost its body:\n%s", view)
	}
}

func TestHintRowNamesTheFlippingCommand(t *testing.T) {
	c := expandTestConversation(t)

	c.SetExpand(false)
	if hint := c.hintRow(6, "lines"); !strings.Contains(hint, "/expand") {
		t.Errorf("collapsed hint must name /expand: %q", hint)
	}

	c.SetExpand(true)
	if hint := c.hintRow(6, "lines"); !strings.Contains(hint, "/collapse") {
		t.Errorf("expanded hint must name /collapse: %q", hint)
	}
}

func TestSettledUnknownDurationNeverReadsAsThinking(t *testing.T) {
	// A settled block whose duration was never stamped (restored session,
	// pre-fix finalize order) must read "✓ Thought" — "● Thinking · /expand"
	// on a finished block was a renderer lie.
	c := expandTestConversation(t)
	c.SetExpand(false)
	c.AddMessage("assistant", "answer", false)
	c.messages[len(c.messages)-1].Thought = "some reasoning"
	// ThoughtDuration deliberately left zero.
	c.invalidateAll()

	view := c.View()
	if strings.Contains(view, "● Thinking") {
		t.Fatalf("settled block rendered as Thinking:\n%s", view)
	}
	if !strings.Contains(view, "✓ Thought") {
		t.Fatalf("settled block missing ✓ Thought header:\n%s", view)
	}
}

func TestThoughtOnlyTurnGetsDurationStamped(t *testing.T) {
	// A turn that produced a thought but no content tokens: finalize must
	// flush the thought builder AND stamp the duration — in that order.
	c := expandTestConversation(t)
	c.AppendThought("reasoning ")
	c.AppendToken("answer")
	c.FinalizeStreamingWithContent("answer")

	found := false
	for _, m := range c.messages {
		if m.Thought != "" && m.ThoughtDuration > 0 {
			found = true
		}
	}
	if !found {
		t.Fatal("thought turn has no stamped duration — header will read as unknown")
	}

	// And a thought-only finalize (no tokens at all) also stamps.
	c2 := expandTestConversation(t)
	c2.AppendThought("pure thought ")
	c2.FinalizeStreamingWithContent("")
	found = false
	for _, m := range c2.messages {
		if m.Thought != "" && m.ThoughtDuration > 0 {
			found = true
		}
	}
	if !found {
		t.Fatal("thought-only turn never got its duration stamped")
	}
}

func TestCollapsedShellTailIsOneLine(t *testing.T) {
	c := expandTestConversation(t)
	c.SetExpand(false)
	if got := c.tailLimit(); got != 1 {
		t.Fatalf("collapsed shell tail limit = %d, want 1", got)
	}
	c.SetExpand(true)
	if got := c.tailLimit(); got != maxTailLines {
		t.Fatalf("expanded shell tail limit = %d, want %d", got, maxTailLines)
	}
}
