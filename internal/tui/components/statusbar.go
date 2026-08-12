package components

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

// StatusBar renders the bottom status bar with a HUD look matching the header.
type StatusBar struct {
	styles    *themes.Styles
	width     int
	status    string
	startTime time.Time
}

// NewStatusBar creates a new StatusBar.
func NewStatusBar(styles *themes.Styles) StatusBar {
	return StatusBar{
		styles:    styles,
		status:    "Ready",
		startTime: time.Now(),
	}
}

// SetWidth updates the status bar width.
func (s *StatusBar) SetWidth(w int) { s.width = w }

// SetStatus updates the status message.
func (s *StatusBar) SetStatus(msg string) { s.status = msg }

func (s *StatusBar) getStatusStyle() lipgloss.Style {
	base := lipgloss.NewStyle().Bold(true).Padding(0, 1).Foreground(s.styles.T.Background)
	status := strings.ToLower(s.status)
	switch {
	case strings.Contains(status, "thinking") || strings.Contains(status, "💭"):
		return base.Background(s.styles.T.Yellow)
	case strings.Contains(status, "error") || strings.Contains(status, "fail"):
		return base.Background(s.styles.T.Red)
	case strings.Contains(status, "ready"):
		return base.Background(s.styles.T.Accent)
	case strings.Contains(status, "done") || strings.Contains(status, "success"):
		return base.Background(s.styles.T.Green)
	default:
		return base.Background(s.styles.T.Blue)
	}
}

// View renders the status bar.
func (s StatusBar) View() string {
	if s.width <= 0 {
		return ""
	}

	// 1. Left Section: Status Badge
	statusBadge := s.getStatusStyle().Render(strings.ToUpper(s.status))
	left := lipgloss.JoinHorizontal(lipgloss.Center, " ", statusBadge)

	// 2. Center Section: Adaptive Shortcuts
	var shortcuts []string
	if s.width > 100 {
		shortcuts = []string{
			"󰌑 SEND",
			"󱊷 REVIEW",
			"󰜺 CANCEL",
			"󰈙 DIFF",
			"󰀦 HELP",
		}
	} else if s.width > 70 {
		shortcuts = []string{
			"ENTER",
			"CTRL+R",
			"ESC",
			"/DIFF",
			"?",
		}
	} else if s.width > 40 {
		shortcuts = []string{"󰌑", "󱊷", "󰜺", "󰈙", "󰀦"}
	}

	shortcutStyle := lipgloss.NewStyle().Foreground(s.styles.T.Muted)
	center := shortcutStyle.Render(strings.Join(shortcuts, "  "))

	// 3. Right Section: Clock & Session Timer
	clock := time.Now().Format("15:04")
	timer := time.Since(s.startTime).Round(time.Minute).String()
	right := lipgloss.NewStyle().Foreground(s.styles.T.Subtext).Render(fmt.Sprintf("%s │ %s ", timer, clock))

	// 4. Composition with Flex Gaps
	leftWidth := lipgloss.Width(left)
	centerWidth := lipgloss.Width(center)
	rightWidth := lipgloss.Width(right)

	availableWidth := s.width - 4
	gap1 := (availableWidth/2 - centerWidth/2) - leftWidth
	gap2 := availableWidth - (leftWidth + gap1 + centerWidth) - rightWidth

	if gap1 < 1 {
		gap1 = 1
	}
	if gap2 < 1 {
		gap2 = 1
	}

	content := lipgloss.JoinHorizontal(lipgloss.Center,
		left,
		strings.Repeat(" ", gap1),
		center,
		strings.Repeat(" ", gap2),
		right,
	)

	// Final Container: Top border for footer, same HUD style as header
	return lipgloss.NewStyle().
		Background(s.styles.T.Background).
		Foreground(s.styles.T.Text).
		Padding(0, 2).
		Width(s.width).
		Border(lipgloss.NormalBorder(), true, false, false, false). // Top border for footer
		BorderForeground(s.styles.T.BorderNormal).
		Render(content)
}
