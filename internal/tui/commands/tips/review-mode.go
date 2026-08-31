package tips

// review-mode tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "review-mode",
		Tip:          "toggle detailed change review for edit proposals",
		Personalized: "",
		Body:         "# /review-mode\n\nToggles detailed change review: while on, every edit proposal renders with\nreview grammar — the proposed change plus `a` to accept and apply, `r` to\nreject — instead of a plain diff card.\n\n## Notes\n- A pure display/interaction toggle; agent behavior is unchanged.\n- The status line and `/review` output use the same review styling.\n\n## Related\n- `/review` — one-off model review of the whole change set.\n- `/mode` — approval autonomy (separate concern).",
	})
}
