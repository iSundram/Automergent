package installer

import (
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/iSundram/Automergent/internal/tui/themes"
)

type state int

const (
	stateDetecting state = iota
	stateDownloading
	stateExtracting
	stateFinishing
	stateDone
	stateError
)

type animTickMsg time.Time

type Model struct {
	state          state
	err            error
	targetProgress float64
	progress       float64
	version        string
	info           *Info
	spinner        spinner.Model
	progressBar    progress.Model
	styles         *themes.Styles
	theme          *themes.Theme
	width          int
	height         int
	status         string
	archive        string
	startTime      time.Time
	lastTick       time.Time
	progressChan   chan float64
	listenProgress func() tea.Cmd
	doneTime       time.Time
	InstallerPath  string

	// Animation states
	animPulse  float64
	entryPhase float64
	shake      float64
}

func NewModel() Model {
	theme := themes.Modern()
	styles := themes.NewStyles(theme)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(theme.Accent)

	p := progress.New(
		progress.WithColors(theme.Accent, theme.Green),
		progress.WithoutPercentage(),
	)

	return Model{
		state:       stateDetecting,
		spinner:     s,
		progressBar: p,
		styles:      styles,
		theme:       theme,
		status:      "Initializing Automergent Environment",
		startTime:   time.Now(),
		lastTick:    time.Now(),
	}
}

type versionMsg string
type infoMsg *Info
type downloadProgressMsg float64
type downloadDoneMsg string
type extractDoneMsg struct{}
type finishMsg struct{}
type errorMsg error

func animTick() tea.Cmd {
	return tea.Tick(time.Second/60, func(t time.Time) tea.Msg {
		return animTickMsg(t)
	})
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		animTick(),
		func() tea.Msg {
			v, err := GetLatestVersion()
			if err != nil {
				return errorMsg(err)
			}
			return versionMsg(v)
		},
		func() tea.Msg {
			info, err := GetSystemInfo()
			if err != nil {
				return errorMsg(err)
			}
			return infoMsg(info)
		},
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		w := m.width - 24
		if w > 60 {
			w = 60
		}
		p := m.progressBar
		p.SetWidth(w)
		m.progressBar = p
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "q" || (m.state == stateDone && (msg.String() == "enter" || msg.String() == "return")) {
			return m, tea.Quit
		}

	case animTickMsg:
		t := time.Since(m.startTime).Seconds()
		m.animPulse = math.Sin(t * 2.0)

		if m.entryPhase < 1.0 {
			m.entryPhase += 0.02
		}

		// Interpolate progress
		if m.progress < m.targetProgress {
			m.progress += (m.targetProgress - m.progress) * 0.15
			if m.targetProgress-m.progress < 0.001 {
				m.progress = m.targetProgress
			}
		}

		if m.shake > 0 {
			m.shake -= 0.1
		}

		m.lastTick = time.Time(msg)
		return m, animTick()

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case versionMsg:
		m.version = string(msg)
		return m.checkReady()

	case infoMsg:
		m.info = (*Info)(msg)
		return m.checkReady()

	case downloadProgressMsg:
		m.targetProgress = float64(msg)
		return m, m.listenProgress()

	case downloadDoneMsg:
		m.archive = string(msg)
		m.state = stateExtracting
		m.status = "Extracting Automergent Core"
		return m, func() tea.Msg {
			err := ExtractBinary(m.archive, m.info.DestDir)
			if err != nil {
				return errorMsg(err)
			}
			return extractDoneMsg{}
		}

	case extractDoneMsg:
		m.state = stateFinishing
		m.status = "Finalizing Installation"
		return m, tea.Batch(
			func() tea.Msg {
				if !CheckBinary("automergent") {
					_ = AddToPath(m.info.DestDir)
				}
				time.Sleep(300 * time.Millisecond)
				return finishMsg{}
			},
		)

	case finishMsg:
		m.state = stateDone
		m.doneTime = time.Now()
		m.status = "System Ready"
		_ = SetupBinary(m.info.DestDir)
		if m.InstallerPath != "" {
			_ = os.Remove(m.InstallerPath)
		}
		return m, nil

	case errorMsg:
		m.err = error(msg)
		m.state = stateError
		m.shake = 1.0
		return m, nil
	}

	return m, nil
}

func (m Model) checkReady() (Model, tea.Cmd) {
	if m.version != "" && m.info != nil {
		m.state = stateDownloading
		m.status = fmt.Sprintf("Downloading Automergent v%s", m.version)
		m.progressChan = make(chan float64)

		download := func() tea.Msg {
			path, err := DownloadBinary(m.version, m.info, m.progressChan)
			close(m.progressChan)
			if err != nil {
				return errorMsg(err)
			}
			return downloadDoneMsg(path)
		}

		m.listenProgress = func() tea.Cmd {
			return func() tea.Msg {
				p, ok := <-m.progressChan
				if !ok {
					return nil
				}
				return downloadProgressMsg(p)
			}
		}

		return m, tea.Batch(
			download,
			m.listenProgress(),
		)
	}
	return m, nil
}

func (m Model) View() tea.View {
	if m.width == 0 {
		return tea.NewView("")
	}
	contentWidth := m.width - 8
	if contentWidth > 78 {
		contentWidth = 78
	}
	if contentWidth < 30 {
		contentWidth = max(20, m.width-2)
	}

	stateLabel := "INSTALLER"
	if m.state == stateDone {
		stateLabel = "COMPLETE"
	} else if m.state == stateError {
		stateLabel = "FAILED"
	}
	version := m.version
	if version == "" {
		version = "detecting…"
	}
	left := m.styles.BrandMark() +
		lipgloss.NewStyle().Foreground(m.theme.Accent).Bold(true).Render(themes.BrandName)
	center := lipgloss.NewStyle().Foreground(m.theme.Subtext).Bold(true).Render(stateLabel)
	right := lipgloss.NewStyle().Foreground(m.theme.Muted).Render(version)
	gap1 := max(1, contentWidth/2-lipgloss.Width(center)/2-lipgloss.Width(left))
	gap2 := max(1, contentWidth-lipgloss.Width(left)-gap1-lipgloss.Width(center)-lipgloss.Width(right))
	header := left + strings.Repeat(" ", gap1) + center + strings.Repeat(" ", gap2) + right
	header = lipgloss.NewStyle().Width(contentWidth).Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(m.theme.BorderNormal).Render(header)

	var body strings.Builder
	if m.state == stateDone {
		m.renderSuccess(&body, contentWidth)
	} else if m.state == stateError {
		m.renderError(&body, contentWidth)
	} else {
		m.renderActive(&body, contentWidth)
	}

	footer := lipgloss.NewStyle().Width(contentWidth).Border(lipgloss.NormalBorder(), true, false, false, false).
		BorderForeground(m.theme.BorderNormal).Render(m.renderFooter())
	page := lipgloss.JoinVertical(lipgloss.Left, header, "", body.String(), "", footer)
	finalView := lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, page)

	v := tea.NewView(finalView)
	v.AltScreen = true
	return v
}

func (m Model) renderActive(body *strings.Builder, width int) {
	body.WriteString(lipgloss.NewStyle().Bold(true).Render("Installing Automergent") + "\n\n")
	steps := []struct {
		label, detail string
		state         state
	}{
		{"Detect system", m.platformDetail(), stateDetecting},
		{"Resolve release", "Automergent " + m.version, stateDetecting},
		{"Download", "", stateDownloading},
		{"Extract binary", m.targetDetail(), stateExtracting},
		{"Configure environment", "PATH and command aliases", stateFinishing},
	}
	for _, step := range steps {
		completed := m.state > step.state
		active := m.state == step.state
		icon, color := "○", m.theme.Muted
		if completed {
			icon, color = "✓", m.theme.Green
		} else if active {
			icon, color = m.spinner.View(), m.theme.Accent
		}
		body.WriteString(lipgloss.NewStyle().Foreground(color).Render(icon) + "  " + lipgloss.NewStyle().Bold(active).Render(step.label) + "\n")
		if step.detail != "" && (completed || active) {
			body.WriteString("    " + m.styles.Dim.Render(step.detail) + "\n")
		}
		if step.label == "Download" && active {
			barWidth := min(48, width-8)
			p := m.progressBar
			p.SetWidth(barWidth)
			body.WriteString("    " + p.ViewAs(m.progress) + "  " + m.styles.Dim.Render(fmt.Sprintf("%d%%", int(m.progress*100))) + "\n")
		}
		body.WriteString("\n")
	}
}

func (m Model) renderSuccess(body *strings.Builder, width int) {
	body.WriteString(lipgloss.NewStyle().Foreground(m.theme.Green).Bold(true).Render("✓  Automergent is ready") + "\n\n")
	body.WriteString(m.field("Binary", m.targetDetail()) + "\n")
	body.WriteString(m.field("Version", m.version) + "\n")
	body.WriteString(m.field("Platform", m.platformDetail()) + "\n")
	body.WriteString(m.field("Environment", "PATH and aliases configured") + "\n\n")
	body.WriteString(m.styles.Dim.Render("Start with either command:") + "\n\n")
	body.WriteString("    " + lipgloss.NewStyle().Foreground(m.theme.Accent).Bold(true).Render("amt") + "\n")
	body.WriteString("    " + lipgloss.NewStyle().Foreground(m.theme.Accent).Bold(true).Render("automergent") + "\n")
}

func (m Model) renderError(body *strings.Builder, width int) {
	body.WriteString(lipgloss.NewStyle().Foreground(m.theme.Red).Bold(true).Render("✗  Installation failed") + "\n\n")
	body.WriteString(m.field("Step", m.status) + "\n")
	if m.info != nil {
		body.WriteString(m.field("Target", m.info.DestDir) + "\n")
	}
	body.WriteString(m.field("Error", m.err.Error()) + "\n\n")
	body.WriteString(m.styles.Dim.Render("Resolve the error and run the installer again."))
}

func (m Model) renderFooter() string {
	helpStyle := lipgloss.NewStyle().Foreground(m.theme.Muted)
	if m.state == stateDone {
		return helpStyle.Render("enter close  ·  Run: amt or automergent  ·  q exit")
	}
	if m.state == stateError {
		return helpStyle.Render("q exit  ·  Check the error above and retry the installer")
	}
	return helpStyle.Render(fmt.Sprintf("INSTALLING  ·  q cancel  ·  %s elapsed", time.Since(m.startTime).Round(time.Second)))
}

func (m Model) platformDetail() string {
	if m.info == nil {
		return "Detecting platform…"
	}
	return m.info.OS + "/" + m.info.Arch
}

func (m Model) targetDetail() string {
	if m.info == nil {
		return "Resolving installation target…"
	}
	return m.info.DestDir
}

func (m Model) field(label, value string) string {
	return m.styles.Dim.Width(13).Render(label) + lipgloss.NewStyle().Foreground(m.theme.Subtext).Render(value)
}
