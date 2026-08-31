package tips

// mode tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "mode",
		Tip:          "switch approval mode — manual, accept-edits, auto, plan",
		Personalized: "current mode is {mode}; shift+tab cycles without typing",
		Body:         "# /mode\n\nSwitches the approval mode that governs what the agent may do unasked:\n\n- **manual** — confirm every write and network-reaching action.\n- **accept-edits** — file edits apply automatically; shell/web/git still ask.\n- **auto** — act freely except destructive operations.\n- **plan** — read-only: edits are refused, output is a plan.\n\n## Usage\n- `/mode` — show the current mode and options.\n- `/mode <name>` — switch.\n\n## Notes\n- The mode chip in the status bar always shows the active mode.\n- `shift+tab` cycles modes from the prompt — no typing required.\n- The agent can also enter plan mode itself via the enter_plan_mode tool.\n\n## Related\n- `/permissions` — the always-allow list that outlives mode switches.\n- `/plan` — prompt-driven plan mode entry.",
	})
}
