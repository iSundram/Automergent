package tips

// cancel tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "cancel",
		Tip:          "cancel the active agent turn (alias /stop)",
		Personalized: "",
		Body:         "# /cancel\n\nCancels the agent's in-flight turn: the running provider request and any\nexecuting tool are stopped, partial output stays in the transcript, and the\nprompt returns to you.\n\n## Usage\n- `/cancel` — stop the current run.\n- `esc` — the keybinding equivalent; `ctrl+c` twice force-quits instead.\n\n## Notes\n- Only meaningful while a run is active; the palette disables it otherwise.\n- Interrupted runs report how many tools completed before the stop.\n- Queued messages stay queued; they deliver on the next run.\n\n## Related\n- `/rewind` — also undo the partial turn.\n- `/goal` — clear a goal to stop the continuation loop.",
	})
}
