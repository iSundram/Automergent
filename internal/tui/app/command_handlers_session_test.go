package app

import (
	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/session"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportConversationMarkdownAndRejectsAbsolutePath(t *testing.T) {
	app := newTestApp(t)
	app.sess.AddMessage(ai.NewTextMessage(ai.RoleUser, "hello"))
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)
	if err := app.exportConversation("chat.md"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "chat.md"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); !strings.Contains(got, "# Automergent Conversation") || !strings.Contains(got, "## User\n\nhello") {
		t.Fatalf("unexpected export:\n%s", got)
	}
	if err := app.exportConversation(filepath.Join(dir, "unsafe.md")); err == nil {
		t.Fatal("expected absolute path rejection")
	}
	if err := app.exportConversation("../outside.md"); err == nil {
		t.Fatal("expected workspace traversal rejection")
	}
}

func TestSearchWorkspaceFindsTextAndSkipsGit(t *testing.T) {
	app := newTestApp(t)
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)
	if err := os.WriteFile("main.go", []byte("package main\n// unique needle\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(".git", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".git/hidden", []byte("needle"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := app.searchWorkspace("needle")
	if !strings.Contains(result, "main.go:2") || strings.Contains(result, ".git/hidden") {
		t.Fatalf("unexpected search result: %s", result)
	}
}

func TestResumeSessionByID(t *testing.T) {
	app := newTestApp(t)
	storage, err := session.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	saved := session.New()
	saved.Provider, saved.Model = app.cfg.Provider, app.cfg.Model
	saved.AddMessage(ai.NewTextMessage(ai.RoleUser, "saved message"))
	if err := storage.Save(saved); err != nil {
		t.Fatal(err)
	}
	app.storage = storage
	app.handleSlashCommand("/resume " + saved.ID)
	if app.sess.ID != saved.ID || app.conversation.MessageCount() != 1 {
		t.Fatalf("session was not resumed: id=%s messages=%d", app.sess.ID, app.conversation.MessageCount())
	}
}

func TestResumeSessionByIDRestoresStructuredConversation(t *testing.T) {
	app := newTestApp(t)
	storage, err := session.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	saved := session.New()
	saved.Provider, saved.Model = app.cfg.Provider, app.cfg.Model
	saved.AddMessage(ai.NewTextMessage(ai.RoleUser, "inspect main.go"))
	saved.AddMessage(ai.Message{Role: ai.RoleAssistant, Content: []ai.ContentPart{
		{Type: ai.ContentTypeThought, Thought: "checking the file"},
		{Type: ai.ContentTypeToolCall, ToolCall: &ai.ToolCall{ID: "call-1", Name: "read_file", Args: map[string]any{"path": "main.go"}}},
	}})
	saved.AddMessage(ai.Message{Role: ai.RoleTool, Content: []ai.ContentPart{
		{Type: ai.ContentTypeToolResult, ToolResult: &ai.ToolResult{ToolCallID: "call-1", Content: "package main"}},
	}})
	if err := storage.Save(saved); err != nil {
		t.Fatal(err)
	}
	app.storage = storage
	app.handleSlashCommand("/resume " + saved.ID)
	view := ansiStrip(app.conversation.View())
	for _, want := range []string{"inspect main.go", "checking the file", "Readfile", "Completed"} {
		if !strings.Contains(view, want) {
			t.Fatalf("direct resume missing %q:\n%s", want, view)
		}
	}
}

func TestNewSessionSavesCurrentHistory(t *testing.T) {
	app := newTestApp(t)
	storage, err := session.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app.storage = storage
	oldID := app.sess.ID
	app.sess.AddMessage(ai.NewTextMessage(ai.RoleUser, "keep me"))
	app.handleSlashCommand("/new")
	if app.sess.ID == oldID {
		t.Fatal("expected a fresh session")
	}
	loaded, err := storage.Load(oldID)
	if err != nil || len(loaded.Messages) != 1 {
		t.Fatalf("old session not saved: %v", err)
	}
}

func TestProjectSessionsFiltersByWorkDir(t *testing.T) {
	app := newTestApp(t)
	app.workDir = "/projects/alpha"

	current := session.New()
	current.Title = "Current project"
	current.WorkDir = "/projects/alpha"

	other := session.New()
	other.Title = "Other project"
	other.WorkDir = "/projects/beta"

	legacy := session.New()
	legacy.Title = "Legacy (no workdir)"

	filtered := app.projectSessions([]*session.Session{current, other, legacy})
	if len(filtered) != 2 {
		t.Fatalf("expected 2 sessions (current + legacy), got %d", len(filtered))
	}
	if filtered[0].Title != "Current project" || filtered[1].Title != "Legacy (no workdir)" {
		t.Fatalf("unexpected filter result: %+v", filtered)
	}
}

func TestShowSessionsPickerShowsProjectSessions(t *testing.T) {
	app := newTestApp(t)
	app.workDir = "/projects/alpha"
	storage, err := session.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app.storage = storage

	current := session.New()
	current.Title = "Current project"
	current.WorkDir = "/projects/alpha"
	if err := storage.Save(current); err != nil {
		t.Fatal(err)
	}
	other := session.New()
	other.Title = "Other project"
	other.WorkDir = "/projects/beta"
	if err := storage.Save(other); err != nil {
		t.Fatal(err)
	}

	app.handleSlashCommand("/sessions")
	if !app.sessionBrowser.Visible() {
		t.Fatal("expected session picker to be open")
	}
	if app.sessionBrowser.ItemCount() != 1 {
		t.Fatalf("expected 1 project session in picker, got %d", app.sessionBrowser.ItemCount())
	}

	app.sessionBrowser.Hide()
	app.handleSlashCommand("/session")
	if !app.sessionBrowser.Visible() {
		t.Fatal("expected /session alias to open the session picker")
	}

	app.sessionBrowser.Hide()
	app.handleSlashCommand("/resume")
	if !app.sessionBrowser.Visible() {
		t.Fatal("expected /resume to open the session picker")
	}
	if app.sessionBrowser.ItemCount() != 1 {
		t.Fatalf("expected 1 project session in picker, got %d", app.sessionBrowser.ItemCount())
	}
}

func TestApprovalsCommandListsAndRevokes(t *testing.T) {
	app := newTestApp(t)
	storage, err := session.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app.storage = storage

	app.sess.AddApproval(`project=/p;a;name="run_shell_command";action=write;risk=high`, "tui")
	app.sess.AddApproval(`name="edit_file";action=write;risk=low`, "tui")

	app.handleSlashCommand("/approvals")
	view := app.conversation.View()
	if !strings.Contains(view, "run_shell_command write") || !strings.Contains(view, "edit_file write") {
		t.Fatalf("approval listing missing entries:\n%s", view)
	}

	app.handleSlashCommand("/approvals revoke 1")
	if app.sess.HasApproval(`project=/p;a;name="run_shell_command";action=write;risk=high`) {
		t.Fatal("expected approval to be revoked")
	}
	if !app.sess.HasApproval(`name="edit_file";action=write;risk=low`) {
		t.Fatal("unrelated approval must be preserved")
	}

	// Revoked approval survives persistence.
	loaded, err := storage.Load(app.sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.AllowedTools) != 1 {
		t.Fatalf("expected 1 persisted approval, got %+v", loaded.AllowedTools)
	}

	app.handleSlashCommand("/approvals revoke 99")
	app.handleSlashCommand("/approvals revoke nope")
}
