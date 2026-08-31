package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iSundram/Automergent/internal/ai"
)

func mustSave(t *testing.T, s *Storage, sess *Session) {
	t.Helper()
	if err := s.Save(sess); err != nil {
		t.Fatalf("Save: %v", err)
	}
}

// Saving twice with one new message must append exactly one message entry —
// the whole history is never rewritten.
func TestTranscript_DeltaAppend(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStorage(dir)
	sess := New()
	sess.WorkDir = "/repo/alpha"
	sess.AddMessage(ai.NewTextMessage(ai.RoleUser, "hello"))
	mustSave(t, s, sess)

	path := s.transcriptPath(sanitizeProjectDir(sess.WorkDir), sess.ID)
	before := transcriptLines(t, path)

	sess.AddMessage(ai.NewTextMessage(ai.RoleAssistant, "world"))
	mustSave(t, s, sess)

	after := transcriptLines(t, path)
	// One message entry plus (by design) a refreshed summary marker.
	if len(after)-len(before) > 2 {
		t.Fatalf("delta save appended %d lines, want ≤2 (before=%d after=%d)",
			len(after)-len(before), len(before), len(after))
	}
	var msgEntries int
	for _, l := range after {
		var e Entry
		if json.Unmarshal([]byte(l), &e) == nil && e.Type == entryMessage {
			msgEntries++
		}
	}
	// First save of a session with history writes a snapshot epoch, not
	// per-message entries; the delta is the single appended message entry.
	if msgEntries != 1 {
		t.Fatalf("message entries = %d, want 1", msgEntries)
	}

	loaded, err := s.Load(sess.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Messages) != 2 {
		t.Fatalf("loaded messages = %d, want 2", len(loaded.Messages))
	}
	if loaded.WorkDir != sess.WorkDir {
		t.Fatalf("workdir lost: %q", loaded.WorkDir)
	}
}

// Rewind (array shrinks) must persist as a snapshot epoch, and the loaded
// session must reflect the rewound history.
func TestTranscript_RewindPersistsSnapshot(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStorage(dir)
	sess := New()
	sess.AddMessage(ai.NewTextMessage(ai.RoleUser, "one"))
	sess.AddMessage(ai.NewTextMessage(ai.RoleAssistant, "two"))
	sess.AddMessage(ai.NewTextMessage(ai.RoleUser, "three"))
	mustSave(t, s, sess)

	// Rewind: drop the last message.
	sess.Messages = sess.Messages[:2]
	mustSave(t, s, sess)

	loaded, err := s.Load(sess.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Messages) != 2 {
		t.Fatalf("loaded messages after rewind = %d, want 2", len(loaded.Messages))
	}

	// Continuing after rewind appends deltas onto the new epoch.
	sess.AddMessage(ai.NewTextMessage(ai.RoleUser, "four"))
	mustSave(t, s, sess)
	loaded, err = s.Load(sess.ID)
	if err != nil {
		t.Fatalf("Load after continue: %v", err)
	}
	if len(loaded.Messages) != 3 {
		t.Fatalf("loaded messages after continue = %d, want 3", len(loaded.Messages))
	}
}

// Large tool results are offloaded to disk and restored on load.
func TestTranscript_ToolResultOffload(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStorage(dir)
	big := strings.Repeat("x", inlineToolResultMax+4096)
	sess := New()
	sess.AddMessage(ai.Message{
		Role: ai.RoleTool,
		Content: []ai.ContentPart{{
			Type:       ai.ContentTypeToolResult,
			ToolResult: &ai.ToolResult{ToolCallID: "tc1", Content: big},
		}},
	})
	mustSave(t, s, sess)

	// The transcript itself must not contain the big payload inline.
	path := s.findTranscript(sess.ID)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	if strings.Contains(string(raw), strings.Repeat("x", 1024)) {
		t.Fatal("oversized tool result persisted inline")
	}
	// json.Marshal HTML-escapes '<', so match without it.
	if !strings.Contains(string(raw), "persisted-output file=") {
		t.Fatal("persisted-output marker missing")
	}

	loaded, err := s.Load(sess.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(loaded.Messages))
	}
	tr := loaded.Messages[0].Content[0].ToolResult
	if tr == nil || tr.Content != big {
		t.Fatalf("tool result not restored from offload (len=%d)", len(tr.Content))
	}
}

// Listing reads only head/tail windows: Messages must be empty but identity,
// title and totals must be correct even for a large transcript.
func TestTranscript_LiteListing(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStorage(dir)
	sess := New()
	sess.WorkDir = "/repo/beta"
	sess.Title = "My Session"
	sess.TotalInputTokens = 1234
	sess.AddMessage(ai.NewTextMessage(ai.RoleUser, "first prompt here"))
	for i := 0; i < 200; i++ {
		sess.AddMessage(ai.NewTextMessage(ai.RoleAssistant, strings.Repeat("m", 500)))
	}
	mustSave(t, s, sess)

	sessions, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
	got := sessions[0]
	if got.ID != sess.ID {
		t.Fatalf("id = %q", got.ID)
	}
	if got.Title != "My Session" {
		t.Fatalf("title = %q, want %q", got.Title, "My Session")
	}
	if got.TotalInputTokens != 1234 {
		t.Fatalf("input tokens = %d, want 1234", got.TotalInputTokens)
	}
	if len(got.Messages) != 0 {
		t.Fatalf("lite listing returned %d messages, want 0", len(got.Messages))
	}
	if got.UpdatedAt.IsZero() {
		t.Fatal("updated-at missing in lite listing")
	}
}

// A legacy single-file JSON session loads, and its next Save migrates it to
// the transcript format (the legacy file disappears).
func TestTranscript_LegacyMigration(t *testing.T) {
	dir := t.TempDir()
	// Write a legacy session by hand.
	legacy := New()
	legacy.WorkDir = "/repo/gamma"
	legacy.Title = "old format"
	legacy.AddMessage(ai.NewTextMessage(ai.RoleUser, "legacy hello"))
	data, err := json.MarshalIndent(legacy, "", "  ")
	if err != nil {
		t.Fatalf("marshal legacy: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, legacy.ID+".json"), data, 0o600); err != nil {
		t.Fatalf("write legacy: %v", err)
	}

	s, _ := NewStorage(dir)

	// Listed and loadable before migration.
	sessions, err := s.List()
	if err != nil || len(sessions) != 1 || sessions[0].ID != legacy.ID {
		t.Fatalf("legacy not listed: %v %d", err, len(sessions))
	}
	loaded, err := s.Load(legacy.ID)
	if err != nil || len(loaded.Messages) != 1 || loaded.Title != "old format" {
		t.Fatalf("legacy load failed: %v", err)
	}

	// Saving migrates to transcript format.
	mustSave(t, s, loaded)
	if _, err := os.Stat(filepath.Join(dir, legacy.ID+".json")); !os.IsNotExist(err) {
		t.Fatal("legacy file still present after migration save")
	}
	if s.findTranscript(legacy.ID) == "" {
		t.Fatal("transcript not created by migration save")
	}

	// And it still lists exactly once.
	sessions, err = s.List()
	if err != nil || len(sessions) != 1 {
		t.Fatalf("post-migration listing: %v count=%d", err, len(sessions))
	}
}

// A torn tail line (crash mid-append) must not lose the messages before it.
func TestTranscript_TornTailTolerated(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStorage(dir)
	sess := New()
	sess.AddMessage(ai.NewTextMessage(ai.RoleUser, "one"))
	mustSave(t, s, sess)

	path := s.findTranscript(sess.ID)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	f.WriteString(`{"type":"message","ts":"20`) // torn JSON
	f.Close()

	loaded, err := s.Load(sess.ID)
	if err != nil {
		t.Fatalf("Load with torn tail: %v", err)
	}
	if len(loaded.Messages) != 1 {
		t.Fatalf("messages = %d, want 1 (torn tail must not lose history)", len(loaded.Messages))
	}
}

// Two saves from different Storage instances (like two processes) must not
// duplicate history: the second rebuilds its delta state from the file.
func TestTranscript_SecondProcessNoDuplication(t *testing.T) {
	dir := t.TempDir()
	first, _ := NewStorage(dir)
	sess := New()
	sess.AddMessage(ai.NewTextMessage(ai.RoleUser, "one"))
	sess.AddMessage(ai.NewTextMessage(ai.RoleAssistant, "two"))
	mustSave(t, first, sess)

	second, _ := NewStorage(dir)
	sess.AddMessage(ai.NewTextMessage(ai.RoleUser, "three"))
	mustSave(t, second, sess)

	loaded, err := second.Load(sess.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Messages) != 3 {
		t.Fatalf("messages = %d, want 3 (history duplicated or lost)", len(loaded.Messages))
	}
}

func transcriptLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	var lines []string
	for _, l := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	return lines
}
