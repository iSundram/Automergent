package components

import (
	"os"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

// Spinner wraps the bubbles spinner component. It honors
// AUTOMERGENT_REDUCED_MOTION=1 by rendering a static indicator instead of an
// animation.
type Spinner struct {
	sp            spinner.Model
	styles        *themes.Styles
	active        bool
	label         string
	meta          string // live parenthetical: "12s • ↓ 1.2k tokens"
	reducedMotion bool
}

// NewSpinner creates a new Spinner component.
func NewSpinner(styles *themes.Styles) Spinner {
	sp := spinner.New()
	if os.Getenv("AUTOMERGENT_REDUCED_MOTION") == "1" || os.Getenv("AUTOMERGENT_REDUCED_MOTION") == "true" {
		sp.Spinner = spinner.Spinner{Frames: []string{"…"}, FPS: 0}
	}
	sp.Spinner = spinner.Points
	sp.Style = lipgloss.NewStyle().Foreground(styles.T.Accent)
	return Spinner{
		sp:            sp,
		styles:        styles,
		label:         "thinking",
		reducedMotion: os.Getenv("AUTOMERGENT_REDUCED_MOTION") == "1" || os.Getenv("AUTOMERGENT_REDUCED_MOTION") == "true",
	}
}

// Start activates the spinner with an optional label.
func (s *Spinner) Start() { s.active = true }

// SetLabel updates the spinner label.
func (s *Spinner) SetLabel(label string) { s.label = label }

// SetMeta sets the live parenthetical shown after the label — the Claude
// Code-style "(12s • ↓ 1.2k tokens)" readout of elapsed time and streamed
// tokens. An empty string drops it.
func (s *Spinner) SetMeta(meta string) { s.meta = meta }

// Stop deactivates the spinner.
func (s *Spinner) Stop() { s.active = false }

// Active reports whether the spinner is running.
func (s Spinner) Active() bool { return s.active }

// Tick returns the tick command for the spinner.
func (s Spinner) Tick() tea.Cmd { return s.sp.Tick }

// Update handles spinner tick messages.
func (s Spinner) Update(msg tea.Msg) (Spinner, tea.Cmd) {
	if s.reducedMotion {
		return s, nil // no animation frames in reduced-motion mode
	}
	sp, cmd := s.sp.Update(msg)
	s.sp = sp
	return s, cmd
}

// View renders the spinner: "✣ thinking… (12s • ↓ 1.2k tokens)" — the verb
// carries the activity, the dim parenthetical carries the live clock and
// token counter, matching Claude Code's spinner row.
func (s Spinner) View() string {
	if !s.active {
		return ""
	}
	label := s.label
	if label == "" {
		label = "thinking"
	}
	if s.meta != "" {
		label += "… (" + s.meta + ")"
	} else {
		label += "…"
	}
	if s.reducedMotion {
		return "… " + s.styles.Dim.Render(label)
	}
	return s.sp.View() + " " + s.styles.Dim.Render(label)
}
