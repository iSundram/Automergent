package tips

// add-dir tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "add-dir",
		Tip:          "add an extra read-only search root",
		Personalized: "",
		Body:         "# /add-dir\n\nAdds a directory outside the workspace as a read-only search root: the\nagent's read/search tools can look there, but writes still require the\nnormal permission flow.\n\n## Usage\n- `/add-dir <path>` — absolute path to add.\n\n## Notes\n- Roots persist for the session and show up in `/directory`.\n- Adding a root does not grant write access — only read/search reach.\n\n## Related\n- `/directory` — list the workspace plus extra roots.",
	})
}
