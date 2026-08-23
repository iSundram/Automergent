package commands

import (
	"strings"
	"testing"
)

// The seven session-lifecycle handlers delegate to Host methods; these tests
// pin their argument handling and delegation contracts.

func TestSessionLifecycleDelegation(t *testing.T) {
	cases := []struct {
		name    string
		run     func(Host, []string) Result
		assert  func(m *mockHost) bool
		message string
	}{
		{"new", handleNew, func(m *mockHost) bool { return m.newSessionCalls == 1 }, "NewSession called"},
		{"sessions", handleSessions, func(m *mockHost) bool { return m.showSessionsCalls == 1 }, "ShowSessions called"},
		{"clear", handleClear, func(m *mockHost) bool { return m.clearConversationCalls == 1 }, "ClearConversationView called"},
		{"reset", handleReset, func(m *mockHost) bool { return m.resetHistoryCalls == 1 }, "ResetSessionHistory called"},
	}
	for _, tc := range cases {
		m := NewMockHost()
		tc.run(m, nil)
		if !tc.assert(m) {
			t.Errorf("/%s: %s", tc.name, tc.message)
		}
	}
}

func TestResumeDelegatesWithAndWithoutID(t *testing.T) {
	m := NewMockHost()
	handleResume(m, []string{"abc-123"})
	if len(m.resumeSessionCalls) != 1 || m.resumeSessionCalls[0] != "abc-123" {
		t.Fatalf("resume should pass the id through: %v", m.resumeSessionCalls)
	}

	m = NewMockHost()
	handleResume(m, nil)
	if len(m.resumeSessionCalls) != 0 || m.showSessionsCalls != 1 {
		t.Fatal("bare /resume must open the browser instead of resuming")
	}
}

func TestExportPassesPathAndReportsSuccess(t *testing.T) {
	m := NewMockHost()

	handleExport(m, []string{"chat.md"})
	if len(m.exportConversationCalls) != 1 || m.exportConversationCalls[0] != "chat.md" {
		t.Fatalf("path not forwarded: %v", m.exportConversationCalls)
	}
	if !strings.Contains(m.systemMessages[0], "chat.md") || lastStatus(m) != "Conversation exported" {
		t.Fatalf("success feedback missing: %v / %q", m.systemMessages, lastStatus(m))
	}
}

func TestExportDefaultsPathAndReportsErrors(t *testing.T) {
	m := NewMockHost()
	handleExport(m, nil)
	if m.exportConversationCalls[0] != "" {
		t.Fatalf("bare /export should forward empty path, got %q", m.exportConversationCalls[0])
	}

	failing := NewMockHost()
	failing.exportErr = errTestFailure{}
	handleExport(failing, []string{"x.md"})
	if len(failing.errorMessages) == 0 {
		t.Fatal("export failure must surface via CommandError")
	}
	if strings.Contains(strings.Join(failing.systemMessages, "\n"), "exported to") {
		t.Fatal("failed export must not claim success")
	}
}

func TestApprovalsForwardsArgs(t *testing.T) {
	m := NewMockHost()
	handleApprovals(m, []string{"revoke", "2"})
	if len(m.handleApprovalsCalls) != 1 {
		t.Fatal("approvals handler not invoked")
	}
	args := m.handleApprovalsCalls[0]
	if len(args) != 2 || args[0] != "revoke" || args[1] != "2" {
		t.Fatalf("args not forwarded verbatim: %v", args)
	}
}

func lastStatus(m *mockHost) string {
	if len(m.statusMessages) == 0 {
		return ""
	}
	return m.statusMessages[len(m.statusMessages)-1]
}

type errTestFailure struct{}

func (errTestFailure) Error() string { return "disk full" }
