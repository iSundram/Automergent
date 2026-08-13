package components

import (
	"fmt"
	"regexp"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/iSundram/Automergent/internal/agent"
	"github.com/iSundram/Automergent/internal/tui/themes"
)

// DiffAcceptMsg is sent when user accepts the diff.
type DiffAcceptMsg struct{}

// DiffRejectMsg is sent when user rejects the diff.
type DiffRejectMsg struct{}

// Hunk represents a single change block in a diff.
type Hunk struct {
	StartLine int
	LineCount int
}

type diffMode int

const (
	diffModeView diffMode = iota
	diffModeConfirm
	diffModeRejectReason
)

// Diff is a fullscreen scrollable diff viewer with inline confirmation.
type Diff struct {
	viewport   viewport.Model
	styles     *themes.Styles
	visible    bool
	rawContent string
	hunks      []Hunk
	hunkCursor int
	Filename   string
	AddCount   int
	DelCount   int
	TotalLines int
	width      int
	height     int

	// Confirmation
	mode        diffMode
	replyCh     chan agent.ConfirmationResponse
	rejectInput textinput.Model
}

// hunkHeaderRe matches unified diff hunk headers
var hunkHeaderRe = regexp.MustCompile(`^@@\s+-\d+(?:,\d+)?\s+\+\d+(?:,\d+)?\s+@@`)

// NewDiff creates a new Diff component.
func NewDiff(styles *themes.Styles) Diff {
	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(20))
	vp.MouseWheelEnabled = true

	ti := textinput.New()
	ti.Placeholder = "optional reason..."
	ti.Prompt = ""
	ti.Focus()

	return Diff{viewport: vp, styles: styles, rejectInput: ti}
}

// SetSize updates dimensions for fullscreen.
func (d *Diff) SetSize(w, h int) {
	d.width = w
	d.height = h
	d.viewport.SetWidth(w - 6)
	d.viewport.SetHeight(h - 5)
	d.refresh()
}

// SetContent parses and sets the diff content.
func (d *Diff) SetContent(content string) {
	d.rawContent = content
	lines := strings.Split(content, "\n")
	d.TotalLines = len(lines)

	// Count changes
	d.AddCount, d.DelCount = 0, 0
	for _, line := range lines {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			d.AddCount++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			d.DelCount++
		}
	}

	// Extract filename
	d.Filename = ""
	for _, line := range lines {
		if strings.HasPrefix(line, "+++ ") {
			d.Filename = strings.TrimPrefix(line, "+++ ")
			d.Filename = strings.TrimPrefix(d.Filename, "b/")
			if idx := strings.Index(d.Filename, "\t"); idx != -1 {
				d.Filename = d.Filename[:idx]
			}
			if strings.HasSuffix(d.Filename, " (proposed)") {
				d.Filename = strings.TrimSuffix(d.Filename, " (proposed)")
			}
			break
		}
	}

	// Find hunks
	d.hunks = nil
	for i, line := range lines {
		if hunkHeaderRe.MatchString(line) {
			d.hunks = append(d.hunks, Hunk{StartLine: i})
		}
	}
	d.hunkCursor = 0

	d.refresh()
}

// ShowWithConfirm shows diff and waits for user confirmation.
func (d *Diff) ShowWithConfirm(replyCh chan agent.ConfirmationResponse) {
	d.visible = true
	d.mode = diffModeConfirm
	d.replyCh = replyCh
	d.rejectInput.Reset()
	d.scrollToFirstChange()
}

// scrollToFirstChange scrolls viewport to show the first changed line (+ or -).
func (d *Diff) scrollToFirstChange() {
	lines := strings.Split(d.rawContent, "\n")

	// Find first actual change line (+ or - but not --- or +++)
	for i, line := range lines {
		if len(line) > 0 {
			if (line[0] == '+' && !strings.HasPrefix(line, "+++")) ||
				(line[0] == '-' && !strings.HasPrefix(line, "---")) {
				// Found first change, scroll to show it with some context above
				target := i - 3
				if target < 0 {
					target = 0
				}
				d.viewport.SetYOffset(target)
				return
			}
		}
	}
	// No changes found, stay at top
	d.viewport.SetYOffset(0)
}

func (d *Diff) refresh() {
	if d.rawContent == "" {
		d.viewport.SetContent("")
		return
	}

	lines := strings.Split(d.rawContent, "\n")
	contentW := d.viewport.Width()

	// Colors
	addBg := lipgloss.Color("#143d14")
	delBg := lipgloss.Color("#3d1414")
	addFg := lipgloss.Color("#a6e3a1")
	delFg := lipgloss.Color("#f38ba8")
	hunkFg := lipgloss.Color("#89b4fa")
	fileFg := lipgloss.Color("#cba6f7")
	numFg := lipgloss.Color("#6c7086")
	ctxFg := lipgloss.Color("#9399b2")
	prefixBg := lipgloss.Color("#1e3a5f") // Blue highlight for +/- prefix

	var sb strings.Builder
	lineNum := 0

	for _, line := range lines {
		pad := func(s string) string {
			w := lipgloss.Width(s)
			if w < contentW {
				return s + strings.Repeat(" ", contentW-w)
			}
			return s
		}

		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			styled := lipgloss.NewStyle().Foreground(fileFg).Bold(true).Render(line)
			sb.WriteString("     " + styled + "\n")

		case strings.HasPrefix(line, "@@"):
			styled := lipgloss.NewStyle().Foreground(hunkFg).Bold(true).Render(line)
			sb.WriteString("\n     " + styled + "\n")
			lineNum = 0

		case strings.HasPrefix(line, "+"):
			lineNum++
			num := lipgloss.NewStyle().Foreground(numFg).Width(4).Align(lipgloss.Right).Render(fmt.Sprintf("%d", lineNum))
			// Blue highlight on +, green on rest
			prefix := lipgloss.NewStyle().Background(prefixBg).Foreground(addFg).Bold(true).Render("+")
			rest := lipgloss.NewStyle().Background(addBg).Foreground(addFg).Render(pad(line[1:]))
			sb.WriteString(num + " " + prefix + rest + "\n")

		case strings.HasPrefix(line, "-"):
			num := lipgloss.NewStyle().Foreground(numFg).Width(4).Align(lipgloss.Right).Render("-")
			// Blue highlight on -, red on rest
			prefix := lipgloss.NewStyle().Background(prefixBg).Foreground(delFg).Bold(true).Render("-")
			rest := lipgloss.NewStyle().Background(delBg).Foreground(delFg).Render(pad(line[1:]))
			sb.WriteString(num + " " + prefix + rest + "\n")

		case line == "":
			lineNum++
			sb.WriteString("\n")

		default:
			lineNum++
			num := lipgloss.NewStyle().Foreground(numFg).Width(4).Align(lipgloss.Right).Render(fmt.Sprintf("%d", lineNum))
			content := lipgloss.NewStyle().Foreground(ctxFg).Render(line)
			sb.WriteString(num + " " + content + "\n")
		}
	}

	d.viewport.SetContent(sb.String())
}

// Toggle visibility.
func (d *Diff) Toggle() { d.visible = !d.visible }

// Show the diff.
func (d *Diff) Show() {
	d.visible = true
	d.scrollToFirstChange()
}

// Hide the diff and reset mode.
func (d *Diff) Hide() {
	d.visible = false
	d.mode = diffModeView
	d.replyCh = nil
	d.rejectInput.Reset()
}

// Visible returns visibility state.
func (d *Diff) Visible() bool { return d.visible }

// Focus is no-op for fullscreen.
func (d *Diff) Focus(_ bool) {}

// Update handles input.
func (d Diff) Update(msg tea.Msg) (Diff, tea.Cmd) {
	if !d.visible {
		return d, nil
	}

	// Handle reject reason input mode
	if d.mode == diffModeRejectReason {
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.String() {
			case "enter":
				// Submit rejection with reason
				reason := strings.TrimSpace(d.rejectInput.Value())
				if d.replyCh != nil {
					select {
					case d.replyCh <- agent.ConfirmationResponse{Allow: false, Feedback: reason}:
					default:
					}
				}
				d.Hide()
				return d, nil
			case "esc":
				// Skip reason, just reject
				if d.replyCh != nil {
					select {
					case d.replyCh <- agent.ConfirmationResponse{Allow: false}:
					default:
					}
				}
				d.Hide()
				return d, nil
			}
		}
		// Update text input
		var cmd tea.Cmd
		d.rejectInput, cmd = d.rejectInput.Update(msg)
		return d, cmd
	}

	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "y", "Y", "enter":
			if d.mode == diffModeConfirm && d.replyCh != nil {
				select {
				case d.replyCh <- agent.ConfirmationResponse{Allow: true}:
				default:
				}
				d.Hide()
				return d, nil
			}
		case "a", "A":
			if d.mode == diffModeConfirm && d.replyCh != nil {
				select {
				case d.replyCh <- agent.ConfirmationResponse{Allow: true, Always: true}:
				default:
				}
				d.Hide()
				return d, nil
			}
		case "n", "N":
			if d.mode == diffModeConfirm && d.replyCh != nil {
				// Enter reject reason mode
				d.mode = diffModeRejectReason
				d.rejectInput.Focus()
				return d, textinput.Blink
			}
			d.visible = false
			return d, nil
		case "q", "esc":
			if d.mode == diffModeConfirm && d.replyCh != nil {
				// Quick reject without reason
				select {
				case d.replyCh <- agent.ConfirmationResponse{Allow: false}:
				default:
				}
				d.Hide()
				return d, nil
			}
			d.visible = false
			return d, nil
		case "g":
			d.viewport.SetYOffset(0)
			return d, nil
		case "G":
			d.viewport.GotoBottom()
			return d, nil
		case "j", "down":
			vp, cmd := d.viewport.Update(msg)
			d.viewport = vp
			return d, cmd
		case "k", "up":
			vp, cmd := d.viewport.Update(msg)
			d.viewport = vp
			return d, cmd
		}
	}

	vp, cmd := d.viewport.Update(msg)
	d.viewport = vp
	return d, cmd
}

// View renders fullscreen diff with inline confirmation.
func (d Diff) View() string {
	if !d.visible {
		return ""
	}

	bg := d.styles.T.Background

	// Header
	icon := lipgloss.NewStyle().Foreground(d.styles.T.Accent).Render("󰈙 ")
	filename := lipgloss.NewStyle().Bold(true).Foreground(d.styles.T.Text).Render(d.Filename)
	addLabel := lipgloss.NewStyle().Foreground(d.styles.T.Green).Bold(true).Render(fmt.Sprintf("+%d", d.AddCount))
	delLabel := lipgloss.NewStyle().Foreground(d.styles.T.Red).Bold(true).Render(fmt.Sprintf("-%d", d.DelCount))

	headerW := d.width - 4
	headerLeft := icon + filename
	headerRight := addLabel + "  " + delLabel
	spacer := headerW - lipgloss.Width(headerLeft) - lipgloss.Width(headerRight)
	if spacer < 1 {
		spacer = 1
	}
	header := headerLeft + strings.Repeat(" ", spacer) + headerRight

	// Separator
	sep := lipgloss.NewStyle().Foreground(d.styles.T.BorderNormal).Render(strings.Repeat("─", headerW))

	// Content
	content := d.viewport.View()

	// Scroll indicator
	yOffset := d.viewport.YOffset()
	var scrollInfo string
	if yOffset > 0 {
		scrollInfo = d.styles.Dim.Render(fmt.Sprintf("↑ %d hidden", yOffset))
	}
	remaining := d.TotalLines - yOffset - d.viewport.Height()
	if remaining > 0 {
		if scrollInfo != "" {
			scrollInfo += "  "
		}
		scrollInfo += d.styles.Dim.Render(fmt.Sprintf("↓ %d more", remaining))
	}

	// Footer - different for each mode
	var footer string
	switch d.mode {
	case diffModeRejectReason:
		// Reject reason input - hide hint once user starts typing
		label := lipgloss.NewStyle().Foreground(d.styles.T.Yellow).Bold(true).Render("Reject reason: ")
		input := d.rejectInput.View()
		var hint string
		if d.rejectInput.Value() == "" {
			hint = d.styles.Dim.Render("  [Enter] submit  [Esc] skip")
		}
		footer = label + input + hint

	case diffModeConfirm:
		// Confirmation buttons in footer
		yBtn := lipgloss.NewStyle().Background(d.styles.T.Green).Foreground(d.styles.T.Background).Bold(true).Padding(0, 1).Render("y Accept")
		aBtn := lipgloss.NewStyle().Background(d.styles.T.Accent).Foreground(d.styles.T.Background).Bold(true).Padding(0, 1).Render("a Always")
		nBtn := lipgloss.NewStyle().Background(d.styles.T.Red).Foreground(d.styles.T.Background).Bold(true).Padding(0, 1).Render("n Reject")
		nav := d.styles.Dim.Render("[j/k] scroll  [esc] cancel")

		buttons := yBtn + "  " + aBtn + "  " + nBtn + "    " + nav

		footerSpacer := headerW - lipgloss.Width(scrollInfo) - lipgloss.Width(buttons)
		if footerSpacer < 1 {
			footerSpacer = 1
		}
		footer = scrollInfo + strings.Repeat(" ", footerSpacer) + buttons

	default:
		help := d.styles.Dim.Render("[j/k] scroll  [g/G] top/end  [q] close")
		footerSpacer := headerW - lipgloss.Width(scrollInfo) - lipgloss.Width(help)
		if footerSpacer < 1 {
			footerSpacer = 1
		}
		footer = scrollInfo + strings.Repeat(" ", footerSpacer) + help
	}

	// Assemble
	view := lipgloss.JoinVertical(lipgloss.Left,
		header,
		sep,
		content,
		sep,
		footer,
	)

	// Fullscreen with solid background
	return lipgloss.NewStyle().
		Background(bg).
		Foreground(d.styles.T.Text).
		Width(d.width).
		Height(d.height).
		Padding(1, 2).
		Render(view)
}
