package themes

import (
	"image/color"
	"sync"

	"charm.land/lipgloss/v2"
)

// ThemeChangedMsg is sent when the active theme changes.
type ThemeChangedMsg struct {
	Theme *Theme
}

// ThemeEngine provides hot-swappable theme management.
type ThemeEngine struct {
	mu sync.RWMutex

	current      *Theme
	styles       *Styles
	themes       map[string]*Theme
	listeners    []func(*Theme)
	syntaxScheme string
}

// NewThemeEngine creates a new theme engine with default themes.
func NewThemeEngine() *ThemeEngine {
	te := &ThemeEngine{
		themes:       make(map[string]*Theme),
		syntaxScheme: "monokai",
	}

	// Register default themes
	te.RegisterTheme(Catppuccin())
	te.RegisterTheme(Dracula())
	te.RegisterTheme(Nord())
	te.RegisterTheme(Gruvbox())
	te.RegisterTheme(OneDark())
	te.RegisterTheme(TokyoNight())
	te.RegisterTheme(SolarizedDark())
	te.RegisterTheme(SolarizedLight())
	te.RegisterTheme(HighContrast())
	te.RegisterTheme(Monokai())
	te.RegisterTheme(Modern())

	// Set default
	te.current = Modern()
	te.styles = NewStyles(te.current)

	return te
}

// RegisterTheme adds a theme to the engine.
func (te *ThemeEngine) RegisterTheme(theme *Theme) {
	te.mu.Lock()
	te.themes[theme.Name] = theme
	te.mu.Unlock()
}

// SetTheme switches to a named theme.
func (te *ThemeEngine) SetTheme(name string) bool {
	te.mu.Lock()
	defer te.mu.Unlock()

	theme, exists := te.themes[name]
	if !exists {
		return false
	}

	te.current = theme
	te.styles = NewStyles(theme)

	// Notify listeners
	for _, listener := range te.listeners {
		listener(theme)
	}

	return true
}

// SetCustomTheme sets a custom theme directly.
func (te *ThemeEngine) SetCustomTheme(theme *Theme) {
	te.mu.Lock()
	defer te.mu.Unlock()

	te.current = theme
	te.themes[theme.Name] = theme
	te.styles = NewStyles(theme)

	for _, listener := range te.listeners {
		listener(theme)
	}
}

// Current returns the current theme.
func (te *ThemeEngine) Current() *Theme {
	te.mu.RLock()
	defer te.mu.RUnlock()
	return te.current
}

// Styles returns the current styles.
func (te *ThemeEngine) Styles() *Styles {
	te.mu.RLock()
	defer te.mu.RUnlock()
	return te.styles
}

// AvailableThemes returns all registered theme names.
func (te *ThemeEngine) AvailableThemes() []string {
	te.mu.RLock()
	defer te.mu.RUnlock()

	names := make([]string, 0, len(te.themes))
	for name := range te.themes {
		names = append(names, name)
	}
	return names
}

// OnThemeChange registers a callback for theme changes.
func (te *ThemeEngine) OnThemeChange(callback func(*Theme)) {
	te.mu.Lock()
	te.listeners = append(te.listeners, callback)
	te.mu.Unlock()
}

// CycleTheme switches to the next theme in order.
func (te *ThemeEngine) CycleTheme() string {
	te.mu.Lock()
	defer te.mu.Unlock()

	names := make([]string, 0, len(te.themes))
	for name := range te.themes {
		names = append(names, name)
	}

	currentIdx := 0
	for i, name := range names {
		if name == te.current.Name {
			currentIdx = i
			break
		}
	}

	nextIdx := (currentIdx + 1) % len(names)
	nextTheme := te.themes[names[nextIdx]]

	te.current = nextTheme
	te.styles = NewStyles(nextTheme)

	for _, listener := range te.listeners {
		listener(nextTheme)
	}

	return nextTheme.Name
}

// SetSyntaxScheme sets the syntax highlighting scheme.
func (te *ThemeEngine) SetSyntaxScheme(scheme string) {
	te.mu.Lock()
	te.syntaxScheme = scheme
	te.mu.Unlock()
}

// SyntaxScheme returns the current syntax highlighting scheme.
func (te *ThemeEngine) SyntaxScheme() string {
	te.mu.RLock()
	defer te.mu.RUnlock()
	return te.syntaxScheme
}

// CreateCustomTheme creates a custom theme from color values.
func CreateCustomTheme(name string, colors map[string]string) *Theme {
	parseColor := func(hex string) color.Color {
		if hex == "" {
			return lipgloss.Color("#ffffff")
		}
		return lipgloss.Color(hex)
	}
	// Orange is optional in custom themes: blend toward red when absent so
	// effort-ladder rendering keeps a distinct warm color.
	orange := colors["orange"]
	if orange == "" {
		orange = colors["red"]
	}

	return &Theme{
		Name:          name,
		Background:    parseColor(colors["background"]),
		Surface:       parseColor(colors["surface"]),
		Overlay:       parseColor(colors["overlay"]),
		Text:          parseColor(colors["text"]),
		Subtext:       parseColor(colors["subtext"]),
		Muted:         parseColor(colors["muted"]),
		Accent:        parseColor(colors["accent"]),
		AccentAlt:     parseColor(colors["accent_alt"]),
		Green:         parseColor(colors["green"]),
		Red:           parseColor(colors["red"]),
		Yellow:        parseColor(colors["yellow"]),
		Orange:        parseColor(orange),
		Blue:          parseColor(colors["blue"]),
		Magenta:       parseColor(colors["magenta"]),
		Cyan:          parseColor(colors["cyan"]),
		BorderNormal:  parseColor(colors["border_normal"]),
		BorderFocused: parseColor(colors["border_focused"]),
	}
}

// Gruvbox returns the Gruvbox dark theme.
func Gruvbox() *Theme {
	return &Theme{
		Name:          "gruvbox",
		Background:    lipgloss.Color("#282828"),
		Surface:       lipgloss.Color("#3c3836"),
		Overlay:       lipgloss.Color("#504945"),
		Text:          lipgloss.Color("#ebdbb2"),
		Subtext:       lipgloss.Color("#d5c4a1"),
		Muted:         lipgloss.Color("#928374"),
		Accent:        lipgloss.Color("#fe8019"),
		AccentAlt:     lipgloss.Color("#fabd2f"),
		Green:         lipgloss.Color("#b8bb26"),
		Red:           lipgloss.Color("#fb4934"),
		Yellow:        lipgloss.Color("#fabd2f"),
		Orange:        lipgloss.Color("#fe8019"),
		Blue:          lipgloss.Color("#83a598"),
		Magenta:       lipgloss.Color("#d3869b"),
		Cyan:          lipgloss.Color("#8ec07c"),
		BorderNormal:  lipgloss.Color("#504945"),
		BorderFocused: lipgloss.Color("#fe8019"),
	}
}

// OneDark returns the Atom One Dark theme.
func OneDark() *Theme {
	return &Theme{
		Name:          "onedark",
		Background:    lipgloss.Color("#282c34"),
		Surface:       lipgloss.Color("#353b45"),
		Overlay:       lipgloss.Color("#3e4451"),
		Text:          lipgloss.Color("#abb2bf"),
		Subtext:       lipgloss.Color("#9da5b4"),
		Muted:         lipgloss.Color("#5c6370"),
		Accent:        lipgloss.Color("#61afef"),
		AccentAlt:     lipgloss.Color("#c678dd"),
		Green:         lipgloss.Color("#98c379"),
		Red:           lipgloss.Color("#e06c75"),
		Yellow:        lipgloss.Color("#e5c07b"),
		Orange:        lipgloss.Color("#d19a66"),
		Blue:          lipgloss.Color("#61afef"),
		Magenta:       lipgloss.Color("#c678dd"),
		Cyan:          lipgloss.Color("#56b6c2"),
		BorderNormal:  lipgloss.Color("#3e4451"),
		BorderFocused: lipgloss.Color("#61afef"),
	}
}

// TokyoNight returns the Tokyo Night theme.
func TokyoNight() *Theme {
	return &Theme{
		Name:          "tokyonight",
		Background:    lipgloss.Color("#1a1b26"),
		Surface:       lipgloss.Color("#24283b"),
		Overlay:       lipgloss.Color("#414868"),
		Text:          lipgloss.Color("#c0caf5"),
		Subtext:       lipgloss.Color("#a9b1d6"),
		Muted:         lipgloss.Color("#565f89"),
		Accent:        lipgloss.Color("#7aa2f7"),
		AccentAlt:     lipgloss.Color("#bb9af7"),
		Green:         lipgloss.Color("#9ece6a"),
		Red:           lipgloss.Color("#f7768e"),
		Yellow:        lipgloss.Color("#e0af68"),
		Orange:        lipgloss.Color("#ff9e64"),
		Blue:          lipgloss.Color("#7aa2f7"),
		Magenta:       lipgloss.Color("#bb9af7"),
		Cyan:          lipgloss.Color("#7dcfff"),
		BorderNormal:  lipgloss.Color("#414868"),
		BorderFocused: lipgloss.Color("#7aa2f7"),
	}
}

// SolarizedDark returns the Solarized Dark theme.
func SolarizedDark() *Theme {
	return &Theme{
		Name:          "solarized-dark",
		Background:    lipgloss.Color("#002b36"),
		Surface:       lipgloss.Color("#073642"),
		Overlay:       lipgloss.Color("#586e75"),
		Text:          lipgloss.Color("#839496"),
		Subtext:       lipgloss.Color("#93a1a1"),
		Muted:         lipgloss.Color("#657b83"),
		Accent:        lipgloss.Color("#268bd2"),
		AccentAlt:     lipgloss.Color("#2aa198"),
		Green:         lipgloss.Color("#859900"),
		Red:           lipgloss.Color("#dc322f"),
		Yellow:        lipgloss.Color("#b58900"),
		Orange:        lipgloss.Color("#cb4b16"),
		Blue:          lipgloss.Color("#268bd2"),
		Magenta:       lipgloss.Color("#d33682"),
		Cyan:          lipgloss.Color("#2aa198"),
		BorderNormal:  lipgloss.Color("#586e75"),
		BorderFocused: lipgloss.Color("#268bd2"),
	}
}

// SolarizedLight returns the Solarized Light theme.
func SolarizedLight() *Theme {
	return &Theme{
		Name:          "solarized-light",
		Background:    lipgloss.Color("#fdf6e3"),
		Surface:       lipgloss.Color("#eee8d5"),
		Overlay:       lipgloss.Color("#93a1a1"),
		Text:          lipgloss.Color("#657b83"),
		Subtext:       lipgloss.Color("#586e75"),
		Muted:         lipgloss.Color("#839496"),
		Accent:        lipgloss.Color("#268bd2"),
		AccentAlt:     lipgloss.Color("#2aa198"),
		Green:         lipgloss.Color("#859900"),
		Red:           lipgloss.Color("#dc322f"),
		Yellow:        lipgloss.Color("#b58900"),
		Orange:        lipgloss.Color("#cb4b16"),
		Blue:          lipgloss.Color("#268bd2"),
		Magenta:       lipgloss.Color("#d33682"),
		Cyan:          lipgloss.Color("#2aa198"),
		BorderNormal:  lipgloss.Color("#93a1a1"),
		BorderFocused: lipgloss.Color("#268bd2"),
	}
}

// HighContrast returns a high contrast accessibility theme.
func HighContrast() *Theme {
	return &Theme{
		Name:          "high-contrast",
		Background:    lipgloss.Color("#000000"),
		Surface:       lipgloss.Color("#1a1a1a"),
		Overlay:       lipgloss.Color("#333333"),
		Text:          lipgloss.Color("#ffffff"),
		Subtext:       lipgloss.Color("#e0e0e0"),
		Muted:         lipgloss.Color("#999999"),
		Accent:        lipgloss.Color("#00ff00"),
		AccentAlt:     lipgloss.Color("#00ffff"),
		Green:         lipgloss.Color("#00ff00"),
		Red:           lipgloss.Color("#ff0000"),
		Yellow:        lipgloss.Color("#ffff00"),
		Orange:        lipgloss.Color("#ff8000"),
		Blue:          lipgloss.Color("#0080ff"),
		Magenta:       lipgloss.Color("#ff00ff"),
		Cyan:          lipgloss.Color("#00ffff"),
		BorderNormal:  lipgloss.Color("#666666"),
		BorderFocused: lipgloss.Color("#ffffff"),
	}
}

// Monokai returns the classic Monokai theme.
func Monokai() *Theme {
	return &Theme{
		Name:          "monokai",
		Background:    lipgloss.Color("#272822"),
		Surface:       lipgloss.Color("#3e3d32"),
		Overlay:       lipgloss.Color("#49483e"),
		Text:          lipgloss.Color("#f8f8f2"),
		Subtext:       lipgloss.Color("#cfcfc2"),
		Muted:         lipgloss.Color("#75715e"),
		Accent:        lipgloss.Color("#a6e22e"),
		AccentAlt:     lipgloss.Color("#66d9ef"),
		Green:         lipgloss.Color("#a6e22e"),
		Red:           lipgloss.Color("#f92672"),
		Yellow:        lipgloss.Color("#e6db74"),
		Orange:        lipgloss.Color("#fd971f"),
		Blue:          lipgloss.Color("#66d9ef"),
		Magenta:       lipgloss.Color("#ae81ff"),
		Cyan:          lipgloss.Color("#66d9ef"),
		BorderNormal:  lipgloss.Color("#49483e"),
		BorderFocused: lipgloss.Color("#a6e22e"),
	}
}
