package agent

import (
	"strings"
	"testing"
	"time"
)

// newTestManager installs a fresh manager and returns it plus one tracked
// instance, so tests never touch the global singleton's state.
func newTestManager(id string) (*AgentManager, *AgentInstance) {
	m := &AgentManager{agents: make(map[string]*AgentInstance)}
	inst := &AgentInstance{
		ID:        id,
		Name:      id,
		Type:      AgentTypeExplore,
		Status:    AgentStatusRunning,
		StartedAt: time.Now(),
		done:      make(chan struct{}),
	}
	m.Create(inst)
	return m, inst
}

func TestNoteActivityBoundedAndOrdered(t *testing.T) {
	m, inst := newTestManager("test-1")

	for i := 0; i < maxActivityLines+20; i++ {
		m.NoteActivity("test-1", "step "+string(rune('a'+i%26))+strings.Repeat("b", i%3))
	}
	lines := inst.ActivityLines()
	if len(lines) != maxActivityLines {
		t.Fatalf("activity log must be bounded at %d, got %d", maxActivityLines, len(lines))
	}

	// Empty lines are dropped, not stored as blanks.
	m.NoteActivity("test-1", "   ")
	if got := len(inst.ActivityLines()); got != maxActivityLines {
		t.Fatalf("blank activity must be dropped, got %d lines", got)
	}

	// ActivityLines returns a copy: mutating it must not corrupt the log.
	lines[0] = "tampered"
	if inst.ActivityLines()[0] == "tampered" {
		t.Fatal("ActivityLines must return a copy")
	}
}

func TestToolActivityLine(t *testing.T) {
	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"grep", map[string]any{"pattern": "palette"}, "grep palette"},
		{"bash", map[string]any{"command": "go test ./..."}, "bash go test ./..."},
		{"read_file", map[string]any{"path": "internal/agent/agent.go"}, "read_file internal/agent/agent.go"},
		{"task", map[string]any{"prompt": "do a big thing"}, "task"},
		{"bash", nil, "bash"},
	}
	for _, c := range cases {
		if got := ToolActivityLine(c.name, c.args); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
	// Long subjects are clipped.
	long := strings.Repeat("x", 100)
	if got := ToolActivityLine("bash", map[string]any{"command": long}); len(got) > 70 {
		t.Errorf("long subject must be clipped, got %d chars", len(got))
	}
}

func TestSnapshotIncludesActivity(t *testing.T) {
	m, inst := newTestManager("test-2")
	m.NoteActivity("test-2", "grep palette internal/tui")
	m.NoteActivity("test-2", "read internal/tui/palette.go")

	snap := inst.Snapshot()
	if len(snap.Activity) != 2 {
		t.Fatalf("snapshot must carry the activity log, got %d lines", len(snap.Activity))
	}
	if snap.Activity[1] != "read internal/tui/palette.go" {
		t.Fatalf("activity order wrong: %v", snap.Activity)
	}

	// Terminal snapshots still carry elapsed time.
	inst.mu.Lock()
	inst.Status = AgentStatusCompleted
	inst.CompletedAt = inst.StartedAt.Add(3 * time.Second)
	inst.mu.Unlock()
	if snap2 := inst.Snapshot(); snap2.Elapsed == "" {
		t.Fatal("completed snapshot must carry elapsed")
	}
}
