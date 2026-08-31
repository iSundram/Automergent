package tips

// expand tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "expand",
		Tip:          "expand every tool card, thinking block and shell output at once",
		Personalized: "",
		Body:         "# /expand\n\nOpens every collapsible conversation block in one move: tool cards show\ntheir full arguments and results, thinking blocks their complete reasoning,\nshell output its whole transcript.\n\n## Notes\n- The status line names the inverse (`/collapse`) after it runs.\n- Individual blocks still expand and collapse on their own; this is the\n  global switch.\n- Clipped blocks always hint at the command that flips the current state.\n\n## Related\n- `/collapse` — the inverse, one line per block.\n- `/review-mode` — richer rendering for edit proposals.",
	})
}
