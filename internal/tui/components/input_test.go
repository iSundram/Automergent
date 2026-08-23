package components

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/iSundram/Automergent/internal/tui/themes"
)

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
			wantLines: 6,
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
}

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
