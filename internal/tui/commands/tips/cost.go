package tips

// cost tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "cost",
		Tip:          "session cost and token totals",
		Personalized: "totals accrue across the whole session, {model} pricing",
		Body:         "# /cost\n\nReports the session's accumulated cost and token totals (input/output),\ncomputed from the provider's live telemetry.\n\n## Notes\n- Totals are per session — `/new` resets the counter.\n- Cost comes from the provider's pricing table; models without published\n  pricing report tokens only.\n\n## Related\n- `/stats` — the fuller dashboard (tokens, cost, tool counts).\n- `/context` — window usage right now.",
	})
}
