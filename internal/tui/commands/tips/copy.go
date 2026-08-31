package tips

// copy tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "copy",
		Tip:          "copy the last reply (or a range) to the clipboard",
		Personalized: "",
		Body:         "# /copy\n\nCopies conversation content to the system clipboard: the last assistant\nreply by default, or a chosen slice.\n\n## Usage\n- `/copy` — last assistant message.\n- `/copy last` — same, explicit.\n- `/copy <n>` — the nth assistant message (1-based from the start).\n\n## Notes\n- Works through the system clipboard tooling; falls back with an error\n  message when no clipboard is available (e.g. headless SSH).\n\n## Related\n- `/export` — the whole conversation to a file instead.",
	})
}
