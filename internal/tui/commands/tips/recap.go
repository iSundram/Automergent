package tips

// recap tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "recap",
		Tip:          "instant deterministic digest of this conversation",
		Personalized: "",
		Body:         "# /recap\n\nPrints a deterministic digest of the current conversation: turn counts,\ntools used, the last user message and timestamps. No model call — instant\nand free.\n\n## When to use\n- Quick orientation after resuming a session.\n- Checking how much tool activity a session accumulated.\n\n## Notes\n- Computed from session internals, so it never hallucinates.\n- For a narrative summary use `/summary` (model-written, costs a call).\n\n## Related\n- `/summary` — the model-written narrative version.\n- `/stats` — token/cost totals rather than shape.",
	})
}
