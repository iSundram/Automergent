package components

// The todo board: the right-side blackboard pane showing the live plan.
//
// It is deliberately only the plan. An earlier version also carried a subagent
// roster here, which split the "what is running" story across two surfaces: the
// board showed agents as a flat list while the dock (which actually owns
// background processes) showed the same agents nested under their spawner, with
// a different mark, colour and status vocabulary for each. Processes are the
// dock's job; the board is what the work is, not who is doing it.

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/iSundram/Automergent/internal/shared"
	"github.com/iSundram/Automergent/internal/tui/render"
	"github.com/iSundram/Automergent/internal/tui/themes"
)

// TaskBoard is the right-side todo pane.
type TaskBoard struct {
	styles   *themes.Styles
	visible  bool
	width    int
	height   int
	todos    []shared.TodoItem
	focusIdx int // highlighted row (-1 none)
}

// NewTaskBoard creates a hidden TaskBoard.
func NewTaskBoard(styles *themes.Styles) *TaskBoard {
	return &TaskBoard{styles: styles, focusIdx: -1}
}

// SetSize updates dimensions.
func (b *TaskBoard) SetSize(w, h int) { b.width, b.height = w, h }

// Toggle shows/hides the board.
func (b *TaskBoard) Toggle() { b.visible = !b.visible }

// Visible reports visibility.
func (b *TaskBoard) Visible() bool { return b.visible }

// SetTodos replaces the todo snapshot.
func (b *TaskBoard) SetTodos(items []shared.TodoItem) { b.todos = items }

// MoveFocus moves the highlight; returns true when handled.
func (b *TaskBoard) MoveFocus(delta int) bool {
	if !b.visible || len(b.todos) == 0 {
		return false
	}
	b.focusIdx += delta
	if b.focusIdx < 0 {
		b.focusIdx = 0
	}
	if b.focusIdx >= len(b.todos) {
		b.focusIdx = len(b.todos) - 1
	}
	return true
}

// View renders the todo plan.
func (b *TaskBoard) View() string {
	if !b.visible {
		return ""
	}
	w := b.width
	if w <= 0 {
		w = 30
	}
	inner := w - 4
	var sb strings.Builder

	title := lipgloss.NewStyle().Foreground(b.styles.T.Accent).Bold(true).Render("BOARD")
	sb.WriteString(title + "\n")
	sb.WriteString(lipgloss.NewStyle().Foreground(b.styles.T.Muted).
		Render(render.GlyphRule + render.GlyphRule + " todos " + render.GlyphRule + render.GlyphRule) + "\n")

	if len(b.todos) == 0 {
		sb.WriteString(b.styles.Dim.Render("no plan yet") + "\n")
		return b.frame(sb.String(), w)
	}

	done := 0
	for i, t := range b.todos {
		if t.Status == shared.TodoStatusCompleted {
			done++
		}
		// One mark and one colour per status, from the shared vocabulary. The
		// old board drew pending and completed with the same glyph, differing
		// only by colour — invisible to anyone without colour, and a lie about
		// state to everyone else.
		mark, style := render.GlyphIdle, lipgloss.NewStyle().Foreground(b.styles.T.Muted)
		switch t.Status {
		case shared.TodoStatusInProgress:
			mark, style = render.GlyphRun, lipgloss.NewStyle().Foreground(b.styles.T.Accent)
		case shared.TodoStatusBlocked:
			mark, style = render.GlyphWarn, lipgloss.NewStyle().Foreground(b.styles.T.Red)
		case shared.TodoStatusCompleted:
			mark, style = render.GlyphOK, lipgloss.NewStyle().Foreground(b.styles.T.Green)
		}

		cursor := "  "
		if i == b.focusIdx {
			cursor = lipgloss.NewStyle().Foreground(b.styles.T.Accent).Bold(true).
				Render(render.GlyphCursor) + " "
			style = lipgloss.NewStyle().Bold(true)
		}
		sb.WriteString(cursor + style.Render(mark+" "+render.Clip(t.Description, inner-4)) + "\n")
	}
	sb.WriteString(lipgloss.NewStyle().Foreground(b.styles.T.Muted).
		Render(fmt.Sprintf("%d/%d done", done, len(b.todos))) + "\n")

	return b.frame(sb.String(), w)
}

func (b *TaskBoard) frame(body string, w int) string {
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(b.styles.T.BorderNormal).
		Padding(0, 1).
		Width(w - 2).
		Render(body)
}
