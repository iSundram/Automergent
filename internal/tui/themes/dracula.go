package themes

import "github.com/charmbracelet/lipgloss"

// Dracula returns the Dracula theme.
func Dracula() *Theme {
	return &Theme{
		Name:          "dracula",
		Background:    lipgloss.Color("#282a36"),
		Surface:       lipgloss.Color("#44475a"),
		Overlay:       lipgloss.Color("#6272a4"),
		Text:          lipgloss.Color("#f8f8f2"),
		Subtext:       lipgloss.Color("#e0e0e0"),
		Muted:         lipgloss.Color("#6272a4"),
		Accent:        lipgloss.Color("#bd93f9"),
		AccentAlt:     lipgloss.Color("#50fa7b"),
		Green:         lipgloss.Color("#50fa7b"),
		Red:           lipgloss.Color("#ff5555"),
		Yellow:        lipgloss.Color("#f1fa8c"),
		Blue:          lipgloss.Color("#8be9fd"),
		Magenta:       lipgloss.Color("#bd93f9"),
		Cyan:          lipgloss.Color("#8be9fd"),
		BorderNormal:  lipgloss.Color("#44475a"),
		BorderFocused: lipgloss.Color("#bd93f9"),
	}
}

// Get returns a theme by name, defaulting to Catppuccin.
func Get(name string) *Theme {
	switch name {
	case "dracula":
		return Dracula()
	case "nord":
		return Nord()
	default:
		return Catppuccin()
	}
}
