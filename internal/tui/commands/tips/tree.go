package tips

// tree tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "tree",
		Tip:          "toggle the file tree sidebar",
		Personalized: "",
		Body:         "# /tree\n\nToggles the workspace file tree sidebar: a live directory view next to the\nconversation, handy for orientation in unfamiliar projects.\n\n## Notes\n- Takes roughly a fifth of the width when open.\n- Hidden directories are skipped; large trees collapse sensibly.\n- Purely a view toggle — the agent's file access is unaffected.\n\n## Related\n- `/files` — the files the agent actually touched this session.\n- `/directory` — the working directory the agent operates in.",
	})
}
