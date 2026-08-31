package themes

import "charm.land/lipgloss/v2"

// Modern returns a clean, grayscale-focused modern theme.
func Modern() *Theme {
	return &Theme{
		Name:          "modern",
		Background:    lipgloss.Color("#2b2b2b"),
		Surface:       lipgloss.Color("#3c3c3c"),
		Overlay:       lipgloss.Color("#4a4a4a"),
		Text:          lipgloss.Color("#ffffff"),
		Subtext:       lipgloss.Color("#d4d4d4"),
		Muted:         lipgloss.Color("#b3b3b3"),
		Accent:        lipgloss.Color("#ffffff"),
		AccentAlt:     lipgloss.Color("#d4d4d4"),
		Green:         lipgloss.Color("#a6e3a1"), // Catppuccin Green for good visibility
		Red:           lipgloss.Color("#f38ba8"), // Catppuccin Red
		Yellow:        lipgloss.Color("#f9e2af"),
		Orange:        lipgloss.Color("#fab387"), // Catppuccin Yellow
		Blue:          lipgloss.Color("#89b4fa"), // Catppuccin Blue
		Magenta:       lipgloss.Color("#cba6f7"), // Catppuccin Magenta
		Cyan:          lipgloss.Color("#89dceb"), // Catppuccin Cyan
		BorderNormal:  lipgloss.Color("#4a4a4a"),
		BorderFocused: lipgloss.Color("#ffffff"),
	}
}
