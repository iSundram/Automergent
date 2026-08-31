package tips

// tldr tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "tldr",
		Tip:          "too long; didn't read — the session in a few lines",
		Personalized: "",
		Body:         "# /tldr\n\nA terse executive digest of the session: what was asked, what changed, what\nremains — a few lines, model-written.\n\n## Usage\n- `/tldr` — the digest.\n\n## Notes\n- Shorter and more opinionated than `/summary`.\n- Costs one small completion; instant alternatives are `/recap`.\n\n## Related\n- `/summary` — the fuller narrative.\n- `/recap` — the deterministic digest.",
	})
}
