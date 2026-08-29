package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// /init — create AUTOMERGENT.md project memory.

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

func initCommand() Command {
	return Command{
		Name:             "init",
		Description:      "Create AUTOMERGENT.md project memory",
		Category:         "Project",
		Icon:             "󰚝",
		Tier:             TierPrimary,
		Type:             CmdPrompt,
		Immediate:        true,
		SupportsHeadless: true,
		WhenToUse:        "When a project has no AUTOMERGENT.md yet",
		PromptTemplate:   "Initialize this project by creating an AUTOMERGENT.md file. Analyze the project structure, dependencies, and conventions. Write a comprehensive guide for AI assistants working on this codebase. Include: project overview, build system, test commands, coding style, key files and directories, and common patterns.",
	}
}

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
