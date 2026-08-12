package themes

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// SyntaxColors provides colors for syntax highlighting.
type SyntaxColors struct {
	Keyword     color.Color
	String      color.Color
	Number      color.Color
	Comment     color.Color
	Function    color.Color
	Variable    color.Color
	Type        color.Color
	Constant    color.Color
	Operator    color.Color
	Punctuation color.Color
	Tag         color.Color
	Attribute   color.Color
	Property    color.Color
	ClassName   color.Color
	Namespace   color.Color
}

// SyntaxScheme represents a complete syntax highlighting scheme.
type SyntaxScheme struct {
	Name   string
	Colors SyntaxColors
}

// GetSyntaxScheme returns a syntax scheme by name.
func GetSyntaxScheme(name string, theme *Theme) SyntaxScheme {
	switch name {
	case "monokai":
		return SyntaxScheme{
			Name: "monokai",
			Colors: SyntaxColors{
				Keyword:     lipgloss.Color("#f92672"),
				String:      lipgloss.Color("#e6db74"),
				Number:      lipgloss.Color("#ae81ff"),
				Comment:     lipgloss.Color("#75715e"),
				Function:    lipgloss.Color("#a6e22e"),
				Variable:    lipgloss.Color("#f8f8f2"),
				Type:        lipgloss.Color("#66d9ef"),
				Constant:    lipgloss.Color("#ae81ff"),
				Operator:    lipgloss.Color("#f92672"),
				Punctuation: lipgloss.Color("#f8f8f2"),
				Tag:         lipgloss.Color("#f92672"),
				Attribute:   lipgloss.Color("#a6e22e"),
				Property:    lipgloss.Color("#66d9ef"),
				ClassName:   lipgloss.Color("#a6e22e"),
				Namespace:   lipgloss.Color("#66d9ef"),
			},
		}
	case "github":
		return SyntaxScheme{
			Name: "github",
			Colors: SyntaxColors{
				Keyword:     lipgloss.Color("#d73a49"),
				String:      lipgloss.Color("#032f62"),
				Number:      lipgloss.Color("#005cc5"),
				Comment:     lipgloss.Color("#6a737d"),
				Function:    lipgloss.Color("#6f42c1"),
				Variable:    lipgloss.Color("#24292e"),
				Type:        lipgloss.Color("#005cc5"),
				Constant:    lipgloss.Color("#005cc5"),
				Operator:    lipgloss.Color("#d73a49"),
				Punctuation: lipgloss.Color("#24292e"),
				Tag:         lipgloss.Color("#22863a"),
				Attribute:   lipgloss.Color("#6f42c1"),
				Property:    lipgloss.Color("#005cc5"),
				ClassName:   lipgloss.Color("#6f42c1"),
				Namespace:   lipgloss.Color("#6f42c1"),
			},
		}
	case "dracula":
		return SyntaxScheme{
			Name: "dracula",
			Colors: SyntaxColors{
				Keyword:     lipgloss.Color("#ff79c6"),
				String:      lipgloss.Color("#f1fa8c"),
				Number:      lipgloss.Color("#bd93f9"),
				Comment:     lipgloss.Color("#6272a4"),
				Function:    lipgloss.Color("#50fa7b"),
				Variable:    lipgloss.Color("#f8f8f2"),
				Type:        lipgloss.Color("#8be9fd"),
				Constant:    lipgloss.Color("#bd93f9"),
				Operator:    lipgloss.Color("#ff79c6"),
				Punctuation: lipgloss.Color("#f8f8f2"),
				Tag:         lipgloss.Color("#ff79c6"),
				Attribute:   lipgloss.Color("#50fa7b"),
				Property:    lipgloss.Color("#8be9fd"),
				ClassName:   lipgloss.Color("#8be9fd"),
				Namespace:   lipgloss.Color("#8be9fd"),
			},
		}
	default:
		return SyntaxScheme{
			Name: "theme",
			Colors: SyntaxColors{
				Keyword:     theme.Magenta,
				String:      theme.Green,
				Number:      theme.Yellow,
				Comment:     theme.Muted,
				Function:    theme.Blue,
				Variable:    theme.Text,
				Type:        theme.Cyan,
				Constant:    theme.Yellow,
				Operator:    theme.Red,
				Punctuation: theme.Subtext,
				Tag:         theme.Red,
				Attribute:   theme.Green,
				Property:    theme.Cyan,
				ClassName:   theme.Accent,
				Namespace:   theme.AccentAlt,
			},
		}
	}
}

// SyntaxStyles builds lipgloss styles for syntax highlighting.
type SyntaxStyles struct {
	Keyword     lipgloss.Style
	String      lipgloss.Style
	Number      lipgloss.Style
	Comment     lipgloss.Style
	Function    lipgloss.Style
	Variable    lipgloss.Style
	Type        lipgloss.Style
	Constant    lipgloss.Style
	Operator    lipgloss.Style
	Punctuation lipgloss.Style
	Tag         lipgloss.Style
	Attribute   lipgloss.Style
	Property    lipgloss.Style
	ClassName   lipgloss.Style
	Namespace   lipgloss.Style
}

// NewSyntaxStyles creates syntax styles from a scheme.
func NewSyntaxStyles(scheme SyntaxScheme) SyntaxStyles {
	return SyntaxStyles{
		Keyword:     lipgloss.NewStyle().Foreground(scheme.Colors.Keyword).Bold(true),
		String:      lipgloss.NewStyle().Foreground(scheme.Colors.String),
		Number:      lipgloss.NewStyle().Foreground(scheme.Colors.Number),
		Comment:     lipgloss.NewStyle().Foreground(scheme.Colors.Comment).Italic(true),
		Function:    lipgloss.NewStyle().Foreground(scheme.Colors.Function),
		Variable:    lipgloss.NewStyle().Foreground(scheme.Colors.Variable),
		Type:        lipgloss.NewStyle().Foreground(scheme.Colors.Type),
		Constant:    lipgloss.NewStyle().Foreground(scheme.Colors.Constant),
		Operator:    lipgloss.NewStyle().Foreground(scheme.Colors.Operator),
		Punctuation: lipgloss.NewStyle().Foreground(scheme.Colors.Punctuation),
		Tag:         lipgloss.NewStyle().Foreground(scheme.Colors.Tag),
		Attribute:   lipgloss.NewStyle().Foreground(scheme.Colors.Attribute),
		Property:    lipgloss.NewStyle().Foreground(scheme.Colors.Property),
		ClassName:   lipgloss.NewStyle().Foreground(scheme.Colors.ClassName),
		Namespace:   lipgloss.NewStyle().Foreground(scheme.Colors.Namespace),
	}
}

// DiffColors provides colors for diff rendering.
type DiffColors struct {
	Added      color.Color
	Removed    color.Color
	Modified   color.Color
	Context    color.Color
	HunkHeader color.Color
	FileHeader color.Color
}

// GetDiffColors returns diff colors based on the current theme.
func GetDiffColors(theme *Theme) DiffColors {
	return DiffColors{
		Added:      theme.Green,
		Removed:    theme.Red,
		Modified:   theme.Yellow,
		Context:    theme.Muted,
		HunkHeader: theme.Blue,
		FileHeader: theme.Magenta,
	}
}

// DiffStyles builds lipgloss styles for diff rendering.
type DiffStyles struct {
	Added      lipgloss.Style
	Removed    lipgloss.Style
	Modified   lipgloss.Style
	Context    lipgloss.Style
	HunkHeader lipgloss.Style
	FileHeader lipgloss.Style
	LineNumber lipgloss.Style
}

// NewDiffStyles creates diff styles from colors and theme.
func NewDiffStyles(colors DiffColors, theme *Theme) DiffStyles {
	surfaceBg := theme.Surface
	return DiffStyles{
		Added:      lipgloss.NewStyle().Foreground(colors.Added).Background(surfaceBg).PaddingLeft(1),
		Removed:    lipgloss.NewStyle().Foreground(colors.Removed).Background(surfaceBg).PaddingLeft(1),
		Modified:   lipgloss.NewStyle().Foreground(colors.Modified).Background(surfaceBg).PaddingLeft(1),
		Context:    lipgloss.NewStyle().Foreground(colors.Context).PaddingLeft(2),
		HunkHeader: lipgloss.NewStyle().Foreground(colors.HunkHeader).Bold(true),
		FileHeader: lipgloss.NewStyle().Foreground(colors.FileHeader).Bold(true).Underline(true),
		LineNumber: lipgloss.NewStyle().Foreground(theme.Muted).Faint(true).PaddingRight(1),
	}
}
