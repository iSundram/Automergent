package commands

import (
	"testing"
)

func TestHandleNew(t *testing.T) {
	t.Run("starts a fresh session and confirms the saved one", func(t *testing.T) {
		m := NewMockHost()
		handleNew(m, nil)
		if m.newSessionCalls != 1 {
			t.Fatalf("expected NewSession called once, got %d", m.newSessionCalls)
		}
		if len(m.systemMessages) != 1 || !contains(m.systemMessages[0], "sess-test") {
			t.Fatalf("expected confirmation naming the previous session, got: %v", m.systemMessages)
		}
	})

	t.Run("refuses to run while the agent is active", func(t *testing.T) {
		m := NewMockHost()
		m.mu.Lock()
		m.thinking = true
		m.mu.Unlock()
		handleNew(m, nil)
		if m.newSessionCalls != 0 {
			t.Fatalf("expected NewSession not called, got %d", m.newSessionCalls)
		}
		if len(m.errorMessages) != 1 || !contains(m.errorMessages[0], "/cancel") {
			t.Fatalf("expected error pointing at /cancel, got: %v", m.errorMessages)
		}
	})
}
