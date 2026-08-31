package tips

// pr-comments tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "pr-comments",
		Tip:          "pull PR review comments and address them",
		Personalized: "",
		Body:         "# /pr-comments\n\nFetches the review comments on a pull request and has the agent address\nthem: each comment is quoted, resolved in code and the reasoning recorded.\n\n## Usage\n- `/pr-comments` — the current branch's PR.\n- `/pr-comments <number>` — a specific pull request.\n\n## Notes\n- Requires `gh` authentication.\n- The agent works comment by comment; unresolved threads are surfaced at\n  the end rather than silently dropped.\n\n## Related\n- `/issue` — the issue-driven loop.\n- `/commit` — commit the fixes afterwards.",
	})
}
