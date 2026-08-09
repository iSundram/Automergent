package components

import (
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/iSundram/Automergent/internal/tui/render"
	"github.com/iSundram/Automergent/internal/tui/themes"
)

// Diff is a scrollable diff pane.
type Diff struct {
	viewport viewport.Model
	styles   *themes.Styles
	visible  bool
}

// NewDiff creates a new Diff component.
func NewDiff(styles *themes.Styles) Diff {
	vp := viewport.New(40, 20)
	return Diff{viewport: vp, styles: styles}
}

// SetSize updates the component dimensions.
func (d *Diff) SetSize(w, h int) {
	d.viewport.Width = w
	d.viewport.Height = h
}

// SetContent sets the diff content.
func (d *Diff) SetContent(content string) {
	d.viewport.SetContent(render.Diff(content))
}

// Toggle shows or hides the diff pane.
func (d *Diff) Toggle() { d.visible = !d.visible }

// Visible reports whether the pane is visible.
func (d *Diff) Visible() bool { return d.visible }

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
	return d.styles.Border.Render(d.viewport.View())
}
