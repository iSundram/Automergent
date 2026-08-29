package components

import (
	"fmt"
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
	var items []ModelItem
	seen := make(map[string]bool)
	for _, md := range live {
		if seen[md.ID] {
			continue
		}
		seen[md.ID] = true
		items = append(items, ModelItem{
			ID:           md.ID,
			Name:         md.Name,
			Provider:     m.host.Provider(),
			ContextLimit: md.ContextLimit,
			InputPrice:   md.InputPrice,
			OutputPrice:  md.OutputPrice,
			Current:      md.ID == m.host.Model(),
			Coding:       true,
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
	const overhead = 8
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
	efforts := []string{"minimal", "low", "medium", "high", "max"}
	current := items[m.cursor].Effort
	idx := 0
	for i, e := range efforts {
		if e == current {
			idx = i
			break
		}
	}
	m.effortCursor = (idx + 1) % len(efforts)
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

	footer := lipgloss.NewStyle().Foreground(m.styles.T.Muted).PaddingLeft(2).
		MaxWidth(max(1, w-2)).Render("↑↓: navigate  •  enter: select  •  tab: cycle effort  •  a: toggle all/coding  •  esc: close")

	return lipgloss.JoinVertical(lipgloss.Left, rule, header, "", strings.Join(rows, "\n"), "", footer)
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
	if item.Effort != "" {
		badges = append(badges, lipgloss.NewStyle().Foreground(m.styles.T.Muted).Render("⚡ "+item.Effort))
	}
	badgeStr := ""
	if len(badges) > 0 {
		badgeStr = " " + strings.Join(badges, " ")
	}
	desc := fmt.Sprintf("Limit: %s", formatContextLimit(item.ContextLimit))
	nameSt := lipgloss.NewStyle().Foreground(m.styles.T.Text)
	if selected {
		nameSt = nameSt.Bold(true)
	}
	line := fmt.Sprintf("%s%s %s%s  %s", ind, icon, nameSt.Render(item.Name), badgeStr, lipgloss.NewStyle().Foreground(m.styles.T.Muted).Render(desc))
	return lipgloss.NewStyle().Width(m.width - 2).MaxWidth(m.width - 2).Render(line)
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
