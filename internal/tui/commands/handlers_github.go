package commands

import (
	"strings"
)

// --- Prompt-type review & GitHub handlers: /summary /security-review
// /issue /pr-comments ---

func handleSummary(host Host, args []string) Result {
	var b strings.Builder
	b.WriteString("Summarize this session so far for the user.\n")
	b.WriteString("Cover: 1) the goals pursued, 2) changes made (files and why), 3) key decisions and their rationale, 4) open items and risks, 5) suggested next steps.\n")
	b.WriteString("Be factual — only claim what is visible in this conversation. Keep it under 300 words.")
	if focus := strings.TrimSpace(strings.Join(args, " ")); focus != "" {
		b.WriteString("\nEmphasis: " + focus)
	}
	host.SetStatus("Preparing summary")
	return Done(host.StartAgent(b.String()))
}

func handleSecurityReview(host Host, args []string) Result {
	var b strings.Builder
	b.WriteString("Perform a security-focused review of the pending workspace changes.\n")
	b.WriteString("1. Inspect `git diff` (staged and unstaged).\n")
	b.WriteString("2. Run the secrets scan and dependency audit tools if available.\n")
	b.WriteString("3. Hunt specifically for: hardcoded credentials/tokens, injection vectors (SQL, shell, path traversal), unsafe deserialization, missing auth checks, insecure crypto, and secrets in new config files.\n")
	b.WriteString("4. Report findings by severity: critical, high, medium, low. For each: file:line, issue, exploit scenario, concrete fix.\n")
	b.WriteString("Do not modify any files unless explicitly asked.")
	if focus := strings.TrimSpace(strings.Join(args, " ")); focus != "" {
		b.WriteString("\nFocus: " + focus)
	}
	host.SetStatus("Preparing security review")
	return Done(host.StartAgent(b.String()))
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
