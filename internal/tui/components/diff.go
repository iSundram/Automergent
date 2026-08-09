package components

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/iSundram/Automergent/internal/tui/render"
	"github.com/iSundram/Automergent/internal/tui/themes"
)

// Hunk represents a single change block in a diff.
type Hunk struct {
	Content string
	Active  bool
}

// Diff is a scrollable diff pane with hunk navigation.
type Diff struct {
	viewport   viewport.Model
	styles     *themes.Styles
	visible    bool
	focused    bool
	hunks      []Hunk
	hunkCursor int
}

// NewDiff creates a new Diff component.
func NewDiff(styles *themes.Styles) Diff {
	vp := viewport.New(40, 20)
	return Diff{viewport: vp, styles: styles}
}

// SetSize updates the component dimensions.
func (d *Diff) SetSize(w, h int) {
	d.viewport.Width = w - 2
	d.viewport.Height = h - 3
	d.refresh()
}

// SetContent sets and parses the diff content.
func (d *Diff) SetContent(content string) {
	// Parse hunks
	rawHunks := strings.Split(content, "@@")
	d.hunks = nil
	if len(rawHunks) > 0 {
		// First part is usually the file header
		d.hunks = append(d.hunks, Hunk{Content: rawHunks[0]})
		for i := 1; i < len(rawHunks); i++ {
			d.hunks = append(d.hunks, Hunk{Content: "@@" + rawHunks[i]})
		}
	}
	d.hunkCursor = 0
	if len(d.hunks) > 1 {
		d.hunkCursor = 1 // Focus first real hunk
	}
	d.refresh()
}

func (d *Diff) refresh() {
	var sb strings.Builder
	for i, hunk := range d.hunks {
		rendered := render.Diff(hunk.Content)
		if i == d.hunkCursor && d.focused {
			// Highlight the active hunk
			lines := strings.Split(rendered, "\n")
			for j, line := range lines {
				if line != "" {
					lines[j] = " " + line // Subtle indent or could use a style
				}
			}
			rendered = strings.Join(lines, "\n")
			// Wrap active hunk in a subtle highlight if theme supports it
			rendered = lipgloss.NewStyle().
				Background(d.styles.T.Surface).
				Width(d.viewport.Width).
				Render(rendered)
		}
		sb.WriteString(rendered + "\n")
	}
	d.viewport.SetContent(sb.String())
}

// Toggle shows or hides the diff pane.
func (d *Diff) Toggle() { d.visible = !d.visible }

// Visible reports whether the pane is visible.
func (d *Diff) Visible() bool { return d.visible }

// Focus sets the focused state
func (d *Diff) Focus(focus bool) {
	d.focused = focus
	d.refresh()
}

// Update processes viewport and hunk navigation.
func (d Diff) Update(msg tea.Msg) (Diff, tea.Cmd) {
	if !d.visible {
		return d, nil
	}

	if km, ok := msg.(tea.KeyMsg); ok && d.focused {
		switch km.String() {
		case "n", "down", "j":
			if d.hunkCursor < len(d.hunks)-1 {
				d.hunkCursor++
				d.refresh()
				// Scroll viewport to active hunk (approximate)
				d.viewport.SetYOffset(d.hunkCursor * 5)
			}
		case "p", "up", "k":
			if d.hunkCursor > 0 {
				d.hunkCursor--
				d.refresh()
				d.viewport.SetYOffset(d.hunkCursor * 5)
			}
		case "a":
			// Accept logic would go here, for now just visual feedback
		case "r":
			// Reject logic would go here
		}
	}

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
	actionBar := d.styles.DiffAction.Render(" [A]ccept Hunk   [R]eject Hunk   [n/p] Next/Prev   [tab] Focus Input")

	layout := lipgloss.JoinVertical(lipgloss.Left, content, "\n", actionBar)

	if d.focused {
		return d.styles.ActivePane.Render(layout)
	}
	return d.styles.InactivePane.Render(layout)
}
