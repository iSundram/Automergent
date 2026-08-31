package tips

// branch tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "branch",
		Tip:          "fork the session into a named branch",
		Personalized: "",
		Body:         "# /branch\n\nForks the current conversation into a new named session. The original stays\nuntouched — useful for exploring an alternative approach without losing the\nbaseline.\n\n## Usage\n- `/branch <name>` — create the fork (name required).\n\n## Notes\n- The branch starts as a copy of the current history and appears in\n  `/sessions` titled \"branch: <name>\".\n- Works while the agent is idle; cancel the run first otherwise.\n\n## Related\n- `/rewind` — truncate in place instead of forking.\n- `/new` — start clean instead of forking.",
	})
}
