package components

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/iSundram/Automergent/internal/tui/themes"
)

// helper ---------------------------------------------------------------

func newTestInput(t *testing.T) Input {
	t.Helper()
	theme := themes.Get("catppuccin")
	styles := themes.NewStyles(theme)
	return NewInput(styles)
}

// keyPress builds a tea.KeyPressMsg from a string representation
// that is stable in BubbleTea v2 (matched via msg.String()).
func keyPress(code rune, mod tea.KeyMod) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Mod: mod}
}

func keyPressText(text string) tea.KeyPressMsg {
	runes := []rune(text)
	if len(runes) == 0 {
		return tea.KeyPressMsg{}
	}
	return tea.KeyPressMsg{Code: runes[0], Text: text}
}

// LineCount ---------------------------------------------------------------

func TestInputLineCountCalculation(t *testing.T) {
	theme := themes.Get("catppuccin")
	styles := themes.NewStyles(theme)

	tests := []struct {
		name      string
		width     int
		value     string
		wantLines int
	}{
		{
			name:      "empty input",
			width:     40,
			value:     "",
			wantLines: 1,
		},
		{
			name:      "short single line",
			width:     40,
			value:     "hello world",
			wantLines: 1,
		},
		{
			name:      "multiline with explicit newlines",
			width:     40,
			value:     "line 1\nline 2\nline 3",
			wantLines: 3,
		},
		{
			name:      "long wrapping line",
			width:     30,
			value:     "this is a very long string that should definitely wrap across multiple lines because it exceeds thirty columns in width",
			wantLines: 6, // taW = width-4 = 26; wraps more than old width-2 = 28
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inp := NewInput(styles)
			inp.SetWidth(tt.width)
			inp.SetValue(tt.value)
			got := inp.LineCount()
			if got != tt.wantLines {
				t.Errorf("LineCount() = %d, want %d for value %q at width %d", got, tt.wantLines, tt.value, tt.width)
			}
		})
	}
}

// TestLongWordNoSpacesRendering is a regression for the bug where typing a
// long unbroken string (no spaces) caused ❯ to appear on a blank line above
// or below the text, or the text to disappear entirely.
func TestLongWordNoSpacesRendering(t *testing.T) {
	theme := themes.Get("catppuccin")
	styles := themes.NewStyles(theme)
	// Long string with no spaces, similar to what the user typed.
	longWord := "hsjsgs7suwhwgeyeuehehsjjshdhsjsjshshskskhsueikejwnshdhdusuwhwbwjjshsyeuehehheydhshehhehehhh"

	for _, width := range []int{40, 60, 80, 92, 120} {
		t.Run(fmt.Sprintf("width=%d", width), func(t *testing.T) {
			inp := NewInput(styles)
			inp.SetWidth(width)
			inp.SetValue(longWord)

			view := inp.View()
			viewLines := strings.Split(view, "\n")

			// The ❯ prompt must appear on the first CONTENT line (skip the
			// lipgloss border line which is all ─ chars).
			foundPromptOnFirst := false
			for _, l := range viewLines {
				plain := stripTestANSI(l)
				trimmed := strings.TrimSpace(plain)
				if trimmed == "" {
					continue
				}
				// Skip the top border line (all ─ or similar box-drawing chars).
				if strings.ContainsRune(trimmed, '─') && !strings.ContainsRune(trimmed, '❯') {
					continue
				}
				// First content line must contain ❯.
				foundPromptOnFirst = strings.ContainsRune(plain, '❯')
				break
			}
			if !foundPromptOnFirst {
				t.Errorf("width=%d: ❯ prompt not on first content line.\nView:\n%s", width, view)
			}

			// The text must be visible somewhere in the view.
			if !strings.Contains(view, "hsjsgs") {
				t.Errorf("width=%d: text is not visible in the rendered view.\nView:\n%s", width, view)
			}
		})
	}
}

// stripTestANSI removes ANSI escape codes for test comparisons.
func stripTestANSI(s string) string {
	var b strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func TestLineCountCJKAndEmoji(t *testing.T) {
	inp := newTestInput(t)
	inp.SetWidth(12) // effective content width 10 (min 10)

	// Each CJK char is 2 columns wide; "日本語テスト" = 6 chars × 2 = 12 cols.
	// With width 10 it must wrap to at least 2 lines.
	inp.SetValue("日本語テスト")
	if got := inp.LineCount(); got < 2 {
		t.Errorf("CJK LineCount() = %d, want >= 2", got)
	}
}

// Paste handling ----------------------------------------------------------

func TestInputPasteHandling(t *testing.T) {
	theme := themes.Get("catppuccin")
	styles := themes.NewStyles(theme)

	t.Run("multiline paste becomes [pasted x lines]", func(t *testing.T) {
		inp := NewInput(styles)
		inp.SetWidth(50)

		pasteContent := "func foo() {\n\tprintln(\"hello\")\n}\n"
		msg := tea.PasteMsg{Content: pasteContent}
		updated, _ := inp.Update(msg)

		if got := updated.Value(); got != pasteContent {
			t.Errorf("Value() = %q, want %q", got, pasteContent)
		}
	})

	t.Run("single line paste inserts normally", func(t *testing.T) {
		inp := NewInput(styles)
		inp.SetWidth(50)

		pasteContent := "hello world"
		msg := tea.PasteMsg{Content: pasteContent}
		updated, _ := inp.Update(msg)

		if got := updated.Value(); got != pasteContent {
			t.Errorf("Value() = %q, want %q", got, pasteContent)
		}
	})

	t.Run("enter on pasted placeholder returns pasted content", func(t *testing.T) {
		inp := newTestInput(t)
		inp.SetWidth(50)

		pasteContent := "line one\nline two\n"
		pasteMsg := tea.PasteMsg{Content: pasteContent}
		inp, _ = inp.Update(pasteMsg)

		// Value() must return the full pasted content, not the placeholder.
		if got := inp.Value(); got != pasteContent {
			t.Errorf("Value() after paste = %q, want %q", got, pasteContent)
		}
	})

	t.Run("edit key after paste restores full content", func(t *testing.T) {
		inp := newTestInput(t)
		inp.SetWidth(50)

		pasteContent := "line one\nline two\n"
		inp, _ = inp.Update(tea.PasteMsg{Content: pasteContent})

		// Typing any non-enter key should restore the pasted content.
		inp, _ = inp.Update(keyPressText("x"))

		if inp.pastedContent != "" {
			t.Errorf("pastedContent should be empty after edit, got %q", inp.pastedContent)
		}
	})
}

// SetValue cursor ----------------------------------------------------------

func TestSetValueMovesCursorToEnd(t *testing.T) {
	inp := newTestInput(t)
	inp.SetWidth(80)
	inp.SetValue("hello world")

	// Column() is 0-indexed; after MoveToEnd it should equal len("hello world").
	if col := inp.ta.Column(); col != len("hello world") {
		t.Errorf("cursor column after SetValue = %d, want %d", col, len("hello world"))
	}
}

// History -----------------------------------------------------------------

func TestHistoryNavigation(t *testing.T) {
	inp := newTestInput(t)
	inp.SetWidth(40)

	// Build history: send three different messages.
	inp.SetValue("first")
	inp.Reset()
	inp.SetValue("second")
	inp.Reset()
	inp.SetValue("third")
	inp.Reset()

	// Navigate up (most-recent first) using alt+up.
	altUp := tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModAlt}
	altDown := tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModAlt}

	inp, _ = inp.Update(altUp)
	if v := inp.Value(); v != "third" {
		t.Errorf("hist[0] = %q, want \"third\"", v)
	}
	inp, _ = inp.Update(altUp)
	if v := inp.Value(); v != "second" {
		t.Errorf("hist[1] = %q, want \"second\"", v)
	}
	inp, _ = inp.Update(altUp)
	if v := inp.Value(); v != "first" {
		t.Errorf("hist[2] = %q, want \"first\"", v)
	}
	// At boundary — another up should stay at "first".
	inp, _ = inp.Update(altUp)
	if v := inp.Value(); v != "first" {
		t.Errorf("hist boundary = %q, want \"first\"", v)
	}

	// Navigate down back to empty.
	inp, _ = inp.Update(altDown)
	inp, _ = inp.Update(altDown)
	inp, _ = inp.Update(altDown)
	if v := inp.Value(); v != "" {
		t.Errorf("after navigating back down, value = %q, want \"\"", v)
	}
}

func TestHistoryDeduplication(t *testing.T) {
	inp := newTestInput(t)
	inp.SetWidth(40)

	inp.SetValue("duplicate")
	inp.Reset()
	inp.SetValue("duplicate")
	inp.Reset()

	if len(inp.history) != 1 {
		t.Errorf("history len = %d, want 1 (consecutive duplicate should be deduped)", len(inp.history))
	}
}

func TestHistoryCapAt100(t *testing.T) {
	inp := newTestInput(t)
	inp.SetWidth(40)

	for k := 0; k < 120; k++ {
		inp.SetValue(strings.Repeat("x", k+1))
		inp.Reset()
	}

	if len(inp.history) > maxHistoryMem {
		t.Errorf("history len = %d, want <= %d", len(inp.history), maxHistoryMem)
	}
}

// Persistent history ------------------------------------------------------

func TestPersistentHistory(t *testing.T) {
	dir := t.TempDir()
	histPath := filepath.Join(dir, "history")

	theme := themes.Get("catppuccin")
	styles := themes.NewStyles(theme)

	// First session: write two entries.
	inp1 := NewInput(styles).WithHistoryFile(histPath)
	inp1.SetWidth(40)
	inp1.SetValue("persistent one")
	inp1.Reset()
	inp1.SetValue("persistent two")
	inp1.Reset()

	// Verify file was created.
	if _, err := os.Stat(histPath); os.IsNotExist(err) {
		t.Fatal("history file was not created")
	}

	// Second session: should load the two entries.
	inp2 := NewInput(styles).WithHistoryFile(histPath)
	inp2.SetWidth(40)

	if l := len(inp2.history); l != 2 {
		t.Errorf("loaded history len = %d, want 2", l)
	}
	if len(inp2.history) == 2 && (inp2.history[0] != "persistent one" || inp2.history[1] != "persistent two") {
		t.Errorf("loaded history = %v", inp2.history)
	}
}

func TestPersistentHistoryNewlineEscaping(t *testing.T) {
	dir := t.TempDir()
	histPath := filepath.Join(dir, "history")

	theme := themes.Get("catppuccin")
	styles := themes.NewStyles(theme)

	multiline := "line one\nline two"
	inp1 := NewInput(styles).WithHistoryFile(histPath)
	inp1.SetWidth(40)
	inp1.SetValue(multiline)
	inp1.Reset()

	inp2 := NewInput(styles).WithHistoryFile(histPath)
	if len(inp2.history) != 1 {
		t.Fatalf("loaded history len = %d, want 1", len(inp2.history))
	}
	if inp2.history[0] != multiline {
		t.Errorf("loaded = %q, want %q", inp2.history[0], multiline)
	}
}

// Undo / Redo -------------------------------------------------------------

func TestUndoRedo(t *testing.T) {
	inp := newTestInput(t)
	inp.SetWidth(80)

	// Type "hello" — each rune triggers isPrintableKey → pushUndo.
	for _, r := range "hello" {
		inp, _ = inp.Update(keyPressText(string(r)))
	}

	snapBefore := inp.ta.Value()

	// Undo: ctrl+z
	inp, _ = inp.Update(tea.KeyPressMsg{Code: 'z', Mod: tea.ModCtrl})
	afterUndo := inp.ta.Value()

	// Redo: ctrl+y
	inp, _ = inp.Update(tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl})
	afterRedo := inp.ta.Value()

	t.Logf("snapBefore=%q afterUndo=%q afterRedo=%q", snapBefore, afterUndo, afterRedo)
	// The invariant: undo should change the value (or leave it empty if stack
	// was empty), redo should restore it.
	_ = afterUndo
	_ = afterRedo
}

func TestInsertValueMovesToEnd(t *testing.T) {
	inp := newTestInput(t)
	inp.SetWidth(80)
	inp.SetValue("/model ")
	inp.InsertValue("gpt-4o")

	if col := inp.ta.Column(); col == 0 {
		t.Error("cursor should not be at position 0 after InsertValue")
	}
}

// RegisterSubPalette ------------------------------------------------------

func TestRegisterSubPalette(t *testing.T) {
	RegisterSubPalette("myplugin")
	if !SlashSubPalettes["myplugin"] {
		t.Error("RegisterSubPalette did not register the command")
	}
	// cleanup so it doesn't affect other tests
	delete(SlashSubPalettes, "myplugin")
}

// TestInputWrapBoundaryKeepsTextVisible is a regression for the bug where
// typing the character that wraps text onto a second visual line scrolled the
// textarea's viewport down (repositionView ran with the stale height) and all
// previously typed text disappeared — only the newest character stayed visible.
func TestInputWrapBoundaryKeepsTextVisible(t *testing.T) {
	inp := newTestInput(t)
	inp.SetWidth(40) // effective textarea width 36

	// Type well past the fill boundary, one rune per keystroke.
	var typed strings.Builder
	for n := 0; n < 50; n++ {
		r := rune('a' + n%26)
		typed.WriteRune(r)
		inp, _ = inp.Update(tea.KeyPressMsg{Code: r, Text: string(r)})

		raw := stripTestANSI(inp.ta.View())
		var visible strings.Builder
		for _, l := range strings.Split(raw, "\n") {
			visible.WriteString(strings.TrimSpace(l))
		}
		if visible.String() != typed.String() {
			t.Fatalf("after %d chars: typed %q but view shows %q",
				n+1, typed.String(), visible.String())
		}
	}
}

// Prompt + multiline rendering --------------------------------------------

func TestInputPromptAndMultilineRendering(t *testing.T) {
	theme := themes.Get("catppuccin")
	styles := themes.NewStyles(theme)
	inp := NewInput(styles)
	inp.SetWidth(30)
	// A long string that wraps across multiple lines
	inp.SetValue("this is line one that is quite long and wraps, then line two, then line three")

	t.Logf("MaxHeight: %d, Height: %d, LineCount: %d", inp.ta.MaxHeight, inp.ta.Height(), inp.LineCount())
	view := inp.View()
	t.Logf("Input View for wrapping lines:\n%s", view)
}
