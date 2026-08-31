package tips

// rewind tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "rewind",
		Tip:          "rewind to a checkpoint captured before an agent turn",
		Personalized: "",
		Body:         "# /rewind\n\nRestores the conversation to the state captured before a chosen agent turn.\nCheckpoints are captured automatically before every turn; the picker shows\neach one's prompt, time and message count.\n\n## Usage\n- `/rewind` — open the checkpoint picker.\n- `/rewind <n>` — jump to checkpoint n directly (1-based, oldest first).\n\n## Notes\n- Checkpoints after the chosen point are discarded.\n- The session is persisted after rewinding.\n- Checkpoints are session-scoped: switching sessions never mixes them.\n\n## Related\n- `/branch` — fork the session instead of truncating it.\n- `/sessions` — switch to an entirely different session.",
	})
}
