package commands

import (
	"testing"
)

func TestHandleRun(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		verify func(*mockHost, *testing.T)
	}{
		{
			name: "usage when no command",
			args: []string{},
			verify: func(m *mockHost, t *testing.T) {
				if len(m.usageMessages) != 1 {
					t.Fatalf("expected usage message")
				}
			},
		},
		{
			name: "run command",
			args: []string{"go", "test", "./..."},
			verify: func(m *mockHost, t *testing.T) {
				if len(m.startAgentCalls) != 1 {
					t.Fatalf("expected StartAgent called, got: %v", m.startAgentCalls)
				}
				if !contains(m.startAgentCalls[0], "go test ./...") {
					t.Fatalf("expected agent prompt to contain command, got: %s", m.startAgentCalls[0])
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMockHost()
			handleRun(m, tt.args)
			tt.verify(m, t)
		})
	}
}

func TestHandleTest(t *testing.T) {
	m := NewMockHost()
	handleTest(m, []string{"./..."})
	if len(m.startAgentCalls) != 1 {
		t.Fatalf("expected StartAgent called, got: %v", m.startAgentCalls)
	}
	if !contains(m.startAgentCalls[0], "./...") {
		t.Fatalf("expected agent prompt to contain target, got: %s", m.startAgentCalls[0])
	}
}

func TestHandleBuild(t *testing.T) {
	m := NewMockHost()
	handleBuild(m, []string{"release"})
	if len(m.startAgentCalls) != 1 {
		t.Fatalf("expected StartAgent called, got: %v", m.startAgentCalls)
	}
	if !contains(m.startAgentCalls[0], "release") {
		t.Fatalf("expected agent prompt to contain target, got: %s", m.startAgentCalls[0])
	}
}

func TestHandleReview(t *testing.T) {
	m := NewMockHost()
	handleReviewMode(m, []string{})
	if m.toggleReviewModeCalls != 1 {
		t.Fatalf("expected ToggleReviewMode called, got %d", m.toggleReviewModeCalls)
	}
}

func TestHandleCancel(t *testing.T) {
	tests := []struct {
		name     string
		thinking bool
		verify   func(*mockHost, *testing.T)
	}{
		{
			name:     "no active request",
			thinking: false,
			verify: func(m *mockHost, t *testing.T) {
				if len(m.statusMessages) != 1 || !contains(m.statusMessages[0], "No active request") {
					t.Fatalf("expected status message, got: %v", m.statusMessages)
				}
				if len(m.cancelActiveRuns) != 0 {
					t.Fatalf("expected no CancelActiveRun call")
				}
			},
		},
		{
			name:     "cancels active run",
			thinking: true,
			verify: func(m *mockHost, t *testing.T) {
				if len(m.cancelActiveRuns) != 1 || m.cancelActiveRuns[0] != "Cancelled by user" {
					t.Fatalf("expected CancelActiveRun called, got: %v", m.cancelActiveRuns)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMockHost()
			m.thinking = tt.thinking
			handleCancel(m, []string{})
			tt.verify(m, t)
		})
	}
}
