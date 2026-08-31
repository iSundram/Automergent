package tips

// security-review tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "security-review",
		Tip:          "security-focused review — secrets, injection, unsafe patterns",
		Personalized: "",
		Body:         "# /security-review\n\nA security-focused pass over the session's changes (or the workspace):\nhard-coded secrets, injection risks, unsafe deserialization, path handling\nand permission escalations.\n\n## Usage\n- `/security-review` — review the current changes.\n\n## Notes\n- Backed by the secrets_scan and dependency_audit tools plus model review.\n- Findings are ranked by severity with file:line references.\n\n## Related\n- `/review` — the general-purpose review.\n- `/doctor` — environment-level security posture.",
	})
}
