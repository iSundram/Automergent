package themes

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Theme defines the color palette for the TUI.
type Theme struct {
	Name string

	Background    color.Color
	Surface       color.Color
	Overlay       color.Color
	Text          color.Color
	Subtext       color.Color
	Muted         color.Color
	Accent        color.Color
	AccentAlt     color.Color
	Green         color.Color
	Red           color.Color
	Yellow        color.Color
	Blue          color.Color
	Magenta       color.Color
	Cyan          color.Color
	BorderNormal  color.Color
	BorderFocused color.Color
}

// Styles pre-builds common lipgloss styles from a theme.
type Styles struct {
	T *Theme

	Base            lipgloss.Style
	Header          lipgloss.Style
	HeaderBrand     lipgloss.Style
	HeaderCenter    lipgloss.Style
	HeaderRight     lipgloss.Style
	HeaderPill      lipgloss.Style
	StatusBar       lipgloss.Style
	StatusBarRight  lipgloss.Style
	Input           lipgloss.Style
	InputFocused    lipgloss.Style
	UserMsg         lipgloss.Style
	UserLabel       lipgloss.Style
	UserBubble      lipgloss.Style
	AssistantMsg    lipgloss.Style
	AssistantLabel  lipgloss.Style
	AssistantBubble lipgloss.Style
	SystemMsg       lipgloss.Style
	ToolCall        lipgloss.Style
	ToolResult      lipgloss.Style
	ToolBox         lipgloss.Style
	ToolName        lipgloss.Style
	ToolHeader      lipgloss.Style
	ToolBody        lipgloss.Style
	ToolStatus      lipgloss.Style
	ToolDuration    lipgloss.Style
	ToolIcon        lipgloss.Style
	ToolAccent      lipgloss.Style
	Error           lipgloss.Style
	Border          lipgloss.Style
	Code            lipgloss.Style
	Dim             lipgloss.Style
	Bold            lipgloss.Style
	Success         lipgloss.Style
	Warning         lipgloss.Style
	Timestamp       lipgloss.Style
	ConfirmBox      lipgloss.Style
	ConfirmAllow    lipgloss.Style
	ConfirmAlways   lipgloss.Style
	ConfirmReject   lipgloss.Style
	ConfirmFeedback lipgloss.Style
	HelpBox         lipgloss.Style
	FileTree        lipgloss.Style
	FileTreeDir     lipgloss.Style
	FileTreeFile    lipgloss.Style
	FileTreeSelect  lipgloss.Style
	DiffPane        lipgloss.Style
	DiffAction      lipgloss.Style
	InactivePane    lipgloss.Style
	ActivePane      lipgloss.Style
	Palette         lipgloss.Style
	PaletteItem     lipgloss.Style
	PaletteSelect   lipgloss.Style
	PaletteDim      lipgloss.Style
}

// NewStyles builds Styles from a Theme.
func NewStyles(t *Theme) *Styles {
	s := &Styles{T: t}
	s.Base = lipgloss.NewStyle().Foreground(t.Text).Background(t.Background)

	// Floating Pill Header
	s.HeaderPill = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BorderNormal).
		Padding(0, 1).
		Margin(1, 2, 1, 2) // Top, Right, Bottom, Left margin to make it "float"

	s.HeaderBrand = lipgloss.NewStyle().
		Foreground(t.Accent).
		Bold(true)
	s.HeaderCenter = lipgloss.NewStyle().
		Foreground(t.Subtext).
		Padding(0, 2)
	s.HeaderRight = lipgloss.NewStyle().
		Foreground(t.Muted)
	s.Header = lipgloss.NewStyle().
		Foreground(t.Text).
		Bold(true)

	// Status bar (Minimalist footer)
	s.StatusBar = lipgloss.NewStyle().
		Foreground(t.Muted).
		Padding(0, 2)
	s.StatusBarRight = lipgloss.NewStyle().
		Foreground(t.Subtext)

	// Input (inline, no box — a thin top rule separates it from the conversation)
	s.Input = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true, false, false, false).
		BorderForeground(t.BorderNormal).
		Padding(0, 1)
	s.InputFocused = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true, false, false, false).
		BorderForeground(t.BorderNormal).
		Padding(0, 1)

	// User message (Right-aligned look via margins in rendering, styled bubble here)
	s.UserLabel = lipgloss.NewStyle().
		Foreground(t.Subtext).
		MarginBottom(1)
	s.UserBubble = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Accent).
		Foreground(t.Text).
		Padding(0, 1)
	s.UserMsg = lipgloss.NewStyle().
		Foreground(t.Text)

	// Assistant message
	s.AssistantLabel = lipgloss.NewStyle().
		Foreground(t.Accent).
		Bold(true)
	s.AssistantBubble = lipgloss.NewStyle().
		Foreground(t.Text).
		Padding(0, 1).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BorderNormal)
	s.AssistantMsg = lipgloss.NewStyle().
		Foreground(t.Text)

	// System message
	s.SystemMsg = lipgloss.NewStyle().
		Foreground(t.Muted).
		Italic(true).
		Padding(0, 1)

	// Tool
	s.ToolCall = lipgloss.NewStyle().
		Foreground(t.Yellow)
	s.ToolResult = lipgloss.NewStyle().
		Foreground(t.Muted)

	s.ToolBox = lipgloss.NewStyle().
		Background(t.Overlay).
		Padding(0, 1).
		MarginBottom(1)

	s.ToolName = lipgloss.NewStyle().
		Foreground(t.Text).
		Bold(true)

	s.ToolHeader = lipgloss.NewStyle().
		Padding(0, 1)

	s.ToolBody = lipgloss.NewStyle().
		Padding(0, 1).
		MarginTop(1)

	s.ToolStatus = lipgloss.NewStyle().
		Faint(true).
		MarginLeft(1)

	s.ToolDuration = lipgloss.NewStyle().
		Foreground(t.Muted).
		Italic(true).
		MarginLeft(1)

	s.ToolIcon = lipgloss.NewStyle().
		MarginRight(1)

	s.ToolAccent = lipgloss.NewStyle().
		PaddingLeft(1).
		Border(lipgloss.NormalBorder(), false, false, false, true) // Left border only

	// Misc
	s.Error = lipgloss.NewStyle().
		Foreground(t.Red).
		Bold(true)
	s.Border = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BorderNormal)
	s.Code = lipgloss.NewStyle().
		Background(t.Surface).
		Foreground(t.Cyan).
		Padding(0, 1).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BorderNormal)
	s.Dim = lipgloss.NewStyle().
		Foreground(t.Muted)
	s.Bold = lipgloss.NewStyle().
		Bold(true).
		Foreground(t.Text)
	s.Success = lipgloss.NewStyle().
		Foreground(t.Green)
	s.Warning = lipgloss.NewStyle().
		Foreground(t.Yellow)
	s.Timestamp = lipgloss.NewStyle().
		Foreground(t.Muted).
		Italic(true)

	// Overlays & Panes
	s.ConfirmBox = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Accent).
		Background(t.Background).
		Padding(1, 2)

	buttonBase := lipgloss.NewStyle().
		Foreground(t.Background).
		Bold(true).
		Padding(0, 1).
		MarginRight(1)

	s.ConfirmAllow = lipgloss.NewStyle().Inherit(buttonBase).Background(t.Green)
	s.ConfirmAlways = lipgloss.NewStyle().Inherit(buttonBase).Background(t.Accent)
	s.ConfirmReject = lipgloss.NewStyle().Inherit(buttonBase).Background(t.Red)
	s.ConfirmFeedback = lipgloss.NewStyle().Inherit(buttonBase).Background(t.Yellow)

	s.HelpBox = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Accent).
		Padding(1, 2)
	s.DiffPane = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BorderNormal).
		Padding(0, 1)
	s.DiffAction = lipgloss.NewStyle().
		Background(t.Surface).
		Foreground(t.Text).
		Padding(0, 1)
	s.ActivePane = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Accent)
	s.InactivePane = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BorderNormal).
		Faint(true)

	// File tree
	s.FileTree = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BorderNormal).
		Padding(0, 1)
	s.FileTreeDir = lipgloss.NewStyle().
		Foreground(t.Blue).
		Bold(true)
	s.FileTreeFile = lipgloss.NewStyle().
		Foreground(t.Text)
	s.FileTreeSelect = lipgloss.NewStyle().
		Background(t.Surface).
		Foreground(t.Accent)

	// Command Palette
	s.Palette = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Accent).
		Background(t.Background).
		Padding(0, 1)
	s.PaletteItem = lipgloss.NewStyle().
		Foreground(t.Text).
		Padding(0, 1)
	s.PaletteSelect = lipgloss.NewStyle().
		Background(t.Surface).
		Foreground(t.Accent).
		Bold(true).
		Padding(0, 1)
	s.PaletteDim = lipgloss.NewStyle().
		Foreground(t.Muted).
		Italic(true).
		Padding(0, 1)

	return s
}
