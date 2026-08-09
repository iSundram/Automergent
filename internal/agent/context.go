package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/tools"
	"github.com/iSundram/Automergent/internal/version"
)

// buildSystemPrompt constructs the system prompt for the AI.
func buildSystemPrompt(cfg *config.Config, reg *tools.Registry) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("You are Automergent %s, an AI coding agent for the terminal. ", version.Version))
	sb.WriteString("You help users with coding tasks: reading, writing, refactoring, debugging, ")
	sb.WriteString("testing, documenting, and explaining code.\n\n")

	sb.WriteString("## Capabilities\n")
	sb.WriteString("- Read and write files with read_file, write_file, patch_file\n")
	sb.WriteString("- Search codebases with grep\n")
	sb.WriteString("- Execute shell commands with run_command\n")
	sb.WriteString("- Browse directories with list_directory\n")
	sb.WriteString("- View git history, status, diffs with git tools\n")
	sb.WriteString("- Fetch web pages and search the web\n\n")

	sb.WriteString("## Behavior Guidelines\n")
	sb.WriteString("- Always read files before editing them — never guess at content\n")
	sb.WriteString("- Make precise, surgical changes; avoid unnecessary rewrites\n")
	sb.WriteString("- Explain what you're doing and why when making changes\n")
	sb.WriteString("- Run tests after making changes when a test suite exists\n")
	sb.WriteString("- Prefer using existing tools/libraries over introducing new dependencies\n")
	sb.WriteString("- If you encounter an error, diagnose it carefully before trying again\n\n")

	sb.WriteString("## Tool Calling Guidelines\n")
	sb.WriteString("- Use tools for every external action instead of claiming you did something\n")
	sb.WriteString("- For tool calls, first reason briefly, then call the most relevant tool\n")
	sb.WriteString("- After a tool result returns, incorporate the actual output before next steps\n")
	sb.WriteString("- If a tool fails, surface the error and either retry with a fix or ask user guidance\n")
	sb.WriteString("- Do not invent tool outputs or pretend a command succeeded\n\n")

	if reg != nil {
		sb.WriteString("## Available Tools\n")
		for _, line := range toolLines(reg) {
			sb.WriteString("- " + line + "\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("## Approval Mode: %s\n", cfg.Mode))
	modeDesc := modeDescription(cfg.Mode)
	if modeDesc != "" {
		sb.WriteString(modeDesc + "\n")
	}
	sb.WriteString("\n")

	// Add working directory context
	cwd, err := os.Getwd()
	if err == nil {
		sb.WriteString(fmt.Sprintf("Working directory: %s\n\n", cwd))
	}

	// Load AUTOMERGENT.md if present
	automergentMD := loadContextFile(cwd)
	if automergentMD != "" {
		sb.WriteString("## Project Context (AUTOMERGENT.md)\n")
		sb.WriteString(automergentMD)
		sb.WriteString("\n\n")
	}

	// Load extra context files
	for _, p := range cfg.ContextFiles {
		data, err := os.ReadFile(p)
		if err == nil {
			sb.WriteString(fmt.Sprintf("## %s\n", filepath.Base(p)))
			sb.Write(data)
			sb.WriteString("\n\n")
		}
	}

	return sb.String()
}

func modeDescription(mode string) string {
	switch mode {
	case "edit":
		return "In edit mode, automatically read files, but ask for confirmation before writing files or running shell commands."
	case "plan":
		return "In plan mode, create a detailed plan of the changes to be made, then ask for approval before executing."
	default:
		return ""
	}
}

func loadContextFile(dir string) string {
	for _, name := range []string{"AUTOMERGENT.md", ".automergent.md"} {
		current := dir
		for {
			path := filepath.Join(current, name)
			data, err := os.ReadFile(path)
			if err == nil {
				return string(data)
			}
			parent := filepath.Dir(current)
			if parent == current {
				break
			}
			current = parent
		}
	}
	return ""
}

func toolLines(reg *tools.Registry) []string {
	toolsList := reg.All()
	lines := make([]string, 0, len(toolsList))
	for _, t := range toolsList {
		lines = append(lines, fmt.Sprintf("%s: %s", t.Name(), t.Description()))
	}
	sort.Strings(lines)
	return lines
}
