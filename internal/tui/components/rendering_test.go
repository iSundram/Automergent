package components

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

func testStyles() *themes.Styles {
	t := themes.Modern()
	return themes.NewStyles(t)
}

func TestInputKeepsTextColorWhenBlurred(t *testing.T) {
	styles := testStyles()
	input := NewInput(styles)
	input.Blur()

	blurred := input.ta.Styles().Blurred
	if got := blurred.Text.GetForeground(); got != styles.T.Text {
		t.Fatalf("blurred input foreground = %v, want %v", got, styles.T.Text)
	}
	if got := blurred.Placeholder.GetForeground(); got != styles.T.Subtext {
		t.Fatalf("blurred placeholder foreground = %v, want %v", got, styles.T.Subtext)
	}
}

func TestHeaderBrandIncludesTrailingSpacing(t *testing.T) {
	header := NewHeader(testStyles())
	header.SetWidth(80)
	view := header.View()
	if !strings.Contains(view, "AUTOMERGENT ") {
		t.Fatalf("header brand has no trailing spacing: %q", view)
	}
}

func TestUserBubbleUsesDarkThemeSurface(t *testing.T) {
	styles := testStyles()
	if got := styles.UserBubble.GetBackground(); got != styles.T.Surface {
		t.Fatalf("user bubble background = %v, want %v", got, styles.T.Surface)
	}
	if styles.UserBubble.GetBorderTop() || styles.UserBubble.GetBorderBottom() ||
		styles.UserBubble.GetBorderLeft() || styles.UserBubble.GetBorderRight() {
		t.Fatal("user bubble must not render a border")
	}
}

func TestAssistantResponseHasNoBubbleBorder(t *testing.T) {
	styles := testStyles()
	if styles.AssistantBubble.GetBorderTop() || styles.AssistantBubble.GetBorderBottom() ||
		styles.AssistantBubble.GetBorderLeft() || styles.AssistantBubble.GetBorderRight() {
		t.Fatal("assistant response must not render a border")
	}
	if got := styles.AssistantBubble.GetPaddingLeft(); got != 0 {
		t.Fatalf("assistant response left padding = %d, want 0", got)
	}
}

func TestAssistantResponseIndentIsConsistent(t *testing.T) {
	got := indentLines("first\nsecond", 1)
	if got != " first\n second" {
		t.Fatalf("assistant indentation = %q", got)
	}
}

func TestFinalizeStreamingPreservesProviderFinalText(t *testing.T) {
	conversation := NewConversation(testStyles())
	conversation.AppendToken("partial respon")
	conversation.FinalizeStreamingWithContent("partial response, completed.")
	last, ok := conversation.LastMessage()
	if !ok {
		t.Fatal("expected finalized assistant message")
	}
	if last.Content != "partial response, completed." {
		t.Fatalf("finalized response = %q", last.Content)
	}
}

func TestConversationEmptyStateDisappearsAfterMessage(t *testing.T) {
	conversation := NewConversation(testStyles())
	conversation.SetSize(80, 12)
	conversation.SetEmptyState(func(_ *themes.Styles, _, _ int) string {
		return "Welcome to Automergent"
	})
	if !strings.Contains(conversation.View(), "Welcome to Automergent") {
		t.Fatal("expected empty-state welcome")
	}
	conversation.AddMessage("user", "hello", false)
	if strings.Contains(conversation.View(), "Welcome to Automergent") {
		t.Fatal("welcome must disappear after the first message")
	}
}

func TestConversationNoWelcomeCard(t *testing.T) {
	conversation := NewConversation(testStyles())
	conversation.SetSize(80, 14)
	view := conversation.View()
	plain := ansi.Strip(view)
	for _, expected := range []string{"AUTOMERGENT", "workspace is ready"} {
		if strings.Contains(plain, expected) {
			t.Fatalf("welcome card must be removed, found %q: %q", expected, plain)
		}
	}
}

func TestSimpleConfirmShowsOnlyFooterActions(t *testing.T) {
	confirm := NewConfirm(testStyles())
	confirm.SetSize(80, 24)
	confirm.ShowSimple("Allow writes in this project?")
	view := confirm.View()
	if !strings.Contains(view, "allow") || !strings.Contains(view, "reject") {
		t.Fatalf("simple confirmation is missing actions: %q", view)
	}
	if strings.Contains(view, "always") || strings.Contains(view, "feedback") {
		t.Fatalf("simple confirmation shows advanced actions: %q", view)
	}
}

func TestTrustConfirmShowsSessionAndRememberChoices(t *testing.T) {
	confirm := NewConfirm(testStyles())
	confirm.SetSize(90, 24)
	confirm.ShowTrust("Trust this project folder?\n/root/Automergent")
	view := confirm.View()
	for _, expected := range []string{"trust this session", "trust and remember", "exit"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("trust confirmation missing %q: %q", expected, view)
		}
	}
}

func TestPermissionConfirmShowsToolDetails(t *testing.T) {
	confirm := NewConfirm(testStyles())
	confirm.SetSize(90, 24)
	confirm.ShowPermission(PermissionInfo{
		Icon:   "󰆍",
		Tool:   "Run",
		Action: "Execute shell command",
		Fields: []PermissionField{{Label: "Command", Value: "go test ./internal/tui/..."}},
		Risk:   "Runs a local process",
	})
	view := confirm.View()
	for _, expected := range []string{"Permission required", "Run", "Execute shell command", "Command", "go test", "Risk", "allow", "always", "reject", "feedback"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("permission confirmation missing %q: %q", expected, view)
		}
	}
}

func TestPermissionStatusReplacesNormalShortcuts(t *testing.T) {
	status := NewStatusBar(testStyles())
	status.SetWidth(100)
	status.SetPermission("Run")
	view := status.View()
	if !strings.Contains(view, "AWAITING PERMISSION") || !strings.Contains(view, "Run") {
		t.Fatalf("permission status is missing context: %q", view)
	}
	if strings.Contains(view, "CTRL+R") || strings.Contains(view, "/DIFF") {
		t.Fatalf("normal shortcuts leaked into permission status: %q", view)
	}
}

func TestToolCallUsesDefaultBackgroundAndCompactFields(t *testing.T) {
	conversation := NewConversation(testStyles())
	tool := conversation.renderToolCall(ConversationMsg{
		Role:        "tool_call",
		ToolName:    "bash",
		ToolArgs:    `{"command":"go build ./..."}`,
		ToolContext: "exec: go build ./...",
		Status:      "done",
		ToolSummary: "build completed",
		Duration:    time.Millisecond,
	}, 80)
	plain := ansi.Strip(tool)
	if strings.Contains(plain, "PARAMETERS") || strings.Contains(plain, "EXECUTION RESULT") {
		t.Fatalf("tool card exposes old section headers: %q", plain)
	}
	// The card names the tool, echoes the command and reports the duration —
	// no "Completed" label, since the status bullet already carries state.
	for _, expected := range []string{"Bash", "go build ./...", "1ms"} {
		if !strings.Contains(plain, expected) {
			t.Fatalf("tool card missing %q: %q", expected, plain)
		}
	}
	if strings.Contains(plain, "Completed") {
		t.Fatalf("status is carried by the bullet glyph, not a word: %q", plain)
	}
}

func TestSelectedPaletteRowUsesNormalBackground(t *testing.T) {
	palette := NewCommandPalette(testStyles())
	row := palette.renderItem(PaletteItem{Label: "model", Description: "Switch AI model"}, true, 60, 20)
	if got := lipgloss.Width(row); got != 60 {
		t.Fatalf("selected row width = %d, want 60", got)
	}
	if strings.Contains(row, "48;2;") {
		t.Fatalf("selected row must not apply a different background: %q", row)
	}
}

func TestPaletteScrollAndResponsiveRows(t *testing.T) {
	palette := NewCommandPalette(testStyles())
	items := make([]PaletteItem, 12)
	for i := range items {
		items[i] = PaletteItem{Label: fmt.Sprintf("command-%d", i), Description: "A command description", Category: "Session"}
	}
	palette.SetSize(42, 15)
	palette.Show(items, "")
	view := palette.View()
	for _, line := range strings.Split(ansi.Strip(view), "\n") {
		if lipgloss.Width(line) > 42 {
			t.Fatalf("palette line exceeds width: %d: %q", lipgloss.Width(line), line)
		}
	}
	updated, _ := palette.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if updated.Selected() == nil || updated.Selected().Label != "command-1" {
		t.Fatalf("mouse wheel did not move palette selection")
	}
}
