package commands

import (
	"fmt"
	"strings"

	"github.com/iSundram/Automergent/internal/tui/components"
)

// /config — show effective settings at a glance.

func configCommand() Command {
	return Command{
		Name:             "config",
		Description:      "Show effective settings at a glance",
		Category:         "Configuration",
		Icon:             "󰒔",
		Tier:             TierTertiary,
		Type:             CmdFullPage,
		FullPageTitle:    "Configuration",
		Immediate:        true,
		SupportsHeadless: true,
		Page:             configPage,
	}
}

func handleConfig(host Host, args []string) Result {
	if len(args) == 0 {
		host.OpenSettingsPicker()
		return Done(nil)
	}
	host.AddSystemMessage(strings.Join(configPage(host).Lines(80), "\n"))
	host.SetStatus("Settings listed")
	return Done(nil)
}

// configPage builds the structured /config page.
func configPage(h Host) components.Page {
	kind, available := h.SandboxStatus()
	pc := h.ProviderConfig(h.Provider())
	keyState := "not set — run /api-key <value>"
	if pc.APIKey != "" {
		keyState = "set (hidden)"
	}
	effort := pc.Effort
	if effort == "" {
		effort = pc.ThinkingLevel
	}
	if effort == "" {
		effort = "default"
	}
	blocked, allowed := h.SecurityPaths()
	sandboxLine := kind
	if !available && kind != "off" && kind != "none" {
		sandboxLine += " (unavailable)"
	}

	validationFlag := components.PageFlag{Label: "Validation", Detail: "ok", Status: components.PageStatusOK}
	if problems := h.ValidateConfig(); len(problems) > 0 {
		validationFlag = components.PageFlag{
			Label:  "Validation",
			Detail: fmt.Sprintf("%d problem(s)", len(problems)),
			Status: components.PageStatusFail,
		}
	}

	return components.Page{
		Title:    "Configuration",
		Subtitle: "Effective settings",
		Sections: []components.PageSection{
			{
				Heading: "Active",
				Rows: [][2]string{
					components.Row("Provider / Model", h.Provider()+" / "+h.Model()),
					components.Row("Mode", h.Mode()+" · Effort: "+effort),
					components.Row("Theme", h.CurrentTheme()+" · Keybindings: "+h.CurrentKeybindings()),
					components.Row("API key", keyState),
					components.Row("Sandbox", sandboxLine),
					components.Row("Workdir", h.WorkDir()),
				},
			},
			{
				Heading: "Security",
				Rows: [][2]string{
					components.Row("Write rules", fmt.Sprintf("%d blocked · %d allowed paths", len(blocked), len(allowed))),
				},
				Flagged: []components.PageFlag{validationFlag},
			},
			{
				Heading: "Notes",
				Lines:   []string{"Full layer breakdown: `automergent config show` from your shell."},
			},
		},
		Actions: []components.PageAction{
			{Key: "p", Label: "Provider", Command: "provider"},
			{Key: "m", Label: "Model", Command: "model"},
			{Key: "d", Label: "Doctor", Command: "doctor"},
			{Key: "k", Label: "Permissions", Command: "permissions"},
		},
	}
}
