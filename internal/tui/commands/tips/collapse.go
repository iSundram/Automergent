package tips

// collapse tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "collapse",
		Tip:          "collapse every block to a single line — cards, thoughts, shell output",
		Personalized: "",
		Body:         "# /collapse\n\nFolds every collapsible conversation block to its header line: tool cards\nto their name and summary, thinking blocks to \"✓ Thought for Ns\", shell\noutput to the command row and a one-line tail. Long sessions get their\nvertical space back.\n\n## Notes\n- The status line names the inverse (`/expand`) after it runs.\n- Collapsed blocks keep their content — expanding restores it exactly.\n\n## Related\n- `/expand` — the inverse.\n- `/compact` — shrink the model's context, not the display.",
	})
}
