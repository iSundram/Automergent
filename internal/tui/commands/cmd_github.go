package commands

import (
	"strings"
)

// /issue — create a GitHub issue via gh.
// /pr-comments — fetch and summarize PR review comments.

func issueCommand() Command {
	return Command{
		Name:             "issue",
		Description:      "Create a GitHub issue via gh",
		Category:         "Workflow",
		Icon:             "󰿚",
		ArgsHint:         "<title>",
		Tier:             TierTertiary,
		SupportsHeadless: true,
	}
}

func handleIssue(host Host, args []string) Result {
	title := strings.TrimSpace(strings.Join(args, " "))
	if title == "" {
		host.CommandUsage("/issue <title>")
		return Done(nil)
	}
	var b strings.Builder
	b.WriteString("Create a GitHub issue using the `gh` CLI (request permission before running it).\n")
	b.WriteString("Title: \"" + title + "\"\n")
	b.WriteString("Draft a concise body from the current session context: problem statement, reproduction or motivation, acceptance criteria.\n")
	b.WriteString("If `gh` is unavailable or unauthenticated, say exactly what failed instead of retrying.")
	host.SetStatus("Preparing issue creation")
	return Done(host.StartAgent(b.String()))
}

func prCommentsCommand() Command {
	return Command{
		Name:             "pr-comments",
		Description:      "Fetch and summarize PR review comments",
		Category:         "Workflow",
		Icon:             "󰣏",
		ArgsHint:         "<PR number or URL>",
		Tier:             TierTertiary,
		SupportsHeadless: true,
	}
}

func handlePRComments(host Host, args []string) Result {
	ref := strings.TrimSpace(strings.Join(args, " "))
	if ref == "" {
		host.CommandUsage("/pr-comments <PR number or URL>")
		return Done(nil)
	}
	if !strings.HasPrefix(ref, "http") && looksLikePRRef(ref) && !strings.HasPrefix(ref, "#") {
		ref = "#" + ref
	}
	prompt := "Fetch the review comments for pull request " + ref + " using `gh pr view` / `gh api` (request permission before running commands).\n" +
		"Summarize each comment: reviewer, file/line if any, the request being made, and whether it appears addressed in the current working tree."
	host.SetStatus("Preparing PR comment fetch")
	return Done(host.StartAgent(prompt))
}
