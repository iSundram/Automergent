package tips

// lsp tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "lsp",
		Tip:          "language-server diagnostics for the workspace",
		Personalized: "",
		Body:         "# /lsp\n\nShows language-server diagnostics for the current workspace and the files\nthe session touched: errors and warnings the LSP sees, the same source the\nagent's lsp_diagnostics tool uses.\n\n## Usage\n- `/lsp` — diagnostics for session-touched files.\n- `/lsp all` — whole-workspace diagnostics.\n\n## Notes\n- Requires a language server for the file type; unsupported types are\n  skipped.\n- Problems the agent introduced are also counted in the status bar.\n\n## Related\n- `/doctor` — whether LSP integration is healthy at all.",
	})
}
