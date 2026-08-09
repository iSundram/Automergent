package components

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"

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
	left := s.styles.StatusBar.Render(fmt.Sprintf("  %s", s.status))
	help := "enter send │ ctrl+c quit │ ctrl+d diff │ ctrl+t tree │ ? help"
	right := s.styles.StatusBarRight.Render(fmt.Sprintf("%s  ", help))

	spacer := s.width - lipgloss.Width(left) - lipgloss.Width(right)
	if spacer < 0 {
		spacer = 0
	}

	// Minimalist footer without heavy background
	return left + lipgloss.NewStyle().Width(spacer).Render("") + right
}
