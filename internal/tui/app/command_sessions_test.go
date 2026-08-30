package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/session"
)

func TestResolveSessionID(t *testing.T) {
	app := newTestApp(t)
	storage, err := session.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app.storage = storage
	app.workDir = "/projects/alpha"

	mk := func(title, id string) *session.Session {
		s := session.New()
		s.ID = id
		s.Title = title
		s.WorkDir = "/projects/alpha"
		return s
	}
	for _, s := range []*session.Session{
		mk("fix parser bug", "aaaaaaaa-1111-1111-1111-111111111111"),
		mk("refactor tools", "aaaaaaaa-2222-2222-2222-222222222222"),
		mk("unrelated", "bbbbbbbb-3333-3333-3333-333333333333"),
	} {
		if err := storage.Save(s); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name    string
		ref     string
		want    string
		wantErr string
	}{
		{"exact id", "bbbbbbbb-3333-3333-3333-333333333333", "bbbbbbbb-3333-3333-3333-333333333333", ""},
		{"unique prefix", "bbbbbbbb-3", "bbbbbbbb-3333-3333-3333-333333333333", ""},
		{"ambiguous prefix", "aaaaaaaa", "", "ambiguous"},
		{"title match", "parser", "aaaaaaaa-1111-1111-1111-111111111111", ""},
		{"nth recent", "#1", "", ""}, // newest of the three, order depends on UpdatedAt ticks
		{"nth out of range", "#9", "", "out of range"},
		{"no match", "does-not-exist", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := app.resolveSessionID(tt.ref)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("want error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tt.want != "" && got != tt.want {
				t.Fatalf("want %q, got %q", tt.want, got)
			}
			if tt.want == "" && tt.name == "nth recent" && got == "" {
				t.Fatal("#1 must resolve to a session")
			}
		})
	}
}

func TestResumeByPrefixResumes(t *testing.T) {
	app := newTestApp(t)
	storage, err := session.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app.storage = storage
	app.workDir = ""

	saved := session.New()
	saved.Title = "inspect main.go"
	saved.AddMessage(ai.NewTextMessage(ai.RoleUser, "check the file"))
	if err := storage.Save(saved); err != nil {
		t.Fatal(err)
	}

	if err := app.resumeSession(saved.ID[:8]); err != nil {
		t.Fatalf("resume by prefix failed: %v", err)
	}
	if app.sess.ID != saved.ID {
		t.Fatalf("expected session %s restored, got %s", saved.ID, app.sess.ID)
	}
	view := ansiStrip(app.conversation.View())
	if !strings.Contains(view, "inspect main.go") && !strings.Contains(view, "check the file") {
		t.Fatalf("resumed conversation missing content:\n%s", view)
	}
}

func TestResumeRefusesWhileAgentRunning(t *testing.T) {
	app := newTestApp(t)
	storage, err := session.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app.storage = storage
	saved := session.New()
	if err := storage.Save(saved); err != nil {
		t.Fatal(err)
	}
	app.thinking = true
	defer func() { app.thinking = false }()
	if err := app.resumeSession(saved.ID); err == nil {
		t.Fatal("expected resume to be refused while agent runs")
	}
}

func TestDeleteSession(t *testing.T) {
	app := newTestApp(t)
	storage, err := session.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app.storage = storage

	other := session.New()
	other.Title = "deleteme"
	if err := storage.Save(other); err != nil {
		t.Fatal(err)
	}

	if err := app.deleteSession(other.ID); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if _, err := storage.Load(other.ID); err == nil {
		t.Fatal("expected session removed from storage")
	}

	if err := app.deleteSession(app.sess.ID); err == nil {
		t.Fatal("expected deleting the active session to be refused")
	}
}

func TestSessionBrowserDeleteKeyRequiresConfirmation(t *testing.T) {
	app := newTestApp(t)
	storage, err := session.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app.storage = storage
	saved := session.New()
	saved.Title = "victim"
	if err := storage.Save(saved); err != nil {
		t.Fatal(err)
	}
	app.handleSlashCommand("/sessions")
	app.sessionBrowser.SetCurrent(app.sess.ID)
	app.sessionBrowser.SetSize(80, 24)

	// First "d" arms, second deletes.
	sb, _ := app.sessionBrowser.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	if !strings.Contains(sb.View(), "delete?") {
		t.Fatal("expected armed-delete indicator in view")
	}
	_, cmd := sb.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("expected SessionDeletedMsg after second d")
	}
	// Deliver the emitted message through the app so the delete actually runs.
	app.Update(cmd())
	if _, err := storage.Load(saved.ID); err == nil {
		t.Fatal("expected session deleted from storage")
	}
	if app.sessionBrowser.ItemCount() != 0 {
		t.Fatalf("expected browser list emptied, got %d", app.sessionBrowser.ItemCount())
	}
}

func TestSessionBrowserRefusesDeletingCurrent(t *testing.T) {
	app := newTestApp(t)
	app.handleSlashCommand("/sessions")
	// The browser holds the active session: arming and pressing d again must
	// not emit a delete.
	sb, _ := app.sessionBrowser.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	sb2, cmd := sb.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	if cmd != nil {
		t.Fatal("delete of active session must not emit a message")
	}
	if strings.Contains(sb2.View(), "delete?") {
		t.Fatal("active session must not show delete indicator")
	}
}
