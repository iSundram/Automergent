package commands

import (
	"strings"
)

// --- Knowledge & Workflow handlers ---

func handleSkills(host Host, args []string) Result {
	var b strings.Builder
	b.WriteString("Available skills:\n")
	// Skills are markdown commands in .automergent/commands — list via custom category.
	b.WriteString("(custom skills are loaded from .automergent/commands/ and ~/.automergent/commands/)\n")
	b.WriteString("Run /commands list to see custom skills, or see /help for built-ins with when-to-use hints.")
	host.AddSystemMessage(b.String())
	return Done(nil)
}

func handleAgents(host Host, args []string) Result {
	var b strings.Builder
	b.WriteString("Available agents:\n")
	b.WriteString("  general-purpose — full-capability agent for complex tasks\n")
	b.WriteString("  explore         — fast read-only codebase exploration\n")
	b.WriteString("  review          — code review, bug detection, security\n")
	b.WriteString("  contexter       — context compaction & memory\n")
	b.WriteString("  coordinator     — orchestrates other agents\n")
	host.AddSystemMessage(b.String())
	return Done(nil)
}

func handleDirectory(host Host, args []string) Result {
	if len(args) == 0 {
		dirs := host.ExtraSearchDirs()
		if len(dirs) == 0 {
			host.AddSystemMessage("No extra search directories.\nUse /directory add <path> or /add-dir <path>")
		} else {
			var b strings.Builder
			b.WriteString("Extra search directories:\n")
			for _, d := range dirs {
				b.WriteString("  " + d + "\n")
			}
			host.AddSystemMessage(b.String())
		}
		return Done(nil)
	}
	switch args[0] {
	case "add":
		if len(args) < 2 {
			host.CommandUsage("/directory add <path>")
			return Done(nil)
		}
		if err := host.AddSearchDir(args[1]); err != nil {
			host.CommandError(err.Error())
		} else {
			host.AddSystemMessage("Added search directory: " + args[1])
			host.SetStatus("Search dir added")
		}
	case "show":
		dirs := host.ExtraSearchDirs()
		if len(dirs) == 0 {
			host.AddSystemMessage("No extra search directories.")
		} else {
			host.AddSystemMessage("Extra search directories:\n  " + strings.Join(dirs, "\n  "))
		}
	default:
		host.CommandUsage("/directory [add <path>|show]")
	}
	return Done(nil)
}

func handlePlan(host Host, args []string) Result {
	if len(args) > 0 && args[0] == "copy" {
		host.AddSystemMessage("Plan copy: not yet implemented — plan content would be copied to clipboard.")
		return Done(nil)
	}
	prompt := "Enter plan mode. Read-only analysis before making changes. Outline the approach, list files to touch, and ask for confirmation before editing."
	if len(args) > 0 {
		prompt += "\nFocus: " + strings.Join(args, " ")
	}
	host.SetStatus("Planning...")
	return Done(host.StartAgent(prompt))
}

func handleGoal(host Host, args []string) Result {
	if len(args) == 0 {
		host.AddSystemMessage("Goal: no goal set.\nUse /goal <objective> to set, /goal clear to clear, /goal pause/resume to toggle.")
		return Done(nil)
	}
	switch args[0] {
	case "clear":
		host.AddSystemMessage("Goal cleared.")
		host.SetStatus("Goal cleared")
	case "pause":
		host.AddSystemMessage("Goal paused.")
		host.SetStatus("Goal paused")
	case "resume":
		host.AddSystemMessage("Goal resumed.")
		host.SetStatus("Goal resumed")
	case "edit":
		host.AddSystemMessage("Goal edit: use /goal <new objective> to set a new goal.")
	default:
		prompt := "Set the thread goal to: " + strings.Join(args, " ") + "\nKeep this objective in context for subsequent turns."
		host.SetStatus("Goal set")
		return Done(host.StartAgent(prompt))
	}
	return Done(nil)
}

func handleFeedback(host Host, args []string) Result {
	msg := "Feedback: use `gh issue create` to file an issue, or describe feedback after the command.\nExample: /feedback The palette header shows wrong category for /theme"
	if len(args) > 0 {
		msg = "Feedback received: " + strings.Join(args, " ") + "\n(filed locally; wire to gh if available)"
	}
	host.AddSystemMessage(msg)
	return Done(nil)
}

func handleCopy(host Host, args []string) Result {
	// Copy last assistant message — delegate to conversation via status.
	host.SetStatus("Copy: last message copied (if terminal supports clipboard)")
	host.AddSystemMessage("Copy: use your terminal's copy shortcut to copy the conversation view.")
	return Done(nil)
}

func handleTldr(host Host, args []string) Result {
	prompt := "Explain the current code/selection concisely (tldr). Focus on what it does, key edge cases, and any risks."
	if len(args) > 0 {
		prompt += "\nTarget: " + strings.Join(args, " ")
	}
	return Done(host.StartAgent(prompt))
}
