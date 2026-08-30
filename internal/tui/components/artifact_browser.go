package components

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

// ArtifactStatus is the review state of one artifact.
type ArtifactStatus int

const (
	ArtifactPending ArtifactStatus = iota
	ArtifactApproved
	ArtifactRejected
)

// ArtifactRow is one artifact in the review list. The app owns the source of
// truth and rebuilds rows on every change; the browser is a pure view over
// them plus cursor/scroll state.
type ArtifactRow struct {
	Name      string
	Path      string
	Kind      string
	Status    ArtifactStatus
	UpdatedAt time.Time
	SizeBytes int64
	Lines     int
}

// ArtifactDecisionMsg reports an approve/reject choice for one artifact.
type ArtifactDecisionMsg struct {
	Path     string
	Approved bool
}

// ArtifactApproveAllMsg approves every pending artifact at once.
type ArtifactApproveAllMsg struct{}

// ArtifactOpenMsg asks the app to open the artifact in the user's editor.
type ArtifactOpenMsg struct {
	Path string
}

// ArtifactBrowser is the /artifact review surface, in the same line-based
// grammar as the session browser: the title sits between rule segments, rows
// are status glyph + name + metadata, and a preview mode shows the artifact
// with line numbers. Approvals are decisions on files the agent produced —
// the app applies them and refreshes the rows.
type ArtifactBrowser struct {
	styles *themes.Styles

	width  int
	height int
	visible bool

	rows          []ArtifactRow
	cursor        int
	scrollOffset  int

	// preview mode
	previewing bool
	preview    []string // rendered file lines
	previewPath string
	previewScroll int

	// search inside the preview
	searching bool
	query     string

	// previewSource holds file contents keyed by path; the app reads the
	// files and pushes content in, keeping the component free of I/O.
	previewSource map[string][]string
}

// NewArtifactBrowser creates a new ArtifactBrowser.
func NewArtifactBrowser(styles *themes.Styles) ArtifactBrowser {
	return ArtifactBrowser{styles: styles}
}

// SetArtifacts replaces the rows and re-reads the preview if it is open.
func (ab *ArtifactBrowser) SetArtifacts(rows []ArtifactRow) {
	ab.rows = rows
	if ab.cursor >= len(ab.rows) {
		ab.cursor = max(0, len(ab.rows)-1)
	}
	if ab.previewing {
		// The previewed artifact may have been rewritten; re-read it so the
		// preview always shows the current content.
		if content, ok := ab.previewSource[ab.previewPath]; ok {
			ab.preview = content
			ab.clampPreviewScroll()
		}
	}
	ab.updateScroll()
}

// SetPreview loads the preview content for a path.
func (ab *ArtifactBrowser) SetPreview(path string, lines []string) {
	if ab.previewSource == nil {
		ab.previewSource = map[string][]string{}
	}
	ab.previewSource[path] = lines
	if ab.previewing && ab.previewPath == path {
		ab.preview = lines
		ab.clampPreviewScroll()
	}
}

// Show displays the browser.
func (ab *ArtifactBrowser) Show() { ab.visible = true }

// Hide hides the browser and resets transient state.
func (ab *ArtifactBrowser) Hide() {
	ab.visible = false
	ab.previewing = false
	ab.searching = false
	ab.query = ""
}

// Visible reports visibility.
func (ab ArtifactBrowser) Visible() bool { return ab.visible }

// ItemCount reports the number of artifact rows.
func (ab ArtifactBrowser) ItemCount() int { return len(ab.rows) }

// PendingCount reports how many artifacts still need a decision.
func (ab ArtifactBrowser) PendingCount() int {
	n := 0
	for _, r := range ab.rows {
		if r.Status == ArtifactPending {
			n++
		}
	}
	return n
}

// SetSize updates dimensions.
func (ab *ArtifactBrowser) SetSize(w, h int) { ab.width, ab.height = w, h }

// Previewing reports whether the preview pane is open.
func (ab ArtifactBrowser) Previewing() bool { return ab.previewing }

func (ab *ArtifactBrowser) updateScroll() {
	maxItems := ab.MaxVisibleItems()
	if ab.cursor < ab.scrollOffset {
		ab.scrollOffset = ab.cursor
	}
	if ab.cursor >= ab.scrollOffset+maxItems {
		ab.scrollOffset = ab.cursor - maxItems + 1
	}
}

// maxPreviewLines reports how many preview lines fit.
func (ab ArtifactBrowser) maxPreviewLines() int {
	maxLines := 12
	if ab.height > 0 {
		maxLines = max(1, ab.height-10)
	}
	return maxLines
}

func (ab *ArtifactBrowser) clampPreviewScroll() {
	if ab.previewScroll < 0 {
		ab.previewScroll = 0
	}
	if max := len(ab.preview) - ab.maxPreviewLines(); ab.previewScroll > max {
		ab.previewScroll = max
	}
	if ab.previewScroll < 0 {
		ab.previewScroll = 0
	}
}

// Update handles events. While visible the browser owns the keyboard.
func (ab ArtifactBrowser) Update(msg tea.Msg) (ArtifactBrowser, tea.Cmd) {
	if !ab.visible {
		return ab, nil
	}
	if m, ok := msg.(tea.KeyMsg); ok {
		key := m.String()

		// Search-typing inside preview mode.
		if ab.searching {
			switch key {
			case "esc":
				ab.searching = false
				ab.query = ""
				return ab, nil
			case "enter":
				ab.searching = false
				ab.jumpToMatch()
				return ab, nil
			case "backspace":
				if ab.query != "" {
					r := []rune(ab.query)
					ab.query = string(r[:len(r)-1])
				}
				return ab, nil
			}
			if text := keyText(m); text != "" {
				ab.query += text
				return ab, nil
			}
			return ab, nil
		}

		if ab.previewing {
			switch key {
			case "esc":
				ab.previewing = false
				ab.previewScroll = 0
				return ab, nil
			case "up", "ctrl+p":
				ab.previewScroll = max(0, ab.previewScroll-1)
			case "down", "ctrl+n":
				ab.previewScroll++
				ab.clampPreviewScroll()
			case "pgup":
				ab.previewScroll = max(0, ab.previewScroll-ab.maxPreviewLines())
			case "pgdown":
				ab.previewScroll += ab.maxPreviewLines()
				ab.clampPreviewScroll()
			case "g":
				ab.previewScroll = 0
			case "G", "shift+g":
				ab.previewScroll = max(0, len(ab.preview)-ab.maxPreviewLines())
			case "/":
				ab.searching = true
				ab.query = ""
				return ab, nil
			case "y":
				return ab.decideOn(ab.previewPath, true)
			case "n":
				return ab.decideOn(ab.previewPath, false)
			case "A", "shift+a":
				return ab, func() tea.Msg { return ArtifactApproveAllMsg{} }
			case "ctrl+g":
				path := ab.previewPath
				return ab, func() tea.Msg { return ArtifactOpenMsg{Path: path} }
			}
			return ab, nil
		}

		switch key {
		case "esc", "q":
			ab.visible = false
			return ab, nil
		case "up", "ctrl+p":
			if len(ab.rows) > 0 {
				ab.cursor = (ab.cursor - 1 + len(ab.rows)) % len(ab.rows)
			}
		case "down", "ctrl+n", "tab":
			if len(ab.rows) > 0 {
				ab.cursor = (ab.cursor + 1) % len(ab.rows)
			}
		case "pgup":
			ab.cursor = max(0, ab.cursor-ab.MaxVisibleItems())
		case "pgdown":
			ab.cursor = min(len(ab.rows)-1, ab.cursor+ab.MaxVisibleItems())
		case "y":
			return ab.decide(true)
		case "n":
			return ab.decide(false)
		case "A", "shift+a":
			return ab, func() tea.Msg { return ArtifactApproveAllMsg{} }
		case "p", "enter":
			if row, ok := ab.selected(); ok {
				ab.previewing = true
				ab.previewPath = row.Path
				ab.previewScroll = 0
				if content, ok := ab.previewSource[row.Path]; ok {
					ab.preview = content
				} else {
					ab.preview = nil
				}
				ab.clampPreviewScroll()
			}
		case "ctrl+g":
			if row, ok := ab.selected(); ok {
				path := row.Path
				return ab, func() tea.Msg { return ArtifactOpenMsg{Path: path} }
			}
		}
		ab.updateScroll()
	}
	return ab, nil
}

// decide emits an approve/reject decision for the selected row and advances
// the cursor to the next pending artifact so y/n flows through the queue.
func (ab ArtifactBrowser) decide(approved bool) (ArtifactBrowser, tea.Cmd) {
	row, ok := ab.selected()
	if !ok {
		return ab, nil
	}
	return ab.decideOn(row.Path, approved)
}

// decideOn emits a decision for a specific path (used from list and preview
// modes alike).
func (ab ArtifactBrowser) decideOn(path string, approved bool) (ArtifactBrowser, tea.Cmd) {
	// Advance to the next still-pending row (wrapping, skipping the row being
	// decided — its status is about to change).
	if len(ab.rows) > 1 {
		for step := 1; step < len(ab.rows); step++ {
			next := (ab.cursor + step) % len(ab.rows)
			if ab.rows[next].Status == ArtifactPending {
				ab.cursor = next
				break
			}
		}
	}
	ab.updateScroll()
	return ab, func() tea.Msg { return ArtifactDecisionMsg{Path: path, Approved: approved} }
}

func (ab ArtifactBrowser) selected() (ArtifactRow, bool) {
	if len(ab.rows) == 0 || ab.cursor < 0 || ab.cursor >= len(ab.rows) {
		return ArtifactRow{}, false
	}
	return ab.rows[ab.cursor], true
}

// jumpToMatch scrolls the preview to the first line matching the query.
func (ab *ArtifactBrowser) jumpToMatch() {
	if ab.query == "" {
		return
	}
	q := strings.ToLower(ab.query)
	for i, line := range ab.preview {
		if strings.Contains(strings.ToLower(line), q) {
			ab.previewScroll = max(0, i-2)
			ab.clampPreviewScroll()
			return
		}
	}
}

// View renders the browser.
func (ab ArtifactBrowser) View() string {
	if !ab.visible || ab.width <= 0 || ab.height <= 0 {
		return ""
	}
	w := max(12, ab.width)
	ruleStyle := lipgloss.NewStyle().Foreground(ab.styles.T.BorderNormal)

	if ab.previewing {
		return ab.viewPreview(w, ruleStyle)
	}

	titleText := "  Artifacts  "
	title := lipgloss.NewStyle().Foreground(ab.styles.T.Accent).Bold(true).Render(titleText)
	ruleWidth := max(0, (w-len(titleText))/2)
	header := ruleStyle.Render(strings.Repeat("─", ruleWidth)) + title +
		ruleStyle.Render(strings.Repeat("─", max(0, w-ruleWidth-len(titleText))))

	pending := ab.PendingCount()
	var statusLine string
	if len(ab.rows) == 0 {
		statusLine = " No artifacts yet — ask the agent for a plan or document"
	} else if pending > 0 {
		statusLine = fmt.Sprintf(" Action required (%d left)", pending)
	} else {
		statusLine = " All artifacts reviewed"
	}
	count := fmt.Sprintf("%d/%d  ", min(ab.cursor+1, len(ab.rows)), len(ab.rows))
	top := joinEnds(
		lipgloss.NewStyle().Foreground(ab.styles.T.Text).Bold(true).Render(statusLine),
		lipgloss.NewStyle().Foreground(ab.styles.T.Muted).Render(count), w)

	lines := []string{header, top, ""}
	maxItems := ab.MaxVisibleItems()
	end := min(ab.scrollOffset+maxItems, len(ab.rows))
	for i := ab.scrollOffset; i < end; i++ {
		lines = append(lines, ab.renderRow(i, i == ab.cursor, w-2))
	}
	if len(ab.rows) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(ab.styles.T.Muted).Italic(true).PaddingLeft(4).Width(w-2).
			Render("Artifacts appear when the agent writes a plan, review or document"))
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().Foreground(ab.styles.T.Muted).Italic(true).PaddingLeft(4).Width(w-2).
			Render("Ask for it by name: “write the plan to .automergent/artifacts/plan.md”"))
	}
	lines = append(lines, "", ruleStyle.Render(strings.Repeat("─", w)))
	lines = append(lines, ab.footerLines(w)...)
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (ab ArtifactBrowser) renderRow(i int, selected bool, width int) string {
	row := ab.rows[i]
	glyph, color := "□", ab.styles.T.Muted
	switch row.Status {
	case ArtifactApproved:
		glyph, color = "✓", ab.styles.T.Green
	case ArtifactRejected:
		glyph, color = "✗", ab.styles.T.Red
	}
	status := lipgloss.NewStyle().Foreground(color).Render(glyph)

	nameStyle := lipgloss.NewStyle().Foreground(ab.styles.T.Text)
	if selected {
		nameStyle = lipgloss.NewStyle().Foreground(ab.styles.T.Subtext).Bold(true)
	}
	indicator := "  "
	if selected {
		indicator = lipgloss.NewStyle().Foreground(ab.styles.T.Accent).Render("▸ ")
	}
	left := "  " + indicator + status + " " + nameStyle.Render(row.Name)
	meta := []string{row.Kind}
	if !row.UpdatedAt.IsZero() {
		meta = append(meta, formatRelativeTime(row.UpdatedAt))
	}
	if row.Lines > 0 {
		meta = append(meta, fmt.Sprintf("%d lines", row.Lines))
	}
	if row.SizeBytes > 0 {
		meta = append(meta, formatBytes(row.SizeBytes))
	}
	metaLine := "      " + lipgloss.NewStyle().Foreground(ab.styles.T.Muted).Render(
		truncateCells(strings.Join(meta, " · "), max(1, width-8)))
	line1 := lipgloss.NewStyle().Width(width).MaxWidth(width).Render(left)
	line2 := lipgloss.NewStyle().Width(width).MaxWidth(width).Render(metaLine)
	return lipgloss.JoinVertical(lipgloss.Left, line1, line2)
}

// viewPreview renders the file preview with line numbers.
func (ab ArtifactBrowser) viewPreview(w int, ruleStyle lipgloss.Style) string {
	titleText := "  " + ab.previewName() + "  "
	title := lipgloss.NewStyle().Foreground(ab.styles.T.Accent).Bold(true).Render(titleText)
	ruleWidth := max(0, (w-len(titleText))/2)
	header := ruleStyle.Render(strings.Repeat("─", ruleWidth)) + title +
		ruleStyle.Render(strings.Repeat("─", max(0, w-ruleWidth-len(titleText))))

	// Search line, mirrored from the session browser's filter line.
	searchText := " /" + ab.query
	if !ab.searching {
		searchText = ""
	}
	pos := fmt.Sprintf("%d-%d/%d", ab.previewScroll+1,
		min(ab.previewScroll+ab.maxPreviewLines(), len(ab.preview)), len(ab.preview))
	top := joinEnds(
		lipgloss.NewStyle().Foreground(ab.styles.T.Muted).Render(searchText),
		lipgloss.NewStyle().Foreground(ab.styles.T.Muted).Render(pos+"  "), w)

	lines := []string{header, top, ""}
	maxLines := ab.maxPreviewLines()
	end := min(ab.previewScroll+maxLines, len(ab.preview))
	for i := ab.previewScroll; i < end; i++ {
		num := lipgloss.NewStyle().Foreground(ab.styles.T.Muted).Render(
			fmt.Sprintf("%5d  ", i+1))
		body := ab.preview[i]
		if ab.query != "" && strings.Contains(strings.ToLower(body), strings.ToLower(ab.query)) {
			body = lipgloss.NewStyle().Foreground(ab.styles.T.Accent).Render(body)
		}
		lines = append(lines, lipgloss.NewStyle().MaxWidth(w-2).Render(num+body))
	}
	if len(ab.preview) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(ab.styles.T.Muted).Italic(true).PaddingLeft(4).
			Render("Empty artifact"))
	}
	lines = append(lines, "", ruleStyle.Render(strings.Repeat("─", w)))
	lines = append(lines, ab.footerLines(w)...)
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (ab ArtifactBrowser) previewName() string {
	for _, r := range ab.rows {
		if r.Path == ab.previewPath {
			return r.Name
		}
	}
	return "preview"
}

func (ab ArtifactBrowser) footerLines(w int) []string {
	var l1, l2 string
	if ab.previewing {
		if ab.searching {
			l1 = "type to search · enter jump to match · esc cancel"
		} else {
			l1 = "↑↓ scroll · pgup/pgdn page · g top · shift+g bottom · / search"
		}
		l2 = "y approve · n reject · ctrl+g editor · esc back to list"
	} else {
		l1 = "↑↓ navigate · y approve · n reject · shift+a approve all"
		l2 = "p preview · ctrl+g editor · esc done"
	}
	style := lipgloss.NewStyle().Foreground(ab.styles.T.Muted).PaddingLeft(2).MaxWidth(max(1, w-2))
	return []string{style.Render(l1), style.Render(l2)}
}

// MaxVisibleItems reports how many artifact rows fit (two lines each).
func (ab ArtifactBrowser) MaxVisibleItems() int {
	maxItems := 6
	if ab.height > 0 && ab.height < 18 {
		maxItems = (ab.height - 6) / 2
	}
	return max(1, maxItems)
}

// Height reports the rendered height for layout.
func (ab ArtifactBrowser) Height() int {
	if !ab.visible {
		return 0
	}
	if ab.previewing {
		return ab.maxPreviewLines() + 8
	}
	visible := min(len(ab.rows), ab.MaxVisibleItems())
	if visible < 1 {
		visible = 1
	}
	return visible*2 + 8
}
