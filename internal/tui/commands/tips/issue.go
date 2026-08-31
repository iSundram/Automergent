package tips

// issue tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "issue",
		Tip:          "work on a GitHub issue — fetch and solve it",
		Personalized: "",
		Body:         "# /issue\n\nFetches a GitHub issue (title, body, comments) and hands it to the agent as\na work order: understand, plan, implement, verify.\n\n## Usage\n- `/issue <number>` — the issue in the default repository.\n- `/issue <owner/repo#number>` — another repository.\n\n## Notes\n- Requires `gh` authentication for private repositories.\n- The issue body enters the conversation verbatim so the agent quotes\n  requirements accurately.\n\n## Related\n- `/pr-comments` — same loop for pull-request feedback.",
	})
}
