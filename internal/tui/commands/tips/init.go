package tips

// init tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "init",
		Tip:          "initialize project docs the agent reads every session",
		Personalized: "",
		Body:         "# /init\n\nGenerates or refreshes the project instruction file (AUTOMERGENT.md) by\nhaving the agent analyze the repository: build commands, test commands,\nconventions and safety rules. The file is injected into every future\nsession's context automatically.\n\n## Usage\n- `/init` — analyze the repo and write/update the instruction file.\n- `/init add <note>` — append a line yourself.\n\n## Notes\n- Review the generated file — it is the agent's standing brief for this\n  project; wrong facts there mislead every session.\n- Keep it short and current; stale instructions are worse than none.\n\n## Related\n- `/doctor` — checks that project config resolves sanely.\n- `/memory` — agent memory management.",
	})
}
