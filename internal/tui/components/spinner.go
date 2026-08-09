package components

import (
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

// Spinner wraps the bubbles spinner component.
type Spinner struct {
	sp     spinner.Model
	styles *themes.Styles
	active bool
}

// NewSpinner creates a new Spinner component.
func NewSpinner(styles *themes.Styles) Spinner {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(styles.T.Accent)
	return Spinner{sp: sp, styles: styles}
}

// Start activates the spinner.
func (s *Spinner) Start() { s.active = true }

// Stop deactivates the spinner.
func (s *Spinner) Stop() { s.active = false }

// Active reports whether the spinner is running.
func (s Spinner) Active() bool { return s.active }

// Tick returns the tick command for the spinner.
func (s Spinner) Tick() tea.Cmd { return s.sp.Tick }

// Update handles spinner tick messages.
func (s Spinner) Update(msg tea.Msg) (Spinner, tea.Cmd) {
	sp, cmd := s.sp.Update(msg)
	s.sp = sp
	return s, cmd
}

// View renders the spinner.
func (s Spinner) View() string {
	if !s.active {
		return ""
	}
	return s.sp.View() + " thinking…"
}
