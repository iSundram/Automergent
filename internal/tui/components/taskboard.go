package components

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/iSundram/Automergent/internal/shared"
	"github.com/iSundram/Automergent/internal/tui/themes"
)

// AgentRow is one roster entry for the board's agents section.
type AgentRow struct {
	ID        string
	Name      string
	Type      string
	Status    string
	Turns     int
	Elapsed   string
	StartedAt time.Time
}

// TaskBoard is the right-side blackboard pane: live todo plan on top,
// subagent roster below (blueprint §4.1/§13 todo board).
type TaskBoard struct {
	styles   *themes.Styles
	visible  bool
	width    int
	height   int
	todos    []shared.TodoItem
	agents   []AgentRow
	focusIdx int // highlighted row across both sections (-1 none)
}

// NewTaskBoard creates a hidden TaskBoard.
func NewTaskBoard(styles *themes.Styles) *TaskBoard {
	return &TaskBoard{styles: styles, focusIdx: -1}
}

// SetSize updates dimensions.
func (b *TaskBoard) SetSize(w, h int) { b.width = w; b.height = h }

// Toggle shows/hides the board.
func (b *TaskBoard) Toggle() { b.visible = !b.visible }

// Visible reports visibility.
func (b *TaskBoard) Visible() bool { return b.visible }

// SetTodos replaces the todo snapshot.
func (b *TaskBoard) SetTodos(items []shared.TodoItem) { b.todos = items }

// SetAgents replaces the subagent roster.
func (b *TaskBoard) SetAgents(rows []AgentRow) { b.agents = rows }

// MoveFocus moves the highlight; returns true when handled.
func (b *TaskBoard) MoveFocus(delta int) bool {
	if !b.visible {
		return false
	}
	total := len(b.todos) + len(b.agents)
	if total == 0 {
		return false
	}
	b.focusIdx += delta
	if b.focusIdx < 0 {
		b.focusIdx = 0
	}
	if b.focusIdx >= total {
		b.focusIdx = total - 1
	}
	return true
}

// FocusedAgent returns the agent under the focus cursor, if any.
func (b *TaskBoard) FocusedAgent() (AgentRow, bool) {
	idx := b.focusIdx - len(b.todos)
	if idx < 0 || idx >= len(b.agents) {
		return AgentRow{}, false
	}
	return b.agents[idx], true
}

// View renders todos + agents sections.
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

	title := lipgloss.NewStyle().Foreground(b.styles.T.Accent).Bold(true).Render("󰒋 BOARD")
	sb.WriteString(title + "\n")

	// --- Todos ---
	sb.WriteString(lipgloss.NewStyle().Foreground(b.styles.T.Muted).Render("── todos ──") + "\n")
	if len(b.todos) == 0 {
		sb.WriteString(b.styles.Dim.Render("no plan yet") + "\n")
	} else {
		done := 0
		for _, t := range b.todos {
			if t.Status == shared.TodoStatusCompleted {
				done++
			}
			icon, style := "󰄬 ", lipgloss.NewStyle().Foreground(b.styles.T.Green)
			switch t.Status {
			case shared.TodoStatusInProgress:
				icon, style = "󰔟 ", lipgloss.NewStyle().Foreground(b.styles.T.Yellow)
			case shared.TodoStatusBlocked:
				icon, style = "󰅙 ", lipgloss.NewStyle().Foreground(b.styles.T.Red)
			case shared.TodoStatusPending:
				icon, style = "󰄬 ", lipgloss.NewStyle().Foreground(b.styles.T.Muted)
			}
			marker := " "
			idx := len(b.todos)
			_ = idx
			row := icon + truncate(t.Description, inner-3)
			if marker == "*" {
				row = lipgloss.NewStyle().Bold(true).Render(row)
			}
			sb.WriteString(style.Render(row) + "\n")
		}
		sb.WriteString(lipgloss.NewStyle().Foreground(b.styles.T.Muted).
			Render(fmt.Sprintf("%d/%d done\n", done, len(b.todos))))
	}

	// --- Agents ---
	sb.WriteString(lipgloss.NewStyle().Foreground(b.styles.T.Muted).Render("── agents ──") + "\n")
	if len(b.agents) == 0 {
		sb.WriteString(b.styles.Dim.Render("no subagents") + "\n")
	} else {
		for i, a := range b.agents {
			global := len(b.todos) + i
			cursor := "  "
			style := lipgloss.NewStyle()
			if global == b.focusIdx {
				cursor = "▸ "
				style = lipgloss.NewStyle().Bold(true)
			}
			statusColor := b.styles.T.Subtext
			switch a.Status {
			case "running":
				statusColor = b.styles.T.Yellow
			case "completed":
				statusColor = b.styles.T.Green
			case "failed", "cancelled", "killed":
				statusColor = b.styles.T.Red
			}
			elapsed := a.Elapsed
			if a.Status == "running" && !a.StartedAt.IsZero() {
				elapsed = time.Since(a.StartedAt).Round(time.Second).String()
			}
			line := fmt.Sprintf("%s%s [%s] %s %s",
				cursor,
				truncate(firstNonEmptyStr(a.Name, a.ID), inner/2),
				a.Status,
				a.Type,
				statusStyleText(statusColor, elapsed),
			)
			sb.WriteString(style.Render(line) + "\n")
		}
		sb.WriteString(b.styles.Dim.Render("m msg · f fork · i interrupt · k kill") + "\n")
	}

	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(b.styles.T.BorderNormal).
		Padding(0, 1).
		Width(w - 2).
		Render(sb.String())
}

func statusStyleText(colorCh interface{ RGBA() (r, g, b, a uint32) }, s string) string {
	if s == "" {
		return ""
	}
	return lipgloss.NewStyle().Foreground(colorCh).Render(s)
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

var _ = time.Now // reserved for elapsed refresh ticks
