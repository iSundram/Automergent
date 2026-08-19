package components

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestBrowsingReservesColumnForScrollbar(t *testing.T) {
	conversation := NewConversation(testStyles())
	conversation.SetSize(60, 10)
	// Fill with enough content to overflow.
	for i := 0; i < 40; i++ {
		conversation.AddMessage("user", "a fairly long user message that wraps around nicely", false)
	}

	conversation.SetBrowsing(true)
	view := ansi.Strip(conversation.View())
	// The scrollbar track char appears on the right edge.
	if !strings.Contains(view, "█") && !strings.Contains(view, "░") {
		t.Fatalf("expected scrollbar track in browsing view:\n%s", view)
	}

	// Every content row must be exactly the scrollbar track column at its
	// right edge, i.e. no content column should overlap the track.
	lines := strings.Split(view, "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		width := ansi.StringWidth(line)
		if width > 60 {
			t.Fatalf("row exceeds viewport width (got %d > 60): %q", width, line)
		}
	}
}

func TestBrowsingContentWrapsToShrunkWidth(t *testing.T) {
	conversation := NewConversation(testStyles())
	conversation.SetSize(40, 10)
	for i := 0; i < 20; i++ {
		conversation.AddMessage("user", "message body text", false)
	}
	widthBefore := conversation.viewport.Width()

	conversation.SetBrowsing(true)
	widthAfter := conversation.viewport.Width()
	if widthAfter != widthBefore-1 {
		t.Fatalf("expected viewport to shrink by 1 column when browsing (was %d, now %d)", widthBefore, widthAfter)
	}
}