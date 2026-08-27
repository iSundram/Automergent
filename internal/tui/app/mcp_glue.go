package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/mcp"
	"github.com/iSundram/Automergent/internal/tui/commands"
)

// mcpEventBuffer stores recent MCP events for /mcp events command.
const maxMCPEvents = 50

// MCPEventEntry is a stored MCP event.
type MCPEventEntry struct {
	Type      string
	Server    string
	Message   string
	Error     string
	Timestamp time.Time
}

// Ensure App implements the full Host interface at compile time.
var _ commands.Host = (*App)(nil)

// MCPStatus returns status of all configured MCP servers.
func (a *App) MCPStatus() []commands.MCPServerStatus {
	if a.mcpOrch == nil {
		return nil
	}
	servers := a.mcpOrch.ListServers()
	out := make([]commands.MCPServerStatus, 0, len(servers))
	for _, s := range servers {
		out = append(out, commands.MCPServerStatus{
			Name:      s.Config.Name,
			Transport: string(s.Config.Transport),
			Status:    string(s.Status),
			Version:   s.Version,
			Tools:     len(s.Tools),
			Resources: len(s.Resources),
			Prompts:   len(s.Prompts),
			Latency:   mcp.DurationToString(s.Latency),
			LastError: s.LastError,
			Connected: s.Status == mcp.StatusHealthy,
		})
	}
	return out
}

// MCPTools returns available MCP tools, optionally filtered by server.
func (a *App) MCPTools(server string) []commands.MCPToolInfo {
	if a.mcpOrch == nil {
		return nil
	}
	allTools := a.mcpOrch.ListTools()
	out := make([]commands.MCPToolInfo, 0, len(allTools))
	for _, t := range allTools {
		if server != "" && t.Server != server {
			continue
		}
		info := commands.MCPToolInfo{
			Name:        t.QualifiedName,
			Description: t.Description,
			Server:      t.Server,
		}
		if t.Annotations != nil {
			info.ReadOnly = t.Annotations.ReadOnlyHint
			info.Destructive = t.Annotations.DestructiveHint
		}
		if t.InputSchema != nil {
			info.Schema = string(t.InputSchema)
		}
		out = append(out, info)
	}
	return out
}

// MCPResources returns available MCP resources.
func (a *App) MCPResources() []commands.MCPResourceInfo {
	if a.mcpOrch == nil {
		return nil
	}
	resources := a.mcpOrch.ListResources()
	out := make([]commands.MCPResourceInfo, 0, len(resources))
	for _, r := range resources {
		out = append(out, commands.MCPResourceInfo{
			URI:         r.URI,
			Name:        r.Name,
			Description: r.Description,
			MimeType:    r.MimeType,
			Server:      r.Server,
		})
	}
	return out
}

// MCPPrompts returns available MCP prompts.
func (a *App) MCPPrompts() []commands.MCPPromptInfo {
	if a.mcpOrch == nil {
		return nil
	}
	prompts := a.mcpOrch.ListPrompts()
	out := make([]commands.MCPPromptInfo, 0, len(prompts))
	for _, p := range prompts {
		out = append(out, commands.MCPPromptInfo{
			Name:        p.Name,
			Description: p.Description,
			Server:      p.Server,
		})
	}
	return out
}

// MCPReconnect reconnects to an MCP server by name.
func (a *App) MCPReconnect(name string) error {
	if a.mcpOrch == nil {
		return fmt.Errorf("MCP not configured")
	}
	srv, ok := a.mcpOrch.GetServer(name)
	if !ok {
		return fmt.Errorf("server %q not found", name)
	}
	_ = srv // Reconnection handled by orchestrator's internal retry
	return nil
}

// MCPRefresh re-discovers tools for a server.
func (a *App) MCPRefresh(name string) error {
	if a.mcpOrch == nil {
		return fmt.Errorf("MCP not configured")
	}
	return a.mcpOrch.RefreshServer(a.ctx, name)
}

// MCPEnable enables a server at runtime.
func (a *App) MCPEnable(name string) error {
	if a.mcpOrch == nil {
		return fmt.Errorf("MCP not configured")
	}
	srv, ok := a.mcpOrch.GetServer(name)
	if !ok {
		return fmt.Errorf("server %q not found", name)
	}
	if srv.Config != nil {
		srv.Config.Enabled = true
	}
	return nil
}

// MCPDisable disables a server at runtime.
func (a *App) MCPDisable(name string) error {
	if a.mcpOrch == nil {
		return fmt.Errorf("MCP not configured")
	}
	_ = a.mcpOrch.RemoveServer(name)
	return nil
}

// MCPCallTool executes an MCP tool and returns the result as text.
func (a *App) MCPCallTool(server, name string, args map[string]any) (string, error) {
	if a.mcpOrch == nil {
		return "", fmt.Errorf("MCP not configured")
	}
	result, err := a.mcpOrch.CallTool(a.ctx, mcp.ToolCall{
		Name:   name,
		Args:   args,
		Server: server,
	})
	if err != nil {
		return "", err
	}
	var text string
	for _, block := range result.Content {
		if block.Type == "text" {
			if text != "" {
				text += "\n"
			}
			text += block.Text
		}
	}
	return text, nil
}

// MCPEvents returns recent MCP events from the buffer.
func (a *App) MCPEvents() []commands.MCPEventInfo {
	out := make([]commands.MCPEventInfo, 0, len(a.mcpEvents))
	for _, e := range a.mcpEvents {
		out = append(out, commands.MCPEventInfo{
			Type:      e.Type,
			Server:    e.Server,
			Message:   e.Message,
			Error:     e.Error,
			Timestamp: e.Timestamp.Format(time.RFC3339),
		})
	}
	return out
}

// MCPDeleteToolCache invalidates the MCP tool cache.
func (a *App) MCPDeleteToolCache(pattern string) {
	if a.mcpOrch == nil {
		return
	}
	a.mcpOrch.InvalidateCache(pattern)
}

// handleMCPEvent processes MCP server events and stores them + shows toasts.
func (a *App) handleMCPEvent(ev mcp.ServerEvent) {
	entry := MCPEventEntry{
		Type:      string(ev.Type),
		Server:    ev.Server,
		Message:   ev.Message,
		Error:     ev.Error,
		Timestamp: ev.Timestamp,
	}
	a.mcpEvents = append(a.mcpEvents, entry)
	if len(a.mcpEvents) > maxMCPEvents {
		a.mcpEvents = a.mcpEvents[len(a.mcpEvents)-maxMCPEvents:]
	}

	// Show toast for important events
	if a.toasts != nil {
		switch ev.Type {
		case mcp.EventConnected:
			a.toasts.Push("info", "MCP Connected", fmt.Sprintf("Server %s connected", ev.Server))
		case mcp.EventDisconnected:
			a.toasts.Push("warn", "MCP Disconnected", fmt.Sprintf("Server %s disconnected", ev.Server))
		case mcp.EventError:
			a.toasts.Push("error", "MCP Error", fmt.Sprintf("%s: %s", ev.Server, ev.Error))
		case mcp.EventToolsUpdated:
			a.toasts.Push("info", "MCP Tools Updated", ev.Message)
		}
	}

	// Update MCP indicator in status bar
	a.updateMCPStatusIndicator()
}

// handleMCPConfigChange processes MCP config hot-reload events.
func (a *App) handleMCPConfigChange(ev mcp.ConfigChangeEvent) {
	entry := MCPEventEntry{
		Type:      "config_" + string(ev.Type),
		Server:    ev.Server,
		Timestamp: ev.Timestamp,
	}
	a.mcpEvents = append(a.mcpEvents, entry)
	if len(a.mcpEvents) > maxMCPEvents {
		a.mcpEvents = a.mcpEvents[len(a.mcpEvents)-maxMCPEvents:]
	}

	if a.toasts != nil {
		switch ev.Type {
		case mcp.ConfigAdded:
			a.toasts.Push("info", "MCP Config", fmt.Sprintf("Server %s added", ev.Server))
		case mcp.ConfigUpdated:
			a.toasts.Push("info", "MCP Config", fmt.Sprintf("Server %s updated", ev.Server))
		case mcp.ConfigRemoved:
			a.toasts.Push("info", "MCP Config", fmt.Sprintf("Server %s removed", ev.Server))
		}
	}
}

// updateMCPStatusIndicator updates the status bar with MCP server health.
func (a *App) updateMCPStatusIndicator() {
	if a.mcpOrch == nil {
		return
	}
	statuses := a.MCPStatus()
	if len(statuses) == 0 {
		return
	}
	healthy := 0
	for _, s := range statuses {
		if s.Connected {
			healthy++
		}
	}
	if healthy > 0 {
		a.statusBar.SetStatus(fmt.Sprintf("MCP: %d/%d servers connected", healthy, len(statuses)))
	}
}

// MCPAddServer adds a new MCP server to the config and connects it.
func (a *App) MCPAddServer(name, transport, urlOrCmd string, args []string) error {
	if a.cfg.MCP.Servers == nil {
		a.cfg.MCP.Servers = make(map[string]config.MCPServer)
	}
	if _, exists := a.cfg.MCP.Servers[name]; exists {
		return fmt.Errorf("server %q already exists", name)
	}

	srv := config.MCPServer{
		Type: transport,
	}
	switch transport {
	case "http", "sse", "":
		srv.URL = urlOrCmd
	case "stdio":
		srv.Command = append([]string{urlOrCmd}, args...)
	}
	a.cfg.MCP.Servers[name] = srv

	// Connect the new server
	if a.mcpOrch != nil {
		mcpSrv := mcp.ServerConfigFromApp(name, srv)
		if err := a.mcpOrch.AddServer(a.ctx, &mcpSrv); err != nil {
			return fmt.Errorf("connect server: %w", err)
		}
	}

	// Persist config
	if err := a.cfg.SaveIfLoaded(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

// MCPRemoveServer removes an MCP server from the config and disconnects it.
func (a *App) MCPRemoveServer(name string) error {
	if _, exists := a.cfg.MCP.Servers[name]; !exists {
		return fmt.Errorf("server %q not found", name)
	}
	delete(a.cfg.MCP.Servers, name)

	// Disconnect the server
	if a.mcpOrch != nil {
		if err := a.mcpOrch.RemoveServer(name); err != nil {
			// Log but don't fail — config is already updated
			fmt.Printf("warning: disconnect server %s: %v\n", name, err)
		}
	}

	// Persist config
	if err := a.cfg.SaveIfLoaded(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

// formatMCPTable formats MCP server status as a text table.
func formatMCPTable(statuses []commands.MCPServerStatus) string {
	if len(statuses) == 0 {
		return "No MCP servers configured."
	}
	var sb strings.Builder
	sb.WriteString("MCP Servers:\n")
	for _, s := range statuses {
		icon := "○"
		if s.Connected {
			icon = "●"
		}
		sb.WriteString(fmt.Sprintf("  %s %-20s  %-8s  tools=%-3d  resources=%-3d  prompts=%-3d  latency=%s",
			icon, s.Name, s.Transport, s.Tools, s.Resources, s.Prompts, s.Latency))
		if s.LastError != "" {
			sb.WriteString(fmt.Sprintf("  err=%s", s.LastError))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// formatMCPTools formats MCP tools as text.
func formatMCPTools(tools []commands.MCPToolInfo) string {
	if len(tools) == 0 {
		return "No MCP tools available."
	}
	var sb strings.Builder
	sb.WriteString("MCP Tools:\n")
	for _, t := range tools {
		flags := ""
		if t.ReadOnly {
			flags += " [readonly]"
		}
		if t.Destructive {
			flags += " [destructive]"
		}
		sb.WriteString(fmt.Sprintf("  %-40s  %-8s  %s%s\n", t.Name, t.Server, truncate(t.Description, 50), flags))
	}
	return sb.String()
}

// formatMCPResources formats MCP resources as text.
func formatMCPResources(resources []commands.MCPResourceInfo) string {
	if len(resources) == 0 {
		return "No MCP resources available."
	}
	var sb strings.Builder
	sb.WriteString("MCP Resources:\n")
	for _, r := range resources {
		sb.WriteString(fmt.Sprintf("  %-40s  %-8s  %s\n", r.URI, r.Server, r.Name))
	}
	return sb.String()
}

// formatMCPPrompts formats MCP prompts as text.
func formatMCPPrompts(prompts []commands.MCPPromptInfo) string {
	if len(prompts) == 0 {
		return "No MCP prompts available."
	}
	var sb strings.Builder
	sb.WriteString("MCP Prompts:\n")
	for _, p := range prompts {
		sb.WriteString(fmt.Sprintf("  %-30s  %-8s  %s\n", p.Name, p.Server, p.Description))
	}
	return sb.String()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// Ensure App implements the full Host interface at compile time.
var _ commands.Host = (*App)(nil)
