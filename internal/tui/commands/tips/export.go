package tips

// export tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "export",
		Tip:          "export the conversation to Markdown",
		Personalized: "",
		Body:         "# /export\n\nWrites the current conversation to a readable, deterministic Markdown\ntranscript: user prompts, assistant replies, tool calls and results.\n\n## Usage\n- `/export` — writes `conversation.md` in the workspace.\n- `/export <relative/path.md>` — choose the destination.\n\n## Notes\n- Paths must be relative to the workspace root.\n- The export reflects the persisted session, including resumed history.\n\n## Related\n- `/summary` — a model-written session summary instead of the raw transcript.",
	})
}
