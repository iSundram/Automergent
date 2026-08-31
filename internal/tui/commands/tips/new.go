package tips

// new tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "new",
		Tip:          "starts a fresh session; the current one is saved and resumable",
		Personalized: "your current session is auto-titled — /resume #1 returns to it anytime",
		Body:         "# /new\n\nStarts a fresh session. The current conversation is saved to disk first\n(when it has messages), the view clears, and a new session begins with the\nsame provider, model and working directory.\n\n## When to use\n- Starting an unrelated task and you want a clean context.\n- The current conversation has drifted and token usage is climbing.\n\n## Behavior\n- The previous session is saved automatically — nothing is lost.\n- Session-scoped state resets: rewind checkpoints, API error history,\n  artifacts and usage stats.\n- Refuses to run while the agent is mid-turn: /cancel first, then /new.\n\n## Related\n- `/sessions` — browse and resume saved sessions (search, rename, delete).\n- `/resume <id|prefix|#N|title>` — jump straight back.\n- `/compact` — alternative when you want to keep the session but shrink it.",
	})
}
