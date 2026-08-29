package components

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

// FullPage displays command output in a full-screen overlay.
type FullPage struct {
	styles  *themes.Styles
	width   int
	height  int
	title   string
	content string
	visible bool
	scroll  int
}

func NewFullPage(styles *themes.Styles) FullPage {
	return FullPage{styles: styles}
}

func (f *FullPage) SetSize(w, h int) { f.width, f.height = w, h }

func (f *FullPage) Show(title, content string) {
	f.title = title
	f.content = content
	f.visible = true
	f.scroll = 0
}

func (f *FullPage) Hide()          { f.visible = false }
func (f FullPage) Visible() bool   { return f.visible }
func (f FullPage) Title() string   { return f.title }
func (f FullPage) Content() string { return f.content }

func (f FullPage) Update(msg tea.Msg) (FullPage, tea.Cmd) {
	if !f.visible {
		return f, nil
	}
	if m, ok := msg.(tea.KeyMsg); ok {
		switch m.String() {
		case "up", "ctrl+p":
			if f.scroll > 0 {
				f.scroll--
			}
		case "down", "ctrl+n":
			f.scroll++
		case "pgup":
			f.scroll = max(0, f.scroll-f.height+6)
		case "pgdown":
			f.scroll += f.height - 6
		case "home", "ctrl+home":
			f.scroll = 0
		case "end", "ctrl+end":
			f.scroll = 999999 // will be clamped in View
		}
	}
	if m, ok := msg.(tea.MouseMsg); ok {
		switch m.Mouse().Button {
		case tea.MouseWheelUp:
			if f.scroll > 0 {
				f.scroll--
			}
		case tea.MouseWheelDown:
			f.scroll++
		}
	}
	return f, nil
}

func (f FullPage) View() string {
	if !f.visible || f.width <= 0 || f.height <= 0 {
		return ""
	}
	w := f.width
	h := f.height

	titleStyle := lipgloss.NewStyle().
		Foreground(f.styles.T.Accent).
		Bold(true).
		Padding(0, 1)

	headerText := "  " + f.title + "  "
	header := titleStyle.Render(headerText)
	rule := lipgloss.NewStyle().Foreground(f.styles.T.BorderNormal).Render(strings.Repeat("─", w))
	footer := lipgloss.NewStyle().Foreground(f.styles.T.Muted).Render("↑↓ scroll · esc close")

	lines := strings.Split(f.content, "\n")
	viewport := h - 6 // rule + header + blank + blank + rule + footer
	if viewport < 1 {
		viewport = 1
	}

	// Clamp scroll.
	maxScroll := 0
	if len(lines) > viewport {
		maxScroll = len(lines) - viewport
	}
	if f.scroll > maxScroll {
		f.scroll = maxScroll
	}
	if f.scroll < 0 {
		f.scroll = 0
	}

	visible := lines
	if f.scroll > 0 && f.scroll < len(lines) {
		visible = lines[f.scroll:]
	}
	if len(visible) > viewport {
		visible = visible[:viewport]
	}

	contentStyle := lipgloss.NewStyle().
		Width(w - 2).
		MaxWidth(w - 2).
		Foreground(f.styles.T.Text)

	var rows []string
	rows = append(rows, rule)
	rows = append(rows, header)
	rows = append(rows, "")
	for _, line := range visible {
		rows = append(rows, contentStyle.Render(line))
	}
	rows = append(rows, "")
	rows = append(rows, rule)
	rows = append(rows, footer)

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}
