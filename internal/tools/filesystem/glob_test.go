package filesystem

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func createTestTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := []string{
		"tuidesign.md",
		"app.go",
		"internal/tui/app.go",
		"internal/tui/sub/file.go",
		"internal/installer/tui.go",
		"internal/config/config.go",
	}
	for _, f := range files {
		p := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func globResult(t *testing.T, root, pattern string) string {
	t.Helper()
	tool := &GlobTool{}
	res, err := tool.Execute(context.Background(), map[string]any{
		"pattern":    pattern,
		"path":       root,
		"max_results": 100,
	})
	if err != nil {
		t.Fatalf("%s: unexpected error: %v", pattern, err)
	}
	if res.IsError {
		t.Fatalf("%s: tool error: %s", pattern, res.Content)
	}
	return res.Content
}

func contains(content, file string) bool {
	for _, line := range strings.Split(content, "\n") {
		if line == file || strings.HasSuffix(line, "/"+file) {
			return true
		}
	}
	return false
}

func TestGlobMatchStarAcrossSegments(t *testing.T) {
	root := createTestTree(t)

	got := globResult(t, root, "**/*tui*")
	if strings.Contains(got, "no matches found") {
		t.Fatalf("**/*tui*: no matches found")
	}
	for _, want := range []string{"tuidesign.md", "internal/tui/app.go", "internal/installer/tui.go", "internal/tui/sub/file.go"} {
		if !contains(got, want) {
			t.Errorf("**/*tui*: missing %s in: %s", want, got)
		}
	}
	if contains(got, "app.go") && !strings.Contains(got, "tui") && got != "tuidesign.md" {
		t.Errorf("**/*tui*: unexpected plain app.go")
	}
}

func TestGlobSlashPattern(t *testing.T) {
	root := createTestTree(t)

	got := globResult(t, root, "internal/tui/*.go")
	if strings.Contains(got, "no matches found") {
		t.Fatalf("internal/tui/*.go: no matches found")
	}
	if !contains(got, "internal/tui/app.go") {
		t.Errorf("missing internal/tui/app.go: %s", got)
	}
	if contains(got, "internal/tui/sub/file.go") {
		t.Errorf("* must not cross /: got %s", got)
	}
}

func TestGlobDoublestarAll(t *testing.T) {
	root := createTestTree(t)

	got := globResult(t, root, "**/*")
	if strings.Contains(got, "no matches found") {
		t.Fatalf("**/*: no matches found")
	}
	for _, want := range []string{"tuidesign.md", "app.go", "internal/tui/app.go", "internal/tui/sub/file.go", "internal/installer/tui.go", "internal/config/config.go"} {
		if !contains(got, want) {
			t.Errorf("**/*: missing %s in: %s", want, got)
		}
	}
}

func TestGlobPlainStarDoesNotCrossSlash(t *testing.T) {
	root := createTestTree(t)

	got := globResult(t, root, "*tui*")
	if !contains(got, "tuidesign.md") {
		t.Errorf("*tui* should match tuidesign.md (base name), got: %s", got)
	}
	if contains(got, "internal/tui/app.go") {
		t.Errorf("*tui* must not match across segments, got: %s", got)
	}
}

func TestGlobNoMatches(t *testing.T) {
	root := createTestTree(t)

	got := globResult(t, root, "zzz*")
	if !strings.Contains(got, "no matches found") {
		t.Errorf("expected no matches found, got: %s", got)
	}
}

func TestGlobLiteral(t *testing.T) {
	root := createTestTree(t)

	got := globResult(t, root, "internal/config/config.go")
	if !contains(got, "internal/config/config.go") {
		t.Errorf("literal path should match, got: %s", got)
	}
}

func TestGlobRelativeRoot(t *testing.T) {
	// Regression: root "." must not be skipped as hidden (d.Name() == ".")
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	got := globResult(t, ".", "**/*")
	if strings.Contains(got, "no matches found") {
		t.Fatalf("**/* from relative root '.': %s", got)
	}
	if !contains(got, "file.txt") {
		t.Errorf("missing file.txt: %s", got)
	}
}
