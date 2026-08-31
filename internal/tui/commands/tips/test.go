package tips

// test tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "test",
		Tip:          "run the project's test suite",
		Personalized: "",
		Body:         "# /test\n\nRuns the project's test suite through the agent (detected per project:\n`go test`, `npm test`, ...) and reports results in the conversation, where\nthe agent can act on failures.\n\n## Usage\n- `/test` — the whole suite.\n- `/test <args>` — extra arguments passed through (e.g. a package or\n  `-run TestName`).\n\n## Notes\n- Test selection is detected from the project manifest; missing tooling\n  reports that plainly.\n- Failures render with enough context for the agent to fix them in the\n  same conversation.\n\n## Related\n- `/build` — compile check without tests.\n- `/run` — arbitrary commands.",
	})
}
