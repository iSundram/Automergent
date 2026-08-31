package tips

// doctor tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "doctor",
		Tip:          "health check — provider, storage, LSP, permissions, paths",
		Personalized: "",
		Body:         "# /doctor\n\nRuns a full health check: provider connectivity, session storage health,\nLSP availability, permission-path sanity, config validity and toolchain\npresence. Each check reports ✓ / ! / ✗ with details.\n\n## Usage\n- `/doctor` — run every check.\n\n## Notes\n- The exit banner's tips aside, this is the first stop when something\n  behaves oddly — it names the failing subsystem precisely.\n- Storage checks cover session files and crash-recovery state.\n\n## Related\n- `/env` — the environment without the checks.\n- `/errors` — the API failure log.",
	})
}
