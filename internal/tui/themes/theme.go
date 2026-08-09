package themes

import "github.com/charmbracelet/lipgloss"

// Theme defines the color palette for the TUI.
type Theme struct {
	Name string

	Background    lipgloss.Color
	Surface       lipgloss.Color
	Overlay       lipgloss.Color
	Text          lipgloss.Color
	Subtext       lipgloss.Color
	Muted         lipgloss.Color
	Accent        lipgloss.Color
	AccentAlt     lipgloss.Color
	Green         lipgloss.Color
	Red           lipgloss.Color
	Yellow        lipgloss.Color
	Blue          lipgloss.Color
	Magenta       lipgloss.Color
	Cyan          lipgloss.Color
	BorderNormal  lipgloss.Color
	BorderFocused lipgloss.Color
}

// Styles pre-builds common lipgloss styles from a theme.
type Styles struct {
	T *Theme

	Base         lipgloss.Style
	Header       lipgloss.Style
	StatusBar    lipgloss.Style
	Input        lipgloss.Style
	UserMsg      lipgloss.Style
	AssistantMsg lipgloss.Style
	ToolCall     lipgloss.Style
	ToolResult   lipgloss.Style
	Error        lipgloss.Style
	Border       lipgloss.Style
	Code         lipgloss.Style
	Dim          lipgloss.Style
	Bold         lipgloss.Style
	Success      lipgloss.Style
	Warning      lipgloss.Style
}

// NewStyles builds Styles from a Theme.
func NewStyles(t *Theme) *Styles {
	s := &Styles{T: t}
	s.Base = lipgloss.NewStyle().Foreground(t.Text).Background(t.Background)
	s.Header = lipgloss.NewStyle().
		Background(t.Surface).
		Foreground(t.Text).
		Padding(0, 1).
		Bold(true)
	s.StatusBar = lipgloss.NewStyle().
		Background(t.Surface).
		Foreground(t.Subtext).
		Padding(0, 1)
	s.Input = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BorderNormal).
		Padding(0, 1)
	s.UserMsg = lipgloss.NewStyle().
		Foreground(t.Blue).
		Bold(true)
	s.AssistantMsg = lipgloss.NewStyle().
		Foreground(t.Text)
	s.ToolCall = lipgloss.NewStyle().
		Foreground(t.Yellow).
		Italic(true)
	s.ToolResult = lipgloss.NewStyle().
		Foreground(t.Muted)
	s.Error = lipgloss.NewStyle().
		Foreground(t.Red).
		Bold(true)
	s.Border = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BorderNormal)
	s.Code = lipgloss.NewStyle().
		Background(t.Surface).
		Foreground(t.Cyan)
	s.Dim = lipgloss.NewStyle().
		Foreground(t.Muted)
	s.Bold = lipgloss.NewStyle().
		Bold(true).
		Foreground(t.Text)
	s.Success = lipgloss.NewStyle().
		Foreground(t.Green)
	s.Warning = lipgloss.NewStyle().
		Foreground(t.Yellow)
	return s
}
