package tui

import (
	"os"
	"os/user"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/iSundram/Automergent/internal/config"
)

// projectApprovalResult is the outcome of the pre-TUI project trust gate.
type projectApprovalResult struct {
	approved bool
	remember bool
}

// approvalStep identifies which screen the gate is currently showing.
type approvalStep int

const (
	stepRootRisk approvalStep = iota
	stepTrust
)

// approvalModel renders the pre-TUI workspace trust prompt. It runs inline
// (no alternate screen) so the folder is approved or rejected before the
// full TUI ever starts.
type approvalModel struct {
	cfg         *config.Config
	projectPath string
	step        approvalStep
	selected    int
	result      projectApprovalResult
}

func newApprovalModel(cfg *config.Config, projectPath string) approvalModel {
	step := stepTrust
	if isElevatedUser() && !cfg.Security.RootRiskAcknowledged {
		step = stepRootRisk
	}
	return approvalModel{
		cfg:         cfg,
		projectPath: projectPath,
		step:        step,
	}
}

func (m approvalModel) Init() tea.Cmd { return nil }

func (m approvalModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch km.String() {
	case "up", "k", "shift+tab":
		if m.selected > 0 {
			m.selected--
		}
	case "down", "j", "tab":
		limit := 2
		if m.step == stepRootRisk {
			limit = 1
		}
		if m.selected < limit {
			m.selected++
		}
	case "enter", "y", "Y":
		return m.confirm()
	case "esc", "q", "Q":
		m.result = projectApprovalResult{approved: false, remember: false}
		return m, tea.Quit
	}
	return m, nil
}

// confirm commits the currently selected option and returns a quit command.
func (m approvalModel) confirm() (tea.Model, tea.Cmd) {
	switch m.step {
	case stepRootRisk:
		if m.selected == 0 {
			// First-time root/admin acknowledgment; persist so the extra
			// warning is not shown on every run.
			m.cfg.Security.RootRiskAcknowledged = true
			_ = m.cfg.SaveIfLoaded()
			m.step = stepTrust
			m.selected = 0
			return m, nil
		}
		m.result = projectApprovalResult{approved: false, remember: false}
		return m, tea.Quit
	case stepTrust:
		switch m.selected {
		case 0:
			m.result = projectApprovalResult{approved: true, remember: false}
		case 1:
			m.result = projectApprovalResult{approved: true, remember: true}
		default:
			m.result = projectApprovalResult{approved: false, remember: false}
		}
		return m, tea.Quit
	}
	return m, nil
}

func (m approvalModel) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = false
	return v
}

func (m approvalModel) render() string {
	if m.step == stepRootRisk {
		return m.renderRootRisk()
	}
	return m.renderTrust()
}

func (m approvalModel) renderRootRisk() string {
	title := lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true).
		Render("⚠  You are running with root/admin privileges")
	body := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).
		Render("Automergent will be able to read, edit, and execute files in this\n" +
			"workspace with elevated privileges. This warning is shown once.")
	options := m.renderOptions([]string{"Yes, I accept the risk", "No, exit"})
	return strings.Join([]string{title, "", body, "", options, "", m.hint()}, "\n")
}

func (m approvalModel) renderTrust() string {
	access := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Bold(true).
		Render("Accessing workspace:")
	path := lipgloss.NewStyle().Foreground(lipgloss.Color("153")).Bold(true).
		Render(m.projectPath)
	question := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Bold(true).
		Render("Do you trust the contents of this project?")
	explain := lipgloss.NewStyle().Foreground(lipgloss.Color("245")).
		Render("Automergent requires permission to read, edit, and execute files here.")
	options := m.renderOptions([]string{
		"Yes, I trust this folder",
		"Yes, and remember this folder",
		"No, exit",
	})
	return strings.Join([]string{access, path, "", question, explain, "", options, "", m.hint()}, "\n")
}

func (m approvalModel) renderOptions(options []string) string {
	lines := make([]string, 0, len(options))
	for i, opt := range options {
		if i == m.selected {
			cursor := lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true).Render("›")
			text := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Bold(true).Render(opt)
			lines = append(lines, "  "+cursor+" "+text)
		} else {
			text := lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(opt)
			lines = append(lines, "  "+text)
		}
	}
	return strings.Join(lines, "\n")
}

func (m approvalModel) hint() string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("↑/↓ navigate · enter confirm · esc cancel")
}

// isElevatedUser reports whether the current user runs with root or
// administrative privileges.
func isElevatedUser() bool {
	if os.Geteuid() == 0 {
		return true
	}
	u, err := user.Current()
	if err != nil {
		return false
	}
	groups, err := u.GroupIds()
	if err != nil {
		return false
	}
	for _, gid := range groups {
		g, err := user.LookupGroupId(gid)
		if err != nil {
			continue
		}
		switch g.Name {
		case "admin", "wheel", "sudo":
			return true
		}
	}
	return false
}

// RunProjectApproval presents the workspace trust gate before the TUI starts.
// It returns whether the folder was approved and whether the choice should be
// remembered. A rejected folder yields approved=false.
func RunProjectApproval(cfg *config.Config, projectPath string) (approved, remember bool, err error) {
	m := newApprovalModel(cfg, projectPath)
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return false, false, err
	}
	if fm, ok := final.(approvalModel); ok {
		return fm.result.approved, fm.result.remember, nil
	}
	return false, false, nil
}
