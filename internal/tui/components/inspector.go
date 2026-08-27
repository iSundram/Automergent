package components

// The background-task inspector: a full-screen live view of one shell or agent.
//
// Inspecting a dock entry used to call diffPane.SetContent with a formatted
// string, which meant a pane built for reviewing diffs was rendering shell
// output. That borrowed the wrong chrome, clobbered the diff pane's file-tab
// strip on the way in, reset the pending-proposal state on the way out, and
// produced a dead snapshot: the output you opened was the output you kept, no
// matter what the process did next.
//
// This is the pane that should have existed. It follows a live task, holds still
// when you scroll back, and knows what it is looking at.

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/iSundram/Automergent/internal/tui/render"
	"github.com/iSundram/Automergent/internal/tui/themes"
)

// InspectorSource supplies the live content for whatever is being inspected.
// The pane holds one of these rather than a string, which is the difference
// between following a task and having photographed it.
type InspectorSource interface {
	// Title is the header line: what this task is.
	Title() string
	// Facts are the header's second line: id, status, counts.
	Facts() []string
	// Lines returns the current body, oldest first.
	Lines() []string
	// Live reports whether more output can still arrive.
	Live() bool
}

// Inspector is the full-screen task viewer.
type Inspector struct {
	styles  *themes.Styles
	src     InspectorSource
	visible bool
	follow  bool
	offset  int
	width   int
	height  int
	filter  string
}

// NewInspector creates a hidden inspector.
func NewInspector(styles *themes.Styles) *Inspector {
	return &Inspector{styles: styles, follow: true}
}

// SetSize updates dimensions.
func (v *Inspector) SetSize(w, h int) { v.width, v.height = w, h }

// Visible reports whether the pane owns the screen.
func (v Inspector) Visible() bool { return v.visible }

// Show opens the pane on a source, following by default: a task you just opened
// is one you want to watch.
func (v *Inspector) Show(src InspectorSource) {
	v.src = src
	v.visible = true
	v.follow = true
	v.offset = 0
	v.filter = ""
}

// Hide closes the pane and releases the source.
func (v *Inspector) Hide() {
	v.visible = false
	v.src = nil
	v.filter = ""
}

// Following reports whether the view is pinned to the newest output.
func (v Inspector) Following() bool { return v.follow }

// ToggleFollow pins or unpins the view from the tail.
func (v *Inspector) ToggleFollow() {
	v.follow = !v.follow
	if v.follow {
		v.offset = 0
	}
}

// Scroll moves the view by delta rows, oldest-ward for negative values. Any
// manual scroll drops follow mode, because a view that yanks itself back to the
// bottom while you are reading is worse than one that does not follow at all.
func (v *Inspector) Scroll(delta int) {
	v.follow = false
	v.offset -= delta
	if v.offset < 0 {
		v.offset = 0
	}
	if max := v.maxOffset(); v.offset > max {
		v.offset = max
	}
}

// GotoEnd returns to the newest output and resumes following.
func (v *Inspector) GotoEnd() {
	v.offset = 0
	v.follow = true
}

// SetFilter keeps only lines containing the substring. Empty clears it.
func (v *Inspector) SetFilter(s string) {
	v.filter = s
	v.offset = 0
}

// Filter returns the active substring filter.
func (v Inspector) Filter() string { return v.filter }

// bodyHeight is how many output rows fit: total minus header (2), rule (1),
// footer (1).
func (v Inspector) bodyHeight() int {
	h := v.height - 4
	if h < 1 {
		h = 1
	}
	return h
}

// lines returns the filtered body.
func (v Inspector) lines() []string {
	if v.src == nil {
		return nil
	}
	all := v.src.Lines()
	if v.filter == "" {
		return all
	}
	needle := strings.ToLower(v.filter)
	kept := make([]string, 0, len(all))
	for _, l := range all {
		if strings.Contains(strings.ToLower(l), needle) {
			kept = append(kept, l)
		}
	}
	return kept
}

func (v Inspector) maxOffset() int {
	over := len(v.lines()) - v.bodyHeight()
	if over < 0 {
		return 0
	}
	return over
}

// Text returns the visible body rows, for tests.
func (v Inspector) Text() []string {
	all := v.lines()
	h := v.bodyHeight()
	if len(all) <= h {
		return all
	}
	end := len(all) - v.offset
	if end > len(all) {
		end = len(all)
	}
	if end < h {
		end = h
	}
	return all[end-h : end]
}

// View renders the pane.
func (v Inspector) View() string {
	if !v.visible || v.src == nil || v.width <= 0 || v.height <= 0 {
		return ""
	}
	t := v.styles.T
	inner := v.width - 4
	if inner < 20 {
		inner = 20
	}

	var b strings.Builder

	title := lipgloss.NewStyle().Foreground(t.Accent).Bold(true).
		Render(render.Clip(v.src.Title(), inner))
	b.WriteString(title + "\n")

	facts := append([]string{}, v.src.Facts()...)
	if v.filter != "" {
		facts = append(facts, fmt.Sprintf("filter %q", v.filter))
	}
	if v.src.Live() {
		if v.follow {
			facts = append(facts, "following")
		} else {
			facts = append(facts, "paused")
		}
	}
	b.WriteString(v.styles.Dim.Render(render.Clip(strings.Join(facts, render.GlyphSep), inner)) + "\n")
	b.WriteString(v.styles.Dim.Render(render.Rule(inner)) + "\n")

	body := v.Text()
	textStyle := lipgloss.NewStyle().Foreground(t.Text)
	for _, l := range body {
		b.WriteString(textStyle.Render(render.Clip(strings.TrimRight(l, "\r"), inner)) + "\n")
	}
	// Pad to a constant height so the footer sits on the bottom edge instead of
	// floating up under a short body.
	for i := len(body); i < v.bodyHeight(); i++ {
		b.WriteString("\n")
	}

	b.WriteString(v.styles.Dim.Render(render.Clip(v.footer(), inner)))

	return lipgloss.NewStyle().
		Background(t.Background).
		Padding(1, 2).
		Width(v.width).
		Height(v.height).
		Render(b.String())
}

func (v Inspector) footer() string {
	hints := []string{
		render.GlyphUp + render.GlyphDown + " scroll",
		"f follow",
		"/ filter",
		"s stop",
		"esc close",
	}
	pos := ""
	if total := len(v.lines()); total > v.bodyHeight() {
		shown := total - v.offset
		pos = fmt.Sprintf("%d/%d", shown, total)
	}
	line := strings.Join(hints, render.GlyphSep)
	if pos != "" {
		line += render.GlyphSep + pos
	}
	return render.GlyphLast + render.GlyphRule + " " + line
}
