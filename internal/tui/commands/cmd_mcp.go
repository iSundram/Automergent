package commands

import (
	"fmt"
	"strings"

	"github.com/iSundram/Automergent/internal/tui/components"
)

// /mcp — manage MCP servers, tools, resources, and prompts.

func mcpCommand() Command {
	return Command{
		Name:          "mcp",
		Description:   "Manage MCP servers, tools, resources, and prompts",
		Category:      "MCP",
		Icon:          "󰌠",
		ArgsHint:      "[sub-command]",
		SubPalette:    "mcp",
		Tier:          TierSecondary,
		Type:          CmdFullPage,
		FullPageTitle: "MCP Servers",
		Page:          mcpPage,
		SubCommands: []SubCommand{
			{Name: "list", Description: "List all MCP servers", Handler: handleMCP},
			{Name: "enable", Description: "Enable an MCP server", ArgsHint: "<name>", Handler: handleMCP, ValueCompletion: mcpServerNameCompletion},
			{Name: "disable", Description: "Disable an MCP server", ArgsHint: "<name>", Handler: handleMCP, ValueCompletion: mcpServerNameCompletion},
			{Name: "reconnect", Description: "Reconnect MCP servers", Handler: handleMCP, ValueCompletion: mcpServerNameCompletion},
			{Name: "tools", Description: "List MCP tools", Handler: handleMCP, ValueCompletion: mcpServerNameCompletion},
			{Name: "refresh", Description: "Refresh MCP tools and resources", Handler: handleMCP},
			{Name: "tools", Description: "List MCP tools", Handler: handleMCP},
			{Name: "resources", Description: "List MCP resources", Handler: handleMCP},
			{Name: "prompts", Description: "List MCP prompts", Handler: handleMCP},
			{Name: "events", Description: "Show MCP events", Handler: handleMCP},
			{Name: "cache", Description: "Manage MCP cache", Handler: handleMCP},
		},
		SupportsHeadless: true,
	}
}

func handleMCP(h Host, args []string) Result {
	if len(args) == 0 {
		return handleMCPStatus(h)
	}
	switch args[0] {
	case "tools":
		server := ""
		if len(args) > 1 {
			server = args[1]
		}
		return handleMCPTools(h, server)
	case "resources":
		return handleMCPResources(h)
	case "prompts":
		return handleMCPPrompts(h)
	case "enable":
		if len(args) < 2 {
			return Result{Text: "Usage: /mcp enable <server>"}
		}
		if err := h.MCPEnable(args[1]); err != nil {
			return Result{Text: fmt.Sprintf("Error: %v", err)}
		}
		return Result{Text: fmt.Sprintf("Server %q enabled", args[1])}
	case "disable":
		if len(args) < 2 {
			return Result{Text: "Usage: /mcp disable <server>"}
		}
		if err := h.MCPDisable(args[1]); err != nil {
			return Result{Text: fmt.Sprintf("Error: %v", err)}
		}
		return Result{Text: fmt.Sprintf("Server %q disabled", args[1])}
	case "reconnect":
		name := ""
		if len(args) > 1 {
			name = args[1]
		}
		if err := h.MCPReconnect(name); err != nil {
			return Result{Text: fmt.Sprintf("Error: %v", err)}
		}
		return Result{Text: "Reconnect initiated"}
	case "refresh":
		name := ""
		if len(args) > 1 {
			name = args[1]
		}
		if err := h.MCPRefresh(name); err != nil {
			return Result{Text: fmt.Sprintf("Error: %v", err)}
		}
		return Result{Text: "Refresh initiated"}
	case "events":
		events := h.MCPEvents()
		if len(events) == 0 {
			return Result{Text: "No recent MCP events."}
		}
		var sb strings.Builder
		sb.WriteString("Recent MCP Events:\n")
		for _, ev := range events {
			sb.WriteString(fmt.Sprintf("  [%s] %s: %s", ev.Timestamp, ev.Type, ev.Server))
			if ev.Message != "" {
				sb.WriteString(" - " + ev.Message)
			}
			if ev.Error != "" {
				sb.WriteString(" ERROR: " + ev.Error)
			}
			sb.WriteString("\n")
		}
		return Result{Text: sb.String()}
	case "cache":
		h.MCPDeleteToolCache("*")
		return Result{Text: "MCP tool cache invalidated"}
	case "add":
		if len(args) < 3 {
			return Result{Text: "Usage: /mcp add <name> <http|stdio|sse> <url|command> [args...]"}
		}
		name := args[1]
		transport := args[2]
		urlOrCmd := args[3]
		var cmdArgs []string
		if len(args) > 4 {
			cmdArgs = args[4:]
		}
		if err := h.MCPAddServer(name, transport, urlOrCmd, cmdArgs); err != nil {
			return Result{Text: fmt.Sprintf("Error: %v", err)}
		}
		return Result{Text: fmt.Sprintf("Server %q added and connecting", name)}
	case "remove":
		if len(args) < 2 {
			return Result{Text: "Usage: /mcp remove <server>"}
		}
		if err := h.MCPRemoveServer(args[1]); err != nil {
			return Result{Text: fmt.Sprintf("Error: %v", err)}
		}
		return Result{Text: fmt.Sprintf("Server %q removed", args[1])}
	case "help":
		return Result{Text: mcpHelp}
	default:
		return Result{Text: fmt.Sprintf("Unknown subcommand: %s\n\n%s", args[0], mcpHelp)}
	}
}

func handleMCPStatus(h Host) Result {
	statuses := h.MCPStatus()
	if len(statuses) == 0 {
		return Result{Text: "No MCP servers configured.\n\nAdd servers to your config under mcp.servers to enable MCP."}
	}
	var sb strings.Builder
	sb.WriteString("MCP Servers:\n\n")
	for _, s := range statuses {
		icon := "○"
		if s.Connected {
			icon = "●"
		}
		sb.WriteString(fmt.Sprintf("  %s %-20s  %-8s  v%-8s\n", icon, s.Name, s.Transport, s.Version))
		sb.WriteString(fmt.Sprintf("    tools=%d  resources=%d  prompts=%d  latency=%s\n", s.Tools, s.Resources, s.Prompts, s.Latency))
		if s.LastError != "" {
			sb.WriteString(fmt.Sprintf("    last_error: %s\n", s.LastError))
		}
		sb.WriteString("\n")
	}
	return Result{Text: sb.String()}
}

// mcpPage builds the structured /mcp status page: one flagged row per server
// plus capability counts and inventory sections, with actions for the common
// sub-commands.
func mcpPage(h Host) components.Page {
	statuses := h.MCPStatus()
	page := components.Page{Title: "MCP Servers"}

	if len(statuses) == 0 {
		page.Subtitle = "No servers configured"
		page.Sections = append(page.Sections, components.PageSection{
			Lines: []string{
				"No MCP servers configured.",
				"Add servers to your config under mcp.servers to enable MCP,",
				"or add one at runtime with /mcp add <name> <http|stdio|sse> <url|command>.",
			},
		})
		page.Actions = []components.PageAction{
			{Key: "h", Label: "Help", Command: "mcp", Args: []string{"help"}},
		}
		return page
	}

	servers := components.PageSection{Heading: "Servers"}
	for _, s := range statuses {
		status := components.PageStatusFail
		switch {
		case s.Connected:
			status = components.PageStatusOK
		case s.LastError != "":
			status = components.PageStatusFail
		default:
			status = components.PageStatusWarn
		}
		detail := fmt.Sprintf("%s · v%s · %d tools, %d resources, %d prompts · %s",
			s.Transport, s.Version, s.Tools, s.Resources, s.Prompts, s.Latency)
		if s.LastError != "" {
			detail += " · last error: " + s.LastError
		}
		servers.Flagged = append(servers.Flagged, components.PageFlag{
			Label: s.Name, Detail: detail, Status: status,
		})
	}
	page.Sections = append(page.Sections, servers)

	if events := h.MCPEvents(); len(events) > 0 {
		sec := components.PageSection{Heading: "Recent Events"}
		for _, ev := range events {
			line := fmt.Sprintf("[%s] %s: %s", ev.Timestamp, ev.Type, ev.Server)
			if ev.Message != "" {
				line += " — " + ev.Message
			}
			if ev.Error != "" {
				line += " ERROR: " + ev.Error
			}
			sec.Lines = append(sec.Lines, line)
		}
		page.Sections = append(page.Sections, sec)
	}

	page.Actions = []components.PageAction{
		{Key: "t", Label: "Tools", Command: "mcp", Args: []string{"tools"}},
		{Key: "r", Label: "Resources", Command: "mcp", Args: []string{"resources"}},
		{Key: "p", Label: "Prompts", Command: "mcp", Args: []string{"prompts"}},
		{Key: "c", Label: "Reconnect", Command: "mcp", Args: []string{"reconnect"}},
		{Key: "f", Label: "Refresh", Command: "mcp", Args: []string{"refresh"}},
		{Key: "e", Label: "Events", Command: "mcp", Args: []string{"events"}},
	}
	return page
}

func handleMCPTools(h Host, server string) Result {
	tools := h.MCPTools(server)
	if len(tools) == 0 {
		return Result{Text: "No MCP tools available."}
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("MCP Tools (%d):\n\n", len(tools)))
	for _, t := range tools {
		flags := ""
		if t.ReadOnly {
			flags += " [readonly]"
		}
		if t.Destructive {
			flags += " [destructive]"
		}
		sb.WriteString(fmt.Sprintf("  %s\n    server: %s%s\n", t.Name, t.Server, flags))
		if t.Description != "" {
			sb.WriteString(fmt.Sprintf("    %s\n", t.Description))
		}
	}
	return Result{Text: sb.String()}
}

func handleMCPResources(h Host) Result {
	resources := h.MCPResources()
	if len(resources) == 0 {
		return Result{Text: "No MCP resources available."}
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("MCP Resources (%d):\n\n", len(resources)))
	for _, r := range resources {
		sb.WriteString(fmt.Sprintf("  %s\n    server: %s  mime: %s\n", r.URI, r.Server, r.MimeType))
		if r.Description != "" {
			sb.WriteString(fmt.Sprintf("    %s\n", r.Description))
		}
	}
	return Result{Text: sb.String()}
}

func handleMCPPrompts(h Host) Result {
	prompts := h.MCPPrompts()
	if len(prompts) == 0 {
		return Result{Text: "No MCP prompts available."}
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("MCP Prompts (%d):\n\n", len(prompts)))
	for _, p := range prompts {
		sb.WriteString(fmt.Sprintf("  %s\n    server: %s\n", p.Name, p.Server))
		if p.Description != "" {
			sb.WriteString(fmt.Sprintf("    %s\n", p.Description))
		}
	}
	return Result{Text: sb.String()}
}

const mcpHelp = `/mcp            - Show MCP server status
/mcp tools      - List MCP tools
/mcp resources  - List MCP resources
/mcp prompts    - List MCP prompts
/mcp enable <s> - Enable a server
/mcp disable <s>- Disable a server
/mcp reconnect  - Reconnect to servers
/mcp refresh    - Re-discover tools
/mcp events     - Show recent MCP events
/mcp cache      - Invalidate tool cache
/mcp add <n> <t> <u> [args] - Add a server
/mcp remove <s> - Remove a server`

// mcpServerNameCompletion offers every configured MCP server's name.
func mcpServerNameCompletion(h Host, _ string) []string {
	var names []string
	for _, s := range h.MCPStatus() {
		names = append(names, s.Name)
	}
	return names
}
