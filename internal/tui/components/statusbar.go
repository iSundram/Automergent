package components

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

// StatusBar renders the bottom status bar with a HUD look matching the header.
type StatusBar struct {
	styles         *themes.Styles
	width          int
	status         string
	startTime      time.Time
	permissionTool string
	browsing       bool

	// HUD segments (IDE chrome)
	ctxPercent   float64 // context window usage, 0-100+
	pendingEdits int     // edits awaiting review
	branch       string  // git branch (+ahead/behind suffix)
	problems     int     // diagnostics introduced this session
	showSegments bool
}

// NewStatusBar creates a new StatusBar.
func NewStatusBar(styles *themes.Styles) StatusBar {
	return StatusBar{
		styles:       styles,
		status:       "Ready",
		startTime:    time.Now(),
		showSegments: true,
	}
}

// SetWidth updates the status bar width.
func (s *StatusBar) SetWidth(w int) { s.width = w }

// SetStatus updates the status message.
func (s *StatusBar) SetStatus(msg string) { s.status = msg }

// SetContextUsage records context-window usage as a percentage (0-100+).
func (s *StatusBar) SetContextUsage(pct float64) { s.ctxPercent = pct }

// SetPendingEdits records how many proposed edits await review.
func (s *StatusBar) SetPendingEdits(n int) { s.pendingEdits = n }

// SetGitBranch records the current branch label ("main", "main+2", …).
func (s *StatusBar) SetGitBranch(b string) { s.branch = b }

// SetProblems records the number of session-introduced diagnostics.
func (s *StatusBar) SetProblems(n int) { s.problems = n }

// SetSegmentsVisible toggles the IDE HUD segments (zen mode hides them).
func (s *StatusBar) SetSegmentsVisible(v bool) { s.showSegments = v }

func (s *StatusBar) SetPermission(tool string) {
	s.permissionTool = tool
	s.status = "Awaiting permission"
}

func (s *StatusBar) ClearPermission() { s.permissionTool = "" }

func (s *StatusBar) SetBrowsing(enabled bool) { s.browsing = enabled }

func (s *StatusBar) getStatusStyle() lipgloss.Style {
	base := lipgloss.NewStyle().Bold(true).Padding(0, 1).Foreground(s.styles.T.Background)
	status := strings.ToLower(s.status)
	switch {
	case strings.Contains(status, "thinking") || strings.Contains(status, "💭"):
		return base.Background(s.styles.T.Yellow)
	case strings.Contains(status, "error") || strings.Contains(status, "fail"):
		return base.Background(s.styles.T.Red)
	case strings.Contains(status, "ready"):
		return base.Background(s.styles.T.Accent)
	case strings.Contains(status, "done") || strings.Contains(status, "success"):
		return base.Background(s.styles.T.Green)
	default:
		return base.Background(s.styles.T.Blue)
	}
}

// View renders the status bar.
func (s StatusBar) View() string {
	if s.width <= 0 {
		return ""
	}
	if s.permissionTool != "" {
		left := lipgloss.NewStyle().Foreground(s.styles.T.Yellow).Bold(true).Render("AWAITING PERMISSION")
		right := lipgloss.NewStyle().Foreground(s.styles.T.Subtext).Render(s.permissionTool + "  ·  y/n confirm")
		gap := s.width - lipgloss.Width(left) - lipgloss.Width(right) - 4
		if gap < 1 {
			gap = 1
		}
		return lipgloss.NewStyle().
			Background(s.styles.T.Background).
			Padding(0, 2).
			Width(s.width).
			Border(lipgloss.NormalBorder(), true, false, false, false).
			BorderForeground(s.styles.T.BorderNormal).
			Render(left + strings.Repeat(" ", gap) + right)
	}
	if s.browsing {
		left := lipgloss.NewStyle().Foreground(s.styles.T.Accent).Bold(true).Render("BROWSING CONVERSATION")
		right := lipgloss.NewStyle().Foreground(s.styles.T.Subtext).Render("↑↓ scroll  ·  PgUp/PgDn  ·  Tab input")
		gap := s.width - lipgloss.Width(left) - lipgloss.Width(right) - 4
		if gap < 1 {
			gap = 1
		}
		return lipgloss.NewStyle().Background(s.styles.T.Background).Padding(0, 2).Width(s.width).
			Border(lipgloss.NormalBorder(), true, false, false, false).
			BorderForeground(s.styles.T.BorderNormal).Render(left + strings.Repeat(" ", gap) + right)
	}

	// 1. Left Section: Status Badge
	statusBadge := s.getStatusStyle().Render(strings.ToUpper(s.status))
	left := lipgloss.JoinHorizontal(lipgloss.Center, " ", statusBadge)

	// 2. Center Section: Adaptive Shortcuts
	var shortcuts []string
	if s.width > 100 {
		shortcuts = []string{
			"󰌑 SEND",
			"󱊷 REVIEW",
			"󰜺 CANCEL",
			"󰈙 DIFF",
			"󰀦 HELP",
		}
	} else if s.width > 70 {
		shortcuts = []string{
			"ENTER",
			"CTRL+R",
			"ESC",
			"/DIFF",
			"?",
		}
	} else if s.width > 40 {
		shortcuts = []string{"󰌑", "󱊷", "󰜺", "󰈙", "󰀦"}
	}

	shortcutStyle := lipgloss.NewStyle().Foreground(s.styles.T.Muted)
	center := shortcutStyle.Render(strings.Join(shortcuts, "  "))

	// 3. Right Section: HUD segments + Clock & Session Timer
	segments := s.hudSegments()
	clock := time.Now().Format("15:04")
	timer := time.Since(s.startTime).Round(time.Minute).String()
	rightParts := segments
	rightParts = append(rightParts,
		lipgloss.NewStyle().Foreground(s.styles.T.Subtext).Render(fmt.Sprintf("%s │ %s", timer, clock)),
	)
	right := lipgloss.JoinHorizontal(lipgloss.Center, rightParts...)

	// 4. Composition with Flex Gaps
	leftWidth := lipgloss.Width(left)
	centerWidth := lipgloss.Width(center)
	rightWidth := lipgloss.Width(right)

	availableWidth := s.width - 4
	gap1 := (availableWidth/2 - centerWidth/2) - leftWidth
	gap2 := availableWidth - (leftWidth + gap1 + centerWidth) - rightWidth

	if gap1 < 1 {
		gap1 = 1
	}
	if gap2 < 1 {
		gap2 = 1
	}

	content := lipgloss.JoinHorizontal(lipgloss.Center,
		left,
		strings.Repeat(" ", gap1),
		center,
		strings.Repeat(" ", gap2),
		right,
	)

	// Final Container: Top border for footer, same HUD style as header
	return lipgloss.NewStyle().
		Background(s.styles.T.Background).
		Foreground(s.styles.T.Text).
		Padding(0, 2).
		Width(s.width).
		Border(lipgloss.NormalBorder(), true, false, false, false). // Top border for footer
		BorderForeground(s.styles.T.BorderNormal).
		Render(content)
}

// hudSegments renders the IDE-chrome segments between the shortcuts and the
// clock: context meter · pending-review · problems · git branch.
func (s StatusBar) hudSegments() []string {
	if !s.showSegments || s.width < 90 {
		return nil
	}
	segStyle := lipgloss.NewStyle().Foreground(s.styles.T.Muted)
	warnStyle := lipgloss.NewStyle().Foreground(s.styles.T.Yellow).Bold(true)
	errStyle := lipgloss.NewStyle().Foreground(s.styles.T.Red).Bold(true)
	accent := lipgloss.NewStyle().Foreground(s.styles.T.Accent)

	var parts []string

	// Context meter: "ctx 34% ###-----", warning tint past 80%.
	if s.ctxPercent > 0 {
		filled := int(s.ctxPercent / 10)
		if filled > 10 {
			filled = 10
		}
		meter := strings.Repeat("#", filled) + strings.Repeat("-", 10-filled)
		style := segStyle
		hint := ""
		if s.ctxPercent >= 80 {
			style = warnStyle
			hint = "!"
		}
		parts = append(parts, style.Render(fmt.Sprintf("ctx %.0f%% %s%s", s.ctxPercent, meter, hint)))
	}

	// Pending review count — jumps to diff pane in app handling.
	if s.pendingEdits > 0 {
		parts = append(parts, warnStyle.Render(fmt.Sprintf("%d pending", s.pendingEdits)))
	}

	// Problems count from session-edited files.
	if s.problems > 0 {
		parts = append(parts, errStyle.Render(fmt.Sprintf("%X problems", s.problems)))
	}

	// Git branch.
	if s.branch != "" {
		parts = append(parts, accent.Render(" "+s.branch))
	}

	for i, p := range parts {
		if i < len(parts)-1 {
			parts[i] = p + segStyle.Render(" │ ")
		}
	}
	return parts
}
