package components

import (
	"fmt"
	"image/color"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/iSundram/Automergent/internal/tui/render"
	"github.com/iSundram/Automergent/internal/tui/themes"
)

// DiffAcceptMsg is sent when user accepts the diff.
type DiffAcceptMsg struct{}

// DiffRejectMsg is sent when user rejects the diff.
type DiffRejectMsg struct{}

type diffMode int

const (
	diffModeView diffMode = iota
	diffModeConfirm
	diffModeRejectReason
)

// diffTab is one modified file in the IDE-style tab strip. Tabs are kept in
// recency order: index 0 is the most recently edited file, which matches the
// VS Code behavior of surfacing what just changed.
type diffTab struct {
	Filename   string // path as written in the +++ header; "" marks a plain (non-file) view
	RawContent string
	AddCount   int
	DelCount   int
	TotalLines int
	TouchedAt  time.Time
}

// Diff is a fullscreen scrollable diff viewer with inline confirmation,
// a rounded recency-ordered tab strip of modified files and a VS Code-style
// minimap column showing where the viewport sits and where edits are.
type Diff struct {
	viewport    viewport.Model
	styles      *themes.Styles
	visible     bool
	tabs        []diffTab // recency-ordered, index 0 = most recent
	active      int       // selected tab index into tabs
	mode        diffMode
	replyCh     chan Confirmation
	rejectInput textinput.Model
	width       int
	height      int
}

// NewDiff creates a new Diff component.
func NewDiff(styles *themes.Styles) Diff {
	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(20))
	vp.MouseWheelEnabled = true

	ti := textinput.New()
	ti.Placeholder = "optional reason..."
	ti.Prompt = ""
	ti.Focus()

	return Diff{viewport: vp, styles: styles, rejectInput: ti}
}

// SetSize updates dimensions for fullscreen.
func (d *Diff) ensureViewport() {
	if d.viewport.Width() == 0 && d.viewport.Height() == 0 {
		vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(20))
		vp.MouseWheelEnabled = true
		d.viewport = vp
	}
}

func (d *Diff) SetSize(w, h int) {
	d.ensureViewport()
	d.width = w
	d.height = h
	d.refresh()
}

// current returns the active tab, or nil when nothing is loaded.
func (d *Diff) current() *diffTab {
	if len(d.tabs) == 0 || d.active < 0 || d.active >= len(d.tabs) {
		return nil
	}
	return &d.tabs[d.active]
}

// parseTab recomputes stats from a tab's raw unified-diff content.
func parseTab(t *diffTab) {
	lines := strings.Split(t.RawContent, "\n")
	t.TotalLines = len(lines)
	t.AddCount, t.DelCount = 0, 0
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			t.AddCount++
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			t.DelCount++
		}
	}
	if t.TouchedAt.IsZero() {
		t.TouchedAt = time.Now()
	}
}

// filenameFromDiff extracts the file path from a "+++ <path>" header.
func filenameFromDiff(content string) string {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "+++ ") {
			name := strings.TrimPrefix(line, "+++ ")
			name = strings.TrimPrefix(name, "b/")
			if idx := strings.Index(name, "\t"); idx != -1 {
				name = name[:idx]
			}
			name = strings.TrimSuffix(name, " (proposed)")
			return name
		}
	}
	return ""
}

// SetContent shows a single plain view (legacy entry point used by the shell
// dock and proposal review). It replaces any file tabs, matching the previous
// replace-everything behavior of those callers.
func (d *Diff) SetContent(content string) {
	tab := diffTab{RawContent: content, TouchedAt: time.Now()}
	parseTab(&tab)
	d.tabs = []diffTab{tab}
	d.active = 0
	d.refresh()
}

// OpenFile upserts a modified-file tab keyed by its path: existing content is
// refreshed, the tab moves to the front (most recently edited) and becomes the
// selected one, so an incoming write/edit lands on screen immediately with the
// other modified files queued to its right.
func (d *Diff) OpenFile(filename, content string) {
	now := time.Now()
	for i := range d.tabs {
		if d.tabs[i].Filename == filename {
			tab := &d.tabs[i]
			tab.RawContent = content
			tab.TouchedAt = now
			parseTab(tab)
			moved := d.tabs[i]
			copy(d.tabs[1:i+1], d.tabs[0:i])
			d.tabs[0] = moved
			d.active = 0
			d.refresh()
			return
		}
	}
	tab := diffTab{Filename: filename, RawContent: content, TouchedAt: now}
	parseTab(&tab)
	d.tabs = append([]diffTab{tab}, d.tabs...)
	d.active = 0
	d.refresh()
}

// TabCount reports how many modified-file tabs are held.
func (d *Diff) TabCount() int {
	n := 0
	for i := range d.tabs {
		if d.tabs[i].Filename != "" {
			n++
		}
	}
	return n
}

// FilePaths lists the paths of all file tabs in strip order (most recently
// touched first). Backs path-gated command visibility: the recent-file set.
func (d *Diff) FilePaths() []string {
	out := make([]string, 0, len(d.tabs))
	for _, idx := range d.visualOrder() {
		if name := d.tabs[idx].Filename; name != "" {
			out = append(out, name)
		}
	}
	return out
}

// ActiveLabel returns the display path of the selected tab.
func (d *Diff) ActiveLabel() string {
	if c := d.current(); c != nil {
		return c.Filename
	}
	return ""
}

func (d *Diff) cycleTab(delta int) {
	if len(d.tabs) < 2 {
		return
	}
	d.active = ((d.active+delta)%len(d.tabs) + len(d.tabs)) % len(d.tabs)
	d.refresh()
	d.scrollToFirstChange()
}

// visualOrder lists tab indices in strip order — the selected tab renders
// first (Chrome-style), then the rest by recency. Digit keys use this order,
// so the number printed on a pill always selects that pill.
func (d Diff) visualOrder() []int {
	order := make([]int, 0, len(d.tabs))
	if len(d.tabs) == 0 {
		return order
	}
	order = append(order, d.active)
	for i := range d.tabs {
		if i != d.active {
			order = append(order, i)
		}
	}
	return order
}

// SelectTab activates the nth visual slot of the tab strip.
func (d *Diff) SelectTab(slot int) bool {
	order := d.visualOrder()
	if slot < 0 || slot >= len(order) {
		return false
	}
	if d.active != order[slot] {
		d.active = order[slot]
		d.refresh()
		d.scrollToFirstChange()
	}
	return true
}

// ShowWithConfirm shows diff and waits for user confirmation.
func (d *Diff) ShowWithConfirm(replyCh chan Confirmation) {
	d.visible = true
	d.mode = diffModeConfirm
	d.replyCh = replyCh
	d.rejectInput.Reset()
	d.scrollToFirstChange()
}

// scrollToFirstChange scrolls viewport to show the first changed line (+ or -).
func (d *Diff) scrollToFirstChange() {
	d.ensureViewport()
	c := d.current()
	if c == nil {
		return
	}
	lines := strings.Split(c.RawContent, "\n")

	// Find first actual change line (+ or - but not --- or +++)
	for i, line := range lines {
		if len(line) > 0 {
			if (line[0] == '+' && !strings.HasPrefix(line, "+++")) ||
				(line[0] == '-' && !strings.HasPrefix(line, "---")) {
				// Found first change, scroll to show it with some context above
				target := i - 3
				if target < 0 {
					target = 0
				}
				d.viewport.SetYOffset(target)
				return
			}
		}
	}
	// No changes found, stay at top
	d.viewport.SetYOffset(0)
}

func (d *Diff) refresh() {
	d.ensureViewport()
	c := d.current()
	if c == nil {
		d.viewport.SetContent("")
		return
	}

	// File views carry the tab strip (3 rows) and minimap column (4 cols);
	// plain legacy views keep their original geometry exactly.
	vw := d.width - 6
	vh := d.height - 5
	if c.Filename != "" {
		vw -= 4
		vh -= 3
	}
	if vw < 10 {
		vw = 10
	}
	if vh < 1 {
		vh = 1
	}
	if d.viewport.Width() != vw {
		d.viewport.SetWidth(vw)
	}
	if d.viewport.Height() != vh {
		d.viewport.SetHeight(vh)
	}
	d.viewport.SetContent(render.DiffWithWidth(c.RawContent, d.viewport.Width()))
}

// Toggle visibility.
func (d *Diff) Toggle() { d.visible = !d.visible }

// Show the diff.
func (d *Diff) Show() {
	d.visible = true
	d.scrollToFirstChange()
}

// Hide the diff and reset mode. Tabs are retained so /diff or ctrl+w can
// reopen the workspace's recently edited files afterwards.
func (d *Diff) Hide() {
	d.visible = false
	d.mode = diffModeView
	d.replyCh = nil
	d.rejectInput.Reset()
}

// Visible returns visibility state.
func (d *Diff) Visible() bool { return d.visible }

// Focus is no-op for fullscreen.
func (d *Diff) Focus(_ bool) {}

// Update handles input.
func (d Diff) Update(msg tea.Msg) (Diff, tea.Cmd) {
	(&d).ensureViewport()
	if !d.visible {
		return d, nil
	}

	// Handle reject reason input mode
	if d.mode == diffModeRejectReason {
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.String() {
			case "enter":
				// Submit rejection with reason
				reason := strings.TrimSpace(d.rejectInput.Value())
				if d.replyCh != nil {
					select {
					case d.replyCh <- Confirmation{Allow: false, Feedback: reason}:
					default:
					}
				}
				d.Hide()
				return d, nil
			case "esc":
				// Skip reason, just reject
				if d.replyCh != nil {
					select {
					case d.replyCh <- Confirmation{Allow: false}:
					default:
					}
				}
				d.Hide()
				return d, nil
			}
		}
		// Update text input
		var cmd tea.Cmd
		d.rejectInput, cmd = d.rejectInput.Update(msg)
		return d, cmd
	}

	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "y", "Y", "enter":
			if d.mode == diffModeConfirm && d.replyCh != nil {
				select {
				case d.replyCh <- Confirmation{Allow: true}:
				default:
				}
				d.Hide()
				return d, nil
			}
		case "a", "A":
			if d.mode == diffModeConfirm && d.replyCh != nil {
				select {
				case d.replyCh <- Confirmation{Allow: true, Always: true}:
				default:
				}
				d.Hide()
				return d, nil
			}
		case "n", "N":
			if d.mode == diffModeConfirm && d.replyCh != nil {
				// Enter reject reason mode
				d.mode = diffModeRejectReason
				d.rejectInput.Focus()
				return d, textinput.Blink
			}
			d.visible = false
			return d, nil
		case "q", "esc":
			if d.mode == diffModeConfirm && d.replyCh != nil {
				// Quick reject without reason
				select {
				case d.replyCh <- Confirmation{Allow: false}:
				default:
				}
				d.Hide()
				return d, nil
			}
			d.visible = false
			return d, nil
		case "g":
			d.viewport.SetYOffset(0)
			return d, nil
		case "G":
			d.viewport.GotoBottom()
			return d, nil
		case "j", "down":
			vp, cmd := d.viewport.Update(msg)
			d.viewport = vp
			return d, cmd
		case "k", "up":
			vp, cmd := d.viewport.Update(msg)
			d.viewport = vp
			return d, cmd
		}

		// Tab-strip navigation: only while not typing a reject reason.
		switch km.String() {
		case "tab", "]", "ctrl+w":
			d.cycleTab(1)
			return d, nil
		case "shift+tab", "[":
			d.cycleTab(-1)
			return d, nil
		default:
			if len(km.String()) == 1 && km.String()[0] >= '1' && km.String()[0] <= '9' {
				n, _ := strconv.Atoi(km.String())
				if d.SelectTab(n - 1) {
					return d, nil
				}
			}
		}
	}

	vp, cmd := d.viewport.Update(msg)
	d.viewport = vp
	return d, cmd
}

// View renders fullscreen diff with tab strip, minimap and inline confirmation.
func (d Diff) View() string {
	(&d).ensureViewport()
	if !d.visible {
		return ""
	}
	c := d.current()
	if c == nil {
		return d.viewEmpty()
	}
	fileMode := c.Filename != ""

	bg := d.styles.T.Background

	// Tab strip (file views only).
	var strip string
	if fileMode {
		strip = d.renderTabStrip()
	}

	// Header
	icon := lipgloss.NewStyle().Foreground(d.styles.T.Accent).Render("▸ ")
	filename := lipgloss.NewStyle().Bold(true).Foreground(d.styles.T.Text).Render(displayPath(c.Filename))
	addLabel := lipgloss.NewStyle().Foreground(d.styles.T.Green).Bold(true).Render(fmt.Sprintf("+%d", c.AddCount))
	delLabel := lipgloss.NewStyle().Foreground(d.styles.T.Red).Bold(true).Render(fmt.Sprintf("-%d", c.DelCount))

	headerW := d.width - 4
	pos := ""
	if len(d.tabs) > 1 {
		pos = fmt.Sprintf(" · %d/%d", d.active+1, len(d.tabs))
	}
	posLabel := d.styles.Dim.Render(pos)
	headerLeft := icon + filename
	headerRight := posLabel + " " + addLabel + " " + delLabel
	spacer := headerW - lipgloss.Width(headerLeft) - lipgloss.Width(headerRight)
	if spacer < 1 {
		spacer = 1
	}
	header := headerLeft + strings.Repeat(" ", spacer) + headerRight

	// Separator
	sep := lipgloss.NewStyle().Foreground(d.styles.T.BorderNormal).Render(strings.Repeat("─", headerW))

	// Content (+ minimap gutter for file views).
	content := d.viewport.View()
	if fileMode {
		content = d.withMinimap(content, c)
	}

	// Scroll indicator
	yOffset := d.viewport.YOffset()
	var scrollInfo string
	if yOffset > 0 {
		scrollInfo = d.styles.Dim.Render(fmt.Sprintf("↑ %d hidden", yOffset))
	}
	remaining := c.TotalLines - yOffset - d.viewport.Height()
	if remaining > 0 {
		if scrollInfo != "" {
			scrollInfo += "  "
		}
		scrollInfo += d.styles.Dim.Render(fmt.Sprintf("↓ %d more", remaining))
	}

	// Footer - different for each mode
	var footer string
	switch d.mode {
	case diffModeRejectReason:
		// Reject reason input - hide hint once user starts typing
		label := lipgloss.NewStyle().Foreground(d.styles.T.Yellow).Bold(true).Render("Reject reason: ")
		input := d.rejectInput.View()
		var hint string
		if d.rejectInput.Value() == "" {
			hint = d.styles.Dim.Render("  [Enter] submit  [Esc] skip")
		}
		footer = label + input + hint

	case diffModeConfirm:
		// Confirmation buttons in footer
		yBtn := lipgloss.NewStyle().Background(d.styles.T.Green).Foreground(d.styles.T.Background).Bold(true).Padding(0, 1).Render("y Accept")
		aBtn := lipgloss.NewStyle().Background(d.styles.T.Accent).Foreground(d.styles.T.Background).Bold(true).Padding(0, 1).Render("a Always")
		nBtn := lipgloss.NewStyle().Background(d.styles.T.Red).Foreground(d.styles.T.Background).Bold(true).Padding(0, 1).Render("n Reject")
		nav := d.styles.Dim.Render("[j/k] scroll  [esc] cancel")
		if len(d.tabs) > 1 {
			nav = d.styles.Dim.Render("[tab] file  [j/k] scroll  [esc] cancel")
		}

		buttons := yBtn + "  " + aBtn + "  " + nBtn + "    " + nav

		footerSpacer := headerW - lipgloss.Width(scrollInfo) - lipgloss.Width(buttons)
		if footerSpacer < 1 {
			footerSpacer = 1
		}
		footer = scrollInfo + strings.Repeat(" ", footerSpacer) + buttons

	default:
		help := d.styles.Dim.Render("[j/k] scroll  [g/G] top/end  [q] close")
		if len(d.tabs) > 1 {
			help = d.styles.Dim.Render("[tab]/[1-9] file  [j/k] scroll  [g/G] top/end  [q] close")
		}
		footerSpacer := headerW - lipgloss.Width(scrollInfo) - lipgloss.Width(help)
		if footerSpacer < 1 {
			footerSpacer = 1
		}
		footer = scrollInfo + strings.Repeat(" ", footerSpacer) + help
	}

	// Assemble
	parts := []string{header, sep}
	if strip != "" {
		parts = append(parts, strip)
	}
	parts = append(parts, content, sep, footer)

	view := lipgloss.JoinVertical(lipgloss.Left, parts...)

	// Fullscreen with solid background
	return lipgloss.NewStyle().
		Background(bg).
		Foreground(d.styles.T.Text).
		Width(d.width).
		Height(d.height).
		Padding(1, 2).
		Render(view)
}

// viewEmpty renders the no-edits-yet state. View must stay nil-safe: /diff
// and ctrl+w can open the pane before the agent has touched any file.
func (d Diff) viewEmpty() string {
	headerW := d.width - 4
	if headerW < 1 {
		headerW = 1
	}
	icon := lipgloss.NewStyle().Foreground(d.styles.T.Accent).Render("▸ ")
	title := lipgloss.NewStyle().Bold(true).Foreground(d.styles.T.Text).Render("Workspace diff")
	sep := lipgloss.NewStyle().Foreground(d.styles.T.BorderNormal).Render(strings.Repeat("─", headerW))
	hint := d.styles.Dim.Render("No modified files yet — files the agent edits appear here as tabs.")

	view := lipgloss.JoinVertical(lipgloss.Left,
		icon+title,
		sep,
		"",
		hint,
	)
	return lipgloss.NewStyle().
		Background(d.styles.T.Background).
		Foreground(d.styles.T.Text).
		Width(d.width).
		Height(d.height).
		Padding(1, 2).
		Render(view)
}

// displayPath shortens long paths from the left so the header stays readable.
func displayPath(path string) string {
	if len(path) <= 64 {
		return path
	}
	return "…" + path[len(path)-63:]
}

// renderTabStrip draws Chrome-style tabs:
//
//	╭─────────────────╮ ╭───────────╮ ╭────────────╮
//	│ README.md +1 -1 │ │ 2 main.go │ │ 3 demo2.md │
//	╯                 ╰─           ──              ─
//
// The selected tab is an "opened" lime pill whose bottom corners turn outward
// into a thin rule (touching, Chrome-style); inactive tabs are short — top
// and middle only — and never merge into the baseline. Switching tabs swaps
// the chrome with them automatically.
func (d Diff) renderTabStrip() string {
	order := d.visualOrder()
	if len(order) == 0 {
		return ""
	}

	maxW := d.width - 8
	if maxW < 24 {
		maxW = 24
	}

	t := d.styles.T
	lime := lipgloss.NewStyle().Foreground(t.Green).Bold(true)
	limeEdge := lipgloss.NewStyle().Foreground(t.Green)
	addStyle := lipgloss.NewStyle().Foreground(t.Green)
	delStyle := lipgloss.NewStyle().Foreground(t.Red)
	numN := lipgloss.NewStyle().Foreground(t.Muted).Bold(true)
	nameN := lipgloss.NewStyle().Foreground(t.Muted)
	edgeN := lipgloss.NewStyle().Foreground(t.BorderNormal)
	ruleStyle := lipgloss.NewStyle().Foreground(t.BorderNormal)
	over := lipgloss.NewStyle().Foreground(t.Muted)

	type pill struct {
		w      int
		mid    string
		active bool
	}

	var pills []pill
	used := 0
	hidden := 0
	for slot, idx := range order {
		tab := d.tabs[idx]
		name := filepath.Base(tab.Filename)
		if name == "" {
			continue
		}
		name = ansi.Truncate(name, 26, "…")

		var mid string
		if slot == 0 {
			inner := lime.Render(name) +
				addStyle.Render(fmt.Sprintf(" +%d", tab.AddCount)) +
				delStyle.Render(fmt.Sprintf(" -%d", tab.DelCount))
			mid = limeEdge.Render("│ ") + inner + " " + limeEdge.Render("│")
		} else {
			inner := numN.Render(strconv.Itoa(slot+1)) + nameN.Render(" "+name)
			mid = edgeN.Render("│ ") + inner + " " + edgeN.Render("│")
		}
		w := lipgloss.Width(mid)

		reserved := 0
		if slot < len(order)-1 {
			reserved = 7 // keep room for a possible "+N more" suffix
		}
		if used+w+reserved > maxW {
			hidden = len(order) - slot
			break
		}
		pills = append(pills, pill{w: w, mid: mid, active: slot == 0})
		used += w + 1
	}

	var topRow, midRow, botRow strings.Builder
	for i, p := range pills {
		if i > 0 {
			topRow.WriteString(" ")
			midRow.WriteString(" ")
			botRow.WriteString(ruleStyle.Render("─")) // thin rule only in gaps
		}
		contentW := p.w - 2
		edge := edgeN
		if p.active {
			edge = limeEdge
		}
		topRow.WriteString(edge.Render("╭") + edge.Render(strings.Repeat("─", contentW)) + edge.Render("╮"))
		midRow.WriteString(p.mid)
		if p.active {
			// Chrome tab bottom: corners turn outward, interior stays open
			// into the diff below.
			botRow.WriteString(limeEdge.Render("╯") + strings.Repeat(" ", contentW) + limeEdge.Render("╰"))
		} else {
			// Inactive tabs stay short and never merge with the baseline.
			botRow.WriteString(strings.Repeat(" ", p.w))
		}
	}
	if hidden > 0 {
		topRow.WriteString(" ")
		midRow.WriteString(" ")
		midRow.WriteString(over.Render(fmt.Sprintf("+%d more", hidden)))
		botRow.WriteString(ruleStyle.Render("─"))
	}

	return topRow.String() + "\n" + midRow.String() + "\n" + botRow.String()
}

// withMinimap zips a VS Code-style overview ruler onto the right edge of the
// viewport output:
//
//   - the translucent-looking slider (Surface background) covers exactly the
//     rows your viewport occupies — the full track when the file fits on
//     screen, a proportional block otherwise;
//   - green ▄ / red ▀ ticks sit at the positions where additions/deletions
//     actually are in the whole diff, so you can see "more edits below" at a
//     glance.
//
// Mapping is built on the rendered rows (not raw lines) so markers land at
// the same relative height as what you see on screen.
func (d Diff) withMinimap(content string, c *diffTab) string {
	rows := strings.Split(content, "\n")
	mapH := len(rows)
	if mapH == 0 || c.TotalLines == 0 {
		return content
	}

	// renderedRow -> raw diff line index, accounting for the blank row the
	// renderer inserts before each "@@" hunk header.
	raw := strings.Split(c.RawContent, "\n")
	srcOfRow := make([]int, 0, len(raw))
	for i, ln := range raw {
		if strings.HasPrefix(ln, "@@") {
			srcOfRow = append(srcOfRow, -1)
		}
		srcOfRow = append(srcOfRow, i)
	}
	totalRows := len(srcOfRow)
	if totalRows == 0 {
		return content
	}

	// Viewport window in rendered-row space, then mapped into minimap space.
	vStart := d.viewport.YOffset()
	vEnd := vStart + d.viewport.Height() - 1
	if vEnd >= totalRows {
		vEnd = totalRows - 1
	}

	// Scale rendered rows into minimap rows: 1:1 when the whole file fits the
	// screen (ticks align exactly with their lines, like VS Code's overview
	// ruler), proportional compression only when the file is taller.
	scale := 1.0
	if totalRows > mapH {
		scale = float64(mapH) / float64(totalRows)
	}
	mStart := int(float64(vStart) * scale)
	mEnd := int(float64(vEnd) * scale)
	if mStart < 0 {
		mStart = 0
	}
	if mEnd >= mapH {
		mEnd = mapH - 1
	}

	t := d.styles.T
	trackBg := lipgloss.NewStyle().Background(t.Background)
	padStyle := lipgloss.NewStyle().Background(t.Background)
	inViewBg := t.Surface

	var out []string
	for r := 0; r < mapH; r++ {
		var sawAdd, sawDel, hasContent bool
		rs := int(float64(r) / scale)
		re := int(float64(r+1) / scale)
		if rs < totalRows {
			hasContent = true
			if re <= rs {
				re = rs + 1
			}
			if re > totalRows {
				re = totalRows
			}
			for row := rs; row < re; row++ {
				if i := srcOfRow[row]; i >= 0 {
					ln := raw[i]
					switch {
					case strings.HasPrefix(ln, "+") && !strings.HasPrefix(ln, "+++"):
						sawAdd = true
					case strings.HasPrefix(ln, "-") && !strings.HasPrefix(ln, "---"):
						sawDel = true
					}
				}
			}
		}

		inView := hasContent && r >= mStart && r <= mEnd

		// Compose the 2-column cell: change tick on the left, then space.
		tick := " "
		var fg color.Color
		switch {
		case sawAdd && sawDel:
			tick = "█"
			fg = t.Green
		case sawAdd:
			tick = "▄"
			fg = t.Green
		case sawDel:
			tick = "▀"
			fg = t.Red
		}

		var cell string
		if inView {
			st := lipgloss.NewStyle().Background(inViewBg)
			if fg != nil {
				st = st.Foreground(fg)
			}
			cell = st.Render(tick + " ")
		} else if fg != nil {
			cell = lipgloss.NewStyle().Foreground(fg).Background(t.Background).Render(tick + " ")
		} else {
			cell = trackBg.Render("  ")
		}

		line := rows[r]
		if pad := d.viewport.Width() - lipgloss.Width(line); pad > 0 {
			line += padStyle.Render(strings.Repeat(" ", pad))
		}
		out = append(out, line+" "+cell)
	}
	return strings.Join(out, "\n")
}
