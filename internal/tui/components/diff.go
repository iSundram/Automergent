package components

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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
	viewport    viewport.Model
	styles      *themes.Styles
	visible     bool
	focused     bool
	hunks       []Hunk
	hunkCursor  int
	Summary     string
	Filename    string
	HideActions bool
}

// NewDiff creates a new Diff component.
func NewDiff(styles *themes.Styles) Diff {
	vp := viewport.New(viewport.WithWidth(40), viewport.WithHeight(20))
	vp.MouseWheelEnabled = false // Enforce keyboard-only scrolling
	return Diff{viewport: vp, styles: styles}
}

// SetSize updates the component dimensions.
func (d *Diff) SetSize(w, h int) {
	if w < 10 {
		w = 10
	}
	if h < 5 {
		h = 5
	}
	d.viewport.SetWidth(w - 2)
	d.viewport.SetHeight(h - 4) // Reserve space for header and optional action bar
	d.refresh()
}

// SetContent sets and parses the diff content.
func (d *Diff) SetContent(content string) {
	// Extract filename and summary
	plus := strings.Count(content, "\n+")
	minus := strings.Count(content, "\n-")
	d.Summary = fmt.Sprintf("󰐙 %d  󰍵 %d", plus, minus)

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "+++ ") {
			d.Filename = strings.TrimPrefix(line, "+++ ")
			if idx := strings.Index(d.Filename, "\t"); idx != -1 {
				d.Filename = d.Filename[:idx]
			}
			d.Filename = strings.TrimPrefix(d.Filename, "b/")
			break
		}
	}

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
		content := hunk.Content

		// If hunk is very large and not focused, truncate it for the preview
		lines := strings.Split(content, "\n")
		if len(lines) > 20 && i != d.hunkCursor {
			// Show first 5 and last 5 lines
			newLines := append(lines[:5], "  ...")
			newLines = append(newLines, lines[len(lines)-5:]...)
			content = strings.Join(newLines, "\n")
		}

		rendered := render.Diff(content)
		if i == d.hunkCursor && d.focused {
			// Clean highlight: Left border and surface background
			rendered = lipgloss.NewStyle().
				Background(d.styles.T.Surface).
				Border(lipgloss.NormalBorder(), false, false, false, true).
				BorderForeground(d.styles.T.Accent).
				Width(d.viewport.Width()-1).
				Padding(0, 1).
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

	// Header
	headerLabel := d.styles.Bold.Foreground(d.styles.T.Accent).Render(" 󰛓 DIFF: " + d.Filename)

	hunkInfo := ""
	if len(d.hunks) > 1 {
		hunkInfo = fmt.Sprintf(" [Hunk %d/%d] ", d.hunkCursor, len(d.hunks)-1)
	}
	summaryLabel := d.styles.Dim.Render(hunkInfo + d.Summary + " ")

	availableWidth := d.viewport.Width() + 2
	spacerWidth := availableWidth - lipgloss.Width(headerLabel) - lipgloss.Width(summaryLabel)
	if spacerWidth < 1 {
		spacerWidth = 1
	}
	header := headerLabel + strings.Repeat(" ", spacerWidth) + summaryLabel

	content := d.viewport.View()

	var sections []string
	sections = append(sections, header, lipgloss.NewStyle().Foreground(d.styles.T.BorderNormal).Faint(true).Render(strings.Repeat("─", availableWidth)), content)

	if !d.HideActions {
		// Floating action bar for Diff - navigation only
		actionBar := d.styles.DiffAction.Render(" [n/p] Next/Prev Hunk   [up/down] Scroll   [tab] Focus Input")
		sections = append(sections, "\n", actionBar)
	}

	layout := lipgloss.JoinVertical(lipgloss.Left, sections...)

	if d.focused {
		return d.styles.ActivePane.Width(availableWidth).Render(layout)
	}
	return d.styles.InactivePane.Width(availableWidth).Render(layout)
}
