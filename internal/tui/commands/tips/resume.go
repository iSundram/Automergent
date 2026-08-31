package tips

// resume tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "resume",
		Tip:          "resume a session by id prefix, #N (newest first) or title match",
		Personalized: "#1 is your newest session; title matches work too",
		Body:         "# /resume\n\nResumes a saved session without opening the picker. The reference resolves\nin this order:\n\n1. **Exact session id** — the full UUID.\n2. **Unique id prefix** — the first characters of the id. Ambiguous\n   prefixes name their match count instead of guessing.\n3. **`#N`** — the Nth most recently updated session (1 = newest).\n4. **Title substring** — case-insensitive; must match exactly one session.\n\nWith no argument it opens the `/sessions` picker instead.\n\n## Notes\n- Refuses to run while the agent is mid-turn (`/cancel` first).\n- Crash-recovery points are preferred over the last clean save when they\n  hold a richer history.\n- Restores provider/model, replays the transcript and this session's\n  artifacts.\n\n## Related\n- `/sessions` — the full picker with search, rename and delete.",
	})
}
