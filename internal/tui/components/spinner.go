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
	label  string
}

// NewSpinner creates a new Spinner component.
func NewSpinner(styles *themes.Styles) Spinner {
	sp := spinner.New()
	sp.Spinner = spinner.Points
	sp.Style = lipgloss.NewStyle().Foreground(styles.T.Accent)
	return Spinner{sp: sp, styles: styles, label: "thinking"}
}

// Start activates the spinner with an optional label.
func (s *Spinner) Start() { s.active = true }

// SetLabel updates the spinner label.
func (s *Spinner) SetLabel(label string) { s.label = label }

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
	label := s.label
	if label == "" {
		label = "thinking"
	}
	return s.sp.View() + " " + s.styles.Dim.Render(label+"…")
}
