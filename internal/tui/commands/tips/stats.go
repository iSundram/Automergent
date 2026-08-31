package tips

// stats tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "stats",
		Tip:          "session stats dashboard — tokens, cost, tools",
		Personalized: "",
		Body:         "# /stats\n\nOpens the statistics dashboard: input/output tokens, cost, tool-call\ncounts and model usage for the current session.\n\n## Usage\n- `/stats` — the dashboard view.\n\n## Notes\n- Token totals update live as the session progresses.\n- Cost uses the provider's published pricing when available.\n\n## Related\n- `/cost` — the one-line version.\n- `/context` — window usage rather than totals.",
	})
}
