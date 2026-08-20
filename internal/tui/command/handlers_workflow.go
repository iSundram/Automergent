package command

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// --- Workflow Handlers ---

func handleRun(host Host, args []string) tea.Cmd {
	if len(args) == 0 {
		host.CommandUsage("/run <command>")
		return nil
	}

	command := strings.TrimSpace(strings.Join(args, " "))
	host.SetStatus("Preparing command permission request")
	return host.StartAgent("Run the following project command using the shell tool. Request permission before execution: " + command)
}

func handleTest(host Host, args []string) tea.Cmd {
	request := "Detect the project's test command and run the test suite using the shell tool. Request permission before execution."
	if len(args) > 0 {
		request = "Run project tests for this target using the shell tool. Request permission before execution: " + strings.Join(args, " ")
	}
	host.SetStatus("Preparing test permission request")
	return host.StartAgent(request)
}

func handleBuild(host Host, args []string) tea.Cmd {
	request := "Detect the project's build command and build the project using the shell tool. Request permission before execution."
	if len(args) > 0 {
		request = "Build this project target using the shell tool. Request permission before execution: " + strings.Join(args, " ")
	}
	host.SetStatus("Preparing build permission request")
	return host.StartAgent(request)
}

func handleReview(host Host, args []string) tea.Cmd {
	host.ToggleReviewMode()
	host.SetStatus(fmt.Sprintf("Review mode %s", map[bool]string{true: "enabled", false: "disabled"}[host.IsReviewMode()]))
	return nil
}

func handleCancel(host Host, args []string) tea.Cmd {
	if !host.Thinking() {
		host.SetStatus("No active request to cancel")
		return nil
	}
	host.CancelActiveRun("Cancelled by user")
	return nil
}