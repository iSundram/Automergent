package commands

import (
	"strings"
)

// /review (alias /pr) — review changes or a pull request.

func reviewCommand() Command {
	return Command{
		Name:             "review",
		Description:      "Review changes or a pull request",
		Category:         "Workflow",
		Icon:             "󰤒",
		Aliases:          []string{"pr"},
		ArgsHint:         "[ref|#PR]",
		Tier:             TierPrimary,
		Type:             CmdPrompt,
		SubPalette:       "review",
		SupportsHeadless: true,
		WhenToUse:        "To get severity-ordered findings on pending changes or a PR",
		PromptTemplate:   "Perform a careful code review.\nTarget: $ARGUMENTS\nReport findings grouped by severity: blocking, should-fix, nit. For each: file:line, issue, suggested fix.\nCheck correctness, edge cases, error handling, security, and test coverage.\nDo not modify any files unless explicitly asked.",
	}
}

func handleReview(host Host, args []string) Result {
	target := strings.TrimSpace(strings.Join(args, " "))
	var b strings.Builder
	b.WriteString("Perform a careful code review.\n")
	if target == "" {
		b.WriteString("Target: uncommitted changes (`git status`, `git diff HEAD`) or, if none, the latest commit (`git show --stat HEAD`).\n")
	} else if looksLikePRRef(target) {
		b.WriteString("Target: pull request " + target + " (use `gh pr view` / `gh pr diff` if available, otherwise say what is missing).\n")
	} else {
		b.WriteString("Target: " + target + " (diff against its merge-base with the default branch).\n")
	}
	b.WriteString("Report findings grouped by severity: blocking, should-fix, nit. For each: file:line, issue, suggested fix.\n")
	b.WriteString("Check correctness, edge cases, error handling, security, and test coverage.\n")
	b.WriteString("Do not modify any files unless explicitly asked.")
	host.SetStatus("Preparing review")
	return Done(host.StartAgent(b.String()))
}

// looksLikePRRef detects GitHub-style PR references like "#123" or "123".
func looksLikePRRef(s string) bool {
	s = strings.TrimPrefix(s, "#")
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
