package tips

// compact tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "compact",
		Tip:          "compact the context window — summarize and reclaim space",
		Personalized: "compaction keeps the recent tail and summarizes the rest",
		Body:         "# /compact\n\nCompacts the conversation: older messages are summarized by the model, the\nrecent tail is kept verbatim, and the freed window is reported. Runs in the\nbackground; the status bar shows progress.\n\n## When to use\n- The context meter passes ~80%.\n- Long sessions where early exploration is no longer needed verbatim.\n\n## Notes\n- Tool results may be dropped to reclaim space — the agent re-runs a tool\n  when it needs the content again.\n- The conversation view keeps its history; only the model's context shrinks.\n- Automatic compaction also exists for long-running work.\n\n## Related\n- `/context` — check usage before deciding.\n- `/new` — the heavier reset when compaction is not enough.",
	})
}
