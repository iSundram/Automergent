package commands

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/iSundram/Automergent/internal/modelsdev"
	"github.com/iSundram/Automergent/internal/tui/components"
)

// /doctor — check configuration, provider and storage health.

func doctorCommand() Command {
	return Command{
		Name:             "doctor",
		Description:      "Check configuration, provider and storage health",
		Category:         "System",
		Icon:             "󰨙",
		Tier:             TierPrimary,
		Type:             CmdFullPage,
		FullPageTitle:    "Doctor",
		Immediate:        true,
		SupportsHeadless: true,
		Page:             doctorPage,
	}
}

func handleDoctor(host Host, args []string) Result {
	page := doctorPage(host)
	var b strings.Builder
	b.WriteString("Automergent doctor:\n")
	failures, warnings := 0, 0
	for _, sec := range page.Sections {
		for _, f := range sec.Flagged {
			marker := "✓"
			switch f.Status {
			case components.PageStatusFail:
				marker = "✗"
				failures++
			case components.PageStatusWarn:
				marker = "!"
				warnings++
			}
			line := marker + " " + f.Label
			if f.Detail != "" {
				line += ": " + f.Detail
			}
			b.WriteString(line + "\n")
		}
	}
	host.AddSystemMessage(strings.TrimRight(b.String(), "\n"))
	switch {
	case failures > 0:
		host.SetStatus(fmt.Sprintf("Doctor: %d issue(s), %d warning(s)", failures, warnings))
	case warnings > 0:
		host.SetStatus(fmt.Sprintf("Doctor: all checks passed with %d warning(s)", warnings))
	default:
		host.SetStatus("Doctor: all checks passed")
	}
	return Done(nil)
}

// doctorPage builds the structured /doctor page.
func doctorPage(h Host) components.Page {
	flags := []components.PageFlag{}

	// Config schema.
	if problems := h.ValidateConfig(); len(problems) > 0 {
		flags = append(flags, components.PageFlag{Label: "Configuration", Detail: strings.Join(problems, "; "), Status: components.PageStatusFail})
	} else {
		flags = append(flags, components.PageFlag{Label: "Configuration", Detail: "valid", Status: components.PageStatusOK})
	}

	// Active provider credentials.
	pc := h.ProviderConfig(h.Provider())
	if pc.APIKey == "" {
		flags = append(flags, components.PageFlag{Label: "API key (" + h.Provider() + ")", Detail: "not set — run /api-key <value>", Status: components.PageStatusFail})
	} else {
		flags = append(flags, components.PageFlag{Label: "API key (" + h.Provider() + ")", Detail: "set", Status: components.PageStatusOK})
	}

	// UI settings resolve to something real.
	themeOK := containsString(h.AvailableThemes(), h.CurrentTheme())
	flags = append(flags, components.PageFlag{Label: "Theme", Detail: h.CurrentTheme(), Status: boolStatus(themeOK)})
	keyOK := containsString(h.AvailableKeybindings(), h.CurrentKeybindings())
	flags = append(flags, components.PageFlag{Label: "Keybindings", Detail: h.CurrentKeybindings(), Status: boolStatus(keyOK)})

	// Sandbox availability.
	kind, available := h.SandboxStatus()
	switch {
	case kind == "off" || kind == "none":
		flags = append(flags, components.PageFlag{Label: "Sandbox", Detail: kind + " (disabled by config)", Status: components.PageStatusOK})
	case available:
		flags = append(flags, components.PageFlag{Label: "Sandbox", Detail: kind + " available", Status: components.PageStatusOK})
	default:
		flags = append(flags, components.PageFlag{Label: "Sandbox", Detail: kind + " unavailable on this system", Status: components.PageStatusWarn})
	}

	// Session storage.
	if err := h.StorageHealth(); err != nil {
		flags = append(flags, components.PageFlag{Label: "Session storage", Detail: err.Error(), Status: components.PageStatusFail})
	} else {
		flags = append(flags, components.PageFlag{Label: "Session storage", Detail: "reachable", Status: components.PageStatusOK})
	}

	// Project memory presence (informational).
	memDetail := "missing — run /init to create"
	if fileExists(filepath.Join(h.WorkDir(), projectMemoryFile)) {
		memDetail = "present"
	}
	flags = append(flags, components.PageFlag{Label: projectMemoryFile, Detail: memDetail, Status: components.PageStatusOK})

	// Model catalog (models.dev) freshness for the active provider.
	flags = append(flags, catalogFlag(h))

	sections := []components.PageSection{{Heading: "Health checks", Flagged: flags}}

	// MCP servers health.
	if mcpStatuses := h.MCPStatus(); len(mcpStatuses) > 0 {
		mcpFlags := make([]components.PageFlag, 0, len(mcpStatuses))
		for _, s := range mcpStatuses {
			detail := s.Transport
			if s.Version != "" {
				detail += " v" + s.Version
			}
			detail += fmt.Sprintf(" tools=%d", s.Tools)
			status := components.PageStatusOK
			if !s.Connected {
				detail += " OFFLINE"
				if s.LastError != "" {
					detail += " (" + s.LastError + ")"
				}
				status = components.PageStatusFail
			} else {
				detail += " " + s.Latency
			}
			mcpFlags = append(mcpFlags, components.PageFlag{Label: "MCP: " + s.Name, Detail: detail, Status: status})
		}
		sections = append(sections, components.PageSection{Heading: "MCP servers", Flagged: mcpFlags})
	} else {
		sections = append(sections, components.PageSection{
			Heading: "MCP servers",
			Lines:   []string{"No servers configured."},
		})
	}

	return components.Page{
		Title:    "Doctor",
		Subtitle: "Configuration, provider and storage health",
		Sections: sections,
		Actions: []components.PageAction{
			{Key: "i", Label: "Init memory", Command: "init"},
			{Key: "k", Label: "Set API key", Command: "api-key"},
			{Key: "m", Label: "MCP", Command: "mcp"},
		},
	}
}

func boolStatus(ok bool) components.PageStatus {
	if ok {
		return components.PageStatusOK
	}
	return components.PageStatusFail
}

// catalogFlag reports the models.dev catalog state for the active provider:
// cache freshness (fresh / stale / missing) and whether the provider has an
// embedded snapshot fallback. Custom endpoints have no catalog and are
// reported as such rather than flagged.
func catalogFlag(h Host) components.PageFlag {
	provider := h.Provider()
	slug, known := modelsdev.SlugFor(provider)
	if !known {
		return components.PageFlag{Label: "Model catalog", Detail: "none for custom providers", Status: components.PageStatusOK}
	}
	n := modelsdev.SnapshotSize(provider)
	detail := fmt.Sprintf("models.dev/%s — embedded fallback: %d models", slug, n)
	_, exists, age, err := modelsdev.CacheInfo()
	switch {
	case err != nil || !exists:
		return components.PageFlag{Label: "Model catalog", Detail: detail + ", cache missing (embedded snapshot in use)", Status: components.PageStatusWarn}
	case age > 7*24*time.Hour:
		return components.PageFlag{Label: "Model catalog", Detail: detail + fmt.Sprintf(", cache %s old — /model refresh", age.Truncate(time.Hour)), Status: components.PageStatusWarn}
	default:
		return components.PageFlag{Label: "Model catalog", Detail: detail + fmt.Sprintf(", cache %s old", age.Truncate(time.Minute)), Status: components.PageStatusOK}
	}
}
