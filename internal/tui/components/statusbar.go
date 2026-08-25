package components

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

// StatusBar renders the bottom status bar with a HUD look matching the header.
//
// The bar is composed of independent slots rather than one status string:
//
//	▐ ACCEPT EDITS ▌  ⚙ read_file…               ctx 12% ##──────   main
//	└ mode            └ activity / outcome       └ HUD segments
//
// Keeping these separate is what lets the mode chip stay put while transient
// activity text churns.
//
// Key hints deliberately do NOT live here: they belong to the `└─` info line
// above the prompt, which has the room to phrase them for the current state.
// Duplicating them in the bar produced two competing sources of truth. There is
// no clock or session timer either — the bar only repaints on events, so any
// wall-clock readout it carried sat visibly stale between them.
type StatusBar struct {
	styles         *themes.Styles
	width          int
	permissionTool string
	browsing       bool

	// mode is the approval mode chip: manual | accept-edits | auto | plan.
	mode string
	// activity is transient: "thinking…", "⚙ read_file…", "retrying (3/10)".
	activity string
	// outcome is sticky until the next run starts: "CANCELLED", "ERROR".
	outcome string
	// queued is how many messages are waiting for the current run to end.
	queued int

	// HUD segments (IDE chrome)
	ctxPercent   float64 // context window usage, 0-100+
	pendingEdits int     // edits awaiting review
	branch       string  // git branch (+ahead/behind suffix)
	problems     int     // diagnostics introduced this session
	showSegments bool
}

// Outcome values. These are display-only sentinels for the sticky slot.
const (
	OutcomeCancelled = "CANCELLED"
	OutcomeError     = "ERROR"
)

// NewStatusBar creates a new StatusBar.
func NewStatusBar(styles *themes.Styles) StatusBar {
	return StatusBar{
		styles:       styles,
		activity:     "Ready",
		mode:         "manual",
		showSegments: true,
	}
}

// SetWidth updates the status bar width.
func (s *StatusBar) SetWidth(w int) { s.width = w }

// SetStatus updates the transient activity text. Kept under its original name
// because ~40 call sites across the app set status this way; the sticky
// outcome and the mode chip are set through their own setters so a passing
// status message can never overwrite them.
func (s *StatusBar) SetStatus(msg string) { s.activity = msg }

// Status returns the current activity text.
func (s StatusBar) Status() string { return s.activity }

// SetMode updates the approval-mode chip.
func (s *StatusBar) SetMode(mode string) { s.mode = mode }

// Mode returns the current mode chip value.
func (s StatusBar) Mode() string { return s.mode }

// SetOutcome sets the sticky outcome badge (OutcomeCancelled, OutcomeError, or
// "" to clear).
func (s *StatusBar) SetOutcome(outcome string) { s.outcome = outcome }

// SetQueued records how many messages are waiting to be delivered.
func (s *StatusBar) SetQueued(n int) { s.queued = n }

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
	s.activity = "Awaiting permission"
}

func (s *StatusBar) ClearPermission() { s.permissionTool = "" }

func (s *StatusBar) SetBrowsing(enabled bool) { s.browsing = enabled }

// segment is one renderable piece of the bar plus the priority that decides
// whether it survives a narrow terminal. Lower priority survives longer;
// priority 0 is never dropped.
type segment struct {
	text     string
	priority int
}

// modeStyle colors the mode chip by how much it lets the agent do without
// asking: the more autonomy, the warmer the color.
func (s StatusBar) modeStyle() lipgloss.Style {
	base := lipgloss.NewStyle().Bold(true).Padding(0, 1).Foreground(s.styles.T.Background)
	switch strings.ToLower(s.mode) {
	case "plan":
		return base.Background(s.styles.T.Accent)
	case "accept-edits":
		return base.Background(s.styles.T.Yellow)
	case "auto":
		return base.Background(s.styles.T.Red)
	default: // manual, and anything unrecognised
		return base.Background(s.styles.T.Blue)
	}
}

// modeLabel renders the chip text, abbreviating when the terminal is narrow.
func (s StatusBar) modeLabel() string {
	mode := strings.ToLower(s.mode)
	if mode == "" {
		mode = "manual"
	}
	if s.width < 70 {
		switch mode {
		case "accept-edits":
			return "EDITS"
		case "manual":
			return "MANUAL"
		case "auto":
			return "AUTO"
		case "plan":
			return "PLAN"
		}
	}
	return strings.ToUpper(strings.ReplaceAll(mode, "-", " "))
}

// activityStyle tints the transient text by what it reports.
func (s StatusBar) activityStyle() lipgloss.Style {
	base := lipgloss.NewStyle()
	activity := strings.ToLower(s.activity)
	switch {
	case s.outcome == OutcomeError, strings.Contains(activity, "error"), strings.Contains(activity, "fail"):
		return base.Foreground(s.styles.T.Red).Bold(true)
	case s.outcome == OutcomeCancelled:
		return base.Foreground(s.styles.T.Yellow).Bold(true)
	case strings.Contains(activity, "retry"), strings.Contains(activity, "retrying"):
		return base.Foreground(s.styles.T.Yellow).Bold(true)
	case strings.Contains(activity, "thinking"), strings.Contains(activity, "💭"):
		return base.Foreground(s.styles.T.Accent)
	case strings.Contains(activity, "ready"):
		return base.Foreground(s.styles.T.Subtext)
	default:
		return base.Foreground(s.styles.T.Text)
	}
}

// View renders the status bar.
func (s StatusBar) View() string {
	if s.width <= 0 {
		return ""
	}
	if s.permissionTool != "" {
		return s.renderBanner(
			lipgloss.NewStyle().Foreground(s.styles.T.Yellow).Bold(true).Render("AWAITING PERMISSION"),
			lipgloss.NewStyle().Foreground(s.styles.T.Subtext).Render(s.permissionTool+"  ·  y/n confirm"),
		)
	}
	if s.browsing {
		return s.renderBanner(
			lipgloss.NewStyle().Foreground(s.styles.T.Accent).Bold(true).Render("BROWSING CONVERSATION"),
			lipgloss.NewStyle().Foreground(s.styles.T.Subtext).Render("↑↓ scroll  ·  PgUp/PgDn  ·  Tab input"),
		)
	}

	// Available content width, inside the container's 2-cell side padding.
	avail := s.width - 4
	if avail < 8 {
		avail = 8
	}

	// The mode chip anchors the left edge and is never dropped or truncated:
	// the user must always be able to see how much the agent can do unasked.
	chip := s.modeStyle().Render(s.modeLabel())
	remaining := avail - ansi.StringWidth(chip) - 1 // 1 = gap after the chip

	left := s.leftSegments()
	right := s.rightSegments()

	// Drop lowest-priority segments until everything fits. Left (activity and
	// outcome) is what the user most needs, so it is fitted first and the HUD
	// competes for what remains.
	leftText := fitSegments(left, remaining)
	remaining -= ansi.StringWidth(leftText)

	rightText := fitSegments(right, maxInt(0, remaining-4))

	content := joinBar(avail, chip, leftText, rightText)

	return lipgloss.NewStyle().
		Background(s.styles.T.Background).
		Foreground(s.styles.T.Text).
		Padding(0, 2).
		Width(s.width).
		Border(lipgloss.NormalBorder(), true, false, false, false). // Top border for footer
		BorderForeground(s.styles.T.BorderNormal).
		Render(content)
}

// renderBanner draws the two-slot layout used by the permission and browsing
// takeovers, where the normal slots do not apply.
func (s StatusBar) renderBanner(left, right string) string {
	gap := s.width - ansi.StringWidth(left) - ansi.StringWidth(right) - 4
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

// leftSegments renders the outcome badge and transient activity that sit just
// right of the mode chip.
func (s StatusBar) leftSegments() []segment {
	var segs []segment
	if s.outcome != "" {
		style := lipgloss.NewStyle().Bold(true).Foreground(s.styles.T.Yellow)
		if s.outcome == OutcomeError {
			style = style.Foreground(s.styles.T.Red)
		}
		segs = append(segs, segment{text: style.Render(s.outcome), priority: 0})
	}
	if activity := strings.TrimSpace(s.activity); activity != "" && !s.activityRedundant(activity) {
		segs = append(segs, segment{text: s.activityStyle().Render(activity), priority: 1})
	}
	if s.queued > 0 {
		label := fmt.Sprintf("⇥ %d queued", s.queued)
		segs = append(segs, segment{
			text:     lipgloss.NewStyle().Foreground(s.styles.T.Cyan).Bold(true).Render(label),
			priority: 0,
		})
	}
	return segs
}

// activityRedundant reports whether the transient activity text would merely
// restate the sticky outcome badge. Legacy call sites set the activity to
// "Cancelled"/"Interrupted" through SetStatus at the same moment the badge is
// raised, which rendered as "CANCELLED  Cancelled".
func (s StatusBar) activityRedundant(activity string) bool {
	if s.outcome == "" {
		return false
	}
	a := strings.ToLower(strings.TrimSpace(activity))
	if a == "" || a == "ready" {
		return true
	}
	// Either direction: "cancelled" under a CANCELLED badge, and also a longer
	// phrase that already contains the badge word.
	o := strings.ToLower(s.outcome)
	if strings.Contains(o, a) || strings.Contains(a, o) {
		return true
	}
	switch s.outcome {
	case OutcomeCancelled:
		return a == "interrupted" || a == "stopped" || a == "aborted"
	case OutcomeError:
		return strings.HasPrefix(a, "error")
	}
	return false
}

// centerSegments is intentionally absent: key hints live in the info line.

// rightSegments renders the IDE-chrome HUD: context meter · pending review ·
// problems · git branch. No timer or clock — see the type comment.
func (s StatusBar) rightSegments() []segment {
	segStyle := lipgloss.NewStyle().Foreground(s.styles.T.Muted)
	warnStyle := lipgloss.NewStyle().Foreground(s.styles.T.Yellow).Bold(true)
	errStyle := lipgloss.NewStyle().Foreground(s.styles.T.Red).Bold(true)
	accent := lipgloss.NewStyle().Foreground(s.styles.T.Accent)

	var segs []segment
	if !s.showSegments {
		return segs
	}

	// Context meter: "ctx 34% ###-------", warning tint past 80%.
	if s.ctxPercent > 0 {
		filled := int(s.ctxPercent / 10)
		if filled > 10 {
			filled = 10
		}
		if filled < 0 {
			filled = 0
		}
		meter := strings.Repeat("#", filled) + strings.Repeat("-", 10-filled)
		style := segStyle
		hint := ""
		if s.ctxPercent >= 80 {
			style = warnStyle
			hint = "!"
		}
		segs = append(segs, segment{
			text:     style.Render(fmt.Sprintf("ctx %.0f%% %s%s", s.ctxPercent, meter, hint)),
			priority: 4,
		})
	}

	// Pending review count — jumps to diff pane in app handling.
	if s.pendingEdits > 0 {
		segs = append(segs, segment{
			text:     warnStyle.Render(fmt.Sprintf("%d pending", s.pendingEdits)),
			priority: 3,
		})
	}

	// Problems count from session-edited files.
	if s.problems > 0 {
		segs = append(segs, segment{
			text:     errStyle.Render(fmt.Sprintf("%d problems", s.problems)),
			priority: 3,
		})
	}

	if s.branch != "" {
		segs = append(segs, segment{text: accent.Render(" " + s.branch), priority: 2})
	}

	return segs
}

// fitSegments joins segments with " │ " separators, dropping the
// highest-priority (least important) ones until the result fits in width. A
// single segment that still does not fit is truncated with an ellipsis.
func fitSegments(segs []segment, width int) string {
	if len(segs) == 0 || width <= 0 {
		return ""
	}
	kept := make([]segment, len(segs))
	copy(kept, segs)

	for {
		joined := joinSegments(kept)
		if ansi.StringWidth(joined) <= width {
			return joined
		}
		// Find the droppable segment with the highest priority number.
		worst, worstIdx := -1, -1
		for i, seg := range kept {
			if seg.priority == 0 {
				continue // never dropped
			}
			if seg.priority > worst {
				worst, worstIdx = seg.priority, i
			}
		}
		if worstIdx < 0 {
			// Everything left is undroppable: truncate the joined result.
			return ansi.Truncate(joined, width, "…")
		}
		kept = append(kept[:worstIdx], kept[worstIdx+1:]...)
		if len(kept) == 0 {
			return ""
		}
	}
}

// joinSegments renders segments with the HUD separator between them.
func joinSegments(segs []segment) string {
	parts := make([]string, 0, len(segs))
	for _, seg := range segs {
		if seg.text != "" {
			parts = append(parts, seg.text)
		}
	}
	return strings.Join(parts, "  ")
}

// joinBar lays out the three slots across the available width, pushing the HUD
// to the right edge. Slots that came back empty consume no space.
func joinBar(avail int, chip, left, right string) string {
	var b strings.Builder
	b.WriteString(chip)
	used := ansi.StringWidth(chip)

	if left != "" {
		b.WriteString("  ")
		b.WriteString(left)
		used += 2 + ansi.StringWidth(left)
	}

	if rightW := ansi.StringWidth(right); rightW > 0 {
		gap := avail - used - rightW
		if gap < 2 {
			gap = 2
		}
		b.WriteString(strings.Repeat(" ", gap))
		b.WriteString(right)
	}

	// Hard cap: whatever the slot math produced, the bar never exceeds the
	// terminal. This is the last line of defence against a wrapped footer.
	out := b.String()
	if ansi.StringWidth(out) > avail {
		out = ansi.Truncate(out, avail, "…")
	}
	return out
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
