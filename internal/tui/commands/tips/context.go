package tips

// context tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "context",
		Tip:          "context-window usage breakdown — what is consuming the window",
		Personalized: "live estimate for {model}; /compact when it climbs past ~80%",
		Body:         "# /context\n\nShows the context-window breakdown: how much of the window the system\nprompt, tool definitions, conversation and loaded context files consume,\nplus the live adaptive token estimate.\n\n## Usage\n- `/context` — overview.\n- `/context detail` — the full itemized breakdown.\n\n## Notes\n- The header's usage meter and this command read the same numbers.\n- The model has its own view of this via the ctx_inspect tool.\n- When usage is high, prefer narrow reads and let compaction reclaim space\n  rather than re-reading whole files.\n\n## Related\n- `/compact` — compact the context now.\n- `/stats` — session-wide token and cost totals.",
	})
}
