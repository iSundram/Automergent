package components

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestConversationMouseWheelScrollsInBrowseMode(t *testing.T) {
	conversation := NewConversation(testStyles())
	conversation.SetBrowsing(true)
	conversation.SetSize(60, 10)
	for i := 0; i < 60; i++ {
		conversation.AddMessage("user", "message number "+strings.Repeat("x", i), false)
	}
	conversation.viewport.GotoTop()

	before := conversation.viewport.YOffset()
	if before != 0 {
		t.Fatalf("expected to start at top, got yoffset=%d", before)
	}

	wheel := func(button tea.MouseButton) tea.MouseWheelMsg {
		return tea.MouseWheelMsg{Button: button}
	}

	updated, _ := conversation.Update(wheel(tea.MouseWheelDown))
	conversation = updated
	if conversation.viewport.YOffset() <= before {
		t.Fatalf("mouse wheel down did not scroll (yoffset %d -> %d)", before, conversation.viewport.YOffset())
	}

	updated, _ = conversation.Update(wheel(tea.MouseWheelUp))
	conversation = updated
	if conversation.viewport.YOffset() >= conversation.viewport.TotalLineCount() {
		t.Fatal("unexpected scroll state after wheel up")
	}
}

func TestConversationMouseWheelIgnoredWhenNotBrowsing(t *testing.T) {
	conversation := NewConversation(testStyles())
	conversation.SetBrowsing(false)
	conversation.SetSize(60, 10)
	for i := 0; i < 20; i++ {
		conversation.AddMessage("user", "message", false)
	}
	conversation.viewport.GotoTop()

	updated, _ := conversation.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if updated.viewport.YOffset() != 0 {
		t.Fatalf("mouse wheel should be ignored when not browsing, yoffset=%d", updated.viewport.YOffset())
	}
}
