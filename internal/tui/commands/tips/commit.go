package tips

// commit tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "commit",
		Tip:          "stage and commit the session's changes",
		Personalized: "",
		Body:         "# /commit\n\nHands the commit job to the agent: it reviews the session's diff, drafts a\nconventional-commit message, stages the changes and commits — asking for\nconfirmation before the actual `git commit`.\n\n## Usage\n- `/commit` — stage everything this session touched and commit.\n- `/commit <message hint>` — steer the message draft.\n\n## Notes\n- The drafted message is shown before anything is committed.\n- Pushing is never implied — commit and push stay separate decisions.\n- Secret scanning runs before staging; flagged content blocks the commit.\n\n## Related\n- `/review` — review the diff before committing.\n- `/diff` — inspect the raw changes yourself.",
	})
}
