package tips

// workflow tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "workflow",
		Tip:          "run saved workflows — chained prompts over the codebase",
		Personalized: "",
		Body:         "# /workflow\n\nRuns saved workflows: named chains of prompts (from `.automergent/workflows/`)\nexecuted in sequence by the agent, with per-step results and run history.\n\n## Usage\n- `/workflow` — list available workflows.\n- `/workflow <name>` — run one.\n- `/workflow status` — the current or last run's progress.\n\n## Notes\n- Workflow definitions are plain Markdown files: one prompt per section.\n- Runs appear in the background dock; `/agents` shows their agents.\n\n## Related\n- `/agents` — the live agent roster.\n- `/run` — one-off shell commands.",
	})
}
