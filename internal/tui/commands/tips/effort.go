package tips

// effort tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "effort",
		Tip:          "set reasoning effort — how hard the model thinks per turn",
		Personalized: "effort trades latency and cost for depth on {model}",
		Body:         "# /effort\n\nSets the reasoning effort for models that support it: how much thinking the\nmodel invests before answering.\n\n## Usage\n- `/effort` — show the current level.\n- `/effort <low|medium|high|max>` — switch.\n\n## Notes\n- Models without effort control ignore the setting gracefully.\n- Higher effort costs more tokens and time; routine edits rarely need it.\n\n## Related\n- `/model` — the model itself.\n- `/provider` — provider-level knobs.",
	})
}
