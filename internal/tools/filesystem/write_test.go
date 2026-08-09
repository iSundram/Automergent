package filesystem

import (
	"os"
	"testing"
)

func TestPatchFileReplaceAll(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/a.txt"
	if err := os.WriteFile(path, []byte("x a x a"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	tool := &PatchFileTool{}
	res, err := tool.Execute(nil, map[string]any{
		"path":        path,
		"old_str":     "a",
		"new_str":     "b",
		"replace_all": true,
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got error: %s", res.Content)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "x b x b" {
		t.Fatalf("unexpected content: %q", string(got))
	}
}
