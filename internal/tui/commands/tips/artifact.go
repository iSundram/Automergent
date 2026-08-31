package tips

// artifact tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "artifact",
		Tip:          "review agent artifacts — approve plans, comment, open in editor",
		Personalized: "{artifacts} artifact(s) in this session; plans need your decision",
		Body:         "# /artifact\n\nOpens the artifact review browser: the deliverables the agent produced\nthis session — plans, reviews, designs, summaries — scoped to this session\nonly.\n\n## Keys\n- `↑↓` — navigate · `p`/`enter` — full-page preview\n- `y` — approve · `n` — reject (a reason is required)\n- `shift+a` — approve every pending plan\n- `c` — comment on the artifact\n- `ctrl+g` — open in your editor · `esc` — done\n\n## Review semantics\n- **Plans** are the only approvable artifacts: `y` puts the agent to work\n  implementing the plan, `n` (with your reason) sends it back for revision.\n- **Other artifacts** (reviews, designs, summaries) are informational —\n  preview, comment, open in an editor; no approve/reject.\n- **Comments** are stored on the artifact; while the agent is working they\n  are steered into the running turn.\n\n## Preview mode\nFull-page with line numbers: `↑↓/pgup/pgdn` scroll, `g`/`shift+g` top and\nbottom, `/` search with enter-to-jump, `esc` back to the list.\n\n## Related\n- `/plan` — the command that usually produces the plan artifact.\n- The status bar shows `N artifacts · /artifact to review` whenever plans\n  are pending.",
	})
}
