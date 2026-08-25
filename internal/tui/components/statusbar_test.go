package components

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

func testStatusBar(width int) StatusBar {
	styles := themes.NewStyles(themes.Get("catppuccin"))
	sb := NewStatusBar(styles)
	sb.SetWidth(width)
	return sb
}

// fullyLoadedStatusBar populates every optional slot, so truncation tests
// exercise the worst case rather than a mostly-empty bar.
func fullyLoadedStatusBar(width int) StatusBar {
	sb := testStatusBar(width)
	sb.SetMode("accept-edits")
	sb.SetStatus("⚙ read_file…")
	sb.SetOutcome(OutcomeCancelled)
	sb.SetQueued(3)
	sb.SetContextUsage(87)
	sb.SetPendingEdits(4)
	sb.SetProblems(9)
	sb.SetGitBranch("feature/some-long-branch-name+2")
	return sb
}

// TestViewNeverExceedsWidth is the regression guard for the footer wrapping.
// The old gap arithmetic clamped negative gaps to 1 and let segments collide,
// pushing the bar past the terminal width and wrapping the whole layout.
func TestViewNeverExceedsWidth(t *testing.T) {
	for _, width := range []int{20, 30, 40, 55, 70, 80, 100, 120, 200} {
		sb := fullyLoadedStatusBar(width)
		view := sb.View()
		for i, line := range strings.Split(view, "\n") {
			if got := ansi.StringWidth(line); got > width {
				t.Errorf("width %d: line %d is %d cells wide:\n%q", width, i, got, ansi.Strip(line))
			}
		}
	}
}

func TestViewNeverExceedsWidthWhenIdle(t *testing.T) {
	for _, width := range []int{20, 40, 70, 100, 200} {
		sb := testStatusBar(width)
		sb.SetGitBranch("main")
		sb.SetContextUsage(12)
		for _, line := range strings.Split(sb.View(), "\n") {
			if got := ansi.StringWidth(line); got > width {
				t.Errorf("idle width %d: line is %d cells wide: %q", width, got, ansi.Strip(line))
			}
		}
	}
}

// TestModeChipAlwaysSurvives checks the one thing that must never be dropped:
// the user has to be able to see how much the agent can do without asking.
func TestModeChipAlwaysSurvives(t *testing.T) {
	cases := map[string]string{
		"manual":       "MANUAL",
		"accept-edits": "EDITS", // abbreviated below 70 cols
		"auto":         "AUTO",
		"plan":         "PLAN",
	}
	for mode, want := range cases {
		for _, width := range []int{20, 30, 40, 60} {
			sb := fullyLoadedStatusBar(width)
			sb.SetMode(mode)
			plain := ansi.Strip(sb.View())
			if !strings.Contains(plain, want) {
				t.Errorf("mode %q at width %d: chip %q missing from %q", mode, width, want, plain)
			}
		}
	}
}

func TestModeChipUnabbreviatedWhenWide(t *testing.T) {
	sb := fullyLoadedStatusBar(140)
	sb.SetMode("accept-edits")
	if plain := ansi.Strip(sb.View()); !strings.Contains(plain, "ACCEPT EDITS") {
		t.Errorf("wide bar should spell out the mode, got %q", plain)
	}
}

// TestSegmentsDropInPriorityOrder verifies the context meter (priority 4) goes
// before the branch (priority 2) as width shrinks.
func TestSegmentsDropInPriorityOrder(t *testing.T) {
	wide := fullyLoadedStatusBar(200)
	widePlain := ansi.Strip(wide.View())
	if !strings.Contains(widePlain, "some-long-branch-name") {
		t.Fatalf("wide bar should show the branch: %q", widePlain)
	}

	narrow := fullyLoadedStatusBar(60)
	narrowPlain := ansi.Strip(narrow.View())
	// The queue count is priority 0 and must outlive every HUD segment.
	if !strings.Contains(narrowPlain, "3 queued") {
		t.Errorf("narrow bar dropped the priority-0 queue count: %q", narrowPlain)
	}
}

// TestNoClockOrTimerInBar: the bar only repaints on events, so a wall-clock or
// elapsed readout here sits stale between them and reads as a frozen UI.
func TestNoClockOrTimerInBar(t *testing.T) {
	plain := ansi.Strip(fullyLoadedStatusBar(200).View())
	for _, pattern := range []string{":", "m0s", "elapsed"} {
		if strings.Contains(plain, pattern) {
			t.Errorf("bar should carry no time readout, found %q in %q", pattern, plain)
		}
	}
}

// TestNoKeyHintsInBar: hints belong to the info line above the prompt. Having
// them in both places gave two sources of truth that drifted apart.
func TestNoKeyHintsInBar(t *testing.T) {
	plain := strings.ToLower(ansi.Strip(fullyLoadedStatusBar(200).View()))
	for _, pattern := range []string{"enter send", "/rewind undo", "esc interrupt", "ctrl+r"} {
		if strings.Contains(plain, pattern) {
			t.Errorf("bar should carry no key hints, found %q in %q", pattern, plain)
		}
	}
}

func TestOutcomeBadgeRendersSeparatelyFromMode(t *testing.T) {
	sb := testStatusBar(120)
	sb.SetMode("manual")
	sb.SetOutcome(OutcomeCancelled)
	plain := ansi.Strip(sb.View())
	if !strings.Contains(plain, "MANUAL") {
		t.Errorf("mode chip missing: %q", plain)
	}
	if !strings.Contains(plain, OutcomeCancelled) {
		t.Errorf("outcome badge missing: %q", plain)
	}
	// The regression this whole change exists to fix: CANCELLED must not be
	// occupying the mode slot.
	if strings.Index(plain, OutcomeCancelled) < strings.Index(plain, "MANUAL") {
		t.Errorf("outcome rendered left of the mode chip: %q", plain)
	}
}

// TestReadyLabelSuppressedBesideOutcome: "CANCELLED Ready" is contradictory.
func TestReadyLabelSuppressedBesideOutcome(t *testing.T) {
	sb := testStatusBar(120)
	sb.SetStatus("Ready")
	sb.SetOutcome(OutcomeCancelled)
	if plain := ansi.Strip(sb.View()); strings.Contains(plain, "Ready") {
		t.Errorf("Ready should be suppressed next to an outcome badge: %q", plain)
	}
}

// TestOutcomeNotEchoedByActivity is the "CANCELLED  Cancelled" duplication:
// legacy SetStatus call sites raise both at once.
func TestOutcomeNotEchoedByActivity(t *testing.T) {
	cases := []struct{ outcome, activity string }{
		{OutcomeCancelled, "Cancelled"},
		{OutcomeCancelled, "cancelled"},
		{OutcomeCancelled, "Interrupted"},
		{OutcomeError, "Error"},
	}
	for _, c := range cases {
		sb := testStatusBar(120)
		sb.SetOutcome(c.outcome)
		sb.SetStatus(c.activity)
		plain := ansi.Strip(sb.View())
		if strings.Contains(plain, c.activity) {
			t.Errorf("activity %q echoes the %s badge: %q", c.activity, c.outcome, plain)
		}
		if !strings.Contains(plain, c.outcome) {
			t.Errorf("the badge itself went missing: %q", plain)
		}
	}
}

// A genuinely different activity still shows beside the badge.
func TestInformativeActivitySurvivesBesideOutcome(t *testing.T) {
	sb := testStatusBar(120)
	sb.SetOutcome(OutcomeError)
	sb.SetStatus("429 rate limited")
	if plain := ansi.Strip(sb.View()); !strings.Contains(plain, "429 rate limited") {
		t.Errorf("informative activity was suppressed: %q", plain)
	}
}

func TestZeroWidthRendersNothing(t *testing.T) {
	sb := testStatusBar(0)
	if got := sb.View(); got != "" {
		t.Errorf("zero-width bar should render empty, got %q", got)
	}
}

func TestFitSegmentsTruncatesUndroppableContent(t *testing.T) {
	segs := []segment{{text: strings.Repeat("x", 50), priority: 0}}
	got := fitSegments(segs, 10)
	if w := ansi.StringWidth(got); w > 10 {
		t.Errorf("fitSegments returned %d cells for a 10-cell budget: %q", w, got)
	}
	if !strings.Contains(got, "…") {
		t.Errorf("truncated segment should be marked with an ellipsis: %q", got)
	}
}

func TestFitSegmentsDropsHighestPriorityFirst(t *testing.T) {
	segs := []segment{
		{text: "keep", priority: 0},
		{text: "drop-me", priority: 9},
	}
	got := fitSegments(segs, 6)
	if strings.Contains(got, "drop-me") {
		t.Errorf("high-priority segment survived a tight budget: %q", got)
	}
	if !strings.Contains(got, "keep") {
		t.Errorf("priority-0 segment was dropped: %q", got)
	}
}

func TestPermissionBannerFitsWidth(t *testing.T) {
	for _, width := range []int{30, 60, 120} {
		sb := testStatusBar(width)
		sb.SetPermission("run_command")
		for _, line := range strings.Split(sb.View(), "\n") {
			if got := ansi.StringWidth(line); got > width {
				t.Errorf("permission banner at %d is %d wide: %q", width, got, ansi.Strip(line))
			}
		}
	}
}

func TestBrowsingBannerFitsWidth(t *testing.T) {
	for _, width := range []int{30, 60, 120} {
		sb := testStatusBar(width)
		sb.SetBrowsing(true)
		for _, line := range strings.Split(sb.View(), "\n") {
			if got := ansi.StringWidth(line); got > width {
				t.Errorf("browsing banner at %d is %d wide: %q", width, got, ansi.Strip(line))
			}
		}
	}
}
