package app

// Permission cards and project approval gating.
// Moved verbatim from internal/tui/app.go.

import (
	"github.com/iSundram/Automergent/internal/agent"
	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/tui/components"
)

func (a *App) requireProjectApproval(projectPath string) {
	a.pendingProjectPath = projectPath
}

func permissionInfoForTool(tc ai.ToolCall, name string) components.PermissionInfo {
	info := components.PermissionInfo{
		Icon:   "●",
		Tool:   name,
		Action: "Requesting permission to use this tool",
		Risk:   "May change workspace or execute external operations",
	}
	add := func(label, value string) {
		if value == "" {
			return
		}
		info.Fields = append(info.Fields, components.PermissionField{Label: label, Value: value})
	}
	str := func(key string) string {
		value, _ := tc.Args[key].(string)
		return value
	}

	switch tc.Name {
	case "read_file", "view":
		info.Icon, info.Action, info.Risk = "▸", "Read file contents", "Reads workspace data"
		add("Path", str("path"))
	case "list_directory":
		info.Icon, info.Action, info.Risk = "▸", "Inspect directory structure", "Reads workspace metadata"
		add("Path", str("path"))
	case "run_shell_command", "bash":
		info.Icon, info.Action, info.Risk = "▲", "Execute shell command", "Runs a local process"
		if agent.CommandIsDangerous(tc.Name, tc.Args) {
			info.Risk = "▲ dangerous pattern — always-allow will match ONLY this exact command"
			info.Dangerous = true
		}
		add("Command", str("command"))
		add("Directory", str("working_directory"))
		add("Always grants", agent.ShellGrantPreview(tc.Name, tc.Args))
	case "web_fetch", "web_search":
		info.Icon, info.Action, info.Risk = "→", "Access web resource", "Sends a network request"
		add("URL", str("url"))
		add("Query", str("query"))
	case "git_commit":
		info.Icon, info.Action, info.Risk = "⎿", "Create git commit", "Changes repository history"
		add("Message", str("message"))
	default:
		if ctx := extractToolContext(tc.Name, tc.Args); ctx != "" {
			add("Context", ctx)
		}
	}
	return info
}

// initActionArgs builds representative tool arguments for an init-phase event
// so the rendered card shows the same Path/Pattern/Command fields a native
// tool call would.

// bridgeConfirmation adapts the UI-layer confirmation channel into the
// agent-layer ConfirmationResponse channel.
func bridgeConfirmation(dst chan agent.ConfirmationResponse) chan components.Confirmation {
	src := make(chan components.Confirmation, 1)
	go func() {
		res, ok := <-src
		if !ok {
			return
		}
		select {
		case dst <- agent.ConfirmationResponse{Allow: res.Allow, Always: res.Always, Feedback: res.Feedback}:
		default:
		}
	}()
	return src
}
