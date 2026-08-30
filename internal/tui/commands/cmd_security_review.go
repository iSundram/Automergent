package commands

import (
	"strings"
)

// /security-review — security-focused review of pending changes.

func securityReviewCommand() Command {
	return Command{
		Name:             "security-review",
		Description:      "Security-focused review of pending changes",
		Category:         "Workflow",
		Icon:             "󰢽",
		ArgsHint:         "[focus]",
		Tier:             TierSecondary,
		Type:             CmdPrompt,
		// A security review reads a lot of code and produces a long report;
		// forking keeps that work out of the main conversation's context —
		// only the findings summary lands back here.
		Fork:             true,
		Immediate:        true,
		SupportsHeadless: true,
		PromptTemplate:   "Perform a security-focused code review of the current changes. Check for: injection vulnerabilities, authentication/authorization issues, data exposure, insecure defaults, dependency vulnerabilities, and OWASP top 10. Report findings with severity levels.$ARGUMENTS",
	}
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
