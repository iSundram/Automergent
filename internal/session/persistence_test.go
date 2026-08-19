package session

import (
	"testing"
	"time"

	"github.com/iSundram/Automergent/internal/ai"
)

func TestPersistenceManager_SaveLoad(t *testing.T) {
	dir := t.TempDir()
	pm, err := NewPersistenceManager(dir)
	if err != nil {
		t.Fatalf("NewPersistenceManager: %v", err)
	}

	sess := New()
	sess.Title = "Test Session"
	pm.SetSession(sess)
	pm.SetWorkDir("/home/user/project")
	pm.SetProjectPath("/home/user/project")
	pm.SetGitBranch("main")
	pm.SetLastFile("main.go")
	pm.SetCursor("main.go", 42, 10)
	pm.SetOpenFiles([]string{"main.go", "utils.go"})
	pm.SetContext("key1", "value1")

	if err := pm.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Create a new manager and load
	pm2, err := NewPersistenceManager(dir)
	if err != nil {
		t.Fatalf("NewPersistenceManager: %v", err)
	}
	if err := pm2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	state := pm2.State()
	if state == nil {
		t.Fatal("State is nil after load")
	}
	if state.Session == nil {
		t.Fatal("Session is nil after load")
	}
	if state.Session.Title != "Test Session" {
		t.Errorf("Title = %q, want %q", state.Session.Title, "Test Session")
	}
	if state.WorkDir != "/home/user/project" {
		t.Errorf("WorkDir = %q, want %q", state.WorkDir, "/home/user/project")
	}
	if state.GitBranch != "main" {
		t.Errorf("GitBranch = %q, want %q", state.GitBranch, "main")
	}
	if state.Cursor == nil || state.Cursor.Line != 42 {
		t.Errorf("Cursor.Line = %v, want 42", state.Cursor)
	}
	if len(state.OpenFiles) != 2 {
		t.Errorf("len(OpenFiles) = %d, want 2", len(state.OpenFiles))
	}
	if v, ok := pm2.GetContext("key1"); !ok || v != "value1" {
		t.Errorf("GetContext(key1) = %q, %v, want %q, true", v, ok, "value1")
	}
}

func TestPersistenceManager_RecoveryPoint(t *testing.T) {
	dir := t.TempDir()
	pm, err := NewPersistenceManager(dir)
	if err != nil {
		t.Fatalf("NewPersistenceManager: %v", err)
	}

	sess := New()
	sess.Title = "Recovery Test"
	pm.SetSession(sess)

	if err := pm.SaveRecoveryPoint(); err != nil {
		t.Fatalf("SaveRecoveryPoint: %v", err)
	}

	if !pm.HasRecoveryState() {
		t.Error("HasRecoveryState = false, want true")
	}

	state, err := pm.LoadRecoveryPoint()
	if err != nil {
		t.Fatalf("LoadRecoveryPoint: %v", err)
	}
	if state.Session.Title != "Recovery Test" {
		t.Errorf("Title = %q, want %q", state.Session.Title, "Recovery Test")
	}

	if err := pm.ClearRecoveryPoint(); err != nil {
		t.Fatalf("ClearRecoveryPoint: %v", err)
	}
	if pm.HasRecoveryState() {
		t.Error("HasRecoveryState = true after clear, want false")
	}
}

func TestPersistenceManager_AutoSave(t *testing.T) {
	dir := t.TempDir()
	pm, err := NewPersistenceManager(dir)
	if err != nil {
		t.Fatalf("NewPersistenceManager: %v", err)
	}

	sess := New()
	pm.SetSession(sess)

	pm.StartAutoSave(1) // 1 second interval
	defer pm.StopAutoSave()

	// Make a change
	pm.SetContext("test", "value")

	// Wait for auto-save
	time.Sleep(1500 * time.Millisecond)

	// Verify saved
	pm2, _ := NewPersistenceManager(dir)
	_ = pm2.Load()
	if v, ok := pm2.GetContext("test"); !ok || v != "value" {
		t.Error("Auto-save did not persist context")
	}
}

func TestPersistenceManager_IsDirty(t *testing.T) {
	dir := t.TempDir()
	pm, err := NewPersistenceManager(dir)
	if err != nil {
		t.Fatalf("NewPersistenceManager: %v", err)
	}

	pm.SetContext("key", "value")
	if !pm.IsDirty() {
		t.Error("IsDirty = false after change, want true")
	}

	if err := pm.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if pm.IsDirty() {
		t.Error("IsDirty = true after save, want false")
	}
}

func TestPersistenceManager_ResumeSession(t *testing.T) {
	dir := t.TempDir()
	storage, err := NewStorage(dir)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	// Create and save a session
	sess := New()
	sess.Title = "Resume Test"
	if err := storage.Save(sess); err != nil {
		t.Fatalf("Save session: %v", err)
	}

	pm, err := NewPersistenceManager(dir)
	if err != nil {
		t.Fatalf("NewPersistenceManager: %v", err)
	}

	// Resume the session
	resumed, err := pm.ResumeSession(sess.ID, storage)
	if err != nil {
		t.Fatalf("ResumeSession: %v", err)
	}
	if resumed.Title != "Resume Test" {
		t.Errorf("Title = %q, want %q", resumed.Title, "Resume Test")
	}
}

func TestResumeSessionPrefersRicherCleanHistoryOverStaleRecovery(t *testing.T) {
	dir := t.TempDir()
	storage, err := NewStorage(dir)
	if err != nil {
		t.Fatal(err)
	}
	clean := New()
	clean.AddMessage(ai.NewTextMessage(ai.RoleUser, "one"))
	clean.AddMessage(ai.NewTextMessage(ai.RoleAssistant, "two"))
	clean.AddMessage(ai.NewTextMessage(ai.RoleUser, "three"))
	if err := storage.Save(clean); err != nil {
		t.Fatal(err)
	}
	recovery := New()
	recovery.ID = clean.ID
	recovery.AddMessage(ai.NewTextMessage(ai.RoleUser, "one"))
	pm, err := NewPersistenceManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	pm.SetSession(recovery)
	if err := pm.SaveRecoveryPoint(); err != nil {
		t.Fatal(err)
	}
	got, err := pm.ResumeSession(clean.ID, storage)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Messages) != len(clean.Messages) {
		t.Fatalf("resume returned stale recovery history: got %d messages, want %d", len(got.Messages), len(clean.Messages))
	}
}
