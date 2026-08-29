package components

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

// PageActionMsg is emitted by the PageViewer when the user presses an
// action's key. The App translates it back into a slash-command dispatch.
type PageActionMsg struct {
	Command string
	Args    []string
}

// PageViewer renders a structured Page full-screen, mirroring the FullPage
// overlay idioms (Show/Hide/Visible/SetSize/scroll) so it drops into the same
// layout slot. Action keys are single letters shown in a bottom bar; pressing
// one emits a PageActionMsg instead of scrolling.
type PageViewer struct {
	styles  *themes.Styles
	width   int
	height  int
	page    Page
	visible bool
	scroll  int
}

func NewPageViewer(styles *themes.Styles) PageViewer {
	return PageViewer{styles: styles}
}

func (v *PageViewer) SetSize(w, h int) { v.width, v.height = w, h }

func (v *PageViewer) Show(page Page) {
	v.page = page
	v.visible = true
	v.scroll = 0
}

func (v *PageViewer) Hide()        { v.visible = false }
func (v PageViewer) Visible() bool { return v.visible }
func (v PageViewer) Title() string { return v.page.Title }

// Update handles scrolling and action keys. It reports whether the key
// triggered an action so the caller does not also route it.
func (v *PageViewer) Update(msg tea.Msg) (PageViewer, bool, tea.Cmd) {
	if !v.visible {
		return *v, false, nil
	}
	if m, ok := msg.(tea.KeyMsg); ok {
		// Action keys take precedence: a visible, labelled shortcut must beat
		// scrolling, and no scroll key is a single letter.
		key := strings.ToLower(m.String())
		for _, action := range v.page.Actions {
			if action.Key != "" && key == strings.ToLower(action.Key) {
				cmd := func() tea.Msg {
					return PageActionMsg{Command: action.Command, Args: action.Args}
				}
				return *v, true, cmd
			}
		}
		switch m.String() {
		case "up", "ctrl+p":
			if v.scroll > 0 {
				v.scroll--
			}
		case "down", "ctrl+n":
			v.scroll++
		case "pgup":
			v.scroll = max(0, v.scroll-v.viewport()+1)
		case "pgdown":
			v.scroll += v.viewport() - 1
		case "home", "ctrl+home":
			v.scroll = 0
		case "end", "ctrl+end":
			v.scroll = 1 << 30 // clamped in View
		}
	}
	if m, ok := msg.(tea.MouseMsg); ok {
		switch m.Mouse().Button {
		case tea.MouseWheelUp:
			if v.scroll > 0 {
				v.scroll--
			}
		case tea.MouseWheelDown:
			v.scroll++
		}
	}
	return *v, false, nil
}

func (v PageViewer) viewport() int {
	// rule + header + blank + blank + rule + action bar + footer
	viewport := v.height - 7
	if viewport < 1 {
		viewport = 1
	}
	return viewport
}

func (v PageViewer) View() string {
	if !v.visible || v.width <= 0 || v.height <= 0 {
		return ""
	}
	w := v.width

	titleStyle := lipgloss.NewStyle().
		Foreground(v.styles.T.Accent).
		Bold(true).
		Padding(0, 1)

	headerLeft := titleStyle.Render("  " + v.page.Title + "  ")
	var header string
	if v.page.Subtitle != "" {
		sub := lipgloss.NewStyle().Foreground(v.styles.T.Muted).Render(v.page.Subtitle)
		gap := max(1, w-lipgloss.Width(headerLeft)-lipgloss.Width(sub)-2)
		header = lipgloss.NewStyle().Width(w).MaxWidth(w).Render(headerLeft + strings.Repeat(" ", gap) + sub + " ")
	} else {
		header = headerLeft
	}
	rule := lipgloss.NewStyle().Foreground(v.styles.T.BorderNormal).Render(strings.Repeat("─", w))

	lines := v.page.Lines(w - 2)
	viewport := v.viewport()

	maxScroll := 0
	if len(lines) > viewport {
		maxScroll = len(lines) - viewport
	}
	if v.scroll > maxScroll {
		v.scroll = maxScroll
	}
	if v.scroll < 0 {
		v.scroll = 0
	}
	visible := lines
	if v.scroll > 0 && v.scroll < len(lines) {
		visible = lines[v.scroll:]
	}
	if len(visible) > viewport {
		visible = visible[:viewport]
	}

	contentStyle := lipgloss.NewStyle().
		Width(w - 2).
		MaxWidth(w - 2).
		Foreground(v.styles.T.Text)

	headingStyle := lipgloss.NewStyle().
		Foreground(v.styles.T.Muted).
		Bold(true)

	var rows []string
	rows = append(rows, rule)
	rows = append(rows, header)
	rows = append(rows, "")
	for _, line := range visible {
		if line != "" && line == strings.ToUpper(line) && !strings.HasPrefix(line, "  ") {
			// Section heading (produced by Page.Lines without leading spaces).
			rows = append(rows, headingStyle.Render(line))
		} else {
			rows = append(rows, contentStyle.Render(line))
		}
	}
	rows = append(rows, "")
	rows = append(rows, rule)
	rows = append(rows, v.actionBar(w))
	rows = append(rows, v.footer(w))

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// actionBar renders the shortcut bar: one line of "[k] label" chips, or the
// muted placeholder when the page has no actions.
func (v PageViewer) actionBar(w int) string {
	if len(v.page.Actions) == 0 {
		return lipgloss.NewStyle().Foreground(v.styles.T.Muted).PaddingLeft(2).Render("no further actions")
	}
	keyStyle := lipgloss.NewStyle().Foreground(v.styles.T.Accent).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(v.styles.T.Text)
	var chips []string
	for _, action := range v.page.Actions {
		chips = append(chips, keyStyle.Render("["+action.Key+"]")+" "+labelStyle.Render(action.Label))
	}
	return lipgloss.NewStyle().PaddingLeft(2).MaxWidth(max(1, w-2)).Render(strings.Join(chips, "   "))
}

func (v PageViewer) footer(w int) string {
	text := "↑↓ scroll · esc close"
	if len(v.page.Actions) > 0 {
		text = "↑↓ scroll · letter keys run actions · esc close"
	}
	return lipgloss.NewStyle().Foreground(v.styles.T.Muted).PaddingLeft(2).
		MaxWidth(max(1, w-2)).Render(text)
}
