package components

import (
	"fmt"
	"time"
	"unicode/utf8"

	"charm.land/lipgloss/v2"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

// StatusBar renders the bottom status bar.
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

// View renders the status bar.
func (s StatusBar) View() string {
	if s.width <= 0 {
		return ""
	}

	help := "enter send │ esc interrupt │ ctrl+c x2 quit │ ctrl+r review"
	leftRaw := fmt.Sprintf("  %s", s.status)
	rightRaw := fmt.Sprintf("%s  ", help)

	// Ensure the composed status line always fits in one terminal row.
	maxLeft := s.width - utf8.RuneCountInString(rightRaw)
	if maxLeft < 1 {
		maxLeft = 1
	}
	if utf8.RuneCountInString(leftRaw) > maxLeft {
		r := []rune(leftRaw)
		if maxLeft > 1 {
			leftRaw = string(r[:maxLeft-1]) + "…"
		} else {
			leftRaw = "…"
		}
	}

	left := s.styles.StatusBar.Render(leftRaw)
	right := s.styles.StatusBarRight.Render(rightRaw)

	spacer := s.width - lipgloss.Width(left) - lipgloss.Width(right)
	if spacer < 0 {
		spacer = 0
	}

	return left + lipgloss.NewStyle().Width(spacer).Render("") + right
}
