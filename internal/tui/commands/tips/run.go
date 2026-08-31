package tips

// run tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "run",
		Tip:          "run a shell command through the agent's shell tool",
		Personalized: "",
		Body:         "# /run\n\nRuns a shell command via the agent's shell subsystem (the same engine as\nthe bash tool): persistent working directory, output capture, background\nsupport — and the output lands in the conversation where the agent can see\nand reason about it.\n\n## Usage\n- `/run <command>` — execute and show output.\n- The `!` prefix in the prompt is the quick equivalent.\n\n## Notes\n- Output caps and stall watchdogs apply exactly as for the agent's own\n  shell calls.\n- For fire-and-forget commands prefer the agent's background shells.\n\n## Related\n- `/test`, `/build` — canned wrappers for the common cases.",
	})
}
