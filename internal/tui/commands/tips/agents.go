package tips

// agents tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "agents",
		Tip:          "the live subagent roster — what every agent is doing",
		Personalized: "",
		Body:         "# /agents\n\nShows the live agent roster: every subagent this session spawned, its type,\ncurrent activity (\"in grep\"), elapsed time, tool calls and terminal\noutcome.\n\n## Usage\n- `/agents` — the roster page.\n- Selecting a row opens the agent's side-channel transcript.\n\n## Notes\n- Background workflow agents appear here with their run state.\n- Killing a subagent also kills its descendants; logs and artifacts are\n  preserved.\n\n## Related\n- `/workflow` — the workflow runs that spawn agents.\n- `/cancel` — stop the main agent turn.",
	})
}
