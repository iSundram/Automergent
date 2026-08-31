package tips

// review tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "review",
		Tip:          "model-driven code review of the session's changes",
		Personalized: "",
		Body:         "# /review\n\nAsks the agent to review the session's diff — correctness, tests, style,\nmissing error handling — and report findings in the conversation. Distinct\nfrom the diff pane: it is an analysis, not a view.\n\n## Usage\n- `/review` — review the current changes.\n- `/review <focus>` — steer what to look for (e.g. \"concurrency\").\n\n## Notes\n- Findings reference file:line so you can jump to them.\n- `/review-mode` makes every edit proposal render with review grammar\n  (a accepts, r rejects) instead of plain diffs.\n\n## Related\n- `/security-review` — the security-focused pass.\n- `/diff` — the raw changes.",
	})
}
