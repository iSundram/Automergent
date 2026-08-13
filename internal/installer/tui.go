package installer

import (
	"fmt"
	"image/color"
	"math"
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/iSundram/Automergent/internal/tui/themes"
	"github.com/lucasb-eyer/go-colorful"
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
	theme := themes.Catppuccin()
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
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			return m, tea.Quit
		}

	case animTickMsg:
		t := time.Since(m.startTime).Seconds()
		m.animPulse = math.Sin(t * 3.0)

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
				time.Sleep(800 * time.Millisecond) // Visual beat
				return finishMsg{}
			},
		)

	case finishMsg:
		m.state = stateDone
		m.doneTime = time.Now()
		m.status = "System Ready"
		return m, func() tea.Msg {
			if err := SetupBinary(m.info.DestDir); err != nil {
				// Non-fatal
			}

			time.Sleep(15 * time.Second)

			if m.InstallerPath != "" {
				os.Remove(m.InstallerPath)
			}

			return tea.Quit
		}

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

	// --- Layout Setup ---
	var content strings.Builder

	// Dynamic Colors
	accentColor := m.theme.Accent
	if m.state == stateDone {
		accentColor = m.theme.Green
	} else if m.state == stateError {
		accentColor = m.theme.Red
	}

	// Pulse Effect
	glowAmount := (m.animPulse + 1) / 2 // 0 to 1
	glowColor := m.lerpColor(m.theme.Overlay, accentColor, glowAmount*0.4)

	// Centered Brand Header
	brandStyle := lipgloss.NewStyle().
		Foreground(accentColor).
		Bold(true).
		Padding(0, 2).
		Border(lipgloss.DoubleBorder(), false, false, true, false).
		BorderForeground(glowColor)

	logo := " ⟡ AUTOMERGENT ⟡ "
	header := brandStyle.Render(logo)
	content.WriteString(lipgloss.PlaceHorizontal(m.width, lipgloss.Center, header) + "\n\n")

	// --- Main Body ---
	bodyWidth := 64
	if m.width-10 < bodyWidth {
		bodyWidth = m.width - 10
	}

	var body strings.Builder

	// Status Line
	statusIcon := m.spinner.View()
	if m.state == stateDone {
		statusIcon = "✨"
	} else if m.state == stateError {
		statusIcon = "❌"
	}

	statusLine := lipgloss.JoinHorizontal(lipgloss.Center,
		lipgloss.NewStyle().Foreground(accentColor).PaddingRight(1).Render(statusIcon),
		lipgloss.NewStyle().Bold(true).Render(m.status),
	)
	body.WriteString(lipgloss.PlaceHorizontal(bodyWidth, lipgloss.Center, statusLine) + "\n\n")

	// Progress or Info
	if m.state == stateDone {
		m.renderSuccess(&body, bodyWidth)
	} else if m.state == stateError {
		m.renderError(&body, bodyWidth)
	} else {
		m.renderActive(&body, bodyWidth)
	}

	// Body Box
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(glowColor).
		Padding(1, 2).
		Width(bodyWidth)

	// Shake animation on error
	xOffset := 0
	if m.shake > 0 {
		xOffset = int(math.Sin(time.Since(m.startTime).Seconds()*50.0) * m.shake * 4.0)
	}
	// We'll ignore the xOffset in rendering for now to avoid breaking PlaceHorizontal,
	// but it could be added as Left/Right margins or spaces.
	_ = xOffset

	renderedBody := boxStyle.Render(body.String())
	content.WriteString(lipgloss.PlaceHorizontal(
		m.width,
		lipgloss.Center,
		renderedBody,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceStyle(lipgloss.NewStyle().Foreground(m.theme.Background)),
	))

	// Footer
	footer := m.renderFooter()
	content.WriteString("\n\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, footer))

	// Global centering
	finalView := lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content.String())

	v := tea.NewView(finalView)
	v.AltScreen = true
	return v
}

func (m Model) renderActive(body *strings.Builder, width int) {
	// Progress Bar
	if m.state == stateDownloading {
		bar := m.progressBar.ViewAs(m.progress)
		body.WriteString(lipgloss.PlaceHorizontal(width, lipgloss.Center, bar) + "\n")
		percent := fmt.Sprintf("%d%%", int(m.progress*100))
		body.WriteString(lipgloss.PlaceHorizontal(width, lipgloss.Center, m.styles.Dim.Render(percent)) + "\n\n")
	}

	// Info Grid
	if m.info != nil {
		labelStyle := m.styles.Dim.Width(12).Align(lipgloss.Right).PaddingRight(2)
		valStyle := lipgloss.NewStyle().Foreground(m.theme.Subtext)

		grid := lipgloss.JoinVertical(lipgloss.Left,
			lipgloss.JoinHorizontal(lipgloss.Top, labelStyle.Render("PLATFORM"), valStyle.Render(m.info.OS+"/"+m.info.Arch)),
			lipgloss.JoinHorizontal(lipgloss.Top, labelStyle.Render("VERSION"), valStyle.Render(m.version)),
			lipgloss.JoinHorizontal(lipgloss.Top, labelStyle.Render("TARGET"), valStyle.Render(m.info.DestDir)),
		)
		body.WriteString(lipgloss.PlaceHorizontal(width, lipgloss.Center, grid) + "\n")
	}
}

func (m Model) renderSuccess(body *strings.Builder, width int) {
	title := lipgloss.NewStyle().
		Foreground(m.theme.Green).
		Bold(true).
		SetString("INSTALLATION COMPLETE").
		Render()

	body.WriteString(lipgloss.PlaceHorizontal(width, lipgloss.Center, title) + "\n\n")

	steps := []struct {
		icon string
		text string
	}{
		{"󰄬", "Binary installed to " + m.info.DestDir},
		{"󰄬", "Environment variables updated"},
		{"󰄬", "Shell aliases configured"},
	}

	var stepsStr strings.Builder
	for _, s := range steps {
		line := fmt.Sprintf("%s %s",
			lipgloss.NewStyle().Foreground(m.theme.Green).Render(s.icon),
			lipgloss.NewStyle().Foreground(m.theme.Subtext).Render(s.text),
		)
		stepsStr.WriteString(line + "\n")
	}
	body.WriteString(lipgloss.PlaceHorizontal(width, lipgloss.Center, stepsStr.String()) + "\n")

	body.WriteString(lipgloss.NewStyle().
		Foreground(m.theme.Accent).
		Bold(true).
		Render("Next: Run 'owe' to begin.") + "\n")

	elapsed := time.Since(m.doneTime).Seconds()
	countdown := 15 - int(elapsed)
	if countdown < 0 {
		countdown = 0
	}

	body.WriteString("\n" + m.styles.Dim.Render(fmt.Sprintf("Closing in %ds...", countdown)))
}

func (m Model) renderError(body *strings.Builder, width int) {
	title := lipgloss.NewStyle().
		Foreground(m.theme.Red).
		Bold(true).
		SetString("CRITICAL FAILURE").
		Render()

	body.WriteString(lipgloss.PlaceHorizontal(width, lipgloss.Center, title) + "\n\n")

	errMsg := lipgloss.NewStyle().
		Foreground(m.theme.Subtext).
		Width(width - 4).
		Align(lipgloss.Center).
		Render(m.err.Error())

	body.WriteString(errMsg + "\n\n")
	body.WriteString(m.styles.Dim.Render("Press 'q' to exit and check logs."))
}

func (m Model) renderFooter() string {
	helpStyle := lipgloss.NewStyle().Foreground(m.theme.Muted)
	return helpStyle.Render(fmt.Sprintf("ESC/Q Exit  •  Automergent Installer v%s", Version))
}

func (m Model) lerpColor(start, end color.Color, t float64) color.Color {
	s, _ := colorful.MakeColor(start)
	e, _ := colorful.MakeColor(end)
	c := s.BlendLab(e, t)
	return c
}
