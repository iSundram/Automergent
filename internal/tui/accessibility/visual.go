package accessibility

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// HighContrastTheme provides high contrast colors for accessibility
type HighContrastTheme struct {
	// Base colors - maximum contrast
	Background color.Color
	Foreground color.Color

	// UI elements
	BorderColor color.Color
	FocusBorder color.Color
	SelectionBg color.Color
	SelectionFg color.Color
	HighlightBg color.Color
	HighlightFg color.Color

	// Status colors - distinct and visible
	Error   color.Color
	Warning color.Color
	Success color.Color
	Info    color.Color

	// Syntax highlighting
	Keyword  color.Color
	String   color.Color
	Number   color.Color
	Comment  color.Color
	Function color.Color
	Type     color.Color
	Operator color.Color

	// Diff colors
	Added   color.Color
	Removed color.Color
	Changed color.Color
}

// NewHighContrastDarkTheme creates a high contrast dark theme
func NewHighContrastDarkTheme() *HighContrastTheme {
	return &HighContrastTheme{
		// Pure black background with white text
		Background: lipgloss.Color("#000000"),
		Foreground: lipgloss.Color("#FFFFFF"),

		// Bright cyan borders for visibility
		BorderColor: lipgloss.Color("#00FFFF"),
		FocusBorder: lipgloss.Color("#FFFF00"),

		// Selection uses inverted colors
		SelectionBg: lipgloss.Color("#FFFFFF"),
		SelectionFg: lipgloss.Color("#000000"),

		// Yellow highlight
		HighlightBg: lipgloss.Color("#FFFF00"),
		HighlightFg: lipgloss.Color("#000000"),

		// Status colors - WCAG AAA compliant
		Error:   lipgloss.Color("#FF6B6B"),
		Warning: lipgloss.Color("#FFD93D"),
		Success: lipgloss.Color("#6BCB77"),
		Info:    lipgloss.Color("#4D96FF"),

		// Syntax - high contrast colors
		Keyword:  lipgloss.Color("#FF79C6"),
		String:   lipgloss.Color("#50FA7B"),
		Number:   lipgloss.Color("#BD93F9"),
		Comment:  lipgloss.Color("#6272A4"),
		Function: lipgloss.Color("#FFB86C"),
		Type:     lipgloss.Color("#8BE9FD"),
		Operator: lipgloss.Color("#FF5555"),

		// Diff - distinct colors
		Added:   lipgloss.Color("#00FF00"),
		Removed: lipgloss.Color("#FF0000"),
		Changed: lipgloss.Color("#FFFF00"),
	}
}

// NewHighContrastLightTheme creates a high contrast light theme
func NewHighContrastLightTheme() *HighContrastTheme {
	return &HighContrastTheme{
		// Pure white background with black text
		Background: lipgloss.Color("#FFFFFF"),
		Foreground: lipgloss.Color("#000000"),

		// Dark borders for visibility
		BorderColor: lipgloss.Color("#000080"),
		FocusBorder: lipgloss.Color("#FF0000"),

		// Selection uses inverted colors
		SelectionBg: lipgloss.Color("#000000"),
		SelectionFg: lipgloss.Color("#FFFFFF"),

		// Blue highlight
		HighlightBg: lipgloss.Color("#0000FF"),
		HighlightFg: lipgloss.Color("#FFFFFF"),

		// Status colors - dark for light background
		Error:   lipgloss.Color("#CC0000"),
		Warning: lipgloss.Color("#996600"),
		Success: lipgloss.Color("#006600"),
		Info:    lipgloss.Color("#000099"),

		// Syntax - dark colors for light background
		Keyword:  lipgloss.Color("#770077"),
		String:   lipgloss.Color("#006600"),
		Number:   lipgloss.Color("#000099"),
		Comment:  lipgloss.Color("#666666"),
		Function: lipgloss.Color("#994400"),
		Type:     lipgloss.Color("#007777"),
		Operator: lipgloss.Color("#990000"),

		// Diff - dark versions
		Added:   lipgloss.Color("#006600"),
		Removed: lipgloss.Color("#990000"),
		Changed: lipgloss.Color("#996600"),
	}
}

// ColorBlindPalette provides color blind friendly palettes
type ColorBlindPalette struct {
	// Primary palette - IBM Design Color Blind Safe
	Primary   color.Color
	Secondary color.Color
	Tertiary  color.Color
	Accent    color.Color

	// Status colors adapted for color blindness
	Error   color.Color
	Warning color.Color
	Success color.Color
	Info    color.Color

	// Diff colors
	Added   color.Color
	Removed color.Color
	Changed color.Color
}

// NewProtanopiaPalette creates a palette for protanopia (red-blind)
func NewProtanopiaPalette() *ColorBlindPalette {
	return &ColorBlindPalette{
		Primary:   lipgloss.Color("#648FFF"), // Blue
		Secondary: lipgloss.Color("#785EF0"), // Purple
		Tertiary:  lipgloss.Color("#DC267F"), // Magenta
		Accent:    lipgloss.Color("#FE6100"), // Orange

		// Avoid red-green, use blue-orange contrast
		Error:   lipgloss.Color("#DC267F"), // Magenta instead of red
		Warning: lipgloss.Color("#FE6100"), // Orange
		Success: lipgloss.Color("#648FFF"), // Blue instead of green
		Info:    lipgloss.Color("#785EF0"), // Purple

		// Diff using patterns + color
		Added:   lipgloss.Color("#648FFF"), // Blue
		Removed: lipgloss.Color("#DC267F"), // Magenta
		Changed: lipgloss.Color("#FE6100"), // Orange
	}
}

// NewDeuteranopiaPalette creates a palette for deuteranopia (green-blind)
func NewDeuteranopiaPalette() *ColorBlindPalette {
	return &ColorBlindPalette{
		Primary:   lipgloss.Color("#0077BB"), // Blue
		Secondary: lipgloss.Color("#33BBEE"), // Cyan
		Tertiary:  lipgloss.Color("#EE7733"), // Orange
		Accent:    lipgloss.Color("#CC3311"), // Red-orange

		// Avoid red-green, use blue-orange contrast
		Error:   lipgloss.Color("#CC3311"), // Red-orange
		Warning: lipgloss.Color("#EE7733"), // Orange
		Success: lipgloss.Color("#0077BB"), // Blue
		Info:    lipgloss.Color("#33BBEE"), // Cyan

		// Diff using patterns + color
		Added:   lipgloss.Color("#0077BB"), // Blue
		Removed: lipgloss.Color("#CC3311"), // Red-orange
		Changed: lipgloss.Color("#EE7733"), // Orange
	}
}

// NewTritanopiaPalette creates a palette for tritanopia (blue-blind)
func NewTritanopiaPalette() *ColorBlindPalette {
	return &ColorBlindPalette{
		Primary:   lipgloss.Color("#EE3377"), // Magenta
		Secondary: lipgloss.Color("#009988"), // Teal
		Tertiary:  lipgloss.Color("#EE7733"), // Orange
		Accent:    lipgloss.Color("#33BBEE"), // Light blue

		// Avoid blue-yellow, use magenta-teal contrast
		Error:   lipgloss.Color("#EE3377"), // Magenta
		Warning: lipgloss.Color("#EE7733"), // Orange
		Success: lipgloss.Color("#009988"), // Teal
		Info:    lipgloss.Color("#33BBEE"), // Light blue

		// Diff using patterns + color
		Added:   lipgloss.Color("#009988"), // Teal
		Removed: lipgloss.Color("#EE3377"), // Magenta
		Changed: lipgloss.Color("#EE7733"), // Orange
	}
}

// TextSize represents text size options
type TextSize int

const (
	TextSizeSmall TextSize = iota
	TextSizeNormal
	TextSizeLarge
	TextSizeExtraLarge
)

// GetFontScale returns the font scale factor for a text size
func (ts TextSize) GetFontScale() float64 {
	switch ts {
	case TextSizeSmall:
		return 0.85
	case TextSizeNormal:
		return 1.0
	case TextSizeLarge:
		return 1.25
	case TextSizeExtraLarge:
		return 1.5
	default:
		return 1.0
	}
}

// String returns a human-readable name for the text size
func (ts TextSize) String() string {
	switch ts {
	case TextSizeSmall:
		return "Small"
	case TextSizeNormal:
		return "Normal"
	case TextSizeLarge:
		return "Large"
	case TextSizeExtraLarge:
		return "Extra Large"
	default:
		return "Normal"
	}
}

// VisualSettings holds all visual accessibility settings
type VisualSettings struct {
	HighContrast     bool
	HighContrastDark bool // true = dark mode, false = light mode
	ColorBlindMode   string
	TextSize         TextSize
	ReducedMotion    bool
	ReducedBlinking  bool
	ShowFocusOutline bool
	UnderlineLinks   bool
	IconLabels       bool // Show text labels next to icons
	MonospaceUI      bool // Use monospace font for all UI
	LineSpacing      float64
	LetterSpacing    float64
}

// DefaultVisualSettings returns the default visual settings
func DefaultVisualSettings() VisualSettings {
	return VisualSettings{
		HighContrast:     false,
		HighContrastDark: true,
		ColorBlindMode:   "none",
		TextSize:         TextSizeNormal,
		ReducedMotion:    false,
		ReducedBlinking:  false,
		ShowFocusOutline: true,
		UnderlineLinks:   false,
		IconLabels:       false,
		MonospaceUI:      true,
		LineSpacing:      1.0,
		LetterSpacing:    0,
	}
}

// VisualManager manages visual accessibility settings
type VisualManager struct {
	settings VisualSettings

	// Theme instances
	highContrastDark  *HighContrastTheme
	highContrastLight *HighContrastTheme

	// Color blind palettes
	protanopiaPalette   *ColorBlindPalette
	deuteranopiaPalette *ColorBlindPalette
	tritanopiaPalette   *ColorBlindPalette

	// Callbacks
	onSettingsChange func(VisualSettings)
}

// NewVisualManager creates a new visual manager
func NewVisualManager() *VisualManager {
	return &VisualManager{
		settings:            DefaultVisualSettings(),
		highContrastDark:    NewHighContrastDarkTheme(),
		highContrastLight:   NewHighContrastLightTheme(),
		protanopiaPalette:   NewProtanopiaPalette(),
		deuteranopiaPalette: NewDeuteranopiaPalette(),
		tritanopiaPalette:   NewTritanopiaPalette(),
	}
}

// GetSettings returns the current visual settings
func (vm *VisualManager) GetSettings() VisualSettings {
	return vm.settings
}

// SetSettings updates all visual settings
func (vm *VisualManager) SetSettings(settings VisualSettings) {
	vm.settings = settings
	if vm.onSettingsChange != nil {
		vm.onSettingsChange(settings)
	}
}

// SetHighContrast enables or disables high contrast mode
func (vm *VisualManager) SetHighContrast(enabled bool) {
	vm.settings.HighContrast = enabled
	if vm.onSettingsChange != nil {
		vm.onSettingsChange(vm.settings)
	}
}

// SetHighContrastDark sets whether to use dark or light high contrast
func (vm *VisualManager) SetHighContrastDark(dark bool) {
	vm.settings.HighContrastDark = dark
	if vm.onSettingsChange != nil {
		vm.onSettingsChange(vm.settings)
	}
}

// SetColorBlindMode sets the color blind accommodation mode
func (vm *VisualManager) SetColorBlindMode(mode string) {
	vm.settings.ColorBlindMode = mode
	if vm.onSettingsChange != nil {
		vm.onSettingsChange(vm.settings)
	}
}

// SetTextSize sets the text size
func (vm *VisualManager) SetTextSize(size TextSize) {
	vm.settings.TextSize = size
	if vm.onSettingsChange != nil {
		vm.onSettingsChange(vm.settings)
	}
}

// SetReducedMotion enables or disables reduced motion
func (vm *VisualManager) SetReducedMotion(enabled bool) {
	vm.settings.ReducedMotion = enabled
	if vm.onSettingsChange != nil {
		vm.onSettingsChange(vm.settings)
	}
}

// SetOnSettingsChange sets the callback for settings changes
func (vm *VisualManager) SetOnSettingsChange(fn func(VisualSettings)) {
	vm.onSettingsChange = fn
}

// GetHighContrastTheme returns the current high contrast theme
func (vm *VisualManager) GetHighContrastTheme() *HighContrastTheme {
	if vm.settings.HighContrastDark {
		return vm.highContrastDark
	}
	return vm.highContrastLight
}

// GetColorBlindPalette returns the current color blind palette
func (vm *VisualManager) GetColorBlindPalette() *ColorBlindPalette {
	switch vm.settings.ColorBlindMode {
	case "protanopia":
		return vm.protanopiaPalette
	case "deuteranopia":
		return vm.deuteranopiaPalette
	case "tritanopia":
		return vm.tritanopiaPalette
	default:
		return nil
	}
}

// ApplyToStyle applies accessibility settings to a lipgloss style
func (vm *VisualManager) ApplyToStyle(style lipgloss.Style) lipgloss.Style {
	settings := vm.settings

	if settings.HighContrast {
		theme := vm.GetHighContrastTheme()
		style = style.
			Background(theme.Background).
			Foreground(theme.Foreground).
			BorderForeground(theme.BorderColor)
	}

	if settings.UnderlineLinks {
		style = style.Underline(true)
	}

	return style
}

// GetStatusColor returns the appropriate status color based on settings
func (vm *VisualManager) GetStatusColor(status string) color.Color {
	if palette := vm.GetColorBlindPalette(); palette != nil {
		switch status {
		case "error":
			return palette.Error
		case "warning":
			return palette.Warning
		case "success":
			return palette.Success
		case "info":
			return palette.Info
		}
	}

	if vm.settings.HighContrast {
		theme := vm.GetHighContrastTheme()
		switch status {
		case "error":
			return theme.Error
		case "warning":
			return theme.Warning
		case "success":
			return theme.Success
		case "info":
			return theme.Info
		}
	}

	// Default colors
	switch status {
	case "error":
		return lipgloss.Color("#FF5555")
	case "warning":
		return lipgloss.Color("#FFB86C")
	case "success":
		return lipgloss.Color("#50FA7B")
	case "info":
		return lipgloss.Color("#8BE9FD")
	default:
		return lipgloss.Color("#FFFFFF")
	}
}

// GetDiffColor returns the appropriate diff color based on settings
func (vm *VisualManager) GetDiffColor(diffType string) color.Color {
	if palette := vm.GetColorBlindPalette(); palette != nil {
		switch diffType {
		case "added":
			return palette.Added
		case "removed":
			return palette.Removed
		case "changed":
			return palette.Changed
		}
	}

	if vm.settings.HighContrast {
		theme := vm.GetHighContrastTheme()
		switch diffType {
		case "added":
			return theme.Added
		case "removed":
			return theme.Removed
		case "changed":
			return theme.Changed
		}
	}

	// Default colors
	switch diffType {
	case "added":
		return lipgloss.Color("#50FA7B")
	case "removed":
		return lipgloss.Color("#FF5555")
	case "changed":
		return lipgloss.Color("#FFB86C")
	default:
		return lipgloss.Color("#FFFFFF")
	}
}

// ShouldAnimate returns whether animations should be shown
func (vm *VisualManager) ShouldAnimate() bool {
	return !vm.settings.ReducedMotion
}

// ShouldBlink returns whether blinking elements should blink
func (vm *VisualManager) ShouldBlink() bool {
	return !vm.settings.ReducedBlinking && !vm.settings.ReducedMotion
}

// GetIconWithLabel returns an icon with optional text label
func (vm *VisualManager) GetIconWithLabel(icon, label string) string {
	if vm.settings.IconLabels {
		return icon + " " + label
	}
	return icon
}
