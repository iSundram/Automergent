package commands

import (
	"fmt"
	"strings"
)

// /mcp — manage MCP servers, tools, resources, and prompts.

func mcpCommand() Command {
	return Command{
		Name:        "mcp",
		Description: "Manage MCP servers, tools, resources, and prompts",
		Category:    "MCP",
		Icon:        "󰌠",
		ArgsHint:    "[sub-command]",
		Tier:        TierSecondary,
		SubCommands: []SubCommand{
			{Name: "list", Description: "List all MCP servers", Handler: handleMCP},
			{Name: "enable", Description: "Enable an MCP server", ArgsHint: "<name>", Handler: handleMCP},
			{Name: "disable", Description: "Disable an MCP server", ArgsHint: "<name>", Handler: handleMCP},
			{Name: "reconnect", Description: "Reconnect MCP servers", Handler: handleMCP},
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
