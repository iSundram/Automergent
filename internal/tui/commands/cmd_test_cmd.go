package commands

import (
	"strings"
)

// /test — detect and run project tests.
// /build — detect and build the project.
//
// Note: this file is named cmd_test_cmd.go rather than cmd_test.go because
// Go treats any *_test.go file as test-only and would exclude it from builds.

func testCommand() Command {
	return Command{
		Name:             "test",
		Description:      "Detect and run project tests",
		Category:         "Workflow",
		Icon:             "󰙨",
		ArgsHint:         "[target]",
		Tier:             TierPrimary,
		Type:             CmdPrompt,
		Immediate:        true,
		SupportsHeadless: true,
		WhenToUse:        "To verify changes against the project test suite",
		PromptTemplate:   "Detect the project's test command and run the test suite using the shell tool. Request permission before execution.$ARGUMENTS",
	}
}

func handleTest(host Host, args []string) Result {
	request := "Detect the project's test command and run the test suite using the shell tool. Request permission before execution."
	if len(args) > 0 {
		request = "Run project tests for this target using the shell tool. Request permission before execution: " + strings.Join(args, " ")
	}
	host.SetStatus("Preparing test permission request")
	return Done(host.StartAgent(request))
}

func buildCommand() Command {
	return Command{
		Name:             "build",
		Description:      "Detect and build the project",
		Category:         "Workflow",
		Icon:             "󰒋",
		ArgsHint:         "[target]",
		Tier:             TierSecondary,
		Type:             CmdPrompt,
		Immediate:        true,
		SupportsHeadless: true,
		PromptTemplate:   "Detect the project's build command and build the project using the shell tool. Request permission before execution.$ARGUMENTS",
	}
}

func handleBuild(host Host, args []string) Result {
	request := "Detect the project's build command and build the project using the shell tool. Request permission before execution."
	if len(args) > 0 {
		request = "Build this project target using the shell tool. Request permission before execution: " + strings.Join(args, " ")
	}
	host.SetStatus("Preparing build permission request")
	return Done(host.StartAgent(request))
}
