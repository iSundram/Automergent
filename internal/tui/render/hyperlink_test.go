package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHyperlinkDisabledByDefault(t *testing.T) {
	t.Setenv("NO_HYPERLINKS", "1")
	t.Setenv("TERM", "xterm-256color")
	if got := Hyperlink("file:///tmp/x", "x"); got != "x" {
		t.Fatalf("expected passthrough when disabled, got %q", got)
	}
	if got := FileLink("definitely-missing-file.xyz"); got != "definitely-missing-file.xyz" {
		t.Fatalf("missing path must pass through: %q", got)
	}
}

func TestHyperlinkEnabledForKnownTerm(t *testing.T) {
	t.Setenv("NO_HYPERLINKS", "")
	t.Setenv("TERM", "xterm-kitty")
	out := Hyperlink("https://example.com", "example")
	if !strings.HasPrefix(out, "\x1b]8;;https://example.com\x1b\\") {
		t.Fatalf("expected OSC8 wrap, got %q", out)
	}

	dir := t.TempDir()
	f := filepath.Join(dir, "main.go")
	os.WriteFile(f, []byte("package main"), 0o644)
	link := FileLink(f)
	if !strings.Contains(link, "file://") {
		t.Fatalf("FileLink should emit file URI: %q", link)
	}
}
