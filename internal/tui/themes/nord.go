package themes

import "github.com/charmbracelet/lipgloss"

// Nord returns the Nord color theme.
func Nord() *Theme {
	return &Theme{
		Name:          "nord",
		Background:    lipgloss.Color("#2e3440"),
		Surface:       lipgloss.Color("#3b4252"),
		Overlay:       lipgloss.Color("#434c5e"),
		Text:          lipgloss.Color("#eceff4"),
		Subtext:       lipgloss.Color("#e5e9f0"),
		Muted:         lipgloss.Color("#4c566a"),
		Accent:        lipgloss.Color("#88c0d0"),
		AccentAlt:     lipgloss.Color("#81a1c1"),
		Green:         lipgloss.Color("#a3be8c"),
		Red:           lipgloss.Color("#bf616a"),
		Yellow:        lipgloss.Color("#ebcb8b"),
		Blue:          lipgloss.Color("#81a1c1"),
		Magenta:       lipgloss.Color("#b48ead"),
		Cyan:          lipgloss.Color("#88c0d0"),
		BorderNormal:  lipgloss.Color("#434c5e"),
		BorderFocused: lipgloss.Color("#88c0d0"),
	}
}
