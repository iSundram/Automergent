package installer

import (
	"fmt"
	"image/color"
	"math"
	"os"
	"strings"
	"time"

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
}

func NewModel() Model {
	theme := themes.Catppuccin()
	styles := themes.NewStyles(theme)
	s := spinner.New()
	s.Spinner = spinner.Points
	s.Style = lipgloss.NewStyle().Foreground(theme.Accent)

	return Model{
		state:     stateDetecting,
		spinner:   s,
		styles:    styles,
		theme:     theme,
		status:    "Detecting system information",
		startTime: time.Now(),
		lastTick:  time.Now(),
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
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			return m, tea.Quit
		}

	case animTickMsg:
		// Interpolate progress
		if m.progress < m.targetProgress {
			m.progress += (m.targetProgress - m.progress) * 0.1
			if m.targetProgress-m.progress < 0.001 {
				m.progress = m.targetProgress
			}
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
		m.status = "Extracting binary"
		return m, func() tea.Msg {
			err := ExtractBinary(m.archive, m.info.DestDir)
			if err != nil {
				return errorMsg(err)
			}
			return extractDoneMsg{}
		}

	case extractDoneMsg:
		m.state = stateFinishing
		m.status = "Finalizing installation"
		return m, tea.Batch(
			func() tea.Msg {
				if !CheckBinary("automergent") {
					_ = AddToPath(m.info.DestDir)
				}
				time.Sleep(500 * time.Millisecond) // Visual beat
				return finishMsg{}
			},
		)

	case finishMsg:
		m.state = stateDone
		m.doneTime = time.Now()
		m.status = "Installation complete!"
		// Setup owe command and cleanup
		return m, func() tea.Msg {
			if err := SetupBinary(m.info.DestDir); err != nil {
				// Non-fatal, just log
			}

			// Wait 20 seconds
			time.Sleep(20 * time.Second)

			// Delete installer binary
			if m.InstallerPath != "" {
				os.Remove(m.InstallerPath)
			}

			return tea.Quit
		}

	case errorMsg:
		m.err = error(msg)
		m.state = stateError
		return m, nil
	}

	return m, nil
}

func (m Model) checkReady() (Model, tea.Cmd) {
	if m.version != "" && m.info != nil {
		m.state = stateDownloading
		m.status = fmt.Sprintf("Downloading v%s", m.version)
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
		v := tea.NewView("Initializing...")
		v.AltScreen = true
		return v
	}

	elapsed := time.Since(m.startTime).Seconds()

	// Staggered entry timing
	headerEntry := math.Min(1, elapsed/0.5)
	mainEntry := math.Max(0, math.Min(1, (elapsed-0.3)/0.5))
	footerEntry := math.Max(0, math.Min(1, (elapsed-0.6)/0.5))

	var sb strings.Builder

	// --- Header (Floating Pill) ---
	if headerEntry > 0 {
		brand := m.styles.HeaderBrand.Render(" ⟡ Automergent ")
		installerLabel := m.styles.Header.Render(" Installer ")
		headerContent := lipgloss.JoinHorizontal(lipgloss.Center, brand, " │ ", installerLabel)

		// Entry animation: slide down + fade (simulated by color)
		yOffset := int((1 - headerEntry) * 5)
		header := m.styles.HeaderPill.MarginTop(1 - yOffset).Render(headerContent)
		sb.WriteString(lipgloss.PlaceHorizontal(m.width, lipgloss.Center, header) + "\n\n")
	} else {
		sb.WriteString("\n\n\n\n")
	}

	// --- Main Content ---
	if mainEntry > 0 {
		var content strings.Builder

		// Animated Label
		label := m.styles.AssistantLabel.Render(" ⟡ Automergent ")
		content.WriteString(label + "\n")

		// Main box body
		var body strings.Builder

		// Status line with animation
		statusLine := m.spinner.View() + " " + m.styles.Bold.Render(m.status) + "…"
		if m.state == stateDone {
			statusLine = m.styles.Success.Render("󰄬 Installation complete!")
		} else if m.state == stateError {
			statusLine = m.styles.Error.Render("󱄊 Installation failed")
		}
		body.WriteString(statusLine + "\n\n")

		// Success Animation for Done State
		if m.state == stateDone {
			doneElapsed := time.Since(m.doneTime).Seconds()

			// Pulsing success message
			pulse := math.Sin(doneElapsed * math.Pi)
			opacityStr := ""
			if pulse > 0.3 {
				opacityStr = "█"
			} else {
				opacityStr = "░"
			}

			// Success message
			thanks := "✨ Thanks for choosing Automergent! ✨"
			body.WriteString(m.styles.Success.Render(thanks) + "\n\n")

			// Installation summary
			summaryStyle := m.styles.Dim
			body.WriteString(summaryStyle.Render("Installation Summary:\n"))
			body.WriteString(summaryStyle.Render(fmt.Sprintf("  • Binary: %s\n", m.info.DestDir)))
			body.WriteString(summaryStyle.Render(fmt.Sprintf("  • Command: owe  or  automergent\n")))
			body.WriteString(summaryStyle.Render(fmt.Sprintf("  • Path: Updated in shell rc\n\n")))

			// How to use
			body.WriteString(m.styles.Bold.Render("Quick Start:\n"))
			body.WriteString(m.styles.Dim.Render("  $ owe          # Launch Automergent\n"))
			body.WriteString(m.styles.Dim.Render("  $ automergent      # Full command\n\n"))

			// Countdown timer
			remainingTime := 20 - int(doneElapsed)
			if remainingTime < 0 {
				remainingTime = 0
			}

			countdownMsg := fmt.Sprintf("Exiting in %d seconds...", remainingTime)
			pulseStyle := lipgloss.NewStyle().Foreground(m.theme.Green)
			body.WriteString(pulseStyle.Render(opacityStr+" "+countdownMsg+" "+opacityStr) + "\n")

			// Cleaning up message
			if doneElapsed > 19 {
				body.WriteString("\n" + m.styles.Dim.Render("🧹 Cleaning up installer..."))
			}
		} else {
			// Progress Bar with Shimmer (for non-done states)
			if m.state == stateDownloading {
				barWidth := m.width - 20
				if barWidth > 40 {
					barWidth = 40
				}
				filled := int(float64(barWidth) * m.progress)
				empty := barWidth - filled

				// Render filled part with shimmer
				shimmerPos := int(math.Mod(elapsed*20, float64(barWidth+10)))
				var barStr strings.Builder
				for i := 0; i < filled; i++ {
					char := "█"
					style := lipgloss.NewStyle().Foreground(m.theme.Green)
					// Shimmer effect
					if i >= shimmerPos-3 && i <= shimmerPos {
						style = style.Foreground(m.theme.Blue)
					}
					barStr.WriteString(style.Render(char))
				}

				track := lipgloss.NewStyle().Foreground(m.theme.Overlay).Render(strings.Repeat("░", empty))
				body.WriteString(fmt.Sprintf("  [%s%s] %d%%\n\n", barStr.String(), track, int(m.progress*100)))
			}

			// Details (Glitch/Typewriter effect simulation)
			if m.info != nil {
				infoText := fmt.Sprintf("System: %s/%s\nTarget: %s", m.info.OS, m.info.Arch, m.info.DestDir)
				// Only show partial text based on time for typewriter effect
				charsToShow := int((elapsed - 0.8) * 50)
				if charsToShow < 0 {
					charsToShow = 0
				}
				if charsToShow > len(infoText) {
					charsToShow = len(infoText)
				}
				body.WriteString(m.styles.Dim.Render(infoText[:charsToShow]) + "\n")
			}
		}

		if m.err != nil {
			body.WriteString("\n" + m.styles.Error.Render("Error: "+m.err.Error()) + "\n")
		}

		// Wrap body in bubble
		bubble := m.styles.AssistantBubble.Width(m.width - 10).Render(body.String())
		content.WriteString(bubble)

		// Entry animation: fade in (simulated)
		sb.WriteString(lipgloss.PlaceHorizontal(m.width, lipgloss.Center, content.String()) + "\n")
	}

	// --- Footer ---
	if footerEntry > 0 {
		footer := m.styles.StatusBar.Width(m.width).Render(fmt.Sprintf("v%s │ Built with love by iSundram │ Press 'q' to exit", Version))
		sb.WriteString(lipgloss.PlaceVertical(m.height-lipgloss.Height(sb.String())-1, lipgloss.Bottom, footer))
	}

	v := tea.NewView(sb.String())
	v.AltScreen = true
	return v
}

func (m Model) lerpColor(start, end color.Color, t float64) color.Color {
	s, _ := colorful.MakeColor(start)
	e, _ := colorful.MakeColor(end)
	c := s.BlendLab(e, t)
	return c
}
