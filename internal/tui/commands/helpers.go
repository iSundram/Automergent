package commands

// Shared helpers used by the cmd_*.go command files.

import (
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
)

// formatContextLimit renders a token count compactly: "1M", "256K", "—".
func formatContextLimit(n int) string {
	switch {
	case n <= 0:
		return "—"
	case n >= 1024*1024:
		return fmt.Sprintf("%gM", float64(n)/(1024*1024))
	case n >= 1024:
		return fmt.Sprintf("%gK", float64(n)/1024)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// normalizeEffort validates a thinking-effort level.
func normalizeEffort(v string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "minimal", "low", "medium", "high":
		return strings.ToLower(strings.TrimSpace(v)), true
	}
	return "", false
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func providerSpecName(name string) string { return name }

func containsString(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

// parseSetupFlags parses the --key value flag list accepted by
// /provider setup. --test is a standalone boolean flag.
func parseSetupFlags(args []string) (map[string]string, bool, string) {
	flags := map[string]string{}
	runTest := false
	known := map[string]bool{
		"api-key": true, "base-url": true, "backend": true,
		"project": true, "location": true, "org": true, "model": true,
	}
	for i := 0; i < len(args); {
		tok := args[i]
		if tok == "--test" {
			runTest = true
			i++
			continue
		}
		if !strings.HasPrefix(tok, "--") {
			return nil, false, fmt.Sprintf("Unexpected argument %q — use --key value flags", tok)
		}
		name := strings.TrimPrefix(tok, "--")
		if !known[name] {
			return nil, false, fmt.Sprintf("Unknown flag %q — valid: --backend --api-key --base-url --project --location --org --model --test", tok)
		}
		if i+1 >= len(args) {
			return nil, false, fmt.Sprintf("Flag %q needs a value", tok)
		}
		flags[name] = args[i+1]
		i += 2
	}
	return flags, runTest, ""
}

// parseURL validates an http(s) URL and returns it without a trailing slash.
func parseURL(raw string) (string, bool) {
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", false
	}
	return strings.TrimRight(raw, "/"), true
}

// parsePositiveInt parses a strict positive integer argument.
func parsePositiveInt(v string) (int, bool) {
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// prefixFilter returns the candidates that start with partial (all of them
// when partial is empty) — the shared completion-filter idiom.
func prefixFilter(cands []string, partial string) []string {
	if partial == "" {
		return cands
	}
	var out []string
	for _, c := range cands {
		if len(partial) <= len(c) && c[:len(partial)] == partial {
			out = append(out, c)
		}
	}
	return out
}

// fileExists reports whether path exists on disk.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
