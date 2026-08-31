package tips

// search tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "search",
		Tip:          "search the workspace — files by name or content",
		Personalized: "",
		Body:         "# /search\n\nSearches the workspace and shows matches in the conversation. By default it\nbehaves like a filename search; quoted arguments switch to content search.\n\n## Usage\n- `/search <term>` — find files whose name matches.\n- `/search \"text\"` — grep the workspace contents for text.\n\n## Notes\n- Honors extra search roots added with `/add-dir`.\n- For anything the agent should act on, just ask it in the prompt — it has\n  its own search tools.\n\n## Related\n- `/files` — what the agent already touched.\n- `/tree` — browse instead of search.",
	})
}
