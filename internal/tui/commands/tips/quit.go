package tips

// quit tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "quit",
		Tip:          "exit the application (alias /quit)",
		Personalized: "",
		Body:         "# /quit\n\nExits the application. The session is saved first; the exit banner shows\nthe session id, duration and the resume command.\n\n## Usage\n- `/quit` or `/exit` — leave.\n- `ctrl+c` twice when idle — the keybinding equivalent.\n\n## Notes\n- The banner's resume line (`automergent -s <id>`) restores exactly this\n  session, artifacts included.\n- A running agent is stopped on exit.\n\n## Related\n- `/sessions` — resume within the app instead.",
	})
}
