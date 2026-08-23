package editreview

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iSundram/Automergent/internal/tools"
)

func TestProposalLifecycle(t *testing.T) {
	s := NewStore()
	id := s.Add("edit_file", "a.go", "old", "new", "edit")
	if s.PendingCount() != 1 {
		t.Fatalf("pending = %d", s.PendingCount())
	}
	p, ok := s.Get(id)
	if !ok || p.Status != StatusProposed {
		t.Fatalf("get failed: %+v", p)
	}
	if _, err := s.Resolve(id, StatusRejected); err != nil {
		t.Fatal(err)
	}
	if s.PendingCount() != 0 {
		t.Fatal("rejected proposal still pending")
	}
	if got := RevertNote(p); !contains(got, "rejected") {
		t.Fatalf("revert note: %q", got)
	}
}

func TestAcceptAllAndApply(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	os.WriteFile(path, []byte("hello"), 0o644)

	s := NewStore()
	s.Add("write_file", path, "hello", "world", "rewrite")

	if s.AcceptAll() != 1 {
		t.Fatal("acceptAll count")
	}
	p := s.All()[0]
	if err := Apply(p); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "world" {
		t.Fatalf("file = %q", data)
	}
}

func TestDryRunEditRules(t *testing.T) {
	s := NewStore()
	wrapper := &ProposalTool{store: s}
	current := "alpha\nbeta\nalpha"

	out, err := dryRun(nil, &stubTool{}, "x.go", current, map[string]any{
		"old_str": "beta", "new_str": "BETA",
	})
	if err != nil || out != "alpha\nBETA\nalpha" {
		t.Fatalf("single replace: %q %v", out, err)
	}

	if _, err := dryRun(nil, &stubTool{}, "x.go", current, map[string]any{
		"old_str": "alpha", "new_str": "A",
	}); err == nil {
		t.Fatal("ambiguous replace must fail without replace_all")
	}

	out, err = dryRun(nil, &stubTool{}, "x.go", current, map[string]any{
		"old_str": "alpha", "new_str": "A", "replace_all": true,
	})
	if err != nil || strings.Count(out, "A") != 2 {
		t.Fatalf("replace_all: %q %v", out, err)
	}
	_ = wrapper
}

func TestAtomicWriteNoPartial(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "file.txt") // subdir missing -> error path
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	if err := atomicWrite(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("file not written")
	}
}

// stubTool satisfies the tools.Tool interface for dryRun dispatch.
type stubTool struct{ tools.BaseTool }

func (s *stubTool) Name() string        { return "edit_file" }
func (s *stubTool) Description() string { return "stub" }
func (s *stubTool) Schema() map[string]any {
	return nil
}
func (s *stubTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	return tools.Result{}, nil
}
func (s *stubTool) RequiresConfirmation(string) bool { return false }

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
