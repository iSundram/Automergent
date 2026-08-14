package shell

import (
	"context"
	"strings"
	"testing"
	"time"
)

func resetSessionManagerForTest() *SessionManager {
	globalManager = &SessionManager{
		sessions: make(map[string]*AsyncSession),
		history:  make(map[string]SessionRecord),
	}
	return GetManager()
}

func TestSessionManagerLifecycleAndHook(t *testing.T) {
	mgr := resetSessionManagerForTest()

	s := &AsyncSession{
		ID:      "shell-1",
		Command: "echo hi",
		Started: time.Now().Add(-1 * time.Second),
	}
	mgr.Create(s.ID, s)
	mgr.MarkBackground(s.ID, true, false)

	var notified SessionNotification
	mgr.RegisterStatusHook(func(n SessionNotification) {
		notified = n
	})

	if ok := mgr.UpdateStatus(s.ID, SessionStatusCompleted, 0, nil); !ok {
		t.Fatalf("expected update to succeed")
	}
	if notified.ID != s.ID || notified.Status != SessionStatusCompleted {
		t.Fatalf("unexpected notification: %+v", notified)
	}

	rec, ok := mgr.GetRecord(s.ID)
	if !ok {
		t.Fatalf("expected history record")
	}
	if rec.Status != SessionStatusCompleted || rec.ExitCode != 0 {
		t.Fatalf("unexpected record: %+v", rec)
	}
}

func TestReadShellFallsBackToHistory(t *testing.T) {
	mgr := resetSessionManagerForTest()

	s := &AsyncSession{
		ID:      "shell-2",
		Command: "false",
		Started: time.Now().Add(-2 * time.Second),
	}
	mgr.Create(s.ID, s)
	mgr.MarkBackground(s.ID, true, false)
	mgr.UpdateStatus(s.ID, SessionStatusFailed, 2, context.DeadlineExceeded)
	mgr.Delete(s.ID)

	tool := &ReadShellTool{}
	res, err := tool.Execute(context.Background(), map[string]any{"shell_id": s.ID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected non-error read result, got: %s", res.Content)
	}
	if !strings.Contains(res.Content, "failed") {
		t.Fatalf("expected failed status in content, got: %s", res.Content)
	}

	runningOnly := mgr.ListRecords(false)
	if len(runningOnly) != 0 {
		t.Fatalf("expected no running records, got %d", len(runningOnly))
	}
}
