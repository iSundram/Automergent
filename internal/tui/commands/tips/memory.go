package tips

// memory tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "memory",
		Tip:          "manage agent memory — durable facts across sessions",
		Personalized: "",
		Body:         "# /memory\n\nManages agent memory: the durable facts, decisions and preferences the\nagent recalls automatically in later sessions. Memory is project-scoped and\nseparate from the conversation.\n\n## Usage\n- `/memory` — list stored memories.\n- `/memory add <text>` — store a fact.\n- `/memory remove <id>` — forget one.\n- `/memory clear` — wipe all.\n\n## Notes\n- `/dream` consolidates the conversation into memory automatically.\n- Memory is injected as context, not commands — the agent treats it as\n  standing knowledge.\n\n## Related\n- `/dream` — the consolidation pass.\n- `/init` — project instructions (a different, file-based channel).",
	})
}
