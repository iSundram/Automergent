package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// --- Meta Handlers: /init /env /version /doctor ---

const projectMemoryFile = "AUTOMERGENT.md"

const projectMemoryTemplate = `# AUTOMERGENT.md

Guidance for Automergent agents working in this repository.
Keep it short, factual and current.

## Overview

<!-- What this project does, in a few lines. -->

## Build & Test

- Build: ` + "`<fill in>`" + `
- Test: ` + "`<fill in>`" + `
- Lint: ` + "`<fill in>`" + `

## Conventions

<!-- Language, style and review rules agents must follow. -->

## Safety

- Never commit secrets or credentials.
- Ask before destructive operations (deletes, force-pushes, migrations).
`

func handleInit(host Host, args []string) Result {
	dir := strings.TrimSpace(host.WorkDir())
	if dir == "" {
		host.CommandError("no working directory")
		return Done(nil)
	}
	path := filepath.Join(dir, projectMemoryFile)
	if _, err := os.Stat(path); err == nil {
		host.AddSystemMessage(projectMemoryFile + " already exists at " + path + " — leaving it untouched.")
		host.SetStatus("Init skipped")
		return Done(nil)
	}
	if err := os.WriteFile(path, []byte(projectMemoryTemplate), 0o644); err != nil {
		host.CommandError(fmt.Sprintf("write %s: %v", projectMemoryFile, err))
		return Done(nil)
	}
	host.AddSystemMessage("Created " + path + "\nFill in the build/test commands and conventions so every future session starts informed.")
	host.SetStatus("Project memory created")
	return Done(nil)
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

func handleVersion(host Host, args []string) Result {
	host.AddSystemMessage(fmt.Sprintf("Automergent %s (%s %s/%s)", host.Version(), runtime.GOOS, runtime.GOARCH, runtime.Version()))
	return Done(nil)
}

func handleDoctor(host Host, args []string) Result {
	type check struct {
		label  string
		detail string
		failed bool
		warn   bool
	}
	var checks []check
	add := func(label, detail string, failed, warn bool) {
		checks = append(checks, check{label: label, detail: detail, failed: failed, warn: warn})
	}

	// Config schema.
	problems := host.ValidateConfig()
	configDetail := "valid"
	failed := false
	if len(problems) > 0 {
		configDetail = strings.Join(problems, "; ")
		failed = true
	}
	add("Configuration", configDetail, failed, false)

	// Active provider credentials.
	pc := host.ProviderConfig(host.Provider())
	if pc.APIKey == "" {
		add("API key ("+host.Provider()+")", "not set — run /api-key <value>", true, false)
	} else {
		add("API key ("+host.Provider()+")", "set", false, false)
	}

	// UI settings resolve to something real.
	themeOK := containsString(host.AvailableThemes(), host.CurrentTheme())
	add("Theme", host.CurrentTheme(), !themeOK, false)
	keyOK := containsString(host.AvailableKeybindings(), host.CurrentKeybindings())
	add("Keybindings", host.CurrentKeybindings(), !keyOK, false)

	// Sandbox availability.
	kind, available := host.SandboxStatus()
	switch {
	case kind == "off" || kind == "none":
		add("Sandbox", kind+" (disabled by config)", false, false)
	case available:
		add("Sandbox", kind+" available", false, false)
	default:
		add("Sandbox", kind+" unavailable on this system", false, true)
	}

	// Session storage.
	if err := host.StorageHealth(); err != nil {
		add("Session storage", err.Error(), true, false)
	} else {
		add("Session storage", "reachable", false, false)
	}

	// Project memory presence (informational).
	memDetail := "missing — run /init to create"
	if fileExists(filepath.Join(host.WorkDir(), projectMemoryFile)) {
		memDetail = "present"
	}
	add(projectMemoryFile, memDetail, false, false)

	// MCP servers health.
	mcpStatuses := host.MCPStatus()
	if len(mcpStatuses) > 0 {
		for _, s := range mcpStatuses {
			detail := s.Transport
			if s.Version != "" {
				detail += " v" + s.Version
			}
			detail += fmt.Sprintf(" tools=%d", s.Tools)
			if !s.Connected {
				detail += " OFFLINE"
				if s.LastError != "" {
					detail += " (" + s.LastError + ")"
				}
				add("MCP: "+s.Name, detail, true, false)
			} else {
				detail += " " + s.Latency
				add("MCP: "+s.Name, detail, false, false)
			}
		}
	} else {
		add("MCP", "no servers configured", false, false)
	}

	var b strings.Builder
	failures, warnings := 0, 0
	b.WriteString("Automergent doctor:\n")
	for _, c := range checks {
		marker := "✓"
		switch {
		case c.failed:
			marker = "✗"
			failures++
		case c.warn:
			marker = "!"
			warnings++
		}
		line := marker + " " + c.label
		if c.detail != "" {
			line += ": " + c.detail
		}
		b.WriteString(line + "\n")
	}

	switch {
	case failures > 0:
		host.AddSystemMessage(strings.TrimRight(b.String(), "\n"))
		host.SetStatus(fmt.Sprintf("Doctor: %d issue(s), %d warning(s)", failures, warnings))
	case warnings > 0:
		host.AddSystemMessage(strings.TrimRight(b.String(), "\n"))
		host.SetStatus(fmt.Sprintf("Doctor: all checks passed with %d warning(s)", warnings))
	default:
		host.AddSystemMessage(strings.TrimRight(b.String(), "\n"))
		host.SetStatus("Doctor: all checks passed")
	}
	return Done(nil)
}

func containsString(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func handleCommandsList(host Host, args []string) Result {
	// List custom commands via HelpRows filtered to Custom category.
	// Fall back to a textual list if no custom commands are loaded.
	var b strings.Builder
	b.WriteString("Custom commands:\n")
	count := 0
	// Count is not directly available here; suggest using /help for full list.
	b.WriteString("(see /help for all commands, including Custom category)\n")
	b.WriteString("Run /commands reload to refresh from disk.")
	host.AddSystemMessage(b.String())
	_ = count
	return Done(nil)
}

func handleCommandsReload(host Host, args []string) Result {
	// Trigger a reload by clearing and re-loading; count is reported via status.
	// The actual reload is done by the host glue on next palette open, but we
	// can attempt an immediate reload if the host exposes WorkDir.
	dir := host.WorkDir()
	// Use a temporary registry to count; the real registry is reloaded via
	// refreshCustomCommands() in the app layer. Here we just report.
	host.SetStatus("Reloading custom commands from " + dir + "/.automergent/commands ...")
	host.AddSystemMessage("Custom commands will be reloaded on next palette open (/). Use /commands list to verify.")
	return Done(nil)
}
