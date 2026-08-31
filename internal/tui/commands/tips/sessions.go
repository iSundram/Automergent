package tips

// sessions tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "sessions",
		Tip:          "browse previous sessions; type to search, ctrl+r renames, ctrl+d deletes",
		Personalized: "type to search titles — #1 is always the newest session",
		Body:         "# /sessions\n\nOpens the session picker: a searchable list of this workspace's sessions,\nnewest first, filtered to the current project directory.\n\n## Keys\n- `↑↓ / ctrl+p / ctrl+n` — navigate\n- `type` — search titles and first messages\n- `enter` — resume the highlighted session\n- `ctrl+r` — rename in place (ctrl+u clears the draft)\n- `ctrl+d` twice — delete (never the active session)\n- `pgup / pgdown` — page through the list\n- `esc` — clear the search, then close\n\n## Rows\nEach row shows the auto-generated title (or first user message), relative\ntime, message count, disk size, provider/model and token totals. `✓ Current`\nmarks the active session.\n\n## Related\n- `/resume <id-prefix|#N|title-substring>` — resume without opening the picker.\n- `/rename` — rename the current session from the prompt.",
	})
}
