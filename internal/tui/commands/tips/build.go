package tips

// build tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "build",
		Tip:          "compile the project and surface build errors",
		Personalized: "",
		Body:         "# /build\n\nBuilds the project (detected per project: `go build`, `npm run build`,\n...) and surfaces errors and warnings in the conversation.\n\n## Usage\n- `/build` — the default build.\n- `/build <args>` — pass extra arguments through.\n\n## Notes\n- Build failures arrive as structured output the agent can iterate on.\n- Faster feedback loop than `/test` when only compilation matters.\n\n## Related\n- `/test` — the full suite.\n- `/lsp` — editor-grade diagnostics without a build.",
	})
}
