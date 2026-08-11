package themes

import "charm.land/lipgloss/v2"

// Catppuccin returns the Catppuccin Mocha theme.
func Catppuccin() *Theme {
	return &Theme{
		Name:          "catppuccin",
		Background:    lipgloss.Color("#1e1e2e"),
		Surface:       lipgloss.Color("#313244"),
		Overlay:       lipgloss.Color("#45475a"),
		Text:          lipgloss.Color("#cdd6f4"),
		Subtext:       lipgloss.Color("#bac2de"),
		Muted:         lipgloss.Color("#6c7086"),
		Accent:        lipgloss.Color("#cba6f7"),
		AccentAlt:     lipgloss.Color("#89b4fa"),
		Green:         lipgloss.Color("#a6e3a1"),
		Red:           lipgloss.Color("#f38ba8"),
		Yellow:        lipgloss.Color("#f9e2af"),
		Blue:          lipgloss.Color("#89b4fa"),
		Magenta:       lipgloss.Color("#cba6f7"),
		Cyan:          lipgloss.Color("#89dceb"),
		BorderNormal:  lipgloss.Color("#45475a"),
		BorderFocused: lipgloss.Color("#cba6f7"),
	}
}
