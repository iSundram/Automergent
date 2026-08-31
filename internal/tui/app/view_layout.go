package app

// View assembly and workspace layout.
// Moved verbatim from internal/tui/app.go.

import (
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
)

func (a *App) layout() {
	if a.width <= 0 || a.height <= 0 {
		return
	}

	a.header.SetWidth(a.width)
	a.statusBar.SetWidth(a.width)
	a.input.SetWidth(a.width)
	a.infoLine.SetWidth(a.width)
	a.palette.SetSize(a.width, a.height)
	a.selector.SetSize(a.width, a.height)
	a.confirm.SetSize(a.width, a.height)
	a.fullPage.SetSize(a.width, a.height)
	a.pageViewer.SetSize(a.width, a.height)
	a.providerStudio.SetSize(a.width, a.height)
	a.modelHub.SetSize(a.width, a.height)

	// Header is always shown (even on welcome/empty screen).
	headerH := lipgloss.Height(a.header.View())
	statusH := lipgloss.Height(a.statusBar.View())
	footerH := 0
	if !a.browsing {
		footerH = lipgloss.Height(a.input.View())
	}
	if a.confirm.Visible() {
		footerH = lipgloss.Height(a.confirm.View())
	}
	if a.thinking {
		footerH++
	}
	// The settled run summary ("✓ Done (…)") occupies the spinner's row
	// after the run ends, so it costs the same row.
	if !a.thinking && a.lastRunSummary != "" {
		footerH++
	}
	// The dock rail is the one always-on row of background-work state; it costs
	// exactly one row whenever there is anything to report and never more.
	if a.dockRailView() != "" {
		footerH++
	}
	// The `└─` info line sits above the prompt (under the spinner while running).
	// It is always rendered, so it always costs a row.
	if a.infoLineVisible() {
		footerH++
	}
	// Palette and secondary confirmations render inline below the input.
	if a.palette.Visible() && !a.confirm.Visible() && !a.browsing {
		footerH += a.palette.Height()
	}
	// Session browser also renders inline below the input.
	if a.sessionBrowser.Visible() && !a.confirm.Visible() && !a.browsing {
		footerH += a.sessionBrowser.Height()
	}
	// Artifact review browser renders inline below the input (not while an
	// artifact preview owns the whole screen).
	if a.artifactBrowser.Visible() && !a.artifactBrowser.Previewing() && !a.confirm.Visible() && !a.browsing {
		footerH += a.artifactBrowser.Height()
	}
	// Bottom dock (background shells/agents) renders under the input.
	// refreshDock runs before HasContent is consulted: the old order asked
	// "is there anything to show" before asking "what is there to show", so the
	// first paint after a task spawned drew no dock until the next refresh.
	dockH := 0
	if a.dock != nil && !a.confirm.Visible() {
		a.refreshDock()
		if a.dock.HasContent() {
			a.dock.SetWidth(a.width)
			dockH = a.dock.Height()
		}
	}
	// The queue strip sits between the input and the dock. Its height is a
	// function of the item count, so it cannot cause the reflow-under-content
	// problem the dock was rebuilt to avoid.
	if a.queueStrip != nil {
		a.refreshQueueStrip()
		if a.queueStrip.Len() > 0 && !a.browsing {
			footerH += a.queueStrip.Height()
		}
	}

	mainH := a.height - headerH - statusH - footerH - dockH
	if mainH < 1 {
		mainH = 1
	}

	// Diff is now fullscreen overlay - always set to full dimensions
	a.diffPane.SetSize(a.width, a.height)
	// So is the inspector, when a background task owns the screen.
	if a.inspector != nil {
		a.inspector.SetSize(a.width, a.height)
	}

	mainW := a.width
	if a.showFileTree {
		treeW := 25
		if a.width > 80 {
			treeW = a.width / 5
		}
		a.fileTree.SetSize(treeW, mainH)
		mainW = a.width - treeW - 1
	}
	a.conversation.SetSize(mainW, mainH)
	a.sessionBrowser.SetSize(a.width, a.height*3/4)
	// Full height while previewing (it renders as a full-page overlay);
	// three quarters inline otherwise.
	if a.artifactBrowser.Previewing() {
		a.artifactBrowser.SetSize(a.width, a.height)
	} else {
		a.artifactBrowser.SetSize(a.width, a.height*3/4)
	}
}

// infoLineVisible reports whether the `└─` hint line should be rendered. It is
// suppressed in zen mode and while a full-screen overlay owns the display,
// where it would either be noise or unreachable.
func (a *App) infoLineVisible() bool {
	if a.zenMode || a.width <= 0 {
		return false
	}
	if a.showHelp || a.selector.Visible() || a.diffPane.Visible() || a.fullPage.Visible() || a.pageViewer.Visible() {
		return false
	}
	return a.infoLine.View() != ""
}

func (a *App) View() tea.View {
	// Helper to ensure all views have consistent settings
	makeView := func(content string) tea.View {
		v := tea.NewView(content)
		v.AltScreen = true
		v.MouseMode = tea.MouseModeCellMotion // Capture mouse to prevent terminal scrollback
		return v
	}

	if a.width <= 0 || a.height <= 0 {
		return makeView("Initializing...")
	}
	if a.showHelp {
		return makeView(a.helpOverlay.View())
	}
	if a.selector.Visible() {
		return makeView(a.selector.View())
	}

	headerView := ""
	if a.zenMode {
		headerView = "" // zen mode hides chrome
	} else {
		headerView = a.header.View()
	}
	statusView := a.statusBar.View()
	var sections []string
	if headerView != "" {
		sections = append(sections, headerView)
	}

	var mainRow string
	convView := a.conversation.View()
	// Only wrap in ActivePane border if we have multiple panes (FileTree or TaskBoard)
	hasOtherPanes := a.showFileTree || a.taskBoard.Visible()
	if a.focus == "conversation" && hasOtherPanes {
		convView = a.styles.ActivePane.Width(lipgloss.Width(convView)).Render(convView)
	}
	if a.showFileTree {
		mainRow = lipgloss.JoinHorizontal(lipgloss.Top, a.fileTree.View(), " ", convView)
	} else {
		mainRow = convView
	}
	if a.taskBoard.Visible() {
		mainRow = lipgloss.JoinHorizontal(lipgloss.Top, mainRow, " ", a.taskBoard.View())
	}

	sections = append(sections, mainRow)

	// Non-blocking toast stack above the footer.
	if a.toasts != nil && a.toasts.Active() {
		sections = append(sections, a.toasts.View())
	}

	// Footer: spinner, info line, input, then inline palette/confirmation panels
	// below the input.
	var footer []string
	if a.thinking {
		footer = append(footer, "  "+a.spin.View())
	} else if a.lastRunSummary != "" {
		// The settled run line: the spinner slot keeps the completed run's
		// readout ("✓ Done (2s • ↓ 20 tokens)") until the next run starts,
		// where the live "(2s • ↓ 20 tokens)" clock was ticking.
		footer = append(footer, "  "+a.styles.Dim.Render(a.lastRunSummary))
	}
	// The `└─` info line sits between the spinner it explains and the prompt it
	// advises, so it reads as a continuation of the run status rather than as
	// something typed. It is never rendered below the input.
	if rail := a.dockRailView(); rail != "" {
		footer = append(footer, rail)
	}
	if a.infoLineVisible() {
		if line := a.infoLine.View(); line != "" {
			footer = append(footer, line)
		}
	}
	// The queue strip lists messages waiting for the run to end, directly above
	// the prompt they will be typed into. Rendered before the input so the
	// visual order matches the delivery order.
	if a.queueStrip != nil && a.queueStrip.Len() > 0 && !a.browsing {
		if strip := a.queueStrip.View(); strip != "" {
			footer = append(footer, strip)
		}
	}
	if a.confirm.Visible() {
		footer = append(footer, a.confirm.View())
	} else if a.questionnaire != nil && a.questionnaire.Visible() {
		footer = append(footer, a.questionnaire.View())
	} else if !a.browsing {
		footer = append(footer, a.input.View())
	}
	if a.palette.Visible() && !a.confirm.Visible() && !a.browsing {
		footer = append(footer, a.palette.View())
	}
	// Session browser renders inline (like the palette) so it never leaves a
	// large empty area where the conversation used to be.
	if a.sessionBrowser.Visible() && !a.confirm.Visible() && !a.browsing {
		footer = append(footer, a.sessionBrowser.View())
	}
	// Artifact review browser renders inline as well — except while it is
	// previewing an artifact, which takes the whole screen.
	if a.artifactBrowser.Visible() && !a.artifactBrowser.Previewing() && !a.confirm.Visible() && !a.browsing {
		footer = append(footer, a.artifactBrowser.View())
	}
	sections = append(sections, lipgloss.JoinVertical(lipgloss.Left, footer...))

	// Bottom dock: background shells + agents, under the input.
	questionnaireVisible := a.questionnaire != nil && a.questionnaire.Visible()
	if a.dock != nil && a.dock.HasContent() && !a.confirm.Visible() && !questionnaireVisible {
		if dockView := a.dock.View(); dockView != "" {
			sections = append(sections, dockView)
		}
	}

	sections = append(sections, statusView)

	fullView := lipgloss.JoinVertical(lipgloss.Left, sections...)

	// Full-page overlay takes priority over everything.
	if a.fullPage.Visible() {
		fullView = a.fullPage.View()
	}
	// The structured page viewer sits at the same layer as the plain full
	// page and only one of them is ever visible at a time.
	if a.pageViewer.Visible() {
		fullView = a.pageViewer.View()
	}
	// Provider Studio overlay.
	if a.providerStudio.Visible() {
		fullView = a.providerStudio.View()
	}
	// Model Hub overlay.
	if a.modelHub.Visible() {
		fullView = a.modelHub.View()
	}
	// An artifact preview is a full-page surface: the file takes the whole
	// screen with its review actions in the footer.
	if a.artifactBrowser.Visible() && a.artifactBrowser.Previewing() {
		fullView = a.artifactBrowser.View()
	}
	// Diff is now fullscreen overlay - render on top of everything
	if a.diffPane.Visible() {
		fullView = a.diffPane.View()
	}
	// The inspector outranks the diff: opening a task viewer from the dock while
	// a diff is up means the task is what the user wants to look at now.
	if a.inspector != nil && a.inspector.Visible() {
		fullView = a.inspector.View()
	}

	return makeView(fullView)
}
