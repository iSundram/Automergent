package commands

import (
	"strings"
)

// --- Git Handlers: /commit /review ---

// These are prompt-type commands: they expand a review discipline into an
// agent prompt, so every check runs through the normal tool-permission flow.

func handleCommit(host Host, args []string) Result {
	var b strings.Builder
	b.WriteString("Create a git commit for the pending workspace changes.\n")
	b.WriteString("1. Run `git status` and `git diff` (staged and unstaged) to understand the change.\n")
	b.WriteString("2. Draft a concise message in the repository's existing style (check `git log`).\n")
	b.WriteString("3. Stage only files relevant to this change and commit. Never push.\n")
	b.WriteString("4. If the diff is empty or unrelated files are mixed in, ask before proceeding.\n")
	if scope := strings.TrimSpace(strings.Join(args, " ")); scope != "" {
		b.WriteString("\nFocus: " + scope)
	}
	host.SetStatus("Preparing commit")
	return Done(host.StartAgent(b.String()))
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
