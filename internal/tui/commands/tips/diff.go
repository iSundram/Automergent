package tips

// diff tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "diff",
		Tip:          "open the diff pane — every file changed this session",
		Personalized: "diffs accumulate per session; empty until the agent edits something",
		Body:         "# /diff\n\nOpens the fullscreen diff overlay: one tab per file the agent touched this\nsession, including writes that never asked for confirmation (accept-edits\nmode, always-allow grants, brand-new files).\n\n## Usage\n- `/diff` — open (or report nothing to review when no changes exist).\n- `ctrl+w` — the keybinding equivalent.\n\n## Keys\n- `tab` — cycle file tabs.\n- `↑↓ / pgup / pgdown` — scroll.\n- `esc` — close.\n\n## Notes\n- Newly created files diff as pure additions.\n- The status bar counts pending modified files while edits accrue.\n\n## Related\n- `/files` — the plain list of touched files.\n- `/review` — a model-driven code review of the changes.",
	})
}
