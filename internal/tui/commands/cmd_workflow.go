package commands

import (
	"fmt"
	"strings"
)

// /workflow — run declarative multi-agent pipelines from .automergent/workflows/.

func workflowCommand() Command {
	return Command{
		Name:        "workflow",
		Aliases:     []string{"workflows"},
		Description: "Run a multi-agent pipeline",
		Category:    "Workflow",
		Icon:        "󰫢",
		ArgsHint:    "[list|run <name> [args...]|resume <run-id>|history]",
		Tier:        TierSecondary,
		SubPalette:  "workflow",
		SubCommands: []SubCommand{
			{Name: "list", Description: "List available workflow specs", Handler: handleWorkflow},
			{Name: "run", Description: "Run a workflow", ArgsHint: "<name> [args...]", Handler: handleWorkflow},
			{Name: "resume", Description: "Resume an interrupted run", ArgsHint: "<run-id>", Handler: handleWorkflow},
			{Name: "history", Description: "Show recent runs", Handler: handleWorkflow},
		},
		Completion: func(h Host, partial string) []string {
			var names []string
			for _, s := range h.WorkflowSpecs() {
				names = append(names, s.Name)
			}
			return prefixFilter(names, partial)
		},
	}
}

func handleWorkflow(host Host, args []string) Result {
	if len(args) == 0 {
		return workflowList(host)
	}

	switch args[0] {
	case "list":
		return workflowList(host)
	case "run":
		if len(args) < 2 {
			host.CommandUsage("/workflow run <name> [args...]")
			return Done(nil)
		}
		specs := host.WorkflowSpecs()
		spec, ok := findWorkflowSpec(specs, args[1])
		if !ok {
			host.CommandError(fmt.Sprintf("no workflow named %q — /workflow list shows what is available", args[1]))
			return Done(nil)
		}
		if err := host.RunWorkflow(spec.Path, args[2:], false); err != nil {
			host.CommandError(fmt.Sprintf("workflow run failed to start: %v", err))
			return Done(nil)
		}
		host.AddSystemMessage(fmt.Sprintf("Workflow %q started — steps and results will arrive as messages.", spec.Name))
		host.SetStatus("Workflow started")
		return Done(nil)
	case "resume":
		if len(args) < 2 {
			host.CommandUsage("/workflow resume <run-id>")
			return Done(nil)
		}
		// A resume needs the spec the run came from; find it in history.
		for _, run := range host.WorkflowRunHistory() {
			if run.RunID == args[1] {
				if err := host.RunWorkflow(run.Name, nil, true); err != nil {
					host.CommandError(fmt.Sprintf("resume failed to start: %v", err))
					return Done(nil)
				}
				host.AddSystemMessage(fmt.Sprintf("Resuming run %s — unchanged steps replay from the journal.", args[1]))
				host.SetStatus("Workflow resumed")
				return Done(nil)
			}
		}
		host.CommandError(fmt.Sprintf("no run %q in history", args[1]))
		return Done(nil)
	case "history":
		runs := host.WorkflowRunHistory()
		if len(runs) == 0 {
			host.AddSystemMessage("No workflow runs yet.")
			return Done(nil)
		}
		var b strings.Builder
		b.WriteString("Workflow runs:\n")
		for _, r := range runs {
			fmt.Fprintf(&b, "  %-28s %-10s %d steps", r.RunID, r.Status, r.Steps)
			if r.Detail != "" {
				b.WriteString(" — " + r.Detail)
			}
			b.WriteString("\n")
		}
		host.AddSystemMessage(b.String())
		return Done(nil)
	}

	host.CommandUsage("/workflow [list|run <name>|resume <run-id>|history]")
	return Done(nil)
}

func workflowList(host Host) Result {
	specs := host.WorkflowSpecs()
	if len(specs) == 0 {
		host.AddSystemMessage("No workflows found. Add YAML specs under .automergent/workflows/.\n\nExample:\n  name: check\n  steps:\n    - id: build\n      prompt: Run the build and report failures.\n    - id: test\n      prompt: Run tests, focusing on what ${build} found.\n      dependsOn: [build]")
		return Done(nil)
	}
	var b strings.Builder
	b.WriteString("Available workflows:\n")
	for _, s := range specs {
		fmt.Fprintf(&b, "  %-20s %s\n", s.Name, s.Description)
	}
	b.WriteString("\nRun with /workflow run <name>")
	host.AddSystemMessage(b.String())
	return Done(nil)
}

func findWorkflowSpec(specs []WorkflowSpecInfo, name string) (WorkflowSpecInfo, bool) {
	for _, s := range specs {
		if s.Name == name {
			return s, true
		}
	}
	return WorkflowSpecInfo{}, false
}
