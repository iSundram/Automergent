package tui

import (
	"fmt"
	"net/url"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// handleWorkflowConfigCommand owns commands which start project workflows. The bool lets the main dispatcher fall
// through for commands owned by other command groups.
func (a *App) handleWorkflowConfigCommand(cmd string, args []string) (bool, tea.Cmd) {
	switch cmd {
	case "/run":
		if len(args) == 0 {
			a.commandUsage("/run <command>")
			return true, nil
		}
		command := strings.TrimSpace(strings.Join(args, " "))
		a.statusBar.SetStatus("Preparing command permission request")
		return true, a.startAgent("Run the following project command using the shell tool. Request permission before execution: " + command)

	case "/test":
		request := "Detect the project's test command and run the test suite using the shell tool. Request permission before execution."
		if len(args) > 0 {
			request = "Run project tests for this target using the shell tool. Request permission before execution: " + strings.Join(args, " ")
		}
		a.statusBar.SetStatus("Preparing test permission request")
		return true, a.startAgent(request)

	case "/build":
		request := "Detect the project's build command and build the project using the shell tool. Request permission before execution."
		if len(args) > 0 {
			request = "Build this project target using the shell tool. Request permission before execution: " + strings.Join(args, " ")
		}
		a.statusBar.SetStatus("Preparing build permission request")
		return true, a.startAgent(request)

	case "/review":
		enabled := !a.conversation.ReviewMode()
		a.conversation.SetReviewMode(enabled)
		if enabled {
			a.statusBar.SetStatus("Review mode enabled")
			a.conversation.AddMessage("system", "Review mode enabled. Tool details and workspace changes will remain expanded.", false)
		} else {
			a.statusBar.SetStatus("Review mode disabled")
			a.conversation.AddMessage("system", "Review mode disabled. Tool details will use the compact view.", false)
		}
		return true, nil

	case "/cancel":
		if !a.thinking {
			a.statusBar.SetStatus("No active request to cancel")
			return true, nil
		}
		a.cancelActiveRun("Cancelled by user")
		return true, nil

	case "/api-key":
		return true, a.updateProviderAPIKey(a.cfg.Provider, args, "/api-key <value>")
	case "/base-url":
		return true, a.updateProviderBaseURL(a.cfg.Provider, args, "/base-url <url>")
	case "/provider-api-key":
		if len(args) < 2 {
			a.commandUsage("/provider-api-key <provider> <value>")
			return true, nil
		}
		return true, a.updateProviderAPIKey(args[0], args[1:], "/provider-api-key <provider> <value>")
	case "/provider-base-url":
		if len(args) != 2 {
			a.commandUsage("/provider-base-url <provider> <url>")
			return true, nil
		}
		return true, a.updateProviderBaseURL(args[0], args[1:], "/provider-base-url <provider> <url>")

	}
	return false, nil
}

func (a *App) updateProviderAPIKey(provider string, values []string, usage string) tea.Cmd {
	if len(values) == 0 || strings.TrimSpace(strings.Join(values, " ")) == "" {
		a.commandUsage(usage)
		return nil
	}
	if !isSupportedProvider(provider) {
		a.commandError(fmt.Sprintf("Unknown provider %q", provider))
		return nil
	}
	a.ensureProviderConfig(provider)
	pc := a.cfg.Providers[provider]
	pc.APIKey = strings.TrimSpace(strings.Join(values, " "))
	a.cfg.Providers[provider] = pc
	if provider == a.cfg.Provider {
		if err := a.switchProvider(provider, a.cfg.Model); err != nil {
			a.commandError(err.Error())
			return nil
		}
	}
	_ = a.persistProjectConfig()
	a.statusBar.SetStatus("API key updated")
	a.conversation.AddMessage("system", fmt.Sprintf("API key updated for %s. The value is hidden.", provider), false)
	return nil
}

func (a *App) updateProviderBaseURL(provider string, values []string, usage string) tea.Cmd {
	if len(values) != 1 {
		a.commandUsage(usage)
		return nil
	}
	if !isSupportedProvider(provider) {
		a.commandError(fmt.Sprintf("Unknown provider %q", provider))
		return nil
	}
	raw := strings.TrimSpace(values[0])
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		a.commandError("Base URL must be a valid http:// or https:// URL")
		return nil
	}
	a.ensureProviderConfig(provider)
	pc := a.cfg.Providers[provider]
	pc.BaseURL = strings.TrimRight(raw, "/")
	a.cfg.Providers[provider] = pc
	if provider == a.cfg.Provider {
		if err := a.switchProvider(provider, a.cfg.Model); err != nil {
			a.commandError(err.Error())
			return nil
		}
	}
	_ = a.persistProjectConfig()
	a.statusBar.SetStatus("Base URL updated")
	a.conversation.AddMessage("system", fmt.Sprintf("Base URL updated for %s", provider), false)
	return nil
}

func (a *App) commandUsage(usage string) {
	a.statusBar.SetStatus("Command needs more information")
	a.conversation.AddMessage("system", "Usage: "+usage, false)
}

func (a *App) commandError(message string) {
	a.statusBar.SetStatus("Command not applied")
	a.conversation.AddMessage("assistant", "Error: "+message, true)
}
