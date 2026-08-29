package commands

import (
	"strings"
	"testing"
	"time"

	"github.com/iSundram/Automergent/internal/tui/components"
)

// Tests for the structured full-page builders (cmd_*.go Page functions).
// Pages are pure functions of the Host, so each test seeds the mock host and
// asserts on the Page model (or its Lines rendering) rather than the viewer.

func TestMCPPageFlagsServers(t *testing.T) {
	m := NewMockHost()
	m.mcpServers = []MCPServerStatus{
		{Name: "fs", Transport: "stdio", Version: "1.0", Tools: 3, Resources: 1, Prompts: 0, Latency: "4ms", Connected: true},
		{Name: "broken", Transport: "http", Version: "1.1", Tools: 0, Resources: 0, Prompts: 0, Latency: "—", LastError: "connection refused"},
	}
	page := mcpPage(m)

	if page.Title != "MCP Servers" {
		t.Fatalf("title = %q", page.Title)
	}
	if len(page.Sections) == 0 || page.Sections[0].Heading != "Servers" {
		t.Fatalf("expected Servers section, got %+v", page.Sections)
	}
	flags := page.Sections[0].Flagged
	if len(flags) != 2 {
		t.Fatalf("expected 2 flagged servers, got %d", len(flags))
	}
	if flags[0].Label != "fs" || flags[0].Status != components.PageStatusOK {
		t.Fatalf("connected server should be OK: %+v", flags[0])
	}
	if flags[1].Label != "broken" || flags[1].Status != components.PageStatusFail {
		t.Fatalf("failed server should be Fail: %+v", flags[1])
	}
	if !strings.Contains(flags[1].Detail, "connection refused") {
		t.Fatalf("last error missing from detail: %q", flags[1].Detail)
	}
	// Actions must dispatch valid sub-commands.
	if len(page.Actions) == 0 {
		t.Fatal("mcp page must offer actions")
	}
	for _, act := range page.Actions {
		if act.Command != "mcp" {
			t.Fatalf("action %q dispatches %q, want mcp", act.Key, act.Command)
		}
	}
}

func TestMCPPageEmpty(t *testing.T) {
	m := NewMockHost()
	page := mcpPage(m)
	if page.Subtitle != "No servers configured" {
		t.Fatalf("subtitle = %q", page.Subtitle)
	}
	lines := strings.Join(page.Lines(80), "\n")
	if !strings.Contains(lines, "No MCP servers configured") {
		t.Fatalf("empty page should explain config location:\n%s", lines)
	}
}

func TestErrorPageSummarizesRecords(t *testing.T) {
	m := NewMockHost()
	m.apiErrors = []APIErrorInfo{
		{Code: "429", Detail: "rate limited", Retrying: true, Attempt: 2, MaxAttempts: 5, At: time.Now().Add(-30 * time.Second)},
		{Code: "500", Retrying: false, MaxAttempts: 3, At: time.Now().Add(-2 * time.Minute)},
	}
	page := errorPage(m)

	if !strings.Contains(page.Subtitle, "2 this session") || !strings.Contains(page.Subtitle, "1 retried") {
		t.Fatalf("subtitle = %q", page.Subtitle)
	}
	flags := page.Sections[0].Flagged
	if len(flags) != 2 {
		t.Fatalf("expected 2 flagged errors, got %d", len(flags))
	}
	if flags[0].Status != components.PageStatusWarn {
		t.Fatalf("retrying error should be Warn: %+v", flags[0])
	}
	if flags[1].Status != components.PageStatusFail {
		t.Fatalf("terminal error should be Fail: %+v", flags[1])
	}
	if !strings.Contains(flags[0].Detail, "retry 2/5") {
		t.Fatalf("retry progress missing: %q", flags[0].Detail)
	}
	// The /error clear action must be wired.
	found := false
	for _, act := range page.Actions {
		if act.Command == "error" && len(act.Args) == 1 && act.Args[0] == "clear" {
			found = true
		}
	}
	if !found {
		t.Fatalf("error page must offer /error clear action: %+v", page.Actions)
	}
}

func TestErrorPageEmpty(t *testing.T) {
	m := NewMockHost()
	page := errorPage(m)
	if page.Subtitle == "" || len(page.Sections) != 0 {
		t.Fatalf("empty error page should be bare: %+v", page)
	}
}

func TestStatsPageSections(t *testing.T) {
	m := NewMockHost()
	page := statsPage(m)

	if page.Sections[0].Heading != "This Session" {
		t.Fatalf("first section = %+v", page.Sections[0])
	}
	joined := strings.Join(page.Lines(80), "\n")
	for _, want := range []string{"100", "50", "0.0010"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("stats page missing %q:\n%s", want, joined)
		}
	}
}

func TestFilesPageCapsAndListsDirs(t *testing.T) {
	m := NewMockHost()
	var files []string
	for i := 0; i < 60; i++ {
		files = append(files, "file"+string(rune('a'+i%26))+string(rune('0'+i%10))+".go")
	}
	m.contextFiles = files
	m.extraDirs = []string{"/tmp/extra"}

	page := filesPage(m)
	if !strings.Contains(page.Subtitle, "60") {
		t.Fatalf("subtitle = %q", page.Subtitle)
	}
	joined := strings.Join(page.Lines(80), "\n")
	if !strings.Contains(joined, "… and 10 more") {
		t.Fatalf("cap note missing:\n%s", joined)
	}
	if !strings.Contains(joined, "/tmp/extra") {
		t.Fatalf("extra search roots missing:\n%s", joined)
	}
}

func TestFilesPageEmpty(t *testing.T) {
	m := NewMockHost()
	page := filesPage(m)
	if page.Subtitle == "" || len(page.Sections) != 0 {
		t.Fatalf("empty files page should be bare: %+v", page)
	}
}

func TestConfigPageShowsWriteRulesAndCLIHint(t *testing.T) {
	m := NewMockHost()
	m.secBlocked = []string{"~/.ssh/**"}
	m.secAllowed = []string{"./**"}
	joined := strings.Join(configPage(m).Lines(80), "\n")
	for _, want := range []string{
		"google / gemini-3.6-flash",
		"1 blocked · 1 allowed",
		"Validation",
		"automergent config show",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("config page missing %q:\n%s", want, joined)
		}
	}
}

func TestProviderUseRejectsUnknownProvider(t *testing.T) {
	m := NewMockHost()
	handleProvider(m, []string{"unknown"})
	if len(m.errorMessages) == 0 || !strings.Contains(m.errorMessages[0], "Unknown provider") {
		t.Fatalf("expected error for unknown provider, got: %v", m.errorMessages)
	}
	if len(m.switchProviderCalls) != 0 {
		t.Fatalf("unknown provider must not reach SwitchProvider: %v", m.switchProviderCalls)
	}
}
