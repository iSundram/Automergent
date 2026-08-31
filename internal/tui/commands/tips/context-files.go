package tips

// context-files tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "context-files",
		Tip:          "the files touched this session — reads, writes, edits",
		Personalized: "",
		Body:         "# /context-files\n\nShows every file the agent touched this session — reads, writes and edits —\nas a full-page list (capped at 50 entries with the remainder counted).\n\n## Usage\n- `/context-files` — the touched-files page.\n\n## Notes\n- Session-scoped: resuming restores that session's list from its history.\n- Pair with `/diff` to see what the writes actually changed.\n\n## Related\n- `/diff` — the content view of the writes.\n- `/files` → renamed: this command was previously `/files`.",
	})
}
