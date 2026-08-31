package tips

// goal tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "goal",
		Tip:          "set an autonomy objective the agent works toward",
		Personalized: "",
		Body:         "# /goal\n\nInstalls an objective with an optional token budget that the agent keeps\nworking toward across turns — the continuation loop drives itself until the\ngoal is met, the budget runs out, or you pause it.\n\n## Usage\n- `/goal <objective>` — set a goal (optionally end with `budget <n>` tokens).\n- `/goal` — snapshot of the current goal and progress.\n- `/goal pause|resume|continue|clear` — control the loop.\n\n## Notes\n- Progress is reported in the header/status while active.\n- Clearing the goal stops the loop immediately.\n- The agent never uses goal mode to skip permission prompts.\n\n## Related\n- `/mode` — approval autonomy is separate from goal autonomy.\n- `/cancel` — stop the current turn without clearing the goal.",
	})
}
