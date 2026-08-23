package render

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// OSC 8 terminal hyperlinks with graceful degradation (blueprint §9):
// unsupported terminals simply ignore the escape and show the label.

// HyperlinksEnabled reports whether the current terminal is known to support
// OSC 8. Detection is allowlist-based on TERM/TERMs plus explicit vars.
func HyperlinksEnabled() bool {
	if os.Getenv("NO_HYPERLINKS") != "" {
		return false
	}
	for _, v := range []string{"KITTY_WINDOW_ID", "WEZTERM_EXECUTABLE", "GHOSTTY_RESOURCES_DIR", "ALACRITTY_LOG"} {
		if os.Getenv(v) != "" {
			return true
		}
	}
	term := strings.ToLower(os.Getenv("TERM"))
	termProgram := strings.ToLower(os.Getenv("TERM_PROGRAM"))
	for _, marker := range []string{
		"kitty", "wezterm", "ghostty", "alacritty", "foot", "contour",
		"xterm-256color", "iterm2", "vscode",
	} {
		if strings.Contains(term, marker) || strings.Contains(termProgram, marker) {
			return true
		}
	}
	return false
}

// Hyperlink wraps label in an OSC 8 sequence pointing at uri. When links are
// disabled or label is empty the label returns unchanged.
func Hyperlink(uri, label string) string {
	label = strings.TrimSpace(label)
	if label == "" || !HyperlinksEnabled() {
		return label
	}
	return fmt.Sprintf("\x1b]8;;%s\x1b\\%s\x1b]8;;\x1b\\", uri, label)
}

// FileLink renders a clickable link for a filesystem path that exists,
// resolving to an absolute file:// URI. Non-existent paths return unchanged.
func FileLink(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || !HyperlinksEnabled() {
		return path
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	if _, err := os.Stat(abs); err != nil {
		return path // only link what exists; avoids fake URLs
	}
	return Hyperlink("file://"+abs, path)
}
