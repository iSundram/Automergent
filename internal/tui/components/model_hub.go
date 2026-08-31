package components

import (
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/tui/themes"
)

type ModelHubHost interface {
	ProviderConfig(name string) config.ProviderConfig
	Provider() string
	Model() string
	AvailableModels() []ai.Model
	SetStatus(status string)
	CommandError(message string)
	AddSystemMessage(text string)
	FetchModels() tea.Cmd
	SwitchProvider(provider, model string) error
	PersistProjectConfig() error
	WorkDir() string
}

type ModelHub struct {
	styles       *themes.Styles
	width        int
	height       int
	visible      bool
	cursor       int
	scrollOffset int
	showAll      bool
	effortCursor int
	failureAlert *FailureAlert
	host         ModelHubHost
	modelList    []ModelItem
}

type ModelItem struct {
	ID           string
	Name         string
	Provider     string
	ContextLimit int
	InputPrice   float64
	OutputPrice  float64
	Current      bool
	Coding       bool
	Effort       string
	// Catalog metadata (models.dev): capabilities, effort levels, cutoffs,
	// release date.
	Reasoning   bool
	Attachment  bool
	Efforts     []string
	Knowledge   string
	Released    string
	OutputLimit int
	Custom      bool
}

type FailureAlert struct {
	ModelID       string
	Provider      string
	FailureCount  int
	LastErrorCode string
	Show          bool
	Cursor        int
}

func NewModelHub(styles *themes.Styles) *ModelHub {
	return &ModelHub{styles: styles, effortCursor: 2}
}

func (m *ModelHub) SetHost(host ModelHubHost) { m.host = host }
func (m *ModelHub) SetSize(w, h int)          { m.width, m.height = w, h }
func (m *ModelHub) Show() {
	m.visible = true
	m.cursor = 0
	m.scrollOffset = 0
	m.showAll = false
	m.refreshModelList()
}
func (m *ModelHub) Hide()         { m.visible = false }
func (m *ModelHub) Visible() bool { return m.visible }
func (m *ModelHub) Title() string { return "Model Hub" }

func (m *ModelHub) refreshModelList() {
	if m.host == nil {
		return
	}
	live := m.host.AvailableModels()
	pc := m.host.ProviderConfig(m.host.Provider())
	var items []ModelItem
	seen := make(map[string]bool)
	for _, md := range live {
		if seen[md.ID] {
			continue
		}
		seen[md.ID] = true
		_, custom := pc.Models[md.ID]
		items = append(items, ModelItem{
			ID:           md.ID,
			Name:         md.Name,
			Provider:     m.host.Provider(),
			ContextLimit: md.ContextLimit,
			InputPrice:   md.InputPrice,
			OutputPrice:  md.OutputPrice,
			Current:      md.ID == m.host.Model(),
			Coding:       true,
			Reasoning:    md.Reasoning,
			Attachment:   md.Attachment,
			Efforts:      md.Efforts,
			Knowledge:    md.Knowledge,
			Released:     md.Released,
			OutputLimit:  md.OutputLimit,
			Custom:       custom,
		})
	}
	m.modelList = items
}

func (m *ModelHub) filteredModels() []ModelItem {
	if m.showAll {
		return m.modelList
	}
	var filtered []ModelItem
	for _, item := range m.modelList {
		if item.Coding {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func (m *ModelHub) maxVisibleItems() int {
	// Overhead: rule, header, blank, detail pane (3 lines), footer, padding.
	const overhead = 10
	if m.height > 0 {
		return max(1, m.height-overhead)
	}
	return 10
}

func (m *ModelHub) updateScroll() {
	items := m.filteredModels()
	maxItems := m.maxVisibleItems()
	if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	}
	if m.cursor >= m.scrollOffset+maxItems {
		m.scrollOffset = m.cursor - maxItems + 1
	}
	_ = items
}

func (m *ModelHub) Update(msg tea.Msg) (*ModelHub, tea.Cmd) {
	if !m.visible {
		return m, nil
	}
	if m.failureAlert != nil && m.failureAlert.Show {
		if k, ok := msg.(tea.KeyMsg); ok {
			switch k.String() {
			case "esc":
				m.failureAlert.Show = false
				m.failureAlert = nil
				return m, nil
			case "left":
				m.failureAlert.Cursor = max(m.failureAlert.Cursor-1, 0)
				return m, nil
			case "right":
				m.failureAlert.Cursor = min(m.failureAlert.Cursor+1, 3)
				return m, nil
			case "enter":
				m.handleFailureAction()
				return m, nil
			}
		}
		return m, nil
	}
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc":
			m.Hide()
			return m, nil
		case "a":
			m.showAll = !m.showAll
			m.cursor = 0
			m.scrollOffset = 0
			return m, nil
		case "i":
			// The detail pane is always visible under the list; "i" prints
			// the same information as a system message so it can be copied
			// and survives closing the hub.
			if items := m.filteredModels(); m.cursor >= 0 && m.cursor < len(items) && m.host != nil {
				item := items[m.cursor]
				var b strings.Builder
				fmt.Fprintf(&b, "%s (%s) — provider %s", item.Name, item.ID, item.Provider)
				if item.ContextLimit > 0 {
					fmt.Fprintf(&b, "\nContext limit: %s", formatContextLimit(item.ContextLimit))
				}
				if item.OutputLimit > 0 {
					fmt.Fprintf(&b, "\nMax output: %s", formatContextLimit(item.OutputLimit))
				}
				if item.InputPrice > 0 || item.OutputPrice > 0 {
					fmt.Fprintf(&b, "\nPrice: $%.4g input / $%.4g output per 1M tokens", item.InputPrice, item.OutputPrice)
				}
				if item.Reasoning {
					if len(item.Efforts) > 0 {
						fmt.Fprintf(&b, "\nReasoning: yes — effort levels: %s", strings.Join(item.Efforts, " · "))
					} else {
						b.WriteString("\nReasoning: yes (no effort control listed)")
					}
				}
				if item.Attachment {
					b.WriteString("\nAttachments: images and files supported")
				}
				if item.Knowledge != "" {
					fmt.Fprintf(&b, "\nKnowledge cutoff: %s", item.Knowledge)
				}
				if item.Released != "" {
					fmt.Fprintf(&b, "\nReleased: %s", item.Released)
				}
				if item.Custom {
					b.WriteString("\nSource: custom (user-registered)")
				} else {
					b.WriteString("\nSource: models.dev catalog")
				}
				m.host.AddSystemMessage(b.String())
			}
			return m, nil
		case "up", "ctrl+p":
			if m.cursor > 0 {
				m.cursor--
			}
			m.updateScroll()
		case "down", "ctrl+n":
			if m.cursor < len(m.filteredModels())-1 {
				m.cursor++
			}
			m.updateScroll()
		case "pgup":
			m.cursor = max(0, m.cursor-10)
			m.updateScroll()
		case "pgdown":
			m.cursor = min(len(m.filteredModels())-1, m.cursor+10)
			m.updateScroll()
		case "enter":
			m.selectModel()
		case "tab":
			m.cycleEffort()
		}
	}
	return m, nil
}

func (m *ModelHub) selectModel() {
	items := m.filteredModels()
	if m.cursor >= 0 && m.cursor < len(items) && m.host != nil {
		item := items[m.cursor]
		if err := m.host.SwitchProvider(item.Provider, item.ID); err != nil {
			m.host.CommandError(err.Error())
		} else {
			m.host.SetStatus(fmt.Sprintf("Switched to %s/%s", item.Provider, item.ID))
		}
	}
	m.Hide()
}

func (m *ModelHub) cycleEffort() {
	items := m.filteredModels()
	if m.cursor < 0 || m.cursor >= len(items) {
		return
	}
	item := items[m.cursor]
	// Cycle through the levels this model actually accepts (models.dev
	// catalog); fall back to a generic ladder when the model has none.
	efforts := item.Efforts
	if len(efforts) == 0 {
		efforts = []string{"minimal", "low", "medium", "high", "max"}
	}
	current := item.Effort
	idx := 0
	for i, e := range efforts {
		if e == current {
			idx = i
			break
		}
	}
	m.effortCursor = (idx + 1) % len(efforts)
	if m.host != nil {
		m.host.SetStatus(fmt.Sprintf("Effort: %s (supported: %s)", efforts[m.effortCursor], strings.Join(efforts, " · ")))
	}
}

func (m *ModelHub) ShowFailureAlert(modelID, provider string, count int, lastErrorCode string) {
	m.failureAlert = &FailureAlert{
		ModelID:       modelID,
		Provider:      provider,
		FailureCount:  count,
		LastErrorCode: lastErrorCode,
		Show:          true,
	}
}

func (m *ModelHub) handleFailureAction() {
	if m.failureAlert == nil {
		return
	}
	switch m.failureAlert.Cursor {
	case 0:
		m.host.SetStatus("Attempting fallback provider...")
	case 1:
		m.host.SetStatus("Continuing with current model")
	case 2:
		m.host.SetStatus("Showing error details...")
	case 3:
		m.host.SetStatus("Downgrading effort for " + m.failureAlert.ModelID)
	}
	m.failureAlert.Show = false
	m.failureAlert = nil
}

func (m *ModelHub) View() string {
	if !m.visible || m.width <= 0 || m.height <= 0 {
		return ""
	}
	if m.failureAlert != nil && m.failureAlert.Show {
		return m.renderFailureAlert()
	}
	return m.renderMainView()
}

func (m *ModelHub) renderMainView() string {
	w := m.width
	rule := lipgloss.NewStyle().Foreground(m.styles.T.BorderNormal).Render(strings.Repeat("─", w))

	title := "Model Hub"
	if !m.showAll {
		title += " (Coding Only)"
	}
	n := len(m.filteredModels())
	count := fmt.Sprintf("%d/%d", min(m.cursor+1, n), n)
	header := lipgloss.JoinHorizontal(lipgloss.Left,
		lipgloss.NewStyle().Foreground(m.styles.T.Accent).Bold(true).Render("  ●  "),
		lipgloss.NewStyle().Foreground(m.styles.T.Text).Bold(true).Render(title),
		lipgloss.NewStyle().Foreground(m.styles.T.Muted).Render(count),
	)

	items := m.filteredModels()
	maxItems := m.maxVisibleItems()
	end := min(m.scrollOffset+maxItems, len(items))
	var rows []string
	for i := m.scrollOffset; i < end; i++ {
		rows = append(rows, m.renderModelItem(items[i], i == m.cursor))
	}
	if len(rows) == 0 {
		rows = append(rows, lipgloss.NewStyle().Foreground(m.styles.T.Muted).Italic(true).PaddingLeft(4).Render("No models available"))
	}

	var sections []string
	sections = append(sections, strings.Join(rows, "\n"))
	if detail := m.renderDetail(items, m.cursor); detail != "" {
		sections = append(sections, "", detail)
	}

	footer := lipgloss.NewStyle().Foreground(m.styles.T.Muted).PaddingLeft(2).
		MaxWidth(max(1, w-2)).Render("↑↓: navigate  •  enter: select  •  tab: cycle effort  •  a: toggle all/coding  •  i: model info  •  esc: close")

	return lipgloss.JoinVertical(lipgloss.Left, rule, header, "", strings.Join(sections, "\n"), "", footer)
}

// renderDetail renders an "about" block for the model under the cursor,
// fed by the models.dev catalog metadata carried on ModelItem.
func (m *ModelHub) renderDetail(items []ModelItem, cursor int) string {
	if cursor < 0 || cursor >= len(items) {
		return ""
	}
	item := items[cursor]

	var lines []string
	title := lipgloss.NewStyle().Foreground(m.styles.T.Accent).Bold(true).Render(
		fmt.Sprintf("%s — %s", item.Name, item.ID))
	lines = append(lines, title)

	var facts []string
	facts = append(facts, fmt.Sprintf("context %s", formatContextLimit(item.ContextLimit)))
	if item.OutputLimit > 0 {
		facts = append(facts, fmt.Sprintf("output %s", formatContextLimit(item.OutputLimit)))
	}
	if item.InputPrice > 0 || item.OutputPrice > 0 {
		facts = append(facts, fmt.Sprintf("$%.4g/$%.4g per 1M", item.InputPrice, item.OutputPrice))
	}
	if item.Reasoning {
		if len(item.Efforts) > 0 {
			// Full level names in the detail pane, colored by the same
			// breadth rule as the list's ladder.
			facts = append(facts, lipgloss.NewStyle().
				Foreground(m.effortLadderColor(len(item.Efforts))).
				Render("reasoning ("+strings.Join(item.Efforts, "/")+")"))
		} else {
			facts = append(facts, "reasoning")
		}
	}
	if item.Attachment {
		facts = append(facts, "attachments")
	}
	if item.Knowledge != "" {
		facts = append(facts, "cutoff "+item.Knowledge)
	}
	if item.Released != "" {
		facts = append(facts, "released "+item.Released)
	}
	if item.Custom {
		facts = append(facts, "custom")
	}
	// The effort fact is already colored; wrap only the plain facts in the
	// muted style so the ladder keeps its color, then join them.
	muted := lipgloss.NewStyle().Foreground(m.styles.T.Muted)
	for i, f := range facts {
		if !strings.HasPrefix(f, "\x1b[") { // unstyled entries only
			facts[i] = muted.Render(f)
		}
	}
	lines = append(lines, strings.Join(facts, " · "))

	return lipgloss.NewStyle().PaddingLeft(2).MaxWidth(max(1, m.width-2)).Render(
		lipgloss.JoinVertical(lipgloss.Left, lines...))
}

func (m *ModelHub) renderFailureAlert() string {
	alert := m.failureAlert
	actions := []struct {
		label string
		hot   rune
	}{
		{"Fallback", 'y'},
		{"Continue", 'n'},
		{"Show Errors", 'e'},
		{"Effort ↓", 'd'},
	}
	var parts []string
	for i, a := range actions {
		st := lipgloss.NewStyle().Foreground(m.styles.T.Muted).Padding(0, 2)
		if i == alert.Cursor {
			st = lipgloss.NewStyle().Foreground(m.styles.T.Accent).Bold(true).Padding(0, 2)
		}
		parts = append(parts, st.Render(fmt.Sprintf("%c) %s", a.hot, a.label)))
	}
	title := lipgloss.NewStyle().Foreground(m.styles.T.Accent).Bold(true).Render(
		fmt.Sprintf("⚠ %s/%s failed %d times (%s)", alert.Provider, alert.ModelID, alert.FailureCount, alert.LastErrorCode))
	body := lipgloss.NewStyle().Foreground(m.styles.T.Text).Render(
		fmt.Sprintf("This model has failed %d times. What would you like to do?", alert.FailureCount))
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.styles.T.Accent).
		Padding(1, 2).
		Width(m.width - 4).
		Render(lipgloss.JoinVertical(lipgloss.Left, title, "", body, "", lipgloss.JoinHorizontal(lipgloss.Left, parts...)))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m *ModelHub) renderModelItem(item ModelItem, selected bool) string {
	ind := "  "
	if selected {
		ind = lipgloss.NewStyle().Foreground(m.styles.T.Accent).Render("▍ ")
	}
	icon := "?"
	if len(item.Provider) > 0 {
		icon = item.Provider[:1]
	}
	var badges []string
	if item.Current {
		badges = append(badges, lipgloss.NewStyle().Foreground(m.styles.T.Green).Render("● Current"))
	}
	if item.Coding {
		badges = append(badges, lipgloss.NewStyle().Foreground(m.styles.T.Blue).Render("⚡ Coding"))
	}
	if item.Reasoning {
		badges = append(badges, lipgloss.NewStyle().Foreground(m.styles.T.Blue).Render("✿ Reasoning"))
	}
	if item.Custom {
		badges = append(badges, lipgloss.NewStyle().Foreground(m.styles.T.Muted).Render("⚙ Custom"))
	}
	if item.Effort != "" {
		badges = append(badges, lipgloss.NewStyle().Foreground(m.styles.T.Muted).Render("⚡ "+item.Effort))
	}
	badgeStr := ""
	if len(badges) > 0 {
		badgeStr = " " + strings.Join(badges, " ")
	}
	desc := fmt.Sprintf("Limit: %s", formatContextLimit(item.ContextLimit))
	if item.InputPrice > 0 || item.OutputPrice > 0 {
		desc += fmt.Sprintf("  $%.4g/$%.4g", item.InputPrice, item.OutputPrice)
	}
	nameSt := lipgloss.NewStyle().Foreground(m.styles.T.Text)
	if selected {
		nameSt = nameSt.Bold(true)
	}
	// The effort ladder renders directly after the name: the short codes
	// of every level this model accepts (e.g. "lo·md·hi"), colored by the
	// breadth of the ladder — 2 levels is a narrow choice (yellow), 3 is
	// the typical ladder (green), 4+ is fine-grained control (orange).
	ladder := ""
	if len(item.Efforts) > 0 {
		ladder = " " + lipgloss.NewStyle().Foreground(m.effortLadderColor(len(item.Efforts))).
			Render("["+strings.Join(effortCodes(item.Efforts), "·")+"]")
	}
	line := fmt.Sprintf("%s%s %s%s%s  %s", ind, icon, nameSt.Render(item.Name), ladder, badgeStr, lipgloss.NewStyle().Foreground(m.styles.T.Muted).Render(desc))
	return lipgloss.NewStyle().Width(m.width - 2).MaxWidth(m.width - 2).Render(line)
}

// effortCodes shortens each effort level to a two-letter code so even a
// five-level ladder fits on one row: minimal→mi, low→lo, medium→md,
// high→hi, max→mx. Unknown levels keep their first two letters.
func effortCodes(efforts []string) []string {
	codes := make([]string, len(efforts))
	for i, e := range efforts {
		switch e {
		case "minimal":
			codes[i] = "mi"
		case "low":
			codes[i] = "lo"
		case "medium":
			codes[i] = "md"
		case "high":
			codes[i] = "hi"
		case "max":
			codes[i] = "mx"
		default:
			if len(e) >= 2 {
				codes[i] = strings.ToLower(e[:2])
			} else {
				codes[i] = strings.ToLower(e)
			}
		}
	}
	return codes
}

// effortLadderColor keys the ladder's color to its breadth: 2 levels
// (yellow) is a limited choice, 3 (green) the typical ladder, 4+ (orange)
// fine-grained control.
func (m *ModelHub) effortLadderColor(levels int) color.Color {
	switch {
	case levels >= 4:
		return m.styles.T.Orange
	case levels == 3:
		return m.styles.T.Green
	default:
		return m.styles.T.Yellow
	}
}

func formatContextLimit(n int) string {
	if n <= 0 {
		return "—"
	}
	if n >= 1024*1024 {
		return fmt.Sprintf("%.1fM", float64(n)/(1024*1024))
	}
	if n >= 1024 {
		return fmt.Sprintf("%.0fK", float64(n)/1024)
	}
	return fmt.Sprintf("%d", n)
}
