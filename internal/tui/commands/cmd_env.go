package commands

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/iSundram/Automergent/internal/tui/components"
)

// /env — show runtime environment details.

func envCommand() Command {
	return Command{
		Name:             "env",
		Description:      "Show runtime environment details",
		Category:         "System",
		Icon:             "󰟀",
		Tier:             TierTertiary,
		Type:             CmdFullPage,
		FullPageTitle:    "Environment",
		Immediate:        true,
		SupportsHeadless: true,
		Page:             envPage,
	}
}

func handleEnv(host Host, args []string) Result {
	kind, available := host.SandboxStatus()
	pc := host.ProviderConfig(host.Provider())
	effort := pc.Effort
	if effort == "" {
		effort = pc.ThinkingLevel
	}
	if effort == "" {
		effort = "default"
	}

	var b strings.Builder
	b.WriteString("Automergent environment:\n")
	fmt.Fprintf(&b, "Version:     %s\n", host.Version())
	fmt.Fprintf(&b, "Runtime:     %s %s/%s\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&b, "Workdir:     %s\n", host.WorkDir())
	sessionLine := host.SessionID()
	if title := host.SessionTitle(); title != "" {
		sessionLine += fmt.Sprintf(" (%s)", title)
	}
	fmt.Fprintf(&b, "Session:     %s\n", sessionLine)
	fmt.Fprintf(&b, "Provider:    %s (model %s)\n", host.Provider(), host.Model())
	fmt.Fprintf(&b, "Mode:        %s · Effort: %s\n", host.Mode(), effort)
	fmt.Fprintf(&b, "Theme:       %s · Keybindings: %s\n", host.CurrentTheme(), host.CurrentKeybindings())
	fmt.Fprintf(&b, "Sandbox:     %s", kind)
	if !available {
		b.WriteString(" (unavailable)")
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "Tokens:      in %d · out %d · $%.4f\n", host.InputTokens(), host.OutputTokens(), host.TotalCost())
	if p := host.GlobalConfigPath(); p != "" {
		mark := "–"
		if fileExists(p) {
			mark = "✓"
		}
		fmt.Fprintf(&b, "%s Global config:  %s\n", mark, p)
	}
	if p := host.ProjectConfigPath(); p != "" {
		mark := "–"
		if fileExists(p) {
			mark = "✓"
		}
		fmt.Fprintf(&b, "%s Project config: %s\n", mark, p)
	}

	host.AddSystemMessage(strings.TrimRight(b.String(), "\n"))
	return Done(nil)
}

// envPage builds the structured /env page.
func envPage(h Host) components.Page {
	kind, available := h.SandboxStatus()
	pc := h.ProviderConfig(h.Provider())
	effort := pc.Effort
	if effort == "" {
		effort = pc.ThinkingLevel
	}
	if effort == "" {
		effort = "default"
	}
	session := h.SessionID()
	if title := h.SessionTitle(); title != "" {
		session = fmt.Sprintf("%s (%s)", session, title)
	}
	sandboxFlag := components.PageFlag{Label: "Sandbox", Detail: kind, Status: components.PageStatusOK}
	if !available {
		sandboxFlag.Status = components.PageStatusWarn
		sandboxFlag.Detail = kind + " (unavailable)"
	}

	sections := []components.PageSection{
		{
			Heading: "Runtime",
			Rows: [][2]string{
				components.Row("Version", h.Version()),
				components.Row("Runtime", fmt.Sprintf("%s %s/%s", runtime.Version(), runtime.GOOS, runtime.GOARCH)),
				components.Row("Workdir", orDash(h.WorkDir())),
				components.Row("Session", orDash(session)),
			},
			Flagged: []components.PageFlag{sandboxFlag},
		},
		{
			Heading: "Active configuration",
			Rows: [][2]string{
				components.Row("Provider", fmt.Sprintf("%s (model %s)", h.Provider(), h.Model())),
				components.Row("Mode", fmt.Sprintf("%s · Effort: %s", h.Mode(), effort)),
				components.Row("Theme", fmt.Sprintf("%s · Keybindings: %s", h.CurrentTheme(), h.CurrentKeybindings())),
				components.Row("Tokens", fmt.Sprintf("in %d · out %d · $%.4f", h.InputTokens(), h.OutputTokens(), h.TotalCost())),
			},
		},
	}

	var configFlags []components.PageFlag
	if p := h.GlobalConfigPath(); p != "" {
		configFlags = append(configFlags, components.PageFlag{
			Label: "Global config", Detail: p,
			Status: fileFlagStatus(fileExists(p)),
		})
	}
	if p := h.ProjectConfigPath(); p != "" {
		configFlags = append(configFlags, components.PageFlag{
			Label: "Project config", Detail: p,
			Status: fileFlagStatus(fileExists(p)),
		})
	}
	if len(configFlags) > 0 {
		sections = append(sections, components.PageSection{Heading: "Config files", Flagged: configFlags})
	}

	return components.Page{
		Title:    "Environment",
		Subtitle: "Runtime details for this Automergent session",
		Sections: sections,
		Actions: []components.PageAction{
			{Key: "d", Label: "Doctor", Command: "doctor"},
			{Key: "c", Label: "Config", Command: "config"},
			{Key: "v", Label: "Version", Command: "version"},
		},
	}
}

func fileFlagStatus(ok bool) components.PageStatus {
	if ok {
		return components.PageStatusOK
	}
	return components.PageStatusWarn
}
