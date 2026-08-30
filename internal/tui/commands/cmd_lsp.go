package commands

// /lsp (alias /diagnostics) — run compiler/LSP diagnostics on a file or the
// current changes via the agent's lsp_diagnostics tool.

func lspCommand() Command {
	return Command{
		Name:             "lsp",
		Aliases:          []string{"diagnostics"},
		Description:      "Show compiler diagnostics for a file or changed files",
		Category:         "Project",
		Icon:             "󰒕",
		ArgsHint:         "[file]",
		Tier:             TierTertiary,
		Type:             CmdPrompt,
		Immediate:        true,
		SupportsHeadless: true,
		WhenToUse:        "To check a file for compile errors before committing or after edits",
		// Only offered once code files have actually been touched — LSP is
		// noise in a fresh session with nothing to diagnose.
		Paths: []string{
			"*.go", "*.rs", "*.ts", "*.tsx", "*.js", "*.jsx", "*.py",
			"*.java", "*.c", "*.cc", "*.cpp", "*.h", "*.rb", "*.zig",
		},
		PromptTemplate:   "Run diagnostics on the workspace using the lsp_diagnostics tool (and the shell tool for other languages if needed).$ARGUMENTS\nReport each finding as file:line, the message, and severity. If there are no findings, say so plainly. Do not modify any files.",
	}
}

func handleLsp(host Host, args []string) Result {
	prompt := "Run compiler diagnostics for the changed files in this workspace using the lsp_diagnostics tool. Report each finding as file:line, message, and severity. If there are no findings, say so plainly. Do not modify any files."
	host.SetStatus("Preparing diagnostics")
	return Done(host.StartAgent(prompt))
}
