package app

// Pure helpers: diffs, error formatting, arg/context extraction.
// Moved verbatim from internal/tui/app.go.

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/iSundram/Automergent/internal/tui/render"
)

func computeSimpleDiff(filename, old, new string) string {
	// The engine lives in render so the conversation's inline edit cards
	// synthesize their previews from the exact same code — one diff language
	// everywhere. This wrapper only adds the fullscreen viewer's chrome.
	body := render.SimpleDiff(old, new)
	if body == "" {
		return ""
	}

	oldLines := strings.Count(old, "\n")
	if !strings.HasSuffix(old, "\n") && old != "" {
		oldLines++
	}
	newLines := strings.Count(new, "\n")
	if !strings.HasSuffix(new, "\n") && new != "" {
		newLines++
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("--- %s (current)\n", filename))
	sb.WriteString(fmt.Sprintf("+++ %s (proposed)\n", filename))
	sb.WriteString(fmt.Sprintf("@@ -1,%d +1,%d @@\n", oldLines, newLines))
	sb.WriteString(body)
	sb.WriteString("\n")
	return sb.String()
}

func formatErrorMessage(errStr string) string {
	// First, sanitize any URLs to hide API keys
	errStr = sanitizeURLs(errStr)

	// Handle user cancellation gracefully
	if isCancellationError(errStr) {
		return "Request cancelled"
	}
	if strings.Contains(errStr, "401") || strings.Contains(errStr, "authentication_error") {
		return "API Key missing or invalid. Use /api-key <value> to set it, or export the appropriate environment variable (e.g., GEMINI_API_KEY)."
	}
	if strings.Contains(errStr, "403") {
		return "API access forbidden. Check your account permissions or billing status."
	}
	if strings.Contains(errStr, "429") {
		return "Rate limit exceeded. Please wait a moment before trying again."
	}
	if strings.Contains(errStr, "connection refused") || strings.Contains(errStr, "no such host") {
		return "Connection failed. Check your network or API endpoint configuration."
	}
	if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline exceeded") {
		return "Request timed out. The server may be busy, please try again."
	}
	return "Error: " + errStr
}

func isCancellationError(errStr string) bool {
	s := strings.ToLower(errStr)
	return strings.Contains(s, "context canceled") || strings.Contains(s, "context cancelled")
}

// sanitizeURLs removes sensitive data (API keys, tokens) from URLs in error messages
func sanitizeURLs(s string) string {
	// Pattern to match common API key patterns in URLs
	patterns := []struct {
		pattern string
		replace string
	}{
		// ?key=xxx or &key=xxx
		{`([?&])key=[^&\s"']+`, `$1key=***`},
		// ?api_key=xxx or &api_key=xxx
		{`([?&])api_key=[^&\s"']+`, `$1api_key=***`},
		// ?apikey=xxx or &apikey=xxx
		{`([?&])apikey=[^&\s"']+`, `$1apikey=***`},
		// ?token=xxx or &token=xxx
		{`([?&])token=[^&\s"']+`, `$1token=***`},
		// ?access_token=xxx
		{`([?&])access_token=[^&\s"']+`, `$1access_token=***`},
		// Bearer tokens in headers shown in errors
		{`Bearer\s+[A-Za-z0-9_\-\.]+`, `Bearer ***`},
		// x-api-key header values
		{`x-api-key:\s*[^\s"']+`, `x-api-key: ***`},
	}

	result := s
	for _, p := range patterns {
		re := regexp.MustCompile(p.pattern)
		result = re.ReplaceAllString(result, p.replace)
	}
	return result
}

func isTransientStatus(s string) bool {
	n := strings.ToLower(strings.TrimSpace(s))
	if n == "thinking" || n == "thinking…" || n == "thinking..." {
		return true
	}
	return strings.HasPrefix(n, "running ")
}

func appendUniquePath(paths []string, path string) []string {
	for _, existing := range paths {
		if existing == path {
			return paths
		}
	}
	return append(paths, path)
}

func initActionArgs(rawTool, target string) map[string]any {
	switch rawTool {
	case "read":
		return map[string]any{"path": target}
	case "glob", "grep":
		return map[string]any{"pattern": target}
	case "bash":
		return map[string]any{"command": target}
	default:
		return map[string]any{"target": target}
	}
}

func extractToolContext(name string, args map[string]any) string {
	if args == nil {
		return ""
	}
	switch name {
	case "read_file", "view":
		if path, ok := args["path"].(string); ok {
			return "reading " + filepath.Base(path)
		}
	case "write_file":
		if path, ok := args["path"].(string); ok {
			return "writing " + filepath.Base(path)
		}
	case "edit_file":
		if path, ok := args["path"].(string); ok {
			return "editing " + filepath.Base(path)
		}
	case "list_directory":
		if path, ok := args["path"].(string); ok {
			return "listing " + filepath.Base(path)
		}
	case "run_shell_command":
		if cmd, ok := args["command"].(string); ok {
			if len(cmd) > 40 {
				return "exec: " + cmd[:37] + "..."
			}
			return "exec: " + cmd
		}
	case "grep_search", "search":
		if pattern, ok := args["pattern"].(string); ok {
			return "search: " + pattern
		}
	case "web_fetch":
		if u, ok := args["url"].(string); ok {
			return "fetch: " + u
		}
	case "web_search":
		if q, ok := args["query"].(string); ok {
			return "web: " + q
		}
	case "lsp_diagnostics":
		if path, ok := args["path"].(string); ok {
			return "diagnostics: " + filepath.Base(path)
		}
	}
	return ""
}

func truncateUIContent(s string, reviewMode bool) string {
	if reviewMode {
		return s
	}
	const maxRunes = 500
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes]) + " … [truncated, press Ctrl+R for full review mode]"
}
