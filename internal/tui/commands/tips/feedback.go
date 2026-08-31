package tips

// feedback tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "feedback",
		Tip:          "send feedback about the tool",
		Personalized: "",
		Body:         "# /feedback\n\nOpens the feedback flow: a short structured report (what happened, what you\nexpected) sent to the project's feedback channel.\n\n## Usage\n- `/feedback <text>` — send inline feedback.\n\n## Notes\n- Nothing about your session content is attached unless you include it.\n- The report includes the version and platform for reproducibility.\n\n## Related\n- `/doctor` — attach diagnostics yourself when reporting a bug.",
	})
}
