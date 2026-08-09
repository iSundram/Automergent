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

	Base           lipgloss.Style
	Header         lipgloss.Style
	HeaderBrand    lipgloss.Style
	HeaderCenter   lipgloss.Style
	HeaderRight    lipgloss.Style
	StatusBar      lipgloss.Style
	StatusBarRight lipgloss.Style
	Input          lipgloss.Style
	UserMsg        lipgloss.Style
	UserLabel      lipgloss.Style
	UserBubble     lipgloss.Style
	AssistantMsg   lipgloss.Style
	AssistantLabel lipgloss.Style
	SystemMsg      lipgloss.Style
	ToolCall       lipgloss.Style
	ToolResult     lipgloss.Style
	Error          lipgloss.Style
	Border         lipgloss.Style
	Code           lipgloss.Style
	Dim            lipgloss.Style
	Bold           lipgloss.Style
	Success        lipgloss.Style
	Warning        lipgloss.Style
	Timestamp      lipgloss.Style
	ConfirmBox     lipgloss.Style
	HelpBox        lipgloss.Style
	FileTree       lipgloss.Style
	FileTreeDir    lipgloss.Style
	FileTreeFile   lipgloss.Style
	FileTreeSelect lipgloss.Style
}

// NewStyles builds Styles from a Theme.
func NewStyles(t *Theme) *Styles {
	s := &Styles{T: t}
	s.Base = lipgloss.NewStyle().Foreground(t.Text).Background(t.Background)

	// Header
	s.HeaderBrand = lipgloss.NewStyle().
		Background(t.Surface).
		Foreground(t.Accent).
		Bold(true).
		Padding(0, 1)
	s.HeaderCenter = lipgloss.NewStyle().
		Background(t.Surface).
		Foreground(t.Subtext).
		Padding(0, 1)
	s.HeaderRight = lipgloss.NewStyle().
		Background(t.Surface).
		Foreground(t.Muted).
		Padding(0, 1)
	s.Header = lipgloss.NewStyle().
		Background(t.Surface).
		Foreground(t.Text).
		Padding(0, 1).
		Bold(true)

	// Status bar
	s.StatusBar = lipgloss.NewStyle().
		Background(t.Surface).
		Foreground(t.Text).
		Padding(0, 1)
	s.StatusBarRight = lipgloss.NewStyle().
		Background(t.Surface).
		Foreground(t.Muted).
		Padding(0, 1)

	// Input
	s.Input = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BorderNormal).
		Padding(0, 1)

	// User message
	s.UserLabel = lipgloss.NewStyle().
		Foreground(t.Blue).
		Bold(true)
	s.UserBubble = lipgloss.NewStyle().
		Background(t.Surface).
		Foreground(t.Text).
		Padding(0, 1).
		MarginLeft(2)
	s.UserMsg = lipgloss.NewStyle().
		Foreground(t.Blue).
		Bold(true)

	// Assistant message
	s.AssistantLabel = lipgloss.NewStyle().
		Foreground(t.Accent).
		Bold(true)
	s.AssistantMsg = lipgloss.NewStyle().
		Foreground(t.Text)

	// System message
	s.SystemMsg = lipgloss.NewStyle().
		Foreground(t.Muted).
		Italic(true).
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(t.Muted).
		Padding(0, 1)

	// Tool
	s.ToolCall = lipgloss.NewStyle().
		Foreground(t.Yellow).
		Italic(true)
	s.ToolResult = lipgloss.NewStyle().
		Foreground(t.Muted)

	// Misc
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
	s.Timestamp = lipgloss.NewStyle().
		Foreground(t.Muted).
		Italic(true)

	// Overlays
	s.ConfirmBox = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Yellow).
		Padding(1, 2).
		Width(50)
	s.HelpBox = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BorderFocused).
		Padding(1, 2)

	// File tree
	s.FileTree = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, true, false, false).
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

	return s
}
