package tips

// dream tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "dream",
		Tip:          "consolidate session memory — reflect and store durable facts",
		Personalized: "",
		Body:         "# /dream\n\nRuns a memory-consolidation pass over the conversation: durable facts,\ndecisions and preferences are distilled into agent memory while the working\ntranscript stays untouched.\n\n## Usage\n- `/dream` — consolidate now.\n\n## Notes\n- The pass is model-driven; it reports what it stored.\n- Consolidated memory is recalled automatically in later sessions.\n- Runs in the background; results arrive as a system message.\n\n## Related\n- `/memory` — inspect and edit stored memory directly.\n- `/summary` — a summary for humans instead of memory.",
	})
}
