package components

import (
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/iSundram/Automergent/internal/tui/render"
	"github.com/iSundram/Automergent/internal/tui/themes"
)

// Diff is a scrollable diff pane.
type Diff struct {
	viewport viewport.Model
	styles   *themes.Styles
	visible  bool
	focused  bool
}

// NewDiff creates a new Diff component.
func NewDiff(styles *themes.Styles) Diff {
	vp := viewport.New(40, 20)
	return Diff{viewport: vp, styles: styles}
}

// SetSize updates the component dimensions.
func (d *Diff) SetSize(w, h int) {
	d.viewport.Width = w - 2 // Account for border
	// Leave space for the action bar at bottom
	d.viewport.Height = h - 3
}

// SetContent sets the diff content.
func (d *Diff) SetContent(content string) {
	d.viewport.SetContent(render.Diff(content))
}

// Toggle shows or hides the diff pane.
func (d *Diff) Toggle() { d.visible = !d.visible }

// Visible reports whether the pane is visible.
func (d *Diff) Visible() bool { return d.visible }

// Focus sets the focused state
func (d *Diff) Focus(focus bool) { d.focused = focus }

// Update processes viewport messages.
func (d Diff) Update(msg tea.Msg) (Diff, tea.Cmd) {
	vp, cmd := d.viewport.Update(msg)
	d.viewport = vp
	return d, cmd
}

// View renders the diff pane.
func (d Diff) View() string {
	if !d.visible {
		return ""
	}

	content := d.viewport.View()

	// Floating action bar for Diff
	actionBar := d.styles.DiffAction.Render(" [A]ccept Hunk   [R]eject Hunk   [E]dit   [↓/↑] Next/Prev")

	layout := lipgloss.JoinVertical(lipgloss.Left, content, "\n", actionBar)

	if d.focused {
		return d.styles.ActivePane.Render(layout)
	}
	return d.styles.InactivePane.Render(layout)
}
